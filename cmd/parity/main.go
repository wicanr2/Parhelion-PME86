//go:build oracle

// parity 拿原版當對照，逐條比對 Go 版的 p-machine。
//
//	PARHELION_DOSGOLEM=../dosgolem-psys PARHELION_ORIG=<psys21> \
//	  tools/go.sh run -tags oracle ./cmd/parity -pme /src/workplace/SYSTEM.PME.86 -n 2000
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/oracle"
)

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	pme := flag.String("pme", "", "抽出來的 SYSTEM.PME.86")
	n := flag.Int("n", 1000, "要對拍幾條 p-code")
	wait := flag.Uint64("wait", 20_000_000, "等直譯器出現最多花幾條指令")
	budget := flag.Uint64("budget", 200_000, "原版每走一條 p-code 的機器指令預算")
	flag.Parse()
	if *pme == "" {
		flag.Usage()
		os.Exit(2)
	}

	s, err := oracle.Boot(*com, *root)
	if err != nil {
		die(err)
	}
	if _, err := s.WaitForPME(*pme, *wait, 0); err != nil {
		die(err)
	}
	// 先走一條，讓原版停在 dispatch 入口——Capture 只有在那一刻有意義。
	if _, err := s.Trace(1, *budget); err != nil {
		die(err)
	}

	res, err := s.Parity(*n, *budget)
	if err != nil {
		die(err)
	}

	fmt.Printf("兩邊一致地走了 %d 條 p-code\n", res.Steps)
	if res.Diverge != nil {
		fmt.Println("分歧：", res.Diverge)
	}
	if res.Err != nil {
		fmt.Println("停下來的原因：", res.Err)
	}
	if res.Diverge == nil && res.Err == nil {
		fmt.Println("走完指定的條數，沒有分歧。")
	}

	type kv struct {
		op uint8
		n  int
	}
	var ops []kv
	for op, c := range res.Ops {
		ops = append(ops, kv{op, c})
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].n > ops[j].n })
	fmt.Printf("\n用到 %d 種 opcode：\n", len(ops))
	for i, o := range ops {
		if i%6 == 0 {
			fmt.Print("  ")
		}
		fmt.Printf("%02X %-6s×%-5d", o.op, pcode.Mnemonic(o.op), o.n)
		if i%6 == 5 {
			fmt.Println()
		}
	}
	fmt.Println()
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "parity:", err)
	os.Exit(1)
}
