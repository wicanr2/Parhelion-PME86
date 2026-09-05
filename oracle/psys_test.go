//go:build oracle

package oracle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wicanr2/Parhelion-PME86/oracle"
)

// 原版素材由使用者自備，不在版控裡。缺檔就跳過——
// **不自製代用品**：安靜的替代品會讓「還沒驗」看起來像「驗過了」。
//
//	PARHELION_ORIG=<psys21 目錄> PARHELION_PME=workplace/SYSTEM.PME.86 \
//	  PARHELION_DOSGOLEM=../dosgolem-psys tools/go.sh test -tags oracle ./oracle/ -v
//
// PARHELION_ORIG 會被唯讀掛到容器裡的 /orig，所以測試看到的路徑是 /orig。
func materials(t *testing.T) (com, root, pme string) {
	t.Helper()
	root = os.Getenv("PARHELION_ORIG")
	pme = os.Getenv("PARHELION_PME")
	if root == "" || pme == "" {
		t.Skip("沒有設 PARHELION_ORIG／PARHELION_PME，跳過")
	}
	if _, err := os.Stat(pme); err != nil {
		t.Skipf("讀不到 %s，跳過", pme)
	}
	com = filepath.Join("/orig", "PSYSTEM.COM")
	if _, err := os.Stat(com); err != nil {
		t.Skipf("讀不到 %s，跳過", com)
	}
	return com, "/orig", pme
}

// bootSteps 是開機到命令列大約要跑的指令數。取得寬鬆一點，
// 反正提早到達不會有壞處。
const bootSteps = 20_000_000

// boot 跑到系統停在命令列為止。
func boot(t *testing.T) (*oracle.System, string) {
	t.Helper()
	s, pme := newSystem(t)
	if err := s.Run(bootSteps); err != nil {
		t.Fatal(err)
	}
	return s, pme
}

// bootToPME 只跑到直譯器出現為止。
//
// **不能先跑完再定位**：跑到底時系統已經在等鍵盤，開機期間執行的 p-code
// 全部錯過了，而那正是要拿來對拍的一段。
func bootToPME(t *testing.T) *oracle.System {
	t.Helper()
	s, pme := newSystem(t)
	if _, err := s.WaitForPME(pme, bootSteps, 0); err != nil {
		t.Fatal(err)
	}
	return s
}

func newSystem(t *testing.T) (*oracle.System, string) {
	t.Helper()
	com, root, pme := materials(t)
	s, err := oracle.Boot(com, root)
	if err != nil {
		t.Fatal(err)
	}
	return s, pme
}

// TestBootReachesTheCommandLine 是這條路的第一個門檻：
// 原版真的跑起來了，而且是跑到可以下指令的地方。
func TestBootReachesTheCommandLine(t *testing.T) {
	s, _ := boot(t)
	// 版本字串是系統自己印的。認它比認「畫面非空」硬得多。
	if !s.ScreenHas("[IV.2.1") {
		t.Errorf("畫面上沒有版本字串；目前畫面：\n%s", strings.Join(s.Screen(), "\n"))
	}
	if len(s.D.Opened) == 0 {
		t.Error("一個檔都沒開過——磁碟映像大概沒掛上")
	}
}

// TestLoaderMovesTheDispatchTableToOffsetZero 釘住整個知識庫最關鍵的一條。
//
// `jmp word ptr cs:[di]` 沒有位移，字面上表示表在 cs:0000；而表在檔案偏移 1D56h。
// 兩件事同時成立的唯一解釋是**載入器把表搬到映像最前面**。
// 這一條錯了，Parhelion 對「表項是什麼」的理解就整個垮掉，
// 而靜態看檔案完全看不出來。
func TestLoaderMovesTheDispatchTableToOffsetZero(t *testing.T) {
	s, pme := boot(t)
	base, err := s.LocatePME(pme)
	if err != nil {
		t.Fatal(err)
	}
	if base%16 != 0 {
		t.Fatalf("映像基底 %05Xh 不是段對齊", base)
	}
	same, total, err := s.DispatchMoved()
	if err != nil {
		t.Fatal(err)
	}
	if same != total {
		t.Fatalf("映像前 %d 個 byte 只有 %d 個與磁碟 %04Xh 起的 dispatch 表相同",
			total, same, oracle.DispatchOff)
	}
	t.Logf("映像基底 %05Xh，dispatch 表 %d／%d byte 相同", base, same, total)
}

// TestImageIsMostlyIntact 抓「指紋找到的其實是別的東西」這種錯。
// 差異應該只落在被表蓋掉的前 512 個 byte 與直譯器的工作區。
func TestImageIsMostlyIntact(t *testing.T) {
	s, pme := boot(t)
	if _, err := s.LocatePME(pme); err != nil {
		t.Fatal(err)
	}
	same, total := s.ImageMatches()
	if total == 0 {
		t.Fatal("沒有比對到任何東西")
	}
	if ratio := float64(same) / float64(total); ratio < 0.80 {
		t.Fatalf("只有 %d／%d（%.0f%%）與磁碟一致，指紋大概找錯地方",
			same, total, ratio*100)
	}
	t.Logf("映像 %d／%d byte 與磁碟一致", same, total)
}

// TestTraceNeedsPMELocated 釘住「沒定位就追蹤」要回錯誤，不是回空的軌跡。
// 回空的話呼叫端會以為「原版沒執行任何 p-code」。
func TestTraceNeedsPMELocated(t *testing.T) {
	com, root, _ := materials(t)
	s, err := oracle.Boot(com, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Trace(1, 1000); err == nil {
		t.Fatal("還沒定位 PME 卻讓 Trace 成功了")
	}
}

// TestCaptureMatchesTheInterpretersOwnState 釘住「抄狀態」這一步。
//
// Capture 從原版抄六個基底給 Go 版當起點。抄錯一個不會報錯——
// 對拍會在幾條之後開始對不上，而症狀看起來像某條指令實作錯了。
func TestCaptureMatchesTheInterpretersOwnState(t *testing.T) {
	s := bootToPME(t)
	if _, err := s.Trace(1, 200_000); err != nil {
		t.Fatal(err)
	}
	got, err := s.Capture()
	if err != nil {
		t.Fatal(err)
	}
	live := s.Regs()

	for _, c := range []struct {
		name      string
		want, got uint16
	}{
		{"IPC（回退到 opcode 本身）", live.SI - 1, got.IPC},
		{"SP", live.SP, got.SP},
		{"區域基底", live.BX, got.Local},
		{"全域基底", live.DX, got.Global},
		{"常數池", live.ConstPool, got.ConstPool},
		{"程序字典", live.ProcDict, got.ProcDict},
		{"MSPROC", live.Proc, got.Proc},
		{"E_Rec", live.ERec, got.ERec},
	} {
		if c.want != c.got {
			t.Errorf("%s：原版 %04X，抄成 %04X", c.name, c.want, c.got)
		}
	}
	// 程式碼要抄的是 ds 指的那一段，資料是 ss——兩個抄反了會安靜地跑很久。
	if want := s.CodeSegment(live.DS, 64); string(got.Code[:64]) != string(want) {
		t.Error("抄到的程式碼不是 ds 指的那一段")
	}
	if got.Env == nil {
		t.Error("沒有接上 Environment，跨段呼叫會做不了")
	}
}

// TestTraceIsDeterministic 釘住 oracle 的根本前提：同一份輸入永遠一樣。
//
// 不成立的話對拍會時好時壞，而看起來像被觀測的程式有問題。
func TestTraceIsDeterministic(t *testing.T) {
	const n = 300
	var runs [2][]string
	for i := range runs {
		s := bootToPME(t)
		rows, err := s.Trace(n, 5_000_000)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != n {
			t.Fatalf("第 %d 次只追到 %d 條", i+1, len(rows))
		}
		for _, r := range rows {
			runs[i] = append(runs[i], r.String())
		}
	}
	for i := range runs[0] {
		if runs[0][i] != runs[1][i] {
			t.Fatalf("第 %d 條不同：\n  %s\n  %s", i, runs[0][i], runs[1][i])
		}
	}
	t.Logf("兩次開機的前 %d 條 p-code 完全相同", n)
}
