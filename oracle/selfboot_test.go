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
	selfBootWant  = 300_000
	selfBootFloor = 40_000
)

func TestSelfBootMatchesTheOriginal(t *testing.T) {
	img, err := os.ReadFile(filepath.Join("/orig", "PSYSTEM.VOL"))
	if err != nil {
		t.Skip("讀不到 PSYSTEM.VOL：", err)
	}
	// 開機日期要與 oracle 那台機器的時鐘一致，不然目錄裡那個 word 會不同——
	// 那不是實作的差異，是**兩台機器的今天不一樣**。
	m, err := psystem.BootWith(img, "SYSTEM.PASCAL", psystem.Options{Date: 0xBA11})
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
		at, tosIn, spIn := m.S.IPC, m.S.TOS(), m.S.SP
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
			t.Logf("  進這一條時 sp=%04X tos=%04X mp=%04X 區域基底=%04X", spIn, tosIn, m.S.Local-8, m.S.Local)
			for _, d := range s.DataDiff(m.S, 40) {
				t.Log("  資料段不同：", d)
			}
			pool := s.PoolBase()
			if d := s.MemDiff(m.Mem, pool, pool+0x20000, 10); len(d) > 0 {
				for _, l := range d {
					t.Log("  codepool 不同：", l)
				}
			} else {
				t.Log("  codepool 逐位元組相同")
			}
			if more, err := s.Trace(5, traceBudget); err == nil {
				for _, r := range more {
					t.Log("  原版接下來：", r)
				}
			}
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

// 開機那一刻的資料段，我們建出來的與 bootstrap 建出來的差在哪。
//
// **這是自舉的清單。** 差一格就是「bootstrap 做了一件我們還沒做的事」，
// 逐條走 p-code 只會在很後面才以別的樣子浮現。
func TestSelfBootInitialStateDiff(t *testing.T) {
	img, err := os.ReadFile(filepath.Join("/orig", "PSYSTEM.VOL"))
	if err != nil {
		t.Skip("讀不到 PSYSTEM.VOL：", err)
	}
	m, err := psystem.BootWith(img, "SYSTEM.PASCAL", psystem.Options{Date: 0xBA11})
	if err != nil {
		t.Fatal(err)
	}
	s := bootToPME(t)
	if _, err := s.Trace(1, traceBudget); err != nil {
		t.Fatal(err)
	}

	diff := s.DataDiff(m.S, 4000)
	for i, d := range diff {
		if i >= 30 {
			t.Logf("  …還有 %d 段", len(diff)-i)
			break
		}
		t.Log("  ", d)
	}
	// 分開數：`0x0192`–`0x06EF` 那一段是 bootstrap 從直譯器映像搬進來的
	// **8086 機器碼**（作業系統用 `NAT` 呼叫它）。我們的宿主是 Go，
	// 那一塊本來就不會有——**這個指標永遠歸不了零**，要分開看。
	native := 0
	for _, d := range diff {
		var lo uint32
		if _, err := fmt.Sscanf(d, "%05X", &lo); err == nil && lo >= 0x0192 && lo < 0x06F0 {
			native++
		}
	}
	t.Logf("開機那一刻資料段有 %d 段不同，其中 %d 段是原生驅動的機器碼",
		len(diff), native)
	if len(diff) > initialDiffCeiling {
		t.Fatalf("不同的段數 %d 超過上限 %d——bootstrap 又多做了什麼沒補上",
			len(diff), initialDiffCeiling)
	}
}

// initialDiffCeiling 是「還沒重建出來的 bootstrap 動作」還剩幾段。
// **只准往下**：補一項就把它調低，才看得出進度。
const initialDiffCeiling = 540
