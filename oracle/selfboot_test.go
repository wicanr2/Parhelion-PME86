//go:build oracle

package oracle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/internal/psystem"
)

// 自舉的驗收條件只有一個：**我們自己建出來的開機狀態，走出來的 p-code
// 要與原版逐條相同**。狀態擺錯一個 word，這裡就會在前幾條之內指出來。
//
// 原版在這裡只是拿來核對，不是跑起來的必要條件——`psystem` 那一側
// 不 import 任何 oracle 的東西。
const (
	// selfBootWant 是想走的條數；selfBootFloor 是「至少要走到這裡」。
	//
	// 下限釘住的是**開機狀態不能建錯**。走不完不是這個測試的失敗——
	// 那表示 bootstrap 還有沒重建出來的東西，而那是下一輪的工作。
	selfBootWant  = 2000
	selfBootFloor = 30
)

func TestSelfBootMatchesTheOriginal(t *testing.T) {
	img, err := os.ReadFile(filepath.Join("/orig", "PSYSTEM.VOL"))
	if err != nil {
		t.Skip("讀不到 PSYSTEM.VOL：", err)
	}
	m, err := psystem.Boot(img, "SYSTEM.PASCAL")
	if err != nil {
		t.Fatal(err)
	}

	s := bootToPME(t)
	rows, err := s.Trace(1, traceBudget)
	if err != nil || len(rows) == 0 {
		t.Fatalf("原版走不到第一條 p-code：%v", err)
	}
	if rows[0].IPC != m.S.IPC {
		t.Fatalf("第一條就不同：原版 %04X，我們 %04X", rows[0].IPC, m.S.IPC)
	}

	steps := 0
	for {
		op := uint8(0)
		if int(m.S.IPC) < len(m.S.Code) {
			op = m.S.Code[m.S.IPC]
		}
		at := m.S.IPC
		if err := m.Step(); err != nil {
			t.Logf("走了 %d 條之後停在 %04Xh：%v", steps, at, err)
			break
		}
		rows, terr := s.Trace(1, traceBudget)
		if terr != nil || len(rows) == 0 {
			t.Fatalf("原版在第 %d 條停了：%v", steps, terr)
		}
		r := rows[0]
		bad := ""
		for _, c := range []struct {
			what      string
			want, got uint16
		}{
			{"IPC", r.IPC, m.S.IPC},
			{"SP", r.SP, m.S.SP},
			{"TOS", r.TOS, m.S.TOS()},
			{"E_Rec", r.ERec, m.S.ERec},
		} {
			if c.want != c.got && bad == "" {
				bad = fmt.Sprintf("第 %d 條（%04Xh %02X %s）之後 %s 對不上：原版 %04X，我們 %04X",
					steps, at, op, pcode.Mnemonic(op), c.what, c.want, c.got)
			}
		}
		if bad != "" {
			t.Log(bad)
			break
		}
		steps++
		if steps >= selfBootWant {
			break
		}
	}
	if steps < selfBootFloor {
		t.Fatalf("只走了 %d 條就對不上，開機狀態沒建對（下限 %d）", steps, selfBootFloor)
	}
	t.Logf("自己開機走了 %d 條 p-code，與原版逐條相同", steps)
}
