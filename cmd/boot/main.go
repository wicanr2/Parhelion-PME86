// boot 不靠原版、不靠 DOS，直接從 .VOL 映像把 p-System 跑起來。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/internal/psystem"
)

func main() {
	volPath := flag.String("vol", "", ".VOL 磁碟映像")
	osFile := flag.String("os", "SYSTEM.PASCAL", "作業系統的 codefile")
	n := flag.Int("n", 1000, "最多走幾條 p-code")
	trace := flag.Int("trace", 0, "印出前幾條的軌跡")
	flag.Parse()
	if *volPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(*volPath)
	if err != nil {
		die(err)
	}
	m, err := psystem.Boot(data, *osFile)
	if err != nil {
		die(err)
	}
	fmt.Printf("磁碟 %q，起始段 %s（block %d、%d words、%d 支常式）\n",
		m.Vol.ID, m.Boot.Name, m.Boot.Block, m.Boot.Words, len(m.Boot.Routines))
	fmt.Printf("起點 IPC %04X  SP %04X  MP %04X  E_Rec %04X\n\n",
		m.S.IPC, m.S.SP, m.S.Local-8, m.S.ERec)

	for i := 0; i < *trace; i++ {
		op := uint8(0)
		if int(m.S.IPC) < len(m.S.Code) {
			op = m.S.Code[m.S.IPC]
		}
		fmt.Printf("%5d %04X %02X %-7s sp=%04X tos=%04X\n",
			i, m.S.IPC, op, pcode.Mnemonic(op), m.S.SP, m.S.TOS())
		if err := m.Step(); err != nil {
			fmt.Println("停：", err)
			report(m)
			return
		}
	}
	steps, err := m.Run(*n - *trace)
	fmt.Printf("走了 %d 條 p-code\n", m.Steps)
	if err != nil {
		fmt.Println("停：", err)
	} else {
		fmt.Printf("走完 %d 條，沒有停下來的理由\n", steps)
	}
	report(m)
}

func report(m *psystem.Machine) {
	if len(m.Console) > 0 {
		fmt.Printf("\n主控台收到 %d 個位元組：\n%s\n", len(m.Console), visible(m.Console))
	}
	for _, l := range m.IOLog {
		fmt.Println("  io:", l)
	}
	if len(m.Traps) == 0 {
		return
	}
	fmt.Println("\n用到的段 1 原生程序：")
	for proc, n := range m.Traps {
		fmt.Printf("  程序 %-3d ×%d\n", proc, n)
	}
}

// visible 把控制字元換成看得見的寫法，好知道螢幕上到底出現了什麼。
func visible(b []byte) string {
	var out []rune
	for _, c := range b {
		switch {
		case c == 0x1b:
			out = append(out, []rune("<ESC>")...)
		case c == '\r':
			out = append(out, '\n')
		case c < 0x20 || c > 0x7e:
			out = append(out, []rune(fmt.Sprintf("<%02X>", c))...)
		default:
			out = append(out, rune(c))
		}
	}
	return string(out)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "boot:", err)
	os.Exit(1)
}
