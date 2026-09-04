// parhelion 是 Parhelion PME 的命令列入口。
//
// 目前只有 codefile 子命令：把一份 UCSD p-System codefile 的靜態結構印出來。
// 直譯器本身還沒開始寫。
package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/wicanr2/Parhelion-PME86/internal/codefile"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "parhelion:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("parhelion codefile", flag.ContinueOnError)
	verbose := fs.Bool("r", false, "連 routine dictionary 一起列出來")

	if len(args) == 0 || args[0] != "codefile" {
		fmt.Fprintln(os.Stderr, "用法：parhelion codefile [-r] <codefile>")
		return fmt.Errorf("要一個子命令：codefile")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("要一個 codefile 路徑")
	}

	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	cf, err := codefile.Parse(data)
	if err != nil {
		return err
	}
	report(os.Stdout, cf, len(data), *verbose)
	return nil
}

func report(out *os.File, cf *codefile.Codefile, size int, verbose bool) {
	fmt.Fprintf(out, "%d 位元組（%d blocks），%d 個 segment\n", size, size/codefile.BlockSize, len(cf.Segments))
	if cf.CopyNote != "" {
		fmt.Fprintf(out, "%s\n", cf.CopyNote)
	}
	fmt.Fprintln(out)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "段名\tblk\twords\t段號\t種類\t機器\t版本\tsex\t常數池\tR\t常式\t無碼\t外層")
	var routines, missing, flipped int
	for _, s := range cf.Segments {
		var absent int
		for _, r := range s.Routines {
			if !r.Present() {
				absent++
			}
		}
		routines += len(s.Routines)
		missing += absent
		sex := "同"
		if s.Flipped {
			sex = "反"
			flipped++
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%v\t%v\t%v\t%s\t%d\t%d\t%d\t%d\t%s\n",
			s.Name, s.Block, s.Words, s.Number, s.Kind, s.Machine, s.Version,
			sex, s.ConstPool, s.RealSize, len(s.Routines), absent, s.Family)
	}
	w.Flush()
	fmt.Fprintf(out, "\n合計 %d 支常式，其中 %d 支沒有碼；%d 個 segment 的 byte sex 與主機相反\n",
		routines, missing, flipped)

	if !verbose {
		return
	}
	for _, s := range cf.Segments {
		fmt.Fprintf(out, "\n== %s（%d 支常式）\n", s.Name, len(s.Routines))
		rw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(rw, "  常式\t碼(word)\tDATASIZE\tEXITIC\t")
		for _, r := range s.Routines {
			if !r.Present() {
				fmt.Fprintf(rw, "  %d\t—\t\t\t（EXTERNAL 或 FORWARD）\n", r.Number)
				continue
			}
			if r.Native {
				fmt.Fprintf(rw, "  %d\t%d\t%d\t—\t原生碼\n", r.Number, r.CodeWord, r.DataSize)
				continue
			}
			fmt.Fprintf(rw, "  %d\t%d\t%d\t%d\t\n", r.Number, r.CodeWord, r.DataSize, r.ExitIC)
		}
		rw.Flush()
	}
}
