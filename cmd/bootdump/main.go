//go:build oracle

// bootdump 把原版「準備好、要跑第一條 p-code」那一刻的記憶體版面量出來。
//
// 自舉要的就是這份版面：codepool 在哪、預先載了哪幾段、E_Vec／E_Rec／SIB
// 怎麼串、TIB 長什麼樣、第一個活動記錄怎麼擺。**照著建得出來，就不需要
// 原版了**——原版在這裡只是拿來確認實際行為。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/Parhelion-PME86/oracle"
)

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	pme := flag.String("pme", "", "抽出來的 SYSTEM.PME.86")
	at := flag.Int("at", 1, "走到第幾條 p-code 才量")
	chunk := flag.Uint64("chunk", 250_000, "找直譯器時每次跑幾條機器指令")
	flag.Parse()
	if *pme == "" {
		flag.Usage()
		os.Exit(2)
	}

	s, err := oracle.Boot(*com, *root)
	if err != nil {
		die(err)
	}
	if _, err := s.WaitForPME(*pme, 20_000_000, *chunk); err != nil {
		die(err)
	}
	rows, err := s.Trace(*at, 5_000_000)
	if err != nil || len(rows) == 0 {
		die(fmt.Errorf("走不到第 %d 條：%v", *at, err))
	}
	r := s.Regs()
	fmt.Printf("找到直譯器時機器走了 %d 條指令\n", s.M.Steps)
	fmt.Printf("停在第 %d 條 p-code：%v\n", *at, rows[len(rows)-1])
	fmt.Printf("段暫存器  cs=%04X ds=%04X ss=%04X\n", r.CS, r.DS, r.SS)

	fmt.Println("\n直譯器狀態（ss 相對）")
	for _, f := range []struct {
		off  uint16
		name string
	}{
		{0x24, "存起來的 IPC"}, {0x26, "區域基底 MP+8"}, {0x28, "全域基底"},
		{0x2a, "程式碼段值"}, {0x2e, "MP"}, {0x30, "BASE"}, {0x32, "MSPROC"},
		{0x34, "SIB"}, {0x36, "程序字典"}, {0x38, "?"}, {0x3a, "E_Vec"},
		{0x3c, "TIB"}, {0x3e, "E_Rec"}, {0x40, "舊 E_Rec"}, {0x42, "常數池"},
		{0x44, "byte sex"}, {0x46, "堆疊需求"}, {0x4e, "中斷向量表"},
		{0xe6, "MSPROC 高位"}, {0x110, "activity 計數"},
	} {
		fmt.Printf("  %04X %-16s %04X\n", f.off, f.name, s.DataWord(f.off))
	}

	tib := s.DataWord(0x3c)
	fmt.Printf("\nTIB @%04X\n", tib)
	for _, f := range []struct {
		off  uint16
		name string
	}{
		{0x00, "等待鏈"}, {0x02, "優先權/旗標"}, {0x04, "?"}, {0x06, "?"},
		{0x08, "SP"}, {0x0a, "MP"}, {0x0c, "?"}, {0x0e, "IPC"},
		{0x10, "E_Rec"}, {0x12, "MSPROC|高位"}, {0x14, "?"}, {0x16, "?"},
	} {
		fmt.Printf("  +%02X %-12s %04X\n", f.off, f.name, s.DataWord(tib+f.off))
	}

	evec := s.DataWord(0x3a)
	fmt.Printf("\nE_Vec @%04X\n", evec)
	fmt.Println("  段  E_Rec  Env_Data  E_Vec  SIB   Seg_Base      Ref  Act  長度  名字")
	for seg := uint16(0); seg < 32; seg++ {
		erec := s.DataWord(evec + 2*seg)
		if erec == 0 {
			continue
		}
		sib := s.DataWord(erec + 4)
		name := ""
		for i := uint16(0); i < 8; i++ {
			w := s.DataWord(sib + 12 + i&^1)
			b := byte(w)
			if i&1 == 1 {
				b = byte(w >> 8)
			}
			name += string(rune(b))
		}
		fmt.Printf("  %2d  %04X   %04X      %04X   %04X  %04X %04X     %3d  %3d  %5d  %q\n",
			seg, erec, s.DataWord(erec), s.DataWord(erec+2), sib,
			s.DataWord(sib), s.DataWord(sib+2),
			s.DataWord(sib+4), s.DataWord(sib+6), s.DataWord(sib+20), name)
	}

	fmt.Println("\n資料段幾塊區域的原始內容")
	for _, r := range []struct {
		from, to uint16
		what     string
	}{{0x0000, 0x0060, "直譯器狀態區"}, {0x00E0, 0x0140, "SYSCOM"}, {0x0140, 0x0260, "TIB 與全域起點"},
		{0xD7A0, 0xD810, "堆疊頂／E_Vec／E_Rec／SIB／程式碼起點"}, {0x3AE0, 0x3B60, "0x3AF2 那一塊"}, {0xF7E0, 0xF860, "SYSCOM+8 指到的地方"}} {
		fmt.Printf("  %s\n", r.what)
		for a := r.from; a < r.to; a += 16 {
			fmt.Printf("    %04X ", a)
			for i := uint16(0); i < 16; i += 2 {
				fmt.Printf("%04X ", s.DataWord(a+i))
			}
			fmt.Println()
		}
	}

	fmt.Printf("\n第一個活動記錄（MP=%04X）\n", s.DataWord(0x2e))
	mp := s.DataWord(0x2e)
	for _, f := range []struct {
		off  uint16
		name string
	}{{0, "MSSTAT"}, {2, "MSDYN"}, {4, "MSIPC"}, {6, "MSENV"}, {8, "MSPROC"}} {
		fmt.Printf("  +%d %-7s %04X\n", f.off, f.name, s.DataWord(mp+f.off))
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "bootdump:", err)
	os.Exit(1)
}
