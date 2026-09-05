//go:build oracle

package oracle_test

import (
	"os"
	"testing"

	"github.com/wicanr2/Parhelion-PME86/internal/codefile"
)

// 這一份是 spec 01 的同狀態驗證：**我們解出來的結構，與原版實際在跑的結構，
// 是不是同一件事。**
//
// 靜態自洽（M0 那一輪做的）只證明「這樣讀不會自相矛盾」。
// 要證明讀對了，得看原版把 IPC 指到哪、把段名放在哪。

// traceCodefile 需要第三份素材：那份被執行的 codefile 本身。
func traceCodefile(t *testing.T) string {
	t.Helper()
	p := os.Getenv("PARHELION_CODEFILE")
	if p == "" {
		t.Skip("沒有設 PARHELION_CODEFILE，跳過")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("讀不到 %s，跳過", p)
	}
	return p
}

const (
	// traceBudget 是原版每走一條 p-code 給的機器指令預算。
	traceBudget = 200_000

	// parityWant 是想走的條數；parityFloor 是「至少要走到這裡」。
	//
	// 下限釘住的是**進度不能倒退**。上限走不完不是失敗——那表示碰到還沒
	// 實作的指令，而那是下一輪的工作，不是這一輪的錯。
	parityWant  = 50_000
	parityFloor = 300
)

func TestExecutedCodeMatchesWhatTheReaderParses(t *testing.T) {
	cfPath := traceCodefile(t)
	s := bootToPME(t)
	rows, err := s.Trace(400, 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("一條 p-code 都沒追到")
	}

	data, err := os.ReadFile(cfPath)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := codefile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*codefile.Segment{}
	for _, seg := range cf.Segments {
		byName[seg.Name] = seg
	}

	// 軌跡碰到的每一個 code segment，都要在我們解出來的清單裡。
	touched := map[uint16]bool{}
	for _, r := range rows {
		if touched[r.Seg] {
			continue
		}
		touched[r.Seg] = true

		name := s.SegmentName(r.Seg)
		seg, ok := byName[name]
		if !ok {
			t.Fatalf("原版在跑段 %q（%04Xh），但讀取器在 %s 裡找不到這個名字",
				name, r.Seg, cfPath)
		}
		t.Logf("段 %q：記憶體 %04Xh，檔案 block %d、%d words、%d 支常式",
			name, r.Seg, seg.Block, seg.Words, len(seg.Routines))

		// 段的內容要與檔案一致。作業系統只會翻轉 routine dictionary，
		// 而 byte sex 相同的段連那個都不動。
		live := s.CodeSegment(r.Seg, seg.Words*2)
		same := 0
		for i, b := range seg.Raw() {
			if live[i] == b {
				same++
			}
		}
		if ratio := float64(same) / float64(seg.Words*2); ratio < 0.99 {
			t.Errorf("段 %q 在記憶體裡只有 %d／%d byte 與檔案相同（%.1f%%）",
				name, same, seg.Words*2, ratio*100)
		}
	}

	// 每一個 IPC 都要落在「表頭之後、routine dictionary 之前」，
	// 而且記憶體裡的那個 byte 要與檔案裡同一個位移的 byte 相同。
	//
	// 後半條是真正的判準：它把「讀取器算出來的段內位移」與
	// 「原版取指令的位址」綁在一起。差一個 word 就會整片對不上。
	const headerBytes = 22 // 11 個 word
	for _, r := range rows {
		seg := byName[s.SegmentName(r.Seg)]
		raw := seg.Raw()
		if int(r.IPC) < headerBytes || int(r.IPC) >= len(raw) {
			t.Fatalf("IPC %04Xh 落在段 %q（%d bytes）的可執行範圍外", r.IPC, seg.Name, len(raw))
		}
		if got, want := raw[r.IPC], r.Op; got != want {
			t.Fatalf("段 %q 位移 %04Xh：原版取到 %02X，檔案裡是 %02X",
				seg.Name, r.IPC, want, got)
		}
	}
	t.Logf("%d 條 p-code 的 opcode 與 codefile 逐位元組相同", len(rows))
}

// TestTraceLooksLikePCode 抓「判準抓錯東西」這種錯：
// 追出來的若不是真的取指令，助記符會有一堆空的，IPC 也不會前進。
func TestTraceLooksLikePCode(t *testing.T) {
	s := bootToPME(t)
	rows, err := s.Trace(200, 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 200 {
		t.Fatalf("只追到 %d 條", len(rows))
	}

	unnamed := 0
	for _, r := range rows {
		if r.Mnemonic() == "" {
			unnamed++
		}
	}
	// 那 44 格在這份直譯器裡指向錯誤 11。真的執行到，系統早就停了。
	if unnamed > 0 {
		t.Errorf("%d 條的 opcode 在 IV.0 表裡沒有指令", unnamed)
	}

	// 連續兩條同一個 IPC 表示判準把「跳進另一支常式的入口」也算成取指令。
	for i := 1; i < len(rows); i++ {
		if rows[i].Seg == rows[i-1].Seg && rows[i].IPC == rows[i-1].IPC {
			t.Fatalf("第 %d、%d 條是同一個 IPC %04Xh（%s）——判準多算了",
				i-1, i, rows[i].IPC, rows[i].Mnemonic())
		}
	}
}

// TestParityAgainstTheOriginal 是 M1 的驗收：Go 版的 p-machine 從原版此刻的
// 狀態展開，逐條走，每一條之後 IPC、SP、TOS 三項都要與原版相同。
//
// **停下來的三種情形要分得出來**：走完、有指令還沒實作、真的對不上。
// 只有第三種是失敗——第二種是進度，會告訴我們下一個要做的是哪一支。
func TestParityAgainstTheOriginal(t *testing.T) {
	s := bootToPME(t)
	if _, err := s.Trace(1, traceBudget); err != nil {
		t.Fatal(err)
	}
	res, err := s.Parity(parityWant, traceBudget)
	if err != nil {
		t.Fatal(err)
	}
	if res.Diverge != nil {
		t.Fatalf("走了 %d 條之後對不上：%v", res.Steps, res.Diverge)
	}
	if res.Steps < parityFloor {
		t.Fatalf("只走了 %d 條就停下來（%v）——比之前少，是不是退步了？", res.Steps, res.Err)
	}
	t.Logf("%d 條 p-code 逐條一致，用到 %d 種 opcode", res.Steps, len(res.Ops))
	if res.Err != nil {
		t.Logf("停下來的原因：%v", res.Err)
	}
}

// TestSegmentResolutionMatchesTheOriginal 驗跨段呼叫那一層。
//
// 判準不是「我們算得出一個數字」，是**算出來的與原版切過去之後的一模一樣**：
// 程式碼段的內容、全域資料基底、程序字典、常數池四項。
//
// 這一條特別重要，因為 Seg_Base 怎麼換算成段值是量出來的，不是手冊寫的
// （PLAN.md 開放項目 #2）。量錯的症狀是「跳進另一段的中間」，不會報錯。
//
// 換段用 dosgolem 的 WatchWord 監看直譯器的 E_Rec 抓，**不輪詢**——
// 兩條 p-code 之間可以換過去又換回來，輪詢會漏掉，而漏掉不會報錯。
func TestSegmentResolutionMatchesTheOriginal(t *testing.T) {
	s := bootToPME(t)
	log, id := s.WatchSegmentSwitches()
	defer s.M.Unwatch(id)

	seen, checked := 0, 0
	for i := 0; i < 20000; i++ {
		if _, err := s.Trace(1, traceBudget); err != nil {
			break
		}
		for seen < len(*log) {
			sw := (*log)[seen]
			seen++
			// 停在 dispatch 邊界，所以直譯器的狀態變數這時是一致的。
			live := s.Regs()
			if live.ERec != sw.To {
				continue // 這一次之後又換過，等下一輪比
			}
			checked++

			got, err := s.ResolveSegment2(sw.To)
			if err != nil {
				t.Fatalf("E_Rec %04X 解不開：%v", sw.To, err)
			}
			for _, c := range []struct {
				name      string
				want, got uint16
			}{
				{"全域基底", live.EnvData + 8, got.Global},
				{"程序字典", live.ProcDict, got.ProcDict},
				{"常數池", live.ConstPool, got.ConstPool},
			} {
				if c.want != c.got {
					t.Errorf("E_Rec %04X 的%s：原版 %04X，我們算出 %04X",
						sw.To, c.name, c.want, c.got)
				}
			}
			if want := s.CodeSegment(live.DS, 64); string(got.Code[:64]) != string(want) {
				t.Errorf("E_Rec %04X：算出來的程式碼與原版切到的那一段不同", sw.To)
			}
		}
	}
	if checked == 0 {
		t.Skip("這段軌跡裡一次換段都沒有")
	}
	t.Logf("監看到 %d 次換段，逐一驗過 %d 次，四項全同", len(*log), checked)
}

// TestParityAcrossASegmentSwitch 讓對拍實際走過一次真的換段。
//
// `TestParityAgainstTheOriginal` 從開機那一刻展開，第一個碰到的跨段呼叫是
// 段 1 的內嵌原生碼，所以走不到換段那條路。這裡改成**先把原版推到
// 下一條就是真換段的地方**再展開——同樣是逐條對拍，只是起點挑過。
func TestParityAcrossASegmentSwitch(t *testing.T) {
	s := bootToPME(t)
	found := false
	for i := 0; i < 20000 && !found; i++ {
		rows, err := s.Trace(1, traceBudget)
		if err != nil || len(rows) == 0 {
			break
		}
		r := rows[0]
		op := r.Op
		if !((op >= 0x70 && op <= 0x77) || (op >= 0x93 && op <= 0x95)) {
			continue
		}
		// 運算元：SCXG 的段號編在 opcode 裡，其餘的段號在第一個位元組。
		code := s.CodeSegment(r.Seg, int(r.IPC)+3)
		seg, proc := uint16(op)-0x6f, uint16(code[r.IPC+1])
		if op >= 0x93 {
			seg, proc = uint16(code[r.IPC+1]), uint16(code[r.IPC+2])
		}
		if seg == 1 && s.IsIntrinsic(proc) {
			continue // 內嵌原生碼，不換段
		}
		if _, err := s.ResolveSegment(seg); err != nil {
			continue // 段不在記憶體，原版會先去載入
		}
		found = true

		before := s.Regs().ERec
		res, err := s.Parity(200, traceBudget)
		if err != nil {
			t.Fatal(err)
		}
		if res.Diverge != nil {
			t.Fatalf("跨段呼叫走了 %d 條之後對不上：%v", res.Steps, res.Diverge)
		}
		if res.Steps == 0 {
			t.Fatalf("一條都沒走成：%v", res.Err)
		}
		after := s.Regs().ERec
		if after == before {
			t.Fatalf("走了 %d 條，E_Rec 還是 %04X——沒有真的換段", res.Steps, before)
		}
		t.Logf("從 %02X（段 %d、程序 %d）展開：%d 條逐條一致，E_Rec %04X → %04X",
			op, seg, proc, res.Steps, before, after)
		if res.Err != nil {
			t.Logf("停下來的原因：%v", res.Err)
		}
	}
	if !found {
		t.Skip("這段軌跡裡沒有真換段又不是內嵌原生碼的呼叫")
	}
}
