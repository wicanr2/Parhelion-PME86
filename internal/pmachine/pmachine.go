// Package pmachine 是 p-machine 的執行核心。
//
// 每一條指令的語意都是從 8086 版直譯器的碼讀出來的，不是從手冊抄的
// （手冊當交叉檢查）。註解裡的位址是 SYSTEM.PME.86 的檔案偏移，
// 可以直接回去對碼。
//
// 這一層**只認 p-code**：不知道 codefile 長什麼樣、不知道 DOS、不知道畫面。
// 它拿到的是「一段程式碼、一段資料、幾個基底」，跑完回報執行了什麼。
package pmachine

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
)

// State 是 p-machine 的執行狀態。
//
// 欄位對應 8086 版直譯器常駐在暫存器裡的那幾個
// （docs/10-interpreter/machine-state.md）。**兩個段就夠**：
// 程式碼在一個段（`ds`／`lodsb` 取指令的地方），
// 求值堆疊、區域變數、全域變數全部在另一個段（`ss`）。
type State struct {
	Code []byte // 目前 code segment 的內容
	Data []byte // 資料段：求值堆疊、區域、全域都在這裡

	IPC uint16 // 下一個 opcode 在 Code 裡的位元組偏移
	SP  uint16 // 求值堆疊頂在 Data 裡的位移，往低位址長

	Local  uint16 // 區域資料基底 ＝ MP+8；變數 n 在 Local+2n
	Global uint16 // 全域資料基底 ＝ Env_Data+8

	// ConstPool 是常數池在 Code 裡的位元組偏移。切段時由段頭第 0x0E 個
	// 位元組乘二得到，直譯器放在 ss:42h。
	ConstPool uint16

	// ProcDict 是程序字典在 Code 裡的位元組偏移（直譯器的 ss:36h）。
	// 字典**往回長**：程序 n 的字典項在 ProcDict − 2n。
	ProcDict uint16

	// Proc／ERec 是目前的 MSPROC 與 E_Rec（直譯器的 ss:32h、ss:3Eh）。
	// 呼叫時要把它們存進 MSCW，返回時要還原——不模型化的話堆疊內容會對不上。
	Proc uint16
	ERec uint16

	// Env 回答「segment number（或 E_Rec）對應哪一段」，跨段呼叫要用。
	// nil 表示只跑得動同一段。
	//
	// **這一層刻意不認得 SIB 與 Codepool。** 那是作業系統的資料結構，
	// 換一個宿主就不一樣；p-machine 只需要「給我那一段的樣子」。
	Env Environment
}

// Segment 是一個載入好的 code segment 在執行期的樣子。
type Segment struct {
	Code      []byte
	Global    uint16 // Env_Data + 8
	ProcDict  uint16 // 程序字典在 Code 裡的位元組偏移
	ConstPool uint16 // 常數池在 Code 裡的位元組偏移
	ERec      uint16
}

// Environment 解析 segment number 與 E_Rec。
type Environment interface {
	// ByNumber 用 segment number 找段。段不在記憶體時回 ErrNotResident。
	ByNumber(seg uint16) (*Segment, error)
	// ByERec 用 E_Rec 指標找段，返回跨段呼叫時要用。
	ByERec(erec uint16) (*Segment, error)

	// Intrinsic 回報「段 1 的這支程序是不是內嵌在宿主裡的原生碼」。
	//
	// 手冊 p.66：segment 號為 1 時程序碼可能內嵌在直譯器裡，由直譯器的表格
	// 給出位置。原版的 CXG **在切段之前**先查那張表（@0x13f4）；
	// 查到就直接跳進機器碼，段完全不換。
	Intrinsic(proc uint16) bool
}

// IntrinsicCall 是「這一支是宿主的原生碼，p-machine 執行不了」。
//
// 與「跨段呼叫還沒做」是兩件事：段的機制是通的，缺的是那一支的行為。
type IntrinsicCall struct{ Proc uint16 }

func (e *IntrinsicCall) Error() string {
	return fmt.Sprintf("pmachine: 段 1 的程序 %d 是內嵌在直譯器裡的原生碼，還沒實作", e.Proc)
}

// ErrNotResident 是「那一段不在記憶體」。原版遇到這個會退回指令開頭、
// 發 segment fault 讓作業系統去載入，然後**重跑同一條指令**。
var ErrNotResident = errors.New("pmachine: 段不在記憶體")

// Unimplemented 是「這個 opcode 還沒做」。
//
// **不回「執行成功」也不 panic**：對拍的價值就在於它會準確指出下一個要做的是哪一支。
type Unimplemented struct {
	Op  uint8
	IPC uint16
}

func (e *Unimplemented) Error() string {
	name := pcode.Mnemonic(e.Op)
	if name == "" {
		name = "（IV.0 表裡沒有這個指令）"
	}
	return fmt.Sprintf("pmachine: %04Xh 的 opcode %02X %s 還沒實作", e.IPC, e.Op, name)
}

// Fault 是執行期錯誤，對應原版跳到錯誤處理的那些情形。
type Fault struct {
	IPC uint16
	Why string
}

func (e *Fault) Error() string { return fmt.Sprintf("pmachine: %04Xh：%s", e.IPC, e.Why) }

// ---- 記憶體與堆疊 --------------------------------------------------------

func (s *State) word(b []byte, off uint16) uint16 {
	if int(off)+1 >= len(b) {
		return 0
	}
	return binary.LittleEndian.Uint16(b[off:])
}

func (s *State) setWord(b []byte, off, v uint16) {
	if int(off)+1 < len(b) {
		binary.LittleEndian.PutUint16(b[off:], v)
	}
}

// Load 讀資料段的一個 word。
func (s *State) Load(off uint16) uint16 { return s.word(s.Data, off) }

// Store 寫資料段的一個 word。
func (s *State) Store(off, v uint16) { s.setWord(s.Data, off, v) }

// TOS 是求值堆疊頂，不動堆疊。
func (s *State) TOS() uint16 { return s.Load(s.SP) }

func (s *State) push(v uint16) {
	s.SP -= 2
	s.Store(s.SP, v)
}

func (s *State) pop() uint16 {
	v := s.Load(s.SP)
	s.SP += 2
	return v
}

// ---- 取指令 --------------------------------------------------------------

func (s *State) fetch() uint8 {
	if int(s.IPC) >= len(s.Code) {
		return 0
	}
	b := s.Code[s.IPC]
	s.IPC++
	return b
}

func (s *State) fetchWord() uint16 {
	lo := uint16(s.fetch())
	return lo | uint16(s.fetch())<<8
}

// big 讀一個變長運算元（手冊 p.16）。最高位為 0 就是它本身（0–127），
// 為 1 則取低 7 位當高位元組、再讀一個位元組當低位元組（0–32767）。
//
// 直譯器裡這段碼原地展開了 19 次，每一次都以 `shl ax,1` 收尾——
// p-code 說 word，8086 要 byte。所以 bytes() 才是實際用的形式。
func (s *State) big() uint16 {
	b := s.fetch()
	if b&0x80 == 0 {
		return uint16(b)
	}
	return uint16(b&0x7f)<<8 | uint16(s.fetch())
}

// bytes 是 big 乘二，也就是換算成位元組偏移。
func (s *State) bytes() uint16 { return s.big() * 2 }

// ---- 執行 ---------------------------------------------------------------

// Step 執行一條 p-code，回傳剛執行的 opcode。
func (s *State) Step() (uint8, error) {
	at := s.IPC
	op := s.fetch()

	switch {
	// SLDC 0x00–0x1F：把 opcode 本身當常數推上去。@0x0319／0x0322
	case op <= 0x1f:
		s.push(uint16(op))
		return op, nil

	// SLDL 0x20–0x2F：推區域變數 n。@0x032d：`mov bp,bx; push [bp+2n]`
	case op >= 0x20 && op <= 0x2f:
		s.push(s.Load(s.Local + 2*uint16(op-0x1f)))
		return op, nil

	// SLDO 0x30–0x3F：推全域變數 n。@0x03fd：`mov bp,dx; push [bp+2n]`
	case op >= 0x30 && op <= 0x3f:
		s.push(s.Load(s.Global + 2*uint16(op-0x2f)))
		return op, nil

	// SLLA 0x60–0x67：推區域變數 n 的位址。@0x04cd：`sub di,0BEh; add di,bx`
	case op >= 0x60 && op <= 0x67:
		s.push(s.Local + 2*uint16(op-0x5f))
		return op, nil

	// SSTL 0x68–0x6F：存進區域變數 n。@0x04dd：`mov bp,bx; pop [bp+2n]`
	case op >= 0x68 && op <= 0x6f:
		s.Store(s.Local+2*uint16(op-0x67), s.pop())
		return op, nil

	// SIND 0x78–0x7F：把 TOS 當位址，推它偏移 n 個 word 的內容。@0x0554
	case op >= 0x78 && op <= 0x7f:
		s.push(s.Load(s.pop() + 2*uint16(op-0x78)))
		return op, nil
	}

	switch op {
	case 0x99: // LSL @0x14ee：往外走 DB 層靜態鏈，推那一層的 MP
		db := uint16(s.fetch())
		bp := s.Local - 8
		for ; db > 0; db-- {
			bp = s.Load(bp)
		}
		s.push(bp)
	case 0x9c: // NOP @0x06b9

	case 0x80: // LDCB：推接下來那個位元組（無號）。@0x05b4
		s.push(uint16(s.fetch()))
	case 0x81: // LDCI：推接下來那個 word。@0x05bf
		s.push(s.fetchWord())

	case 0x85: // LDO：推全域變數。@0x05ff
		s.push(s.Load(s.Global + s.bytes()))
	case 0x86: // LAO：推全域變數的位址。@0x061b
		s.push(s.Global + s.bytes())
	case 0x82: // LCO：推常數池裡某個 word 的偏移。@0x05ca：`add ax, ss:42h`
		s.push(s.ConstPool + s.bytes())
	case 0x84: // LLA：推區域變數的位址。@0x05e6
		s.push(s.Local + s.bytes())
	case 0x87: // LDL：推區域變數。@0x0634
		s.push(s.Load(s.Local + s.bytes()))

	// 中介層：先走 DB 層靜態鏈再加偏移。助手 @0x093a：
	// `bp = bx − 8`（回到 MP）→ 走 DB 次 `bp = [bp]`（MSSTAT）→ `bp += 2n + 8`。
	case 0x88: // LDA @0x0650
		s.push(s.intermediate(uint16(s.fetch())))
	case 0x89: // LOD @0x065d
		s.push(s.Load(s.intermediate(uint16(s.fetch()))))
	case 0xa6: // STR @0x0742
		a := s.intermediate(uint16(s.fetch()))
		s.Store(a, s.pop())
	case 0xad: // SLOD1 @0x0775：層數固定 1，從助手中段進來，省掉取 DB
		s.push(s.Load(s.intermediate(1)))
	case 0xae: // SLOD2 @0x0787
		s.push(s.Load(s.intermediate(2)))
	case 0xa4: // STL：存進區域變數。@0x070a
		off := s.bytes()
		s.Store(s.Local+off, s.pop())
	case 0xa5: // SRO：存進全域變數。@0x0726
		off := s.bytes()
		s.Store(s.Global+off, s.pop())

	case 0x98: // LDCN：推 NIL。@0x0693：`xor ax,ax; push ax`
		s.push(0)
	case 0xe2: // DUP1：複製 TOS。@0x08bc
		s.push(s.TOS())

	case 0xc4: // STO：`pop ax; pop bp; [bp] = ax`。@0x0814
		v := s.pop()
		s.Store(s.pop(), v)
	case 0xa7: // LDB：位元組索引取值。@0x0751：`pop di; pop bp; mov al,[bp+di]`
		i := s.pop()
		b := s.pop()
		s.push(uint16(s.byteAt(b + i)))
	case 0xc8: // STB @0x0822：`pop ax; pop di; pop bp; mov [bp+di],al`
		v := s.pop()
		i := s.pop()
		s.setByteAt(s.pop()+i, uint8(v))

	case 0xe6: // IND：把 TOS 當位址，推它偏移 B 個 word 的內容。@0x08d7
		off := s.bytes()
		s.push(s.Load(s.pop() + off))

	// 算術。兩個運算元的次序在原版是 `pop ax; pop bp; op bp,ax`——
	// 也就是 TOS 是右運算元。
	case 0xa0: // LOR  @0x06d2
		b := s.pop()
		s.push(s.pop() | b)
	case 0xa1: // LAND @0x06e0
		b := s.pop()
		s.push(s.pop() & b)
	case 0xa2: // ADI  @0x06ee
		b := s.pop()
		s.push(s.pop() + b)
	case 0xa3: // SBI  @0x06fc
		b := s.pop()
		s.push(s.pop() - b)
	case 0x8c: // MPI  @0x066c：`imul bp`，只留低 16 位
		b := int16(s.pop())
		s.push(uint16(int16(s.pop()) * b))
	case 0x8f: // MODI @0x0bbc：餘數為負時加上除數的絕對值
		b := int16(s.pop())
		if b == 0 {
			return op, &Fault{at, "整數取餘除以零"}
		}
		r := int16(s.pop()) % b
		if r < 0 {
			if b < 0 {
				r -= b
			} else {
				r += b
			}
		}
		s.push(uint16(r))
	case 0x8d: // DVI  @0x067f：`cwd; idiv bp`
		b := int16(s.pop())
		if b == 0 {
			return op, &Fault{at, "整數除以零"}
		}
		s.push(uint16(int16(s.pop()) / b))
	case 0xe1: // NGI  @0x08af
		s.push(uint16(-int16(s.pop())))
	case 0xe0: // ABI  @0x089a
		v := int16(s.pop())
		if v < 0 {
			v = -v
			if v < 0 { // 只有 0x8000 會這樣，原版走錯誤路徑
				return op, &Fault{at, "取絕對值溢位"}
			}
		}
		s.push(uint16(v))
	case 0xe5: // LNOT @0x08ca：整個 word 取反，不是只有最低位
		s.push(^s.pop())
	case 0x9f: // BNOT @0x06c2：取反之後只留最低位
		s.push(^s.pop() & 1)
	case 0xe7: // INC  @0x08f2：TOS 加 B 個 word
		off := s.bytes()
		s.push(s.pop() + off)
	case 0xee: // DECI @0x0918
		s.push(s.pop() - 1)
	case 0xd7: // IXA  @0x0864：TOS 是索引、TOS-1 是基底，B 是元素 word 數
		size := s.bytes()
		idx := s.pop()
		if idx != 1 {
			size *= idx
		}
		s.push(s.pop() + size)

	// 比較。原版一律 `pop ax; pop bp; cmp bp,ax; j<cc> 推 1；否則推 0`。
	case 0xb0: // EQUI  @0x0799
		s.compare(func(a, b int16) bool { return a == b })
	case 0xb1: // NEQI  @0x07ab
		s.compare(func(a, b int16) bool { return a != b })
	case 0xb2: // LEQI  @0x07be
		s.compare(func(a, b int16) bool { return a <= b })
	case 0xb3: // GEQI  @0x07d0
		s.compare(func(a, b int16) bool { return a >= b })
	case 0xb4: // LEUSW @0x07e2：`jbe`，無號比較
		s.compareU(func(a, b uint16) bool { return a <= b })
	case 0xb5: // GEUSW @0x07f4：`jb` 推 0，其餘推 1
		s.compareU(func(a, b uint16) bool { return a >= b })

	// 跳躍。位移都是**相對於運算元之後**的位置，因為原版是先 lodsb／lodsw
	// 再 `add si, ax`。
	case 0x8a: // UJP  @0x0c47：帶號位元組
		s.jumpByte()
	case 0x8b: // JPL  @0x0c84：帶號 word
		s.jumpWord()
	case 0xd4: // FJP  @0x0848：TOS 最低位為 1 就不跳
		if s.pop()&1 != 0 {
			s.IPC++
			break
		}
		s.jumpByte()
	case 0xd5: // FJPL @0x0c74
		if s.pop()&1 != 0 {
			s.IPC += 2
			break
		}
		d := int16(s.fetchWord())
		s.IPC = uint16(int16(s.IPC) + d)

	case 0xf1: // TJP @0x0c38：TOS 最低位為 1 就跳
		if s.pop()&1 != 0 {
			s.jumpByte()
			break
		}
		s.IPC++
	case 0xd2: // EFJ @0x0c54：兩個值不相等才跳
		b := s.pop()
		if s.pop() != b {
			s.jumpByte()
			break
		}
		s.IPC++
	case 0xd3: // NFJ @0x0c64：兩個值相等才跳（帶號位元組位移）
		b := s.pop()
		if s.pop() == b {
			s.jumpByte()
			break
		}
		s.IPC++

	// packed 欄位。原版用 cs:1FB6h 那張現成的遮罩表（第 n 項 ＝ (1<<n)−1）；
	// 這裡直接算，結果一樣。三元組的次序見 docs/10-interpreter/addressing.md。
	case 0xc9: // LDP @0x0a77
		right := s.pop()
		width := s.pop()
		v := s.Load(s.pop()) >> right
		s.push(v & mask(width))
	case 0xca: // STP @0x0a91
		data := s.pop() & mask(s.TOS0(2))
		right := s.pop()
		m := mask(s.pop())
		addr := s.pop()
		v := ror(s.Load(addr), right)
		s.Store(addr, rol((v&^m)|data, right))
	case 0xd8: // IXP @0x0a50：一次除法同時得到「第幾個 word」與「word 內第幾格」
		per := s.big()   // UB_1：每個 word 幾個元素
		width := s.big() // UB_2：欄位位元數
		if per == 0 {
			return op, &Fault{at, "packed 欄位每個 word 裝 0 個元素"}
		}
		idx := s.pop()
		base := s.pop()
		s.push(base + idx/per*2)
		s.push(width)
		s.push(idx % per * width)

	case 0xd0: // LDM @0x09b8：推 UB 個 word，位址低的那個留在 TOS
		n := uint16(s.fetch())
		addr := s.pop()
		for i := n; i > 0; i-- {
			s.push(s.Load(addr + 2*(i-1)))
		}
	case 0xa9: // NAT-INFO @0x075f：跳過 B 個**位元組**的原生碼資訊（沒有乘二）
		s.IPC += s.big()

	// 呼叫。MSCW 的推入順序在 @0x1057 一目了然：MSPROC、MSENV、MSIPC、
	// MSDYN、MSSTAT，堆疊往下長，所以從 MP 往上就是 Figure 5 的欄位順序。
	case 0x90: // CPL @0x1310：靜態鏈是呼叫者自己的框
		if err := s.call(uint16(s.fetch()), s.Local-8); err != nil {
			return op, err
		}
	case 0x91: // CPG @0x1337：靜態鏈是 BASE
		if err := s.call(uint16(s.fetch()), s.Global-8); err != nil {
			return op, err
		}
	case 0x92: // CPI @0x1368：往外走 DB 層再當靜態鏈
		db := uint16(s.fetch())
		if err := s.call(uint16(s.fetch()), s.chain(db)); err != nil {
			return op, err
		}
	case 0xef: // SCPI1 @0x092f 那一族：層數固定 1
		if err := s.call(uint16(s.fetch()), s.chain(1)); err != nil {
			return op, err
		}
	case 0xf0: // SCPI2：層數固定 2
		if err := s.call(uint16(s.fetch()), s.chain(2)); err != nil {
			return op, err
		}

	// 跨段呼叫。切段（@0x0fab）之後才建框，所以 @0x1057 推進 MSCW 的
	// `word_3E` 已經是**新的** E_Rec；@0x10db 再把 `[MP+6]` 改回舊的。
	// 這裡直接推舊的，效果一樣。
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77: // SCXG @0x0545：段號編在 opcode 裡
		if err := s.callExternal(uint16(op)-0x6f, uint16(s.fetch()), globalLink); err != nil {
			return op, err
		}
	case 0x93: // CXL @0x13b8：靜態鏈是呼叫者自己的框
		seg := uint16(s.fetch())
		if err := s.callExternal(seg, uint16(s.fetch()), localLink); err != nil {
			return op, err
		}
	case 0x94: // CXG @0x13eb／@0x1413：靜態鏈是目標段的 BASE
		seg := uint16(s.fetch())
		if err := s.callExternal(seg, uint16(s.fetch()), globalLink); err != nil {
			return op, err
		}
	case 0x95: // CXI @0x1457／@0x1475：往外走 DB 層
		seg := uint16(s.fetch())
		db := uint16(s.fetch())
		if err := s.callExternal(seg, uint16(s.fetch()), db); err != nil {
			return op, err
		}

	case 0x96: // RPU @0x1102
		if err := s.returnFrom(s.bytes()); err != nil {
			return op, err
		}

	case 0xcb: // CHK @0x0830：TOS 上界、TOS-1 下界、TOS-2 值；值留在堆疊上
		hi, lo := int16(s.pop()), int16(s.pop())
		v := int16(s.TOS())
		if v < lo || v > hi {
			return op, &Fault{at, fmt.Sprintf("值 %d 不在 %d..%d 之內", v, lo, hi)}
		}

	default:
		s.IPC = at // 沒執行成功就不要動 IPC
		return op, &Unimplemented{Op: op, IPC: at}
	}
	return op, nil
}

// chain 往外走 db 層靜態鏈，回那一層的 MP。MSSTAT 在 MP+0。
func (s *State) chain(db uint16) uint16 {
	bp := s.Local - 8
	for ; db > 0; db-- {
		bp = s.Load(bp)
	}
	return bp
}

// codeWord 讀程式碼段的一個 word。
func (s *State) codeWord(off uint16) uint16 {
	if int(off)+1 >= len(s.Code) {
		return 0
	}
	return binary.LittleEndian.Uint16(s.Code[off:])
}

// call 建一個活動記錄然後跳進去（@0x101b → @0x103a → @0x1057）。
//
// 字典項指的是 DATASIZE，第一條指令在它後面一個 word（spec 01 §5.4）。
func (s *State) call(proc, static uint16) error {
	entry := s.codeWord(s.ProcDict - 2*proc)
	if entry == 0 {
		return &Fault{s.IPC, fmt.Sprintf("程序 %d 的字典項是 0", proc)}
	}
	hdr := entry * 2
	dataSize := int16(s.codeWord(hdr))
	if dataSize < 0 {
		return &Fault{s.IPC, fmt.Sprintf("程序 %d 的第一條是原生碼", proc)}
	}

	// 配置區域資料：新的 SP ＝ 舊的 SP − 2×DATASIZE（@0x02da 算出來的值）。
	s.SP -= 2 * uint16(dataSize)

	mp := s.Local - 8
	s.push(s.Proc) // MSPROC
	s.Proc = proc
	s.push(s.ERec) // MSENV
	s.push(s.IPC)  // MSIPC：呼叫端的下一條
	s.push(mp)     // MSDYN
	s.push(static) // MSSTAT
	s.Local = s.SP + 8
	s.IPC = hdr + 2
	return nil
}

// callExternal 是跨段呼叫：先切段，再照同段的方式建框。
//
// link 是靜態鏈的來源：localLink 用呼叫者的 MP、globalLink 用**目標段**的 BASE，
// 其餘的值是要往外走幾層靜態鏈。
func (s *State) callExternal(seg, proc, link uint16) error {
	if s.Env == nil {
		return &Fault{s.IPC, "沒有 Environment，跨段呼叫做不了"}
	}
	// 段 1 的內嵌程序在切段之前就攔下來——原版也是這個順序（@0x13f4），
	// 而且它根本不換段。
	if seg == 1 && s.Env.Intrinsic(proc) {
		return &IntrinsicCall{Proc: proc}
	}
	target, err := s.Env.ByNumber(seg)
	if err != nil {
		return err
	}

	// 靜態鏈要在切段**之前**算——localLink 與走鏈都是呼叫端這一側的東西。
	var static uint16
	switch link {
	case localLink:
		static = s.Local - 8
	case globalLink:
		static = target.Global - 8
	default:
		static = s.chain(link)
	}

	old := s.ERec
	s.enter(target)
	if err := s.call(proc, static); err != nil {
		return err
	}
	s.Store(s.Local-8+6, old) // @0x10db：MSENV 改回呼叫端的 E_Rec
	return nil
}

// enter 把目前的執行環境換成另一段（@0x0fba）。
func (s *State) enter(t *Segment) {
	s.Code = t.Code
	s.Global = t.Global
	s.ProcDict = t.ProcDict
	s.ConstPool = t.ConstPool
	s.ERec = t.ERec
}

// 靜態鏈的來源。0 與 1 以上都是「往外走幾層」，所以哨兵值要挑不會撞到的。
const (
	localLink  = 0xFFFF
	globalLink = 0xFFFE
)

// returnFrom 拆掉活動記錄（@0x1102）。paramBytes 是要從堆疊上削掉的參數位元組數。
func (s *State) returnFrom(paramBytes uint16) error {
	mp := s.Local - 8
	// MSCW 裡的 E_Rec 與目前的不同就是跨段返回，要先把段換回去。
	if erec := s.Load(mp + 6); erec != s.ERec {
		if s.Env == nil {
			return &Fault{s.IPC, "沒有 Environment，跨段返回做不了"}
		}
		back, err := s.Env.ByERec(erec)
		if err != nil {
			return err
		}
		s.enter(back)
	}
	s.SP = mp - 2
	s.pop()          // [MP−2]：原版把它放進 di 就沒再用
	s.pop()          // MSSTAT
	newMP := s.pop() // MSDYN
	s.Local = newMP + 8
	s.IPC = s.pop()  // MSIPC
	s.pop()          // MSENV：段已經換回去了，這裡只是把它丟掉
	s.Proc = s.pop() // MSPROC
	s.SP += paramBytes
	return nil
}

// intermediate 算中介層變數的位址：往外走 db 層靜態鏈，再加變數偏移。
//
// 助手 @0x093a：`bp = bx − 8` 回到 MP，走 db 次 `bp = [bp]`（MSSTAT 在 MP+0），
// 最後 `bp += 2n + 8` 把基底加回去。db 為 0 就是目前這一層。
func (s *State) intermediate(db uint16) uint16 {
	bp := s.Local - 8
	for ; db > 0; db-- {
		bp = s.Load(bp)
	}
	return bp + s.bytes() + 8
}

// jumpByte／jumpWord 是相對跳躍：位移從**運算元之後**算起，
// 因為原版是先 lodsb／lodsw 再 `add si, ax`。
//
// ⚠ 不要寫成 `s.IPC = uint16(int16(s.IPC+1) + int16(int8(s.fetch())))`。
// Go 沒有規定 `a + b` 兩個運算元的求值順序，`s.IPC+1` 可能在 fetch
// 已經推進 IPC **之後**才被讀到——結果是時對時錯的差一個位元組。
// 先取出來放進變數，順序就沒有懸念。
func (s *State) jumpByte() {
	d := int8(s.fetch())
	s.IPC = uint16(int16(s.IPC) + int16(d))
}

func (s *State) jumpWord() {
	d := int16(s.fetchWord())
	s.IPC = uint16(int16(s.IPC) + d)
}

// TOS0 讀堆疊上第 n 個 word（0 是 TOS），不動堆疊。
func (s *State) TOS0(n uint16) uint16 { return s.Load(s.SP + 2*n) }

// mask 是 n 個位元的遮罩，也就是原版 cs:1FB6h 那張表的第 n 項。
func mask(n uint16) uint16 {
	if n >= 16 {
		return 0xFFFF
	}
	return 1<<n - 1
}

func ror(v, n uint16) uint16 { n &= 15; return v>>n | v<<(16-n)&0xFFFF }
func rol(v, n uint16) uint16 { n &= 15; return v<<n&0xFFFF | v>>(16-n) }

func (s *State) byteAt(off uint16) uint8 {
	if int(off) >= len(s.Data) {
		return 0
	}
	return s.Data[off]
}

func (s *State) setByteAt(off uint16, v uint8) {
	if int(off) < len(s.Data) {
		s.Data[off] = v
	}
}

func (s *State) compare(ok func(a, b int16) bool) {
	b := int16(s.pop())
	if ok(int16(s.pop()), b) {
		s.push(1)
	} else {
		s.push(0)
	}
}

func (s *State) compareU(ok func(a, b uint16) bool) {
	b := s.pop()
	if ok(s.pop(), b) {
		s.push(1)
	} else {
		s.push(0)
	}
}
