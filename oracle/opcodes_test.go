//go:build oracle

package oracle_test

import (
	"math"
	"testing"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/Parhelion-PME86/oracle"
)

// 對拍走的是作業系統開機那條路，走不到的指令就永遠沒有證據。
// 這一份把那個洞補起來：指令與運算元由我們指定，兩邊各走一條、逐 word 比。
//
// **這才是浮點的證據。** 在這之前那 16 支只有單元測試撐著，
// 而單元測試與實作出自同一份理解——我讀錯原版的話兩邊會一起錯。

// realWords 把一個 IEEE 754 binary64 拆成堆疊上的四個 word，
// 低位的那個在最上面（`LDCRL` @0x2A64 的四個 `movsw` 就是這個次序）。
func realWords(v float64) []uint16 {
	b := math.Float64bits(v)
	return []uint16{uint16(b), uint16(b >> 16), uint16(b >> 32), uint16(b >> 48)}
}

func wordsReal(w []uint16) float64 {
	var b uint64
	for i := 0; i < 4 && i < len(w); i++ {
		b |= uint64(w[i]) << (16 * i)
	}
	return math.Float64frombits(b)
}

// setWords 把一個集合排成堆疊上的樣子：N 個 word，頂端再放一個 N。
func setWords(w ...uint16) []uint16 {
	return append([]uint16{uint16(len(w))}, w...)
}

// pstr 把一個 UCSD 字串（長度位元組 ＋ 字元）打包成堆疊上的 word。
func pstr(v string) []uint16 {
	b := append([]byte{byte(len(v))}, v...)
	for len(b)%2 != 0 {
		b = append(b, 0)
	}
	w := make([]uint16, len(b)/2)
	for i := range w {
		w[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return w
}

// strCase 造一個字串比較的案例：兩個字串就擺在堆疊上，位址往上指。
//
// 堆疊由頂往下是 右字串位址、左字串位址，接著兩個字串本體。
// `sp` 是 Exercise 擺運算元的位置。
func strCase(name string, op uint8, left, right string) opCase {
	const sp = 0xD000
	l, r := pstr(left), pstr(right)
	stack := []uint16{sp + 4 + uint16(2*len(l)), sp + 4}
	stack = append(stack, l...)
	stack = append(stack, r...)
	return opCase{name, []byte{op, 0, 0}, stack, 1, false}
}

type opCase struct {
	name  string
	code  []byte
	stack []uint16 // 由堆疊頂往下
	want  int      // 結果佔幾個 word
	real  bool     // 結果印成實數
}

func runOpCases(t *testing.T, cases []opCase) {
	t.Helper()
	s := bootToPME(t)
	if _, err := s.Trace(1, traceBudget); err != nil {
		t.Fatal(err)
	}
	runOpCasesOn(t, s, cases)
}
func runOpCasesOn(t *testing.T, s *oracle.System, cases []opCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			theirs, ours, err := s.Exercise(tt.code, tt.stack, tt.want)
			if err != nil {
				t.Fatalf("%02X %s：%v", tt.code[0], pcode.Mnemonic(tt.code[0]), err)
			}
			for i := range theirs {
				if theirs[i] != ours[i] {
					t.Fatalf("%s 第 %d 個 word：原版 %04X，我們 %04X",
						pcode.Mnemonic(tt.code[0]), i, theirs[i], ours[i])
				}
			}
			if tt.real {
				t.Logf("%s ＝ %v", pcode.Mnemonic(tt.code[0]), wordsReal(theirs))
			} else {
				t.Logf("%s ＝ %v", pcode.Mnemonic(tt.code[0]), theirs)
			}
		})
	}
}

func TestFloatOpsMatchTheOriginal(t *testing.T) {
	bin := func(name string, op uint8, a, b float64) opCase {
		return opCase{name, []byte{op}, append(realWords(b), realWords(a)...), 4, true}
	}
	cmp := func(name string, op uint8, a, b float64) opCase {
		return opCase{name, []byte{op}, append(realWords(b), realWords(a)...), 1, false}
	}
	un := func(name string, op uint8, a float64, want int) opCase {
		return opCase{name, []byte{op}, realWords(a), want, want == 4}
	}

	runOpCases(t, []opCase{
		bin("ADR", 0xC0, 3.5, 1.25),
		bin("ADR 一正一負", 0xC0, -3.5, 1.25),
		bin("ADR 差很多級", 0xC0, 1e300, 1e-300),
		bin("SBR", 0xC1, 3.5, 1.25),
		bin("SBR 減成負的", 0xC1, 1.25, 3.5),
		bin("MPR", 0xC2, 3.5, 1.25),
		bin("MPR 很小的數", 0xC2, 1e-300, 1e-5),
		bin("MPR 負負得正", 0xC2, -3.5, -1.25),
		bin("DVR", 0xC3, 3.5, 1.25),
		bin("DVR 除不盡", 0xC3, 1, 3),
		bin("DVR 除以負數", 0xC3, 1, -3),

		un("NGR", 0xE4, 3.5, 4),
		un("NGR 對零", 0xE4, 0, 4),
		un("NGR 對負數", 0xE4, -3.5, 4),
		un("ABR", 0xE3, -3.5, 4),
		un("ABR 對正數", 0xE3, 3.5, 4),
		un("DUPR", 0xC6, 2.75, 8),
		un("TNC 往零取整", 0xBE, 3.9, 1),
		un("TNC 負數往零取整", 0xBE, -3.9, 1),
		un("RND 四捨五入", 0xBF, 3.5, 1),
		un("RND 負數", 0xBF, -3.5, 1),

		cmp("EQREAL 相等", 0xCD, 3.5, 3.5),
		cmp("EQREAL 不等", 0xCD, 3.5, 1.25),
		cmp("LEREAL 小於", 0xCE, 1.25, 3.5),
		cmp("LEREAL 大於", 0xCE, 3.5, 1.25),
		cmp("GEREAL 大於", 0xCF, 3.5, 1.25),
		cmp("GEREAL 相等", 0xCF, 3.5, 3.5),

		{"FLT 整數轉實數", []byte{0xCC}, []uint16{42}, 4, true},
		{"FLT 轉負數", []byte{0xCC}, []uint16{0xFFD6}, 4, true},
		{"FLT 轉零", []byte{0xCC}, []uint16{0}, 4, true},
	})
}

func TestOtherUnexercisedOpsMatchTheOriginal(t *testing.T) {
	runOpCases(t, []opCase{
		{"DVI", []byte{0x8D}, []uint16{3, 17}, 1, false},
		{"DVI 除負數", []byte{0x8D}, []uint16{0xFFFD, 17}, 1, false},
		{"MODI", []byte{0x8F}, []uint16{3, 17}, 1, false},
		{"MODI 被除數是負的", []byte{0x8F}, []uint16{3, 0xFFEF}, 1, false},
		{"ABI", []byte{0xE0}, []uint16{0xFFFB}, 1, false},
		{"ABI 對正數", []byte{0xE0}, []uint16{5}, 1, false},
		{"SWAP", []byte{0xBD}, []uint16{0x1111, 0x2222}, 2, false},
		{"NOP", []byte{0x9C}, []uint16{0x1234}, 1, false},
		{"CHK 在範圍內", []byte{0xCB}, []uint16{10, 1, 5}, 3, false},

		// 集合：N 個 word ＋ 頂端一個 N。
		{"UNI 聯集", []byte{0xDB}, append(setWords(0x000F), setWords(0x00F0)...), 2, false},
		{"INT 交集", []byte{0xDC}, append(setWords(0x00FF), setWords(0x0F0F)...), 2, false},
		{"DIF 差集", []byte{0xDD}, append(setWords(0x000F), setWords(0x00FF)...), 2, false},
		{"UNI 長度不同", []byte{0xDB}, append(setWords(0x0001), setWords(0x0002, 0x0004)...), 3, false},
		{"INN 在集合裡", []byte{0xDA}, append(setWords(0x0008), []uint16{3}...), 1, false},
		{"INN 不在集合裡", []byte{0xDA}, append(setWords(0x0008), []uint16{4}...), 1, false},
		{"EQPWR 相等", []byte{0xB6}, append(setWords(0x00FF), setWords(0x00FF)...), 1, false},
		{"EQPWR 不等", []byte{0xB6}, append(setWords(0x00FF), setWords(0x000F)...), 1, false},
		{"LEPWR 子集", []byte{0xB7}, append(setWords(0x00FF), setWords(0x000F)...), 1, false},
		{"GEPWR 超集", []byte{0xB8}, append(setWords(0x000F), setWords(0x00FF)...), 1, false},
		{"SRS 造子範圍", []byte{0xBC}, []uint16{7, 3}, 3, false},
		{"SRS 空的", []byte{0xBC}, []uint16{2, 5}, 1, false},

		strCase("EQSTR 相同", 0xE8, "ABC", "ABC"),
		strCase("EQSTR 不同", 0xE8, "ABC", "ABD"),
		strCase("LESTR 短的在前", 0xE9, "AB", "ABC"),
		strCase("LESTR 大於", 0xE9, "ABD", "ABC"),
		strCase("GESTR 大於", 0xEA, "ABD", "ABC"),
		strCase("GESTR 長度不同", 0xEA, "ABC", "AB"),

		{"MPI", []byte{0x8C}, []uint16{7, 6}, 1, false},
		{"MPI 溢位只留低 16 位", []byte{0x8C}, []uint16{0x1000, 0x1000}, 1, false},
		{"MPI 負數", []byte{0x8C}, []uint16{0xFFFF, 5}, 1, false},

		// 短形式的幾格：開機路徑剛好沒踩到的那幾個編號。
		{"SLDC 09", []byte{0x09}, nil, 1, false},
		{"SLDC 13", []byte{0x13}, nil, 1, false},
		{"SLDC 1C", []byte{0x1C}, nil, 1, false},
		{"SLDC 1D", []byte{0x1D}, nil, 1, false},
		{"SLDC 1F", []byte{0x1F}, nil, 1, false},
		{"SLDO11", []byte{0x3A}, nil, 1, false},
		{"SLDO12", []byte{0x3B}, nil, 1, false},
		{"SLDO16", []byte{0x3F}, nil, 1, false},

		// LPR 讀處理器暫存器：兩邊讀的是同一份 TIB，答案該一樣。
		{"LPR 讀 TIB 指標", []byte{0x9D}, []uint16{0xFFFF}, 1, false},
		{"LPR 讀 E_Vec", []byte{0x9D}, []uint16{0xFFFE}, 1, false},

		// 實數的載入與存回：位址就指堆疊上更下面那幾個 word。
		{"LDRD 從位址載實數", []byte{0xF3},
			append([]uint16{0xD002}, realWords(6.25)...), 4, true},
		{"STRL 把實數存回位址", []byte{0xF4},
			append(append(realWords(2.5), 0xD00A), 0, 0, 0, 0), 5, false},

		{"CSTR 長度在範圍內", []byte{0xEC},
			append([]uint16{3, 0xD004}, pstr("ABCDE")...), 2, false},
	})
}
