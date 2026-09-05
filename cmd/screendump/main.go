//go:build oracle

// screendump 把原版開機到底之後的螢幕印出來。
//
// 那份螢幕是 DOS 端的驅動寫進顯示記憶體的，所以它是**終端機模擬的對照組**：
// 我們自己的畫面模型渲染出來的東西，應該與它逐格相同。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wicanr2/Parhelion-PME86/oracle"
)

func main() {
	com := flag.String("com", "/orig/PSYSTEM.COM", "PSYSTEM.COM")
	root := flag.String("root", "/orig", "磁碟映像目錄")
	steps := flag.Uint64("steps", 20_000_000, "跑幾條機器指令")
	flag.Parse()

	s, err := oracle.Boot(*com, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := s.Run(*steps); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	for i, line := range s.Screen() {
		fmt.Printf("%2d |%s|\n", i, strings.TrimRight(line, " "))
	}
}
