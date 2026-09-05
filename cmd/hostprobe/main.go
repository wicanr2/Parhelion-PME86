//go:build oracle

// hostprobe 記錄開機途中每一次「交給宿主」的呼叫：段 1 的內嵌原生程序與 CSP。
//
// 對每一次記下程序號、進去前的堆疊、出來後的堆疊。目的是把那 26 支原生程序的
// 介面（吃幾個參數、回不回值）從實際行為量出來，作為自舉時重做它們的依據。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/oracle"
)

type call struct {
	op       uint8
	proc     uint16
	before   []uint16
	after    []uint16
	spBefore uint16
	spAfter  uint16
	bytes    []byte
}

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	pme := flag.String("pme", "", "抽出來的 SYSTEM.PME.86")
	n := flag.Int("n", 100000, "最多走幾條 p-code")
	flag.Parse()

	s, err := oracle.Boot(*com, *root)
	must(err)
	_, err = s.WaitForPME(*pme, 20_000_000, 200_000)
	must(err)

	var calls []call
	total := 0
	peek := func(sp uint16, n int) []uint16 {
		out := make([]uint16, n)
		for i := range out {
			out[i] = s.DataWord(sp + uint16(2*i))
		}
		return out
	}

	for i := 0; i < *n; i++ {
		rows, err := s.Trace(1, 400_000)
		if err != nil || len(rows) == 0 {
			fmt.Println("停：", err)
			break
		}
		r := rows[0]
		// 這一條是不是要交給宿主？SCXG1（段 1 的跨段呼叫）與 CSP。
		total++
		if r.Op != 0x70 && r.Op != 0xac {
			continue
		}
		code := s.CodeSegment(r.Seg, int(r.IPC)+2)
		proc := uint16(code[r.IPC+1])
		c := call{op: r.Op, proc: proc, spBefore: r.SP, before: peek(r.SP, 6),
			bytes: append([]byte(nil), code[r.IPC:]...)}
		next, err := s.Trace(1, 400_000)
		if err != nil || len(next) == 0 {
			fmt.Println("停在呼叫之後：", err)
			break
		}
		c.spAfter = next[0].SP
		c.after = peek(next[0].SP, 6)
		calls = append(calls, c)
	}

	// 依 (op, proc) 聚合
	type key struct {
		op   uint8
		proc uint16
	}
	agg := map[key][]call{}
	for _, c := range calls {
		agg[key{c.op, c.proc}] = append(agg[key{c.op, c.proc}], c)
	}
	keys := make([]key, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].op != keys[j].op {
			return keys[i].op < keys[j].op
		}
		return keys[i].proc < keys[j].proc
	})

	fmt.Printf("\n走了 %d 條 p-code；交給宿主 %d 次，%d 種\n\n", total, len(calls), len(keys))
	for _, k := range keys {
		cs := agg[k]
		c := cs[0]
		delta := int(c.spAfter) - int(c.spBefore)
		fmt.Printf("%-6s proc %-3d  ×%-4d  sp %+d word  bytes %02X\n",
			pcode.Mnemonic(k.op), k.proc, len(cs), delta/2, c.bytes)
		for j, c := range cs {
			if j >= 3 {
				break
			}
			fmt.Printf("      進 %04X: %v\n      出 %04X: %v\n",
				c.spBefore, c.before, c.spAfter, c.after)
		}
	}
	_ = filepath.Join
	_ = os.Stdout
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
