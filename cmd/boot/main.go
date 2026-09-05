// boot 不靠原版、不靠 DOS，直接從 .VOL 映像把 p-System 跑起來。
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/internal/psystem"
)

func main() {
	volPath := flag.String("vol", "", ".VOL 磁碟映像")
	osFile := flag.String("os", "SYSTEM.PASCAL", "作業系統的 codefile")
	n := flag.Int("n", 1000, "最多走幾條 p-code")
	trace := flag.Int("trace", 0, "印出前幾條的軌跡")
	peek := flag.String("peek", "", "跑完印出這些資料段位址的 word，逗號分隔")
	keys := flag.String("keys", "", "開完機之後從鍵盤送進去的字")
	more := flag.Int("more", 200000, "送字之後再走幾條")
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
	if psystem.WaitingForInput(err) {
		fmt.Printf("走了 %d 條，停在等鍵盤\n", m.Steps)
		err = nil
	}
	if err == nil && *keys != "" {
		before := len(m.Console)
		m.Keys = append(m.Keys, []byte(*keys)...)
		steps, err = m.Run(*more)
		fmt.Printf("送進 %q 之後又走了 %d 條，主控台多了 %d 個位元組\n",
			*keys, steps, len(m.Console)-before)
		if psystem.WaitingForInput(err) {
			fmt.Println("又停在等鍵盤")
			err = nil
		}
	}
	fmt.Printf("走了 %d 條 p-code\n", m.Steps)
	if err != nil {
		fmt.Println("停：", err)
	} else {
		fmt.Printf("走完 %d 條，沒有停下來的理由\n", steps)
	}
	report(m)
	if *peek != "" {
		fmt.Println("\n看幾個位址：")
		for _, f := range strings.Split(*peek, ",") {
			off, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(f), "0x"), 16, 16)
			if err != nil {
				continue
			}
			fmt.Printf("  %04X = %04X\n", off, m.Word(uint16(off)))
		}
	}
}

func report(m *psystem.Machine) {
	if len(m.Console) > 0 {
		fmt.Printf("\n畫面（%d×%d）：\n%s\n", m.Screen.W, m.Screen.H,
			strings.Join(boxed(m.Screen.Lines()), "\n"))
		if len(m.Screen.Unknown) > 0 {
			fmt.Println("認不得的控制序列：", m.Screen.Unknown)
		}
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

// boxed 把畫面每一列框起來，好看清楚哪裡是空白。
func boxed(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fmt.Sprintf("%2d |%s|", i, l)
	}
	return out
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
