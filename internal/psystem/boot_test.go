package psystem

import (
	"os"
	"strings"
	"testing"
)

// 自舉的終點測試：一片 `.VOL` 進去，p-System 自己開到命令列。
//
// **這一份不需要原版直譯器，也不需要 DOS。** 逐條對拍是另一回事
// （在 `oracle/`），這裡問的是「它到底有沒有開起來」。
func TestBootReachesTheCommandLine(t *testing.T) {
	img, err := os.ReadFile("/orig/PSYSTEM.VOL")
	if err != nil {
		t.Skip("讀不到 /orig/PSYSTEM.VOL，跳過（跳過不等於通過）")
	}
	m, err := Boot(img, "SYSTEM.PASCAL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(400_000); err != nil {
		t.Fatalf("走了 %d 條之後停了：%v", m.Steps, err)
	}

	screen := string(m.Console)
	for _, want := range []string{
		"Copyright 1979 U.C. Regents",
		"Startup Utility",
		"SYSTEM.PASCAL is on RAMDISK",
		"Command: E(dit, R(un, F(ile",
		"[IV.2.1 R3.3]",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("主控台上沒有 %q", want)
		}
	}
	if t.Failed() {
		t.Logf("主控台收到 %d 個位元組：\n%s", len(m.Console), screen)
	}
}
