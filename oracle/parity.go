//go:build oracle

package oracle

import (
	"fmt"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/internal/pmachine"
	"github.com/wicanr2/dosgolem"
)

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
		Env:       s.M.Read16(uint32(dataSeg)*16 + 0x3e),
	}, nil
}

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
	res := &ParityResult{Ops: map[uint8]int{}}

	for i := 0; i < n; i++ {
		at, want := ours.IPC, uint8(0)
		if int(at) < len(ours.Code) {
			want = ours.Code[at]
		}

		op, err := ours.Step()
		if err != nil {
			res.Err = err
			return res, nil
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
			res.Err = fmt.Errorf("oracle: 原版沒有走到下一條 p-code（預算 %d 條機器指令）", budget)
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
