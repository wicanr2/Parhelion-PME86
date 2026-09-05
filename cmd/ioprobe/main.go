//go:build oracle

// ioprobe 記錄原版每一次裝置 I/O：段 1 的程序 18 與 19。
//
// 兩支的機器碼只差一個模式碼（1／2），參數塊在直譯器映像的 0x2AA0。
// 這裡不讀碼，直接量**實際發生的事**：進去前堆疊上那六個 word 是什麼、
// 出來後緩衝區有沒有被填、IORESULT 是多少。讀與寫因此分得出來。
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
	n := flag.Int("n", 300000, "最多走幾條 p-code")
	want := flag.Int("want", 20, "記幾次")
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

	seen := 0
	for i := 0; i < *n && seen < *want; i++ {
		rows, err := s.Trace(1, 400_000)
		if err != nil || len(rows) == 0 {
			fmt.Println("停：", err)
			break
		}
		r := rows[0]
		if r.Op != 0x70 {
			continue
		}
		code := s.CodeSegment(r.Seg, int(r.IPC)+2)
		proc := uint16(code[r.IPC+1])
		if proc == 34 || proc == 44 {
			unit := s.DataWord(r.SP)
			if next, err := s.Trace(1, 400_000); err != nil || len(next) == 0 {
				break
			}
			fmt.Printf("等 unit %-3d → IORESULT %d\n", unit, s.DataWord(0xe6))
			seen++
			continue
		}
		if proc != 18 && proc != 19 {
			continue
		}
		// 堆疊上由頂往下：mode、blocknum、length、緩衝區的偏移與基底、unit。
		w := func(k uint16) uint16 { return s.DataWord(r.SP + 2*k) }
		mode, blk, length := w(0), w(1), w(2)
		buf := w(3) + w(4)
		unit := w(5)
		before := snapshot(s, buf, length)

		next, err := s.Trace(1, 4_000_000)
		if err != nil || len(next) == 0 {
			fmt.Println("停在呼叫之後：", err)
			break
		}
		after := snapshot(s, buf, length)
		changed := 0
		for j := range before {
			if before[j] != after[j] {
				changed++
			}
		}
		fmt.Printf("程序 %d  unit %-3d block %-5d 長度 %-5d 緩衝 %04X  mode %d  "+
			"IORESULT %d  緩衝變了 %d／%d byte\n",
			proc, unit, blk, length, buf, mode, s.DataWord(0xe6), changed, len(before))
		if changed > 0 {
			fmt.Printf("    出來後前 16 byte：% X\n", after[:min(16, len(after))])
		} else if proc == 19 && length > 0 {
			fmt.Printf("    寫出去的：%q\n", string(before[:min(48, len(before))]))
		}
		seen++
	}
}

func snapshot(s *oracle.System, off, n uint16) []byte {
	if n > 4096 {
		n = 4096
	}
	out := make([]byte, n)
	for i := uint16(0); i < n; i++ {
		w := s.DataWord(off + i&^1)
		if i&1 == 0 {
			out[i] = byte(w)
		} else {
			out[i] = byte(w >> 8)
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "ioprobe:", err)
	os.Exit(1)
}
