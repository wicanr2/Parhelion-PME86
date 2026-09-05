package pcode

import "testing"

func TestTableLoads(t *testing.T) {
	if err := Err(); err != nil {
		t.Fatal(err)
	}
}

// 幾個從直譯器的碼本身獨立確認過的格（見 docs/10-interpreter/opcode-map.md）。
// 表整個位移一格是最容易發生又最難發現的錯，這幾個釘住它。
func TestKnownOpcodes(t *testing.T) {
	for _, tt := range []struct {
		op   uint8
		want string
	}{
		{0x00, "SLDC"}, {0x1f, "SLDC"},
		{0x81, "LDCI"}, {0x82, "LCO"}, {0x83, "LDC"},
		{0x86, "LAO"}, {0x8d, "DVI"},
		{0x96, "RPU"}, {0x97, "CPF"}, {0x9e, "BPT"}, {0x9f, "BNOT"},
		{0xac, "CSP"}, {0xbc, "SRS"}, {0xc5, "MOV"},
		{0xc9, "LDP"}, {0xca, "STP"}, {0xd6, "XJP"}, {0xd8, "IXP"},
	} {
		if got := Mnemonic(tt.op); got != tt.want {
			t.Errorf("%#02x = %q，該是 %q", tt.op, got, tt.want)
		}
	}
}

func TestUnusedSlotsHaveNoName(t *testing.T) {
	// 0x40–0x5F 在 IV.0 表裡沒有指令，這份直譯器把它們指向錯誤 11。
	// 硬給名字會讓「執行到沒有的指令」看起來像正常的一條。
	for op := 0x40; op <= 0x5f; op++ {
		if got := Mnemonic(uint8(op)); got != "" {
			t.Fatalf("%#02x 有名字 %q", op, got)
		}
	}
}
