package pmachine

import (
	"math"
	"testing"
)

// 8 個位元組是 IEEE 754 binary64，little-endian——word 0 在堆疊最上面。
// 位元組序搞反不會報錯，只會讓每一個實數都變成別的數字。

func TestRealRoundTripsThroughTheStack(t *testing.T) {
	for _, v := range []float64{0, 1, -1, 3.14159265358979, 1e300, -2.5e-10} {
		s := newState()
		s.pushReal(v)
		// 最上面那個 word 要是 64 位元值的最低 16 位。
		if got, want := s.TOS0(0), uint16(math.Float64bits(v)); got != want {
			t.Fatalf("%g 的最上面那個 word 是 %04X，該是 %04X", v, got, want)
		}
		if got := s.popReal(); got != v {
			t.Errorf("取回 %g，該是 %g", got, v)
		}
		if s.SP != 0x0F00 {
			t.Errorf("來回一趟之後 SP = %04X", s.SP)
		}
	}
}

func TestRealArithmetic(t *testing.T) {
	for _, tt := range []struct {
		name string
		op   byte
		a, b float64
		want float64
	}{
		{"ADR @0x23c2", 0xc0, 1.5, 2.25, 3.75},
		{"SBR @0x23bc 不對稱", 0xc1, 1.5, 2.25, -0.75},
		{"MPR @0x24e1", 0xc2, 1.5, 2.0, 3.0},
		{"DVR @0x262a 不對稱", 0xc3, 3.0, 2.0, 1.5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.op)
			s.pushReal(tt.a)
			s.pushReal(tt.b)
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if got := s.popReal(); got != tt.want {
				t.Errorf("%g op %g = %g，該是 %g", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNGRLeavesTrueZeroAlone(t *testing.T) {
	// @0x278c 檢查四個 word 全零才跳過翻符號。直接用 Go 的 -v
	// 會把 0 變成 −0.0，位元不同——而位元不同不會報錯。
	s := newState(0xe4)
	s.pushReal(0)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.popBits(); got != 0 {
		t.Errorf("−0 的位元是 %016X，該還是 0", got)
	}

	s = newState(0xe4)
	s.pushReal(2.5)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.popReal(); got != -2.5 {
		t.Errorf("取負得到 %g", got)
	}
}

func TestABRClearsOnlyTheSignBit(t *testing.T) {
	s := newState(0xe3)
	s.pushReal(-3.75)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.popReal(); got != 3.75 {
		t.Errorf("取絕對值得到 %g", got)
	}
}

func TestIntegerConversions(t *testing.T) {
	for _, tt := range []struct {
		name string
		op   byte
		in   float64
		want int16
	}{
		{"TNC 往零截斷（正）", 0xbe, 2.9, 2},
		{"TNC 往零截斷（負）", 0xbe, -2.9, -2},
		{"RND 四捨五入（正）", 0xbf, 2.5, 3},
		{"RND 四捨五入（負）", 0xbf, -2.5, -3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.op)
			s.pushReal(tt.in)
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if got := int16(s.pop()); got != tt.want {
				t.Errorf("%g → %d，該是 %d", tt.in, got, tt.want)
			}
		})
	}
	// FLT 反過來。
	s := newState(0xcc)
	s.push(uint16(0xFFF9)) // −7
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.popReal(); got != -7 {
		t.Errorf("FLT −7 得到 %g", got)
	}
}

func TestRealComparisonsTakeTOSAsTheRightOperand(t *testing.T) {
	for _, tt := range []struct {
		name string
		op   byte
		want uint16
	}{
		{"EQREAL", 0xcd, 0},
		{"LEREAL 1.0 <= 2.0", 0xce, 1},
		{"GEREAL 1.0 >= 2.0", 0xcf, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.op)
			s.pushReal(1.0)
			s.pushReal(2.0)
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if got := s.pop(); got != tt.want {
				t.Errorf("得到 %d，該是 %d", got, tt.want)
			}
		})
	}
}

func TestRealLoadStoreAndDuplicate(t *testing.T) {
	// STRL 的位址壓在實數底下（@0x2a19 的 `mov di,[si+8]`）；
	// 位址與實數的先後搞反會寫到別的地方，而那不會報錯。
	s := newState(0xf4) // STRL
	s.push(0x0800)      // 位址
	s.pushReal(1.25)
	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if s.SP != 0x0F00 {
		t.Errorf("STRL 之後 SP = %04X，該把實數與位址都丟掉", s.SP)
	}

	s2 := newState(0xf3) // LDRD
	s2.Data = s.Data
	s2.push(0x0800)
	if _, err := s2.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s2.popReal(); got != 1.25 {
		t.Errorf("LDRD 讀回 %g", got)
	}

	s3 := newState(0xc6) // DUPR
	s3.pushReal(9.5)
	if _, err := s3.Step(); err != nil {
		t.Fatal(err)
	}
	if a, b := s3.popReal(), s3.popReal(); a != 9.5 || b != 9.5 {
		t.Errorf("DUPR 之後是 %g %g", a, b)
	}
}
