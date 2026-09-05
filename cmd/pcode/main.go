//go:build oracle

// pcode 把原版直譯器實際執行的 p-code 印出來。
//
//	PARHELION_DOSGOLEM=../dosgolem-psys PARHELION_ORIG=<psys21> \
//	  tools/go.sh run -tags oracle ./cmd/pcode \
//	    -com /orig/PSYSTEM.COM -root /orig -pme /src/workplace/SYSTEM.PME.86 -n 40
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
	n := flag.Int("n", 32, "要記錄幾條 p-code")
	wait := flag.Uint64("wait", 20_000_000, "等直譯器出現最多花幾條指令")
	budget := flag.Uint64("budget", 5_000_000, "追蹤最多花幾條指令")
	flag.Parse()
	if *pme == "" {
		flag.Usage()
		os.Exit(2)
	}

	s, err := oracle.Boot(*com, *root)
	if err != nil {
		die(err)
	}
	base, err := s.WaitForPME(*pme, *wait, 0)
	if err != nil {
		die(err)
	}
	_, seg, _ := s.PME()
	fmt.Printf("直譯器在 %05Xh（段 %04Xh），開機跑了 %d 條指令\n", base, seg, s.M.Steps)

	same, total, err := s.DispatchMoved()
	if err != nil {
		die(err)
	}
	fmt.Printf("dispatch 表：%d／%d byte 與磁碟 %04Xh 起相同\n", same, total, oracle.DispatchOff)

	rows, err := s.Trace(*n, *budget)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pcode:", err)
	}
	fmt.Printf("\n軌跡 %d 條（追蹤花了到第 %d 條機器指令）：\n", len(rows), s.M.Steps)
	for i, r := range rows {
		fmt.Printf("  %3d  %v\n", i, r)
	}

	seen := map[uint16]bool{}
	fmt.Println("\n軌跡碰到的 code segment：")
	for _, r := range rows {
		if seen[r.Seg] {
			continue
		}
		seen[r.Seg] = true
		fmt.Printf("  %04Xh  表頭段名 %q\n", r.Seg, s.SegmentName(r.Seg))
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "pcode:", err)
	os.Exit(1)
}
