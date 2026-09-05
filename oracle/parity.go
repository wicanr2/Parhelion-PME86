//go:build oracle

package oracle

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/internal/pmachine"
	"github.com/wicanr2/dosgolem"
)

// errOriginalIdle 是「原版沒有再執行 p-code」。
//
// **這不是失敗。** 開機跑完之後系統停在等鍵盤的迴圈，那時它就不再執行
// 任何 p-code；對拍走到這裡表示整段工作量都走完了。
var errOriginalIdle = errors.New("oracle: 原版沒有再執行 p-code（多半停在等輸入的迴圈）")

// OriginalIdle 回報停下來的原因是不是「原版沒事做了」。
func OriginalIdle(err error) bool { return errors.Is(err, errOriginalIdle) }

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// Capture 把原版此刻的 p-machine 狀態抄成一份可以自己跑的 State。
//
// **只在剛落到 dispatch 目標時呼叫才有意義**（也就是 Trace 停下來的那一刻）：
// 那時 opcode 已經被 lodsb 取走、常式本體還沒跑，所以堆疊是「這條指令執行前」
// 的樣子。IPC 因此要回退一個位元組，指回那個 opcode 本身。
//
// 兩個段就夠：程式碼在 ds、求值堆疊與區域全域都在 ss
// （docs/10-interpreter/machine-state.md）。
func (s *System) Capture() (*pmachine.State, error) {
	if s.targets == nil {
		return nil, fmt.Errorf("oracle: 還沒定位 PME")
	}
	c := s.M.CPU
	codeSeg, dataSeg := c.Seg[dosgolem.DS], c.Seg[dosgolem.SS]
	code := s.M.SegmentBytes(codeSeg, segLen(codeSeg))
	data := s.M.SegmentBytes(dataSeg, segLen(dataSeg))
	if code == nil || data == nil {
		return nil, fmt.Errorf("oracle: 段 %04Xh／%04Xh 讀不出來", codeSeg, dataSeg)
	}
	return &pmachine.State{
		Code:   code,
		Data:   data,
		IPC:    c.R[dosgolem.SI] - 1, // 回退到 opcode 本身
		SP:     c.R[dosgolem.SP],
		Local:  c.R[dosgolem.BX],
		Global: c.R[dosgolem.DX],
		// 直譯器的狀態區就在同一個段的低位址處
		// （docs/10-interpreter/machine-state.md 的表）。
		ConstPool: s.M.Read16(uint32(dataSeg)*16 + 0x42),
		ProcDict:  s.M.Read16(uint32(dataSeg)*16 + 0x36),
		Proc:      s.M.Read16(uint32(dataSeg)*16 + 0x32),
		ERec:      s.M.Read16(uint32(dataSeg)*16 + 0x3e),
		Flipped:   s.M.Read16(uint32(dataSeg)*16+0x44) != 1,
		TIB:       s.M.Read16(uint32(dataSeg)*16 + 0x3c),
		ProcHigh:  s.M.Read8(uint32(dataSeg)*16 + 0xe6),
		Env:       &liveEnv{s},
	}, nil
}

// ResolveSegment 用原版執行時的資料結構算出某個 segment number 對應哪一段。
// 給測試與探路用；跑起來的時候是 pmachine 自己透過 Environment 呼叫。
func (s *System) ResolveSegment(seg uint16) (*pmachine.Segment, error) {
	return (&liveEnv{s}).ByNumber(seg)
}

// ResolveSegment2 用 E_Rec 指標算出那一段的執行期樣貌。給測試與探路用。
func (s *System) ResolveSegment2(erec uint16) (*pmachine.Segment, error) {
	return (&liveEnv{s}).ByERec(erec)
}

// SegmentSwitch 是一次換段：直譯器的 E_Rec 從 From 變成 To。
type SegmentSwitch struct {
	From, To uint16
	Step     uint64 // 發生在第幾條機器指令
	InsnCS   uint16 // 做這件事的那條機器指令在哪
	InsnIP   uint16
}

// WatchSegmentSwitches 監看直譯器的 E_Rec（`ss:3Eh`），把每一次換段記下來。
//
// **不用輪詢。** 兩條 p-code 之間可以換過去又換回來，輪詢只會看到沒變——
// 而那不會報錯，只會讓某些換段看起來從沒發生過。
//
// 回傳的 slice 會隨著跑而長；停止監看用 s.M.Unwatch(id)。
func (s *System) WatchSegmentSwitches() (log *[]SegmentSwitch, id int) {
	out := new([]SegmentSwitch)
	addr := uint32(s.M.CPU.Seg[dosgolem.SS])*16 + erecOff
	id = s.M.WatchWord(addr, func(m *dosgolem.Machine, _ uint32, from, to uint16) {
		cs, ip := m.Insn()
		*out = append(*out, SegmentSwitch{From: from, To: to, Step: m.Steps, InsnCS: cs, InsnIP: ip})
	})
	return out, id
}

// IsIntrinsic 回報段 1 的這支程序是不是內嵌在直譯器裡的原生碼。
func (s *System) IsIntrinsic(proc uint16) bool { return (&liveEnv{s}).Intrinsic(proc) }

// liveEnv 用原版執行時的資料結構解析段：E_Vec → E_Rec → SIB → Codepool。
//
// **這一段知識刻意留在這裡，不放進 pmachine。** SIB 與 Codepool 是作業系統的
// 資料結構，換一個宿主（例如以後從 codefile 自己載入）就完全不同；
// p-machine 只需要「給我那一段的樣子」。
type liveEnv struct{ s *System }

func (e *liveEnv) ByNumber(seg uint16) (*pmachine.Segment, error) {
	erec := e.s.DataWord(e.s.DataWord(evecOff) + 2*seg)
	if erec == 0 {
		return nil, fmt.Errorf("oracle: E_Vec 裡沒有段 %d", seg)
	}
	return e.ByERec(erec)
}

// ByERec 照 @0x0fba 的做法把一個 E_Rec 換算成執行期的段。
//
// `Seg_Base` 是 SIB 開頭的兩個 word：第一個是**指向 Codepool 基底的指標**
// （相對直譯器的資料段），第二個是**池內的位元組偏移**。
// 段值 ＝ Codepool 基底的 paragraph ＋ 偏移 ÷ 16。
//
// 指標為 0 表示那一段就在直譯器自己的段裡（@0x1BEE 的 `test bp,bp; jz`）。
// 偏移為 0 表示段不在記憶體——原版這時退回指令開頭發 segment fault（@0x143d）。
func (e *liveEnv) ByERec(erec uint16) (*pmachine.Segment, error) {
	envData := e.s.DataWord(erec)
	sib := e.s.DataWord(erec + 4)
	if sib == 0 {
		return nil, fmt.Errorf("oracle: E_Rec %04Xh 沒有 SIB", erec)
	}
	ptr, off := e.s.DataWord(sib), e.s.DataWord(sib+2)
	if off == 0 {
		return nil, pmachine.ErrNotResident
	}
	pool := e.s.M.CPU.Seg[dosgolem.SS]
	if ptr != 0 {
		lo, hi := e.s.DataWord(ptr), e.s.DataWord(ptr+2)
		pool = hi>>4 | (lo&0xF)<<12
	}
	seg := pool + off>>4

	code := e.s.M.SegmentBytes(seg, segLen(seg))
	if len(code) < codeHeaderBytes {
		return nil, fmt.Errorf("oracle: 段值 %04Xh 讀不出程式碼", seg)
	}
	return &pmachine.Segment{
		Code:      code,
		Global:    envData + 8,
		ProcDict:  binary.LittleEndian.Uint16(code[0:]) * 2,
		ConstPool: binary.LittleEndian.Uint16(code[0x0e:]) * 2,
		ERec:      erec,
	}, nil
}

// Intrinsic 查直譯器裡的內嵌程序表。
//
// 表在映像偏移 0x1f56 起 48 格，以程序號為索引；非零就是有內嵌實作
// （docs/10-interpreter/segment-switching.md）。原版的 CXG 在切段之前查它。
func (e *liveEnv) Intrinsic(proc uint16) bool {
	base, _, ok := e.s.PME()
	if !ok || proc > intrinsicMax {
		return false
	}
	return e.s.M.Read16(base+intrinsicTable+uint32(proc)*2) != 0
}

// 直譯器狀態區裡用得到的偏移、段頭長度、內嵌程序表。
const (
	evecOff         = 0x3a
	erecOff         = 0x3e
	codeHeaderBytes = 22
	intrinsicTable  = 0x1f56
	intrinsicMax    = 0x2f
)

// segLen 是一個段從基底到記憶體上緣能取多少，最多一整個段。
func segLen(seg uint16) int {
	room := dosgolem.MemSize - int(seg)*16
	if room > 0x10000 {
		return 0x10000
	}
	return room
}

// Divergence 是對拍上第一個對不起來的地方。
type Divergence struct {
	Step  int    // 第幾條 p-code 之後
	Op    uint8  // 剛執行的 opcode
	IPC   uint16 // 剛執行的那條在哪
	Field string // 哪一項對不上
	Want  uint16 // 原版是多少
	Got   uint16 // 我們是多少
}

func (d *Divergence) Error() string {
	name := pcode.Mnemonic(d.Op)
	if name == "" {
		name = "?"
	}
	return fmt.Sprintf("第 %d 條（%04Xh %02X %s）之後 %s 對不上：原版 %04X，我們 %04X",
		d.Step, d.IPC, d.Op, name, d.Field, d.Want, d.Got)
}

// ParityResult 是一次對拍的結果。
type ParityResult struct {
	Steps   int           // 兩邊一致地走了幾條
	Ops     map[uint8]int // 走過哪些 opcode、各幾次
	Diverge *Divergence   // 第一個分歧；nil 表示走完都一致
	Err     error         // 我們這邊停下來的原因（通常是還沒實作的 opcode）

	// Resyncs 是「讓原版自己走、我們重抄狀態」的次數，Skipped 是那幾條各是什麼。
	//
	// **這些條沒有被驗證過。** 分母要看 Steps，不是 Steps+Resyncs。
	Resyncs int
	Skipped map[uint8]int
}

// hostOwned 回報這個錯誤是不是「本來就該由宿主做的事」。
//
// 四種，每一種都是 p-machine 依定義就要交出去的：
//
//   - 段 1 的內嵌原生程序：直譯器自己的機器碼
//   - `NAT`：程式碼段裡的 8086 機器碼，要執行它得先有一台 8086
//   - 段還沒載入：作業系統要去磁碟讀
//   - 換 task：排程器把整個狀態換成另一個 TIB 的
//
// **p-code 指令沒實作不算。** 那種也重抄的話，「還沒做」會看起來像「做完了」，
// 而對拍就失去意義了。
func hostOwned(err error) bool {
	var ic *pmachine.IntrinsicCall
	var nc *pmachine.NativeCall
	var ts *pmachine.TaskSwitch
	return errors.As(err, &ic) || errors.As(err, &nc) || errors.As(err, &ts) ||
		errors.Is(err, pmachine.ErrNotResident)
}

// Parity 從原版此刻的狀態展開，兩邊各走 n 條 p-code，逐條比對。
//
// 比的是**執行後的狀態**：下一條的位址、求值堆疊頂的位置與內容。
// 三項只要有一項對不上就停——繼續跑下去只會讓錯誤累積成看不懂的雜訊。
//
// 停下來的三種情形都要分得出來：走完 n 條、我們有指令還沒實作、真的對不上。
func (s *System) Parity(n int, budget uint64) (*ParityResult, error) {
	ours, err := s.Capture()
	if err != nil {
		return nil, err
	}
	res := &ParityResult{Ops: map[uint8]int{}, Skipped: map[uint8]int{}}

	for i := 0; i < n; i++ {
		at, want := ours.IPC, uint8(0)
		if int(at) < len(ours.Code) {
			want = ours.Code[at]
		}

		op, err := ours.Step()
		if err != nil {
			if !hostOwned(err) {
				res.Err = err
				return res, nil
			}
			// 宿主自己該做的事：讓原版走完那一條，再從它的狀態重抄一份。
			// 這一條**沒有被驗證**，所以不進 Steps。
			if rows, terr := s.Trace(1, budget); terr != nil || len(rows) == 0 {
				res.Err = firstErr(terr, errOriginalIdle)
				return res, nil
			}
			next, cerr := s.Capture()
			if cerr != nil {
				res.Err = cerr
				return res, nil
			}
			ours = next
			res.Resyncs++
			res.Skipped[op]++
			continue
		}
		if op != want {
			return nil, fmt.Errorf("oracle: 取指令自己就不一致（%04Xh 該是 %02X，取到 %02X）", at, want, op)
		}
		res.Ops[op]++

		// 原版走同一條。Trace 停在下一條的入口，所以回來的那一列
		// 描述的是「執行完之後」的狀態。
		rows, terr := s.Trace(1, budget)
		if terr != nil {
			res.Err = terr
			return res, nil
		}
		if len(rows) == 0 {
			res.Err = errOriginalIdle
			return res, nil
		}
		r := rows[0]

		for _, chk := range []struct {
			field     string
			want, got uint16
		}{
			{"IPC", r.IPC, ours.IPC},
			{"SP", r.SP, ours.SP},
			{"TOS", r.TOS, ours.TOS()},
		} {
			if chk.want != chk.got {
				res.Diverge = &Divergence{
					Step: i, Op: op, IPC: at,
					Field: chk.field, Want: chk.want, Got: chk.got,
				}
				return res, nil
			}
		}
		res.Steps++
	}
	return res, nil
}
