package pmachine

import "testing"

// 集合在堆疊上的表示法是 **N 個 word ＋ 頂端的 N**，word k 在 sp+2k
// （由 INN @0x0dc6 的 `sp += 2k; pop` 讀出來）。
//
// 長度取錯不會報錯，只會讓成員判斷在邊界上出錯；推入方向搞反也不會報錯，
// 只會讓整個位元向量倒過來——而每一個 word 都還是合法的值。

func setState(code ...byte) *State { return newState(code...) }

func TestSetRoundTripsThroughTheStack(t *testing.T) {
	s := setState()
	want := []uint16{0x0001, 0x8000, 0x1234}
	s.pushSet(want)
	// 頂端要是長度。
	if n := s.TOS0(0); n != 3 {
		t.Fatalf("頂端是 %d，該是長度 3", n)
	}
	// word 0 要在長度底下第一個。
	if w := s.TOS0(1); w != want[0] {
		t.Fatalf("word 0 是 %04X，該是 %04X", w, want[0])
	}
	got := s.popSet()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("取回 %v，該是 %v", got, want)
		}
	}
	if s.SP != 0x0F00 {
		t.Errorf("來回一趟之後 SP = %04X，沒有回到原位", s.SP)
	}
}

func TestSRSBuildsTheRightBits(t *testing.T) {
	for _, tt := range []struct {
		lo, hi int16
		want   []uint16
	}{
		{0, 0, []uint16{0x0001}},
		{3, 5, []uint16{0x0038}},
		{15, 16, []uint16{0x8000, 0x0001}}, // 跨 word 邊界
		{0, 31, []uint16{0xFFFF, 0xFFFF}},
		{20, 18, nil}, // 下界大於上界 → 空集合
	} {
		s := setState(0xbc)
		s.push(uint16(tt.lo))
		s.push(uint16(tt.hi))
		if _, err := s.Step(); err != nil {
			t.Fatalf("%d..%d：%v", tt.lo, tt.hi, err)
		}
		got := s.popSet()
		if len(got) != len(tt.want) {
			t.Errorf("%d..%d 得到 %d 個 word：%04X，該是 %04X", tt.lo, tt.hi, len(got), got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%d..%d 得到 %04X，該是 %04X", tt.lo, tt.hi, got, tt.want)
				break
			}
		}
	}
}

func TestSRSRejectsOutOfRangeBounds(t *testing.T) {
	// 原版檢查下界為負與上界超過 0FEFh，兩種都跳執行期錯誤。
	for _, tt := range [][2]int16{{-1, 5}, {0, 0xFF0}} {
		s := setState(0xbc)
		s.push(uint16(tt[0]))
		s.push(uint16(tt[1]))
		if _, err := s.Step(); err == nil {
			t.Errorf("%d..%d 該回錯誤", tt[0], tt[1])
		}
	}
}

func TestINNLooksAtTheRightWordAndBit(t *testing.T) {
	// 位元編號是 v%16、word 是 v/16。兩者對調在 v<16 時結果相同，
	// 所以要測跨 word 的值。
	set := []uint16{0x0002, 0x0100} // 元素 1 與 24
	for _, tt := range []struct {
		v    int16
		want uint16
	}{{1, 1}, {0, 0}, {24, 1}, {23, 0}, {32, 0}, {-1, 0}} {
		s := setState(0xda)
		s.push(uint16(tt.v))
		s.pushSet(set)
		if _, err := s.Step(); err != nil {
			t.Fatal(err)
		}
		if got := s.pop(); got != tt.want {
			t.Errorf("%d 在不在集合裡：得到 %d，該是 %d", tt.v, got, tt.want)
		}
	}
}

func TestSetOperationsAndTheirLengths(t *testing.T) {
	// 長度規則：聯集取長的、交集取短的、差集取左邊那個。
	a := []uint16{0xF0F0, 0x00FF}
	b := []uint16{0x0FF0}
	for _, tt := range []struct {
		name string
		op   byte
		want []uint16
	}{
		{"UNI 取長的", 0xdb, []uint16{0xFFF0, 0x00FF}},
		{"INT 取短的", 0xdc, []uint16{0x00F0}},
		{"DIF 取左邊", 0xdd, []uint16{0xF000, 0x00FF}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := setState(tt.op)
			s.pushSet(a)
			s.pushSet(b)
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			got := s.popSet()
			if len(got) != len(tt.want) {
				t.Fatalf("得到 %d 個 word：%04X，該是 %04X", len(got), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("得到 %04X，該是 %04X", got, tt.want)
				}
			}
		})
	}
}

func TestSetComparisonsPadTheShorterSide(t *testing.T) {
	// 長度不同的集合要當成後面補零來比。不補的話「短的一定相等」，
	// 而那是安靜的錯誤。
	a := []uint16{0x000F}
	long := []uint16{0x000F, 0x0000}
	other := []uint16{0x000F, 0x0001}
	for _, tt := range []struct {
		name string
		op   byte
		x, y []uint16
		want uint16
	}{
		{"EQPWR 補零之後相等", 0xb6, a, long, 1},
		{"EQPWR 後面有東西就不等", 0xb6, a, other, 0},
		{"LEPWR 子集", 0xb7, a, other, 1},
		{"LEPWR 反過來不是", 0xb7, other, a, 0},
		{"GEPWR 超集", 0xb8, other, a, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := setState(tt.op)
			s.pushSet(tt.x)
			s.pushSet(tt.y)
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if got := s.pop(); got != tt.want {
				t.Errorf("得到 %d，該是 %d", got, tt.want)
			}
		})
	}
}

func TestADJDropsTheLengthWord(t *testing.T) {
	// ADJ 之後集合是**固定長度**的，堆疊上不再有長度 word——
	// 留著的話後面每一條的堆疊都會差一個 word。
	s := setState(0xc7, 0x03) // ADJ 3
	s.pushSet([]uint16{0x1111, 0x2222})
	spBefore := s.SP
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	// 原本 2 個 word ＋ 長度 ＝ 3 個 word；調成 3 個 word 沒有長度 ＝ 3 個。
	if s.SP != spBefore {
		t.Errorf("SP = %04X，該是 %04X", s.SP, spBefore)
	}
	for i, want := range []uint16{0x1111, 0x2222, 0x0000} {
		if got := s.pop(); got != want {
			t.Errorf("第 %d 個 word 是 %04X，該是 %04X", i, got, want)
		}
	}
}

func TestSwapExchangesTheTopTwo(t *testing.T) {
	s := setState(0xbd)
	s.push(0x1111)
	s.push(0x2222)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if a, b := s.pop(), s.pop(); a != 0x1111 || b != 0x2222 {
		t.Errorf("對調之後是 %04X %04X", a, b)
	}
}
