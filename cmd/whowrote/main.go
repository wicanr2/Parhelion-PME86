//go:build oracle

// whowrote 回答「這個資料段位址是被哪一條 p-code 寫的」。
//
// 自舉與原版分歧時，症狀常常是「原版那裡有東西、我們那裡是零」。
// 差異的位置好找，**寫的人不好找**——中間可能隔了幾千條指令。
// 監看點加上「剛剛執行的那條 p-code 在哪」就直接把人指出來。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/Parhelion-PME86/oracle"
	"github.com/wicanr2/dosgolem"
)

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	pme := flag.String("pme", "", "抽出來的 SYSTEM.PME.86")
	at := flag.Uint("at", 0, "要監看的資料段位址")
	span := flag.Uint("span", 2, "監看幾個位元組")
	n := flag.Int("n", 60000, "最多走幾條 p-code")
	want := flag.Int("want", 12, "記幾次寫入")
	flag.Parse()
	if *pme == "" || *at == 0 {
		flag.Usage()
		os.Exit(2)
	}

	s, err := oracle.Boot(*com, *root)
	if err != nil {
		die(err)
	}
	if _, err := s.WaitForPME(*pme, 20_000_000, 0); err != nil {
		die(err)
	}

	base := uint32(s.M.CPU.Seg[dosgolem.SS]) * 16
	hits := 0
	s.M.WatchWrite(base+uint32(*at), base+uint32(*at)+uint32(*span)-1,
		func(m *dosgolem.Machine, addr uint32, old, now uint8) {
			if hits >= *want {
				return
			}
			hits++
			c := m.CPU
			fmt.Printf("%04X ← %02X（原本 %02X）：p-code 在 %04X:%04X，8086 在 %04X:%04X\n",
				addr-base, now, old, c.Seg[dosgolem.DS], c.R[dosgolem.SI]-1,
				func() uint16 { cs, _ := m.Insn(); return cs }(),
				func() uint16 { _, ip := m.Insn(); return ip }())
		})

	for i := 0; i < *n && hits < *want; i++ {
		rows, err := s.Trace(1, 400_000)
		if err != nil || len(rows) == 0 {
			fmt.Println("停：", err)
			break
		}
	}
	if hits == 0 {
		fmt.Printf("走完了都沒有人寫 %04X\n", *at)
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "whowrote:", err)
	os.Exit(1)
}
