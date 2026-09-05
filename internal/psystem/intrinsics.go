package psystem

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// 段 1 的內嵌原生程序。**這是宿主與 p-machine 之間唯一的介面**：
// 作業系統要搬記憶體、碰磁碟、開關中斷，都是從這裡出去的。
//
// 每一支的參數次序都由原版那一支的 pop 順序讀出來，位址記在各自的註解裡。

// intrinsic 執行段 1 的程序 proc。呼叫者已經把參數推在堆疊上。
func (m *Machine) intrinsic(proc uint16, at, sp uint16) error {
	s := m.S
	if resumeAtOperand[proc] {
		// **這一支做完會回到運算元那個位元組，不是它後面。**
		//
		// `SCXG` @0x0545 一進來就把 `si` 存進 `ss:24h`，那時 si 指著程序號；
		// @0x1410 的 `inc si` 之後才跳進原生碼。多數原生程序自己會再存一次
		// （例如 `MOVELEFT` @0x192C），所以還原到的是程序號**後面**；
		// 沒有自己存的那幾支還原到的就是程序號本身，那個位元組會被當成
		// 下一條指令執行。程序 4 的號碼是 `04` ＝ `SLDC 4`。
		defer func() { s.IPC-- }()
	}
	switch proc {
	case 15: // MOVELEFT @0x191E：由低位址往高位址搬
		n := int16(s.Pop())
		dst := m.popAddr()
		src := m.popAddr()
		if n > 0 {
			copy(s.Data[dst:dst+uint16(n)], s.Data[src:src+uint16(n)])
		}
	case 16: // MOVERIGHT @0x1970：由高位址往低位址搬（重疊時方向相反）
		n := int16(s.Pop())
		dst := m.popAddr()
		src := m.popAddr()
		for i := int16(n) - 1; i >= 0; i-- {
			s.Data[dst+uint16(i)] = s.Data[src+uint16(i)]
		}
	case 21: // FILLCHAR @0x18DD
		ch := byte(s.Pop())
		n := int16(s.Pop())
		dst := m.popAddr()
		for i := int16(0); i < n; i++ {
			s.Data[dst+uint16(i)] = ch
		}
	case 18, 40: // UNITREAD @0x2C5A
		return m.unitIO(true, at, sp)
	case 19, 41: // UNITWRITE @0x2C5F
		return m.unitIO(false, at, sp)

	case 4: // 照 relocation list 修剛載入那一段 @0x1B2A
		return m.relocate()

	case 24: // 從 codepool 搬進資料段 @0x1A6E
		n := s.Pop()
		src := s.Pop()
		pool := m.poolFromPtr(s.Pop())
		dst := s.Pop()
		return m.poolCopy(uint32(pool)*16+uint32(src), uint32(dataSeg)*16+uint32(dst), n)
	case 25: // 從資料段搬進 codepool @0x1A94
		n := s.Pop()
		dst := s.Pop()
		pool := m.poolFromPtr(s.Pop())
		src := s.Pop()
		return m.poolCopy(uint32(dataSeg)*16+uint32(src), uint32(pool)*16+uint32(dst), n)
	case 26: // 把載入的段逐 word 交換位元組 @0x1ABA
		n := s.Pop()
		at := s.Pop()
		sib := m.word(s.Pop() + 4)
		base := uint32(m.poolBase(sib))*16 + uint32(m.word(sib+2)) + 2*uint32(at)
		for i := uint32(0); i < uint32(n); i++ {
			a := base + 2*i
			if int(a)+1 >= len(m.Mem) {
				break
			}
			m.Mem[a], m.Mem[a+1] = m.Mem[a+1], m.Mem[a]
		}
	case 14: // 在兩個 codepool 位置之間搬整段 @0x1AE4
		src := s.Pop()
		pool := m.poolFromPtr(s.Pop())
		sib := s.Pop()
		n := m.word(sib+0x14) * 2
		return m.poolCopy(uint32(pool)*16+uint32(src),
			uint32(m.poolBase(sib))*16+uint32(m.word(sib+2)), n)

	case 39, 46: // 把一段從磁碟讀進 codepool @0x1BAF
		return m.loadSegment()

	case 38: // 用 8 個位元組的名字查一棵二元樹 @0x1CF6
		key := s.Pop()
		out := s.Pop()
		node := s.Pop()
		s.Pop() // 函式結果的位置
		res, at := m.lookup(node, key)
		m.setWord(out, at)
		s.Push(uint16(res))

	case 29: // ATTACH @0x1841：把號誌掛到中斷向量
		vec := s.Pop()
		sem := s.Pop()
		m.setWord(vectorTable+2*vec, sem)

	case 22: // SCAN @0x1992
		s.Pop() // 函式結果的位置，最後推回去
		at := m.popAddr()
		ch := byte(s.Pop())
		mode := s.Pop()
		n := int16(s.Pop())
		s.Pop() // @0x19A2 把它蓋掉了，沒有用到
		s.Push(uint16(m.scan(at, ch, mode&0xFF == 0, n)))

	case 36, 45: // 問裝置狀態 @0x2C1B：模式碼 8
		ctrl := s.Pop()
		buf := s.Pop()
		u := s.Pop()
		m.setWord(ioResult, m.unitStatus(u))
		m.logIO("問 unit %d 狀態（control %04X，緩衝 %04X）→ IORESULT %d",
			u, ctrl, buf, m.word(ioResult))

	case 34, 44: // 等裝置做完 @0x2C36：模式碼 4，只吃一個 unit
		u := s.Pop()
		m.setWord(ioResult, m.unitStatus(u))
		m.logIO("等 unit %d → IORESULT %d", u, m.word(ioResult))

	case 20: // TIME(var hi, lo) @0x2CB8：系統時鐘，單位是 1／60 秒
		lo := s.Pop()
		hi := s.Pop()
		m.setWord(lo, uint16(m.Clock))
		m.setWord(hi, uint16(m.Clock>>16))

	case 30: // IORESULT @0x2B02：丟掉函式結果的位置，換成 ss:0E6h
		s.Pop()
		s.Push(m.word(ioResult))
	case 23: // IOCHECK @0x2B0D：IORESULT 不是 0 就發執行期錯誤
		if r := m.word(ioResult); r != 0 {
			return &IOError{Result: r, IPC: s.IPC}
		}

	case 27, 28: // @0x19F4／@0x1A00：關中斷／開中斷
		// 我們是單執行緒，沒有真的中斷可關。
	default:
		return &Trap{Proc: proc, IPC: s.IPC, Why: nativeName(proc)}
	}
	return nil
}

// popAddr 取一個陣列參數：堆疊上是（基底, 偏移）兩個 word，位址是兩者之和。
//
// 分成兩句是刻意的——Go **不定義** `a + b` 裡兩個呼叫的先後，
// 而 pop 有副作用。這裡相加剛好可交換，但寫成一句遲早會有不能交換的版本。
func (m *Machine) popAddr() uint16 {
	a := m.S.Pop()
	b := m.S.Pop()
	return a + b
}

// resumeAtOperand 是「做完之後回到程序號那個位元組」的那幾支：
// 它們沒有自己把 `si` 存進 `ss:24h`，出口卻是 @0x0200（會還原 si）。
var resumeAtOperand = map[uint16]bool{4: true}

// nativeName 是目前對那一支的認識。**沒讀過碼的就寫「還沒讀」**——
// 猜一個好聽的名字會讓「還沒查證」看起來像「已經知道」。
func nativeName(proc uint16) string {
	switch proc {

	case 14:
		return "在兩個池內段之間搬 word @0x1AE4"

	case 31, 42, 33, 43:
		return "裝置模式碼 3（清除？）@0x2BF1／@0x2BF7"
	case 32:
		return "還沒讀 @0x2826"
	case 37:
		return "還沒讀 @0x1C0B"

	case 24:
		return "從池內段搬進資料段 @0x1A6E"
	case 25:
		return "從資料段搬進池內段 @0x1A94"
	case 26:
		return "把載入的段逐 word 交換位元組 @0x1ABA"

	case 47:
		return "換一組浮點常式 @0x1A0C"
	}
	return "還沒讀"
}

// unitIO 是 `UNITREAD`（程序 18）與 `UNITWRITE`（程序 19）。
//
// 兩支的機器碼只差一個模式碼（@0x2C5A 的 `mov al,1`／@0x2C5F 的 `mov al,2`），
// 參數處理完全共用。堆疊上由頂往下是：
//
//	mode、block、length、緩衝區的偏移與基底、unit
//
// 也就是 Pascal 的 `UNITREAD(unit, buf, length, block, mode)` 由左往右推。
// 做完把結果寫進 IORESULT（`ss:0E6h`，@0x2B3B 一進去就把它清成 0）。
func (m *Machine) unitIO(read bool, at, sp uint16) error {
	s := m.S
	_ = s.Pop() // mode：目前沒有用到的位元
	blk := s.Pop()
	n := s.Pop()
	buf := m.popAddr()
	unit := s.Pop()

	m.setWord(ioResult, 0)
	defer func() {
		what := "寫"
		if read {
			what = "讀"
		}
		m.logIO("%s unit %d block %d 長度 %d 緩衝 %04X → IORESULT %d",
			what, unit, blk, n, buf, m.word(ioResult))
	}()
	if int(buf)+int(n) > len(s.Data) {
		m.setWord(ioResult, ioBadRequest)
		return nil
	}
	switch {
	case unit == hostGate: // DOS 主機的檔案系統閘道
		return m.hostRequest(buf, n, read)

	case unit == 1 || unit == 2: // CONSOLE:／SYSTERM:
		if read {
			// **鍵盤沒東西就不要硬回。** 真機器上這個讀是會等的；
			// 我們一條一條走沒辦法等，所以交回去讓使用端補字再繼續。
			// 隨便回一個「讀到 0 個位元組、IORESULT 0」的話，
			// 作業系統會把緩衝區裡的舊字元當成新按鍵，一路重複下去。
			if len(m.Keys) < int(n) {
				m.S.SP = sp // 退回去，補完字之後整條重跑
				m.S.IPC = at
				return &NeedInput{Want: int(n), Have: len(m.Keys)}
			}
			copy(s.Data[buf:buf+n], m.Keys[:n])
			m.Keys = m.Keys[n:]
			return nil
		}
		m.Console = append(m.Console, s.Data[buf:buf+n]...)
		return nil

	default:
		if r := m.unitStatus(unit); r != 0 {
			m.setWord(ioResult, r)
			return nil
		}
		// 「這台裝置在」與「這台裝置有磁碟」是兩件事：unit 4 與 13 會回答
		// 「在」，但我們沒有掛磁碟給它們。
		v := m.Units[unit]
		if v == nil {
			m.setWord(ioResult, ioNoVolume)
			return nil
		}
		lo := int(blk) * Block
		if lo < 0 || lo+int(n) > len(v.data) {
			m.setWord(ioResult, ioBadBlock)
			return nil
		}
		if read {
			copy(s.Data[buf:buf+n], v.data[lo:lo+int(n)])
			return nil
		}
		// 寫只改記憶體裡那份映像。要不要落回檔案是使用端的決定。
		v.Write(lo, s.Data[buf:buf+n])
		return nil
	}
}

// logIO 記一行裝置呼叫。上限是刻意的：開機會發幾百次，全留會蓋掉重點。
func (m *Machine) logIO(format string, a ...any) {
	if len(m.IOLog) < 200 {
		m.IOLog = append(m.IOLog, fmt.Sprintf(format, a...))
	}
}

// hostRequest 是 unit 128：DOS 主機的檔案系統閘道。
//
// 協定還沒解開，**目前照量到的行為回答**：七個位元組的請求，
// 驅動把第一個 word 換成 3 然後回 IORESULT 0；兩個位元組的請求原封不動，
// 回 IORESULT 10（沒有這個檔案）。作業系統開機時拿它問 DOS 那邊有沒有檔案，
// 得到「沒有」是正常結果。
//
// **這一格是「宿主答什麼」，不是「p-machine 怎麼算」**——真的要支援
// DOS 檔案系統得先把協定解出來。
func (m *Machine) hostRequest(buf, n uint16, read bool) error {
	m.logIO("主機閘道：%d 個位元組 %04X（%s）", n, buf, map[bool]string{true: "讀", false: "寫"}[read])
	if n >= 7 {
		m.setWord(buf, hostNoDOSFiles)
		return nil
	}
	m.setWord(ioResult, ioNoFile)
	return nil
}

// unitStatus 回報這個 unit 現在能不能用。
//
// **沒掛磁碟不是「錯的 unit 編號」。** 作業系統開機時會一個一個問過去，
// 得到 9（沒有這片磁碟）是正常結果，得到 0 會讓它以為那台有東西。
func (m *Machine) unitStatus(unit uint16) uint16 {
	if m.Units[unit] != nil {
		return 0
	}
	if r, ok := deviceStatus[unit]; ok {
		return r
	}
	return ioNoVolume
}

// deviceStatus 是這台機器上除了磁碟之外還有哪些裝置在。
//
// **量原版量出來的**：作業系統開機時會把 unit 2 到 22 問過一遍，
// 答案不是「有」就是「沒有」——只有 unit 3 回的是 3（不支援這種請求），
// 那是它與「沒有這片磁碟」的差別。unit 128 是這台 DOS 主機自己的檔案系統閘道。
var deviceStatus = map[uint16]uint16{
	1: 0, 2: 0, // CONSOLE:／SYSTERM:
	3:   ioBadRequest,
	4:   0,
	13:  0,
	128: 0,
}

// IORESULT 與幾個用得到的碼（手冊 p.117 的 I/O 錯誤表）。
const (
	ioResult       = 0x00E6
	ioBadBlock     = 1
	ioBadUnit      = 2
	ioBadRequest   = 3
	ioNoInput      = 4
	ioNoVolume     = 9
	ioNoFile       = 10
	hostGate       = 128
	hostNoDOSFiles = 3
	// vectorTable 是中斷向量對號誌的表（`ss:4Eh` 起，上限 64 個）。
	// 中斷來的時候直譯器就 SIGNAL 對應的那個號誌（@0x1878）。
	vectorTable    = 0x004E
	ioWriteProtect = 16
)

// loadSegment 把一段程式碼從磁碟讀進 codepool（程序 39／46，@0x1BAF）。
//
// 參數只有一個 E_Rec，其餘全部從它的 SIB 讀出來——@0x1BB8 起那幾條
// 逐一取的是 `Seg_Base`（+0／+2）、`Seg_Leng`（+14h，word 數，`shl` 成位元組）、
// 磁碟塊號（+16h）與 unit 的來源（+18h 是指標，指向的那個 word 才是 unit）。
// 讀完之後 @0x1BE0 把裝置層的回傳值推回堆疊。
//
// **目的地在 codepool，不在資料段**——那正是 `Seg_Base` 要用兩個 word 的理由。
func (m *Machine) loadSegment() error {
	s := m.S
	erec := s.Pop()
	s.Pop() // 函式結果的位置，等一下推回去

	sib := m.word(erec + 4)
	if sib == 0 {
		s.Push(ioBadRequest)
		return nil
	}
	n := int(m.word(sib+0x14)) * 2
	blk := int(m.word(sib + 0x16))
	unit := m.word(m.word(sib + 0x18))
	dst := int(m.poolBase(sib))*16 + int(m.word(sib+2))

	m.setWord(ioResult, 0)
	v := m.Units[unit]
	switch {
	case v == nil:
		m.setWord(ioResult, ioBadUnit)
	case blk*Block+n > len(v.data) || dst+n > len(m.Mem):
		m.setWord(ioResult, ioBadBlock)
	default:
		copy(m.Mem[dst:dst+n], v.data[blk*Block:])
	}
	m.logIO("載段 E_Rec %04X SIB %04X：base %04X/%04X leng %04X blk %04X unitp %04X"+
		" → unit %d block %d 長度 %d 池 %05X，IORESULT %d",
		erec, sib, m.word(sib), m.word(sib+2), m.word(sib+0x14), m.word(sib+0x16),
		m.word(sib+0x18), unit, blk, n, dst, m.word(ioResult))
	s.Push(m.word(ioResult))
	return nil
}

// poolFromPtr 把一對 Mem_Ptr（20 bit 的位元組位址拆成兩個 word）算成 paragraph
// （助手 @0x1BEE）。指標為 0 表示「就在直譯器自己的資料段裡」。
func (m *Machine) poolFromPtr(ptr uint16) uint16 {
	if ptr == 0 {
		return dataSeg
	}
	lo, hi := m.word(ptr), m.word(ptr+2)
	return hi>>4 | (lo&0xF)<<12
}

// poolBase 是那一段所在的 codepool paragraph。
func (m *Machine) poolBase(sib uint16) uint16 { return m.poolFromPtr(m.word(sib)) }

// poolCopy 在實體記憶體之間搬位元組。**要能處理重疊**——@0x1AE4 就是為了
// 重疊才判斷方向的，而 Go 的 copy 本來就處理得了。
func (m *Machine) poolCopy(src, dst uint32, n uint16) error {
	if int(src)+int(n) > len(m.Mem) || int(dst)+int(n) > len(m.Mem) {
		m.setWord(ioResult, ioBadRequest)
		return fmt.Errorf("psystem: 搬 %d 個位元組 %05X → %05X 超出記憶體", n, src, dst)
	}
	copy(m.Mem[dst:dst+uint32(n)], m.Mem[src:src+uint32(n)])
	return nil
}

// relocate 照 relocation list 修剛載入那一段裡的位址（程序 4，@0x1B2A）。
//
// 清單從段內 word 1 指的地方**往回走**（@0x1B56 的 `std`），一組一組讀：
//
//	A：高位元組是型別，低位元組是 segment number
//	B：這一組有幾項
//	接著 B 個 word，每個是段內的位元組偏移
//
// A 的高位元組為 0 就結束。三種型別：
//
//	1  跳過（@0x1BA5 只把游標往回移，不改任何東西）
//	2  加上某一段的 `Env_Data`——那一段的全域基底（@0x1B8A）
//	3  原生碼用的指標，會寫進 `cs`（@0x1B73）
//
// 型別 3 只有原生碼段才有，p-code 段碰不到。碰到就當場說，不要假裝做完了。
func (m *Machine) relocate() error {
	s := m.S
	erec := s.Pop()
	evec := m.word(erec + 2)
	sib := m.word(erec + 4)
	seg := (uint32(m.poolBase(sib)) + uint32(m.word(sib+2))>>4) * 16

	rw := func(off uint16) uint16 {
		a := seg + uint32(off)
		if int(a)+1 >= len(m.Mem) {
			return 0
		}
		return binary.LittleEndian.Uint16(m.Mem[a:])
	}
	ww := func(off, v uint16) {
		a := seg + uint32(off)
		if int(a)+1 < len(m.Mem) {
			binary.LittleEndian.PutUint16(m.Mem[a:], v)
		}
	}

	si := rw(2) * 2
	if si == 0 {
		return nil // 沒有 relocation list
	}
	for {
		a := rw(si)
		si -= 2
		if a>>8 == 0 {
			return nil
		}
		n := rw(si)
		si -= 2
		if n == 0 {
			continue
		}
		switch a >> 8 {
		case 1:
			si -= 2 * n
		case 2:
			add := m.word(m.word(evec + 2*(a&0xff)))
			for i := uint16(0); i < n; i++ {
				at := rw(si)
				si -= 2
				ww(at, rw(at)+add)
			}
		default:
			return &Trap{Proc: 4, IPC: s.IPC,
				Why: fmt.Sprintf("relocation 型別 %d 是原生碼用的，還沒做", a>>8)}
		}
	}
}

// scan 是 `SCAN(length, partial, array)`（程序 22，@0x1992）。
//
// `repnz scasb`／`repz scasb` 兩條路：**mode 為 0 找相等，不是 0 找不等**
// （@0x19AB 的 `test ah,ah`）。個數帶正負號，負的往回掃（@0x19A8 的 `jl`）。
//
// 回傳走了幾格，帶方向；掃完沒中就回原來的個數。
func (m *Machine) scan(at uint16, ch byte, wantEqual bool, n int16) int16 {
	step, count := int16(1), n
	if n < 0 {
		step, count = -1, -n
	}
	for i := int16(0); i < count; i++ {
		if (m.S.Data[at+uint16(i*step)] == ch) == wantEqual {
			return i * step
		}
	}
	return n
}

// lookup 用 8 個位元組的名字在一棵二元樹裡找（程序 38，@0x1CF6）。
//
// 節點的版面：名字 8 個位元組、`+8` 是「比較小的那邊」、`+0Ah` 是另一邊
// （@0x1D47／@0x1D38 各走一條）。回傳 0 找到、1 或 −1 是停在哪一邊沒路了，
// 並且**不管有沒有找到都把停下來的那個節點寫回去**（@0x1D1A）。
func (m *Machine) lookup(node, key uint16) (int16, uint16) {
	for node != 0 {
		cmp := bytes.Compare(m.S.Data[node:node+8], m.S.Data[key:key+8])
		switch {
		case cmp == 0:
			return 0, node
		case cmp < 0:
			next := m.word(node + 8)
			if next == 0 {
				return 1, node
			}
			node = next
		default:
			next := m.word(node + 0x0A)
			if next == 0 {
				return -1, node
			}
			node = next
		}
	}
	return -1, node
}
