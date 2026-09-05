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

// Step 走一條 p-code；碰到宿主該做的事就當場做掉，做完算同一步。
func (m *Machine) Step() error {
	_, err := m.S.Step()
	m.Steps++
	if err == nil {
		return nil
	}

	var ic *pmachine.IntrinsicCall
	if errors.As(err, &ic) {
		m.Traps[ic.Proc]++
		return m.intrinsic(ic.Proc)
	}
	if errors.Is(err, pmachine.ErrNotResident) {
		return m.segmentFault()
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

// segmentFault 是「要用的段還沒載入」。原版在這裡把 fault 交給作業系統的
// 載入 task，那條路徑還沒做。
func (m *Machine) segmentFault() error {
	return fmt.Errorf("psystem: 段還沒載入，segment fault 的處理還沒做（IPC %04Xh）", m.S.IPC)
}
