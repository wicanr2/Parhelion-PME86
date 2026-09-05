//go:build oracle

// segprobe 在原版執行跨段呼叫的那一刻把相關的狀態全部量出來。
//
// 目的是解開「SIB 怎麼算出程式碼段的段值」——靜態讀那段碼推不出來
// （PLAN.md 的開放項目 #2），但執行時的真實數字會直接把答案攤開。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/oracle"
)

func isCrossSegmentCall(op uint8) bool {
	return (op >= 0x70 && op <= 0x77) || (op >= 0x93 && op <= 0x95)
}

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	pme := flag.String("pme", "", "抽出來的 SYSTEM.PME.86")
	n := flag.Int("n", 3, "量幾次跨段呼叫")
	budget := flag.Uint64("budget", 2_000_000, "每走一條 p-code 的機器指令預算")
	flag.Parse()
	if *pme == "" {
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

	// 先把整張 E_Vec 攤開：每一段的 E_Rec、SIB、Seg_Base 兩個 word、段名，
	// 再回頭在記憶體裡找那個名字，看它實際載在哪個段。
	// Seg_Base 怎麼換算成段值，用真實數字比讀那段碼快。
	r0 := s.Regs()
	fmt.Printf("E_Vec 在 %04X，目前 E_Rec %04X\n", r0.EVec, r0.ERec)
	fmt.Println("段號  E_Rec  SIB   Seg_Base    Seg_Leng 段名      實際載在")
	for seg := uint16(0); seg < 24; seg++ {
		erec := s.DataWord(r0.EVec + 2*seg)
		if erec == 0 {
			continue
		}
		sib := s.DataWord(erec + 4)
		if sib == 0 {
			continue
		}
		name := ""
		for i := uint16(0); i < 8; i++ {
			b := byte(s.DataWord(sib+12+(i/2)*2) >> (8 * (i % 2)))
			if b >= 0x20 && b < 0x7f {
				name += string(b)
			}
		}
		lo, hi := s.DataWord(sib), s.DataWord(sib+2)
		where := "（找不到）"
		if hits := s.FindName(name); len(hits) > 0 {
			where = ""
			for _, h := range hits {
				where += fmt.Sprintf(" %04X", h)
			}
		}
		fmt.Printf("%4d  %04X   %04X  %04X %04X   %5d    %-9s%s\n",
			seg, erec, sib, lo, hi, s.DataWord(sib+20), name, where)
	}

	// Seg_Base 的第一個 word 是指標，指向一個兩個 word 的 20-bit 位址
	// （helper @0x1BEE：`{高 4 bit 的 word, 低 16 bit 的 word}`）。
	// 那個指標是相對哪一個段？直接把候選都印出來。
	fmt.Println()
	for _, cand := range []struct {
		name string
		seg  uint16
	}{{"ds（呼叫端的程式碼段）", r0.DS}, {"ss（直譯器資料段）", r0.SS}, {"cs（PME）", r0.CS}} {
		lo := s.M.Read16(uint32(cand.seg)*16 + 0x11AC)
		hi := s.M.Read16(uint32(cand.seg)*16 + 0x11AE)
		para := (hi >> 4) | (lo&0xF)<<12
		fmt.Printf("  %-24s %04X:11AC → {%04X, %04X}  →  paragraph %04X\n",
			cand.name, cand.seg, lo, hi, para)
	}

	seen := 0
	for seen < *n {
		rows, err := s.Trace(1, *budget)
		if err != nil || len(rows) == 0 {
			fmt.Println("追不下去了：", err)
			return
		}
		r := rows[0]
		if !isCrossSegmentCall(r.Op) {
			continue
		}
		seen++

		before := s.Regs()
		// 跨段呼叫的第一個運算元就是段號（SCXG 把段號編在 opcode 裡）。
		seg := uint16(0)
		if r.Op >= 0x70 && r.Op <= 0x77 {
			seg = uint16(r.Op) - 0x6f
		} else {
			seg = uint16(s.CodeSegment(r.Seg, int(r.IPC)+2)[r.IPC+1])
		}
		erec := s.DataWord(before.EVec + 2*seg)

		fmt.Printf("\n=== 第 %d 次：%04X:%04X %02X %s，段號 %d\n",
			seen, r.Seg, r.IPC, r.Op, pcode.Mnemonic(r.Op), seg)
		fmt.Printf("  執行前  ds=%04X  E_Rec=%04X  E_Vec=%04X  SIB=%04X  Env_Data=%04X  字典=%04X  常數池=%04X\n",
			before.DS, before.ERec, before.EVec, before.SIB, before.EnvData,
			before.ProcDict, before.ConstPool)
		fmt.Printf("  目標段的 E_Rec=%04X → Env_Data=%04X  E_Vec=%04X  SIB=%04X\n",
			erec, s.DataWord(erec), s.DataWord(erec+2), s.DataWord(erec+4))
		sib := s.DataWord(erec + 4)
		fmt.Print("  目標 SIB 前 12 個 word：")
		for i := uint16(0); i < 12; i++ {
			fmt.Printf(" %04X", s.DataWord(sib+2*i))
		}
		fmt.Println()

		// 讓原版真的走完這一條，看它切到哪裡。
		if _, err := s.Trace(1, *budget); err != nil {
			fmt.Println("  走不完：", err)
			return
		}
		after := s.Regs()
		fmt.Printf("  執行後  ds=%04X  E_Rec=%04X  Env_Data=%04X  字典=%04X  常數池=%04X  ss:2Ah=%04X\n",
			after.DS, after.ERec, after.EnvData, after.ProcDict, after.ConstPool, after.CodeSeg)
		fmt.Printf("  → 段值 %04X 對應線性 %05X；SIB[0]=%04X SIB[1]=%04X\n",
			after.DS, uint32(after.DS)*16, s.DataWord(sib), s.DataWord(sib+2))
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "segprobe:", err)
	os.Exit(1)
}
