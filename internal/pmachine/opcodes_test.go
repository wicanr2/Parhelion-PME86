package pmachine

import (
	"errors"
	"testing"
)

// 逐條釘 opcode 的語意。每一個案例的名字帶著它在 SYSTEM.PME.86 裡的檔案偏移，
// 對不上的時候可以直接回去對碼。
//
// 真正的判準是 oracle 那邊與原版的逐條對拍；這裡釘的是
// **對拍走不到、或走到了也看不出來**的情形：邊界值、負數、除以零、
// 以及「兩種寫法在多數輸入下結果相同」的那些。

// exec 擺好堆疊、跑 n 條、回堆疊上剩下的東西（由 TOS 往下數）。
func exec(t *testing.T, code []byte, push []uint16, want int, setup func(*State)) []uint16 {
	t.Helper()
	s := newState(code...)
	if setup != nil {
		setup(s)
	}
	for _, v := range push {
		s.push(v)
	}
	if _, err := s.Step(); err != nil {
		t.Fatalf("%v", err)
	}
	out := make([]uint16, want)
	for i := range out {
		out[i] = s.pop()
	}
	return out
}

func TestArithmeticTakesTOSAsTheRightOperand(t *testing.T) {
	// 原版一律 `pop ax; pop bp; op bp, ax`。加法與位元運算顛倒過來看不出差別，
	// 減法與除法會整個錯——所以每一個都要測不對稱的輸入。
	for _, tt := range []struct {
		name string
		op   byte
		a, b uint16 // a 是左、b 是右（右邊是 TOS）
		want uint16
	}{
		{"ADI @0x06ee", 0xa2, 7, 3, 10},
		{"SBI @0x06fc", 0xa3, 7, 3, 4},
		{"SBI 借位", 0xa3, 3, 7, 0xFFFC},
		{"MPI @0x066c", 0x8c, 7, 3, 21},
		{"MPI 負數", 0x8c, 0xFFFF, 3, 0xFFFD},
		{"MPI 只留低 16 位", 0x8c, 0x1000, 0x0100, 0x0000},
		{"DVI @0x067f", 0x8d, 7, 3, 2},
		{"DVI 往零截斷", 0x8d, 0xFFF9, 3, 0xFFFE}, // −7 ÷ 3 ＝ −2，不是 −3
		{"LOR @0x06d2", 0xa0, 0xF0F0, 0x0F0F, 0xFFFF},
		{"LAND @0x06e0", 0xa1, 0xF0F0, 0xFF00, 0xF000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := exec(t, []byte{tt.op}, []uint16{tt.a, tt.b}, 1, nil)[0]; got != tt.want {
				t.Errorf("%04X op %04X = %04X，該是 %04X", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMODIFollowsPascalNotC(t *testing.T) {
	// @0x0bbc：餘數為負時加上除數的絕對值。C 的 % 不會做這件事，
	// 而兩者在「被除數為正」時結果相同——所以只測正數會看不出差別。
	for _, tt := range []struct {
		a, b, want int16
	}{
		{7, 3, 1},
		{-7, 3, 2},
		{7, -3, 1},
		{-7, -3, 2},
	} {
		got := exec(t, []byte{0x8f}, []uint16{uint16(tt.a), uint16(tt.b)}, 1, nil)[0]
		if int16(got) != tt.want {
			t.Errorf("%d MOD %d = %d，該是 %d", tt.a, tt.b, int16(got), tt.want)
		}
	}
}

func TestUnaryOperations(t *testing.T) {
	for _, tt := range []struct {
		name string
		op   byte
		in   uint16
		want uint16
	}{
		{"NGI @0x08af", 0xe1, 5, 0xFFFB},
		{"ABI @0x089a 正數不動", 0xe0, 5, 5},
		{"ABI 負數取反", 0xe0, 0xFFFB, 5},
		{"DECI @0x0918", 0xee, 5, 4},
		{"DECI 借位", 0xee, 0, 0xFFFF},
		// LNOT 動整個 word，BNOT 只留最低位——差別只有在值不是 0/1 時看得出來。
		{"LNOT @0x08ca", 0xe5, 0x00FF, 0xFF00},
		{"BNOT @0x06c2 真變假", 0x9f, 1, 0},
		{"BNOT 假變真", 0x9f, 0, 1},
		{"BNOT 只看最低位", 0x9f, 2, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := exec(t, []byte{tt.op}, []uint16{tt.in}, 1, nil)[0]; got != tt.want {
				t.Errorf("%04X → %04X，該是 %04X", tt.in, got, tt.want)
			}
		})
	}
}

func TestSignedAndUnsignedComparisonsDiffer(t *testing.T) {
	// 帶號與無號比較在「有一邊的最高位是 1」時結果相反。
	// 只用小的正數測，兩組會全部通過而且看不出接錯了。
	const neg, pos = 0xFFFF, 0x0001 // 帶號是 −1 < 1，無號是 65535 > 1
	for _, tt := range []struct {
		name string
		op   byte
		want uint16
	}{
		{"LEQI 帶號 @0x07be", 0xb2, 1},
		{"GEQI 帶號 @0x07d0", 0xb3, 0},
		{"LEUSW 無號 @0x07e2", 0xb4, 0},
		{"GEUSW 無號 @0x07f4", 0xb5, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := exec(t, []byte{tt.op}, []uint16{neg, pos}, 1, nil)[0]; got != tt.want {
				t.Errorf("%04X vs %04X = %d，該是 %d", neg, pos, got, tt.want)
			}
		})
	}
	// EQUI／NEQI 不分帶號，兩邊都要對。
	if got := exec(t, []byte{0xb0}, []uint16{neg, neg}, 1, nil)[0]; got != 1 {
		t.Error("EQUI 相等卻推 0")
	}
	if got := exec(t, []byte{0xb1}, []uint16{neg, neg}, 1, nil)[0]; got != 0 {
		t.Error("NEQI 相等卻推 1")
	}
}

func TestIXAScalesByElementSize(t *testing.T) {
	// @0x0864：索引為 1 時原版跳過乘法。兩條路徑結果必須相同——
	// 跳過那條走錯只有在「元素不只一個 word」時才看得出來。
	for _, tt := range []struct {
		name      string
		words     byte // 每個元素幾個 word
		base, idx uint16
		want      uint16
	}{
		{"索引 1 走捷徑", 3, 0x100, 1, 0x100 + 6},
		{"索引 2 走乘法", 3, 0x100, 2, 0x100 + 12},
		{"索引 0", 3, 0x100, 0, 0x100},
		{"單 word 元素", 1, 0x100, 5, 0x100 + 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := exec(t, []byte{0xd7, tt.words}, []uint16{tt.base, tt.idx}, 1, nil)[0]
			if got != tt.want {
				t.Errorf("得到 %04X，該是 %04X", got, tt.want)
			}
		})
	}
}

func TestStaticChainWalksTheRightNumberOfLevels(t *testing.T) {
	// 助手 @0x093a：從 MP（＝Local−8）開始，走 DB 次 MSSTAT，再加 2n+8。
	// 走錯層數不會報錯，只會讀到另一層的變數——而那看起來像合法的值。
	s := newState()
	const mp0, mp1, mp2 = 0x0200, 0x0300, 0x0400
	s.Local = mp0 + 8
	s.Store(mp0, mp1) // 外一層
	s.Store(mp1, mp2) // 外兩層
	s.Store(mp0+8+2, 0x1111)
	s.Store(mp1+8+2, 0x2222)
	s.Store(mp2+8+2, 0x3333)

	for _, tt := range []struct {
		db   uint16
		want uint16
	}{{0, 0x1111}, {1, 0x2222}, {2, 0x3333}} {
		// intermediate 會吃掉一個 big 運算元，每次都要重擺。
		s.Code, s.IPC = []byte{0x01}, 0
		if got := s.Load(s.intermediate(tt.db)); got != tt.want {
			t.Errorf("走 %d 層讀到 %04X，該是 %04X", tt.db, got, tt.want)
		}
	}
}

func TestIntermediateInstructionsShareTheHelper(t *testing.T) {
	// LOD／STR／LDA 都是 `DB, B` 兩個運算元；SLOD1／SLOD2 的層數固定，
	// 少一個運算元——讀多或讀少一個位元組會讓後面整串錯位。
	base := func(s *State) {
		s.Local = 0x0208
		s.Store(0x0200, 0x0300) // 外一層的 MP
		s.Store(0x0300, 0x0400) // 外兩層
		s.Store(0x0308+2, 0xAAAA)
		s.Store(0x0408+2, 0xBBBB)
	}
	if got := exec(t, []byte{0x89, 0x01, 0x01}, nil, 1, base)[0]; got != 0xAAAA { // LOD 1,1
		t.Errorf("LOD 讀到 %04X", got)
	}
	if got := exec(t, []byte{0xad, 0x01}, nil, 1, base)[0]; got != 0xAAAA { // SLOD1 1
		t.Errorf("SLOD1 讀到 %04X", got)
	}
	if got := exec(t, []byte{0xae, 0x01}, nil, 1, base)[0]; got != 0xBBBB { // SLOD2 1
		t.Errorf("SLOD2 讀到 %04X", got)
	}
	if got := exec(t, []byte{0x88, 0x01, 0x01}, nil, 1, base)[0]; got != 0x0308+2 { // LDA
		t.Errorf("LDA 推了 %04X", got)
	}
	// LSL 推的是那一層的 MP，不是變數位址。
	if got := exec(t, []byte{0x99, 0x02}, nil, 1, base)[0]; got != 0x0400 {
		t.Errorf("LSL 2 推了 %04X，該是 0400", got)
	}
}

func TestLDMLeavesTheLowestAddressOnTop(t *testing.T) {
	// @0x09b8 從高位址往低位址推，所以 [addr] 最後才進去，留在 TOS。
	// 方向顛倒的話多字資料會整組反過來，而每一個 word 都還是合法的值。
	s := newState(0xd0, 0x03) // LDM 3
	s.Store(0x800, 0x1111)
	s.Store(0x802, 0x2222)
	s.Store(0x804, 0x3333)
	s.push(0x800)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	for i, want := range []uint16{0x1111, 0x2222, 0x3333} {
		if got := s.pop(); got != want {
			t.Fatalf("第 %d 個是 %04X，該是 %04X", i, got, want)
		}
	}
}

func TestByteIndexingIsInBytesNotWords(t *testing.T) {
	// LDB @0x0751 是 `mov al,[bp+di]`——索引是位元組。
	// 當成 word 索引的話奇數位置永遠讀不到。
	s := newState(0xa7) // LDB
	s.Data[0x805] = 0x5A
	s.push(0x800)
	s.push(5)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.pop(); got != 0x5A {
		t.Errorf("LDB 讀到 %04X", got)
	}
	// STB @0x0822 只寫一個位元組，不能動到鄰居。
	s = newState(0xc8)
	s.Store(0x800, 0xFFFF)
	s.push(0x800)
	s.push(1)
	s.push(0x11)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(0x800); got != 0x11FF {
		t.Errorf("STB 之後那個 word 是 %04X，該是 11FF", got)
	}
}

func TestReturnPopsTheParameters(t *testing.T) {
	// RPU 的 B 是**位元組數**（big × 2）。少削一個 word，呼叫端後面每一條
	// 的堆疊都會差兩個位元組，而症狀出現在很後面。
	s := newState(0x90, 0x01) // CPL 1
	s.Code = append(s.Code, make([]byte, 0x100)...)
	s.ProcDict = 0x40
	putWord(s.Code, 0x40-2, 0x10) // 程序 1 → word 0x10
	putWord(s.Code, 0x20, 0)      // DATASIZE = 0
	s.Code[0x22] = 0x96           // RPU
	s.Code[0x23] = 0x02           // B = 2 個 word
	s.ERec = 0x1111
	s.push(0xAAAA) // 兩個參數
	s.push(0xBBBB)
	spBefore := s.SP

	s.run(t, 2) // CPL、RPU

	if want := spBefore + 4; s.SP != want {
		t.Errorf("返回之後 SP = %04X，該是 %04X（削掉兩個 word 的參數）", s.SP, want)
	}
	if s.IPC != 2 {
		t.Errorf("返回之後 IPC = %04X", s.IPC)
	}
}

func TestFaultsAreReportedNotSwallowed(t *testing.T) {
	// 每一種都對應原版跳到錯誤處理。安靜地回一個值會讓錯誤在很後面才浮現，
	// 而且看起來像別的問題。
	for _, tt := range []struct {
		name string
		code []byte
		push []uint16
		want string
	}{
		{"DVI 除以零 @0x067f", []byte{0x8d}, []uint16{7, 0}, "除以零"},
		{"MODI 除以零 @0x0bbc", []byte{0x8f}, []uint16{7, 0}, "除以零"},
		{"ABI 溢位 @0x089a", []byte{0xe0}, []uint16{0x8000}, "溢位"},
		{"CHK 超出範圍 @0x0830", []byte{0xcb}, []uint16{99, 1, 10}, "不在"},
		{"IXP 每個 word 裝 0 個", []byte{0xd8, 0x00, 0x04}, []uint16{0x800, 1}, "0 個元素"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.code...)
			for _, v := range tt.push {
				s.push(v)
			}
			_, err := s.Step()
			var f *Fault
			if !errors.As(err, &f) {
				t.Fatalf("回的是 %v", err)
			}
			if !contains(f.Error(), tt.want) {
				t.Errorf("錯誤訊息 %q 沒有提到 %q", f, tt.want)
			}
		})
	}
	// CHK 在範圍內時值要留在堆疊上。
	if got := exec(t, []byte{0xcb}, []uint16{5, 1, 10}, 1, nil)[0]; got != 5 {
		t.Errorf("CHK 通過之後 TOS = %d，該是 5", got)
	}
}

func TestConstantPoolIsCodeRelative(t *testing.T) {
	// LCO @0x05ca 推的是「常數池基底 ＋ 偏移」。基底沒跟著切段換的話
	// 會讀到別段的常數——而那看起來像合法的資料。
	got := exec(t, []byte{0x82, 0x05}, nil, 1, func(s *State) { s.ConstPool = 0x0400 })[0]
	if want := uint16(0x0400 + 10); got != want {
		t.Errorf("LCO 推了 %04X，該是 %04X", got, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
