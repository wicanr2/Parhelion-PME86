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
	// 停下來的理由必須是「在等鍵盤」——開機跑完就是停在那裡。
	if _, err := m.Run(400_000); !WaitingForInput(err) {
		t.Fatalf("走了 %d 條之後停在 %v，該是停在等鍵盤", m.Steps, err)
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

// 打字進去，看它有沒有反應。
//
// **開得起來與用得起來是兩回事。** 這一份走完整條路：開機 → 命令列 →
// `F` 進 Filer → `L` 列目錄 → 讀出作業系統自己在開機時複製過去的五個檔案。
// 過得了這一關，表示 p-machine、段載入、檔案系統、記憶體磁碟、磁碟寫入、
// 主控台與鍵盤全部串起來了。
func TestFilerListsTheRAMDisk(t *testing.T) {
	img, err := os.ReadFile("/orig/PSYSTEM.VOL")
	if err != nil {
		t.Skip("讀不到 /orig/PSYSTEM.VOL，跳過（跳過不等於通過）")
	}
	m, err := Boot(img, "SYSTEM.PASCAL")
	if err != nil {
		t.Fatal(err)
	}

	// 開機到命令列。停下來的理由必須是「在等鍵盤」——
	// 停在別的地方就是還有東西沒做完，不能當成功。
	if _, err := m.Run(400_000); !WaitingForInput(err) {
		t.Fatalf("走了 %d 條之後停在 %v，該是停在等鍵盤", m.Steps, err)
	}

	m.Keys = append(m.Keys, []byte("FLRAMDISK:\r")...)
	if _, err := m.Run(600_000); err != nil && !WaitingForInput(err) {
		t.Fatalf("打字之後停在 %v", err)
	}

	screen := string(m.Console)
	for _, want := range []string{
		"Filer: L(dir, R(em, C(hng",
		"Dir listing of what vol ? RAMDISK:",
		"SYSTEM.MISCINFO",
		"SYSTEM.PASCAL",
		"SYSTEM.FILER",
		"5/5 files<listed/in-dir>",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("主控台上沒有 %q", want)
		}
	}
	if t.Failed() {
		t.Logf("主控台收到 %d 個位元組：\n%s", len(m.Console), screen)
	}
}
