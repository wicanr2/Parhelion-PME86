package psystem

import (
	"errors"
	"fmt"

	"github.com/wicanr2/Parhelion-PME86/internal/pmachine"
)

// Trap 是「跑不下去了，而且是宿主這一側缺東西」。
//
// 與 p-code 指令沒實作分得開：那個是 pmachine.Unimplemented。
type Trap struct {
	Proc uint16
	IPC  uint16
	Why  string
}

func (t *Trap) Error() string {
	return fmt.Sprintf("psystem: %04Xh 的段 1 程序 %d（%s）還沒做", t.IPC, t.Proc, t.Why)
}

// IOError 是作業系統自己檢查出來的 I/O 錯誤（`IOCHECK`，@0x2B0D）。
//
// **與「我們做不出來」是兩件事**：這表示宿主的裝置層回了一個錯誤碼，
// 而作業系統照它自己的規則決定停下來。
type IOError struct {
	Result uint16
	IPC    uint16
}

func (e *IOError) Error() string {
	return fmt.Sprintf("psystem: %04Xh 的 IOCHECK 發現 IORESULT ＝ %d", e.IPC, e.Result)
}

// NeedInput 是「鍵盤沒東西可以讀」。
//
// **與錯誤不同**：機器好好的，只是在等人打字。補上 `Machine.Keys` 再呼叫
// `Run` 就會從同一條指令繼續。
type NeedInput struct{ Want, Have int }

func (e *NeedInput) Error() string {
	return fmt.Sprintf("psystem: 在等鍵盤（要 %d 個位元組，手上有 %d 個）", e.Want, e.Have)
}

// WaitingForInput 回報停下來的原因是不是「在等鍵盤」。
func WaitingForInput(err error) bool {
	var ni *NeedInput
	return errors.As(err, &ni)
}

// Step 走一條 p-code；碰到宿主該做的事就當場做掉，做完算同一步。
func (m *Machine) Step() error {
	at, sp := m.S.IPC, m.S.SP
	_, err := m.S.Step()
	m.Steps++
	if err == nil {
		return nil
	}

	var ic *pmachine.IntrinsicCall
	if errors.As(err, &ic) {
		m.Traps[ic.Proc]++
		return m.intrinsic(ic.Proc, at, sp)
	}
	var nr *pmachine.NotResident
	if errors.As(err, &nr) {
		return m.segmentFault(nr.ERec)
	}
	return err
}

// Run 最多走 n 條，回報實際走了幾條與停下來的原因。
func (m *Machine) Run(n int) (int, error) {
	for i := 0; i < n; i++ {
		if err := m.Step(); err != nil {
			return i, err
		}
	}
	return n, nil
}

// segmentFault 是「要用的段還沒載入」（@0x143D → @0x0273）。
//
// 做三件事：把 fault 的資料填進 `ss:F8h`–`ss:FEh`、`SIGNAL` 那個號誌
// 叫醒作業系統的載入 task、然後回去重跑同一條指令——IPC 已經被
// `Step` 退回這一條的開頭了。
//
// 種類碼 `0x80` 是 segment fault，`0x81` 是堆疊爆掉（@0x02B1 那條路）。
func (m *Machine) segmentFault(erec uint16) error {
	m.setWord(faultTask, m.word(0x3c))
	m.setWord(faultERec, erec)
	m.setWord(faultERec+2, erec)
	m.setWord(faultKind, faultSegment)
	m.Faults++
	return m.S.Signal(faultSem)
}

// fault 的資料放在直譯器狀態區的這幾格（@0x0273 起）。
const (
	faultSem     = 0x00F4 // 叫醒載入 task 的號誌
	faultTask    = 0x00F8 // 是哪個 task 出的錯
	faultERec    = 0x00FA // 出錯的段
	faultKind    = 0x00FE
	faultSegment = 0x80
)
