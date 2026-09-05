package pmachine

import (
	"encoding/binary"
	"errors"
	"testing"
)

// 這一份測試不需要原版素材：程式碼是手寫的 p-code，狀態自己擺。
// 真正的判準是 oracle 那邊的逐條對拍——這裡釘的是「改壞了會立刻知道」的地方。

// newState 造一台機器：Code 是給的 p-code，Data 是 4 KB，堆疊從頂端往下長。
func newState(code ...byte) *State {
	s := &State{
		Code:   append([]byte(nil), code...),
		Data:   make([]byte, 0x1000),
		SP:     0x0F00,
		Local:  0x0200,
		Global: 0x0400,
	}
	return s
}

func (s *State) run(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := s.Step(); err != nil {
			t.Fatalf("第 %d 條：%v", i, err)
		}
	}
}

func TestShortFormsUseTheRightBase(t *testing.T) {
	s := newState(0x21, 0x31, 0x61)
	s.Store(s.Local+4, 0xAAAA)  // 區域變數 2
	s.Store(s.Global+4, 0xBBBB) // 全域變數 2
	s.run(t, 3)
	// SLLA2 推的是位址，不是內容——推錯的話後面的 SIND 會讀到別的地方，
	// 而那不會報錯。
	if got := s.pop(); got != s.Local+4 {
		t.Errorf("SLLA2 推了 %04X，該是 %04X", got, s.Local+4)
	}
	if got := s.pop(); got != 0xBBBB {
		t.Errorf("SLDO2 推了 %04X", got)
	}
	if got := s.pop(); got != 0xAAAA {
		t.Errorf("SLDL2 推了 %04X", got)
	}
}

func TestBigOperandIsWordCountTimesTwo(t *testing.T) {
	// LDL 0x0102（兩個位元組的形式）→ 區域變數 258 → Local+516
	s := newState(0x87, 0x81, 0x02)
	s.Store(s.Local+516, 0x1234)
	s.run(t, 1)
	if got := s.pop(); got != 0x1234 {
		t.Errorf("LDL 讀到 %04X", got)
	}
}

func TestRelativeJumpsCountFromAfterTheOperand(t *testing.T) {
	// 這是最容易寫錯又最難發現的一條：位移是從**運算元之後**算起。
	// 差一個位元組會落到運算元中間，然後把資料當成指令執行。
	for _, tt := range []struct {
		name string
		at   uint16
		code []byte
		want uint16
	}{
		{"UJP 往前", 0, []byte{0x8a, 0x03}, 2 + 3},
		{"UJP 往回", 2, []byte{0x00, 0x00, 0x8a, 0xFC}, 4 - 4},
		{"JPL", 0, []byte{0x8b, 0x10, 0x00}, 3 + 0x10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.code...)
			s.IPC = tt.at
			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if s.IPC != tt.want {
				t.Errorf("跳到 %04X，該是 %04X", s.IPC, tt.want)
			}
		})
	}
}

func TestConditionalJumpsTestTheLowestBit(t *testing.T) {
	// 原版用 `shr ax,1` 看進位，也就是只看最低位。
	// 拿整個 word 當真假會在「值是 2」的時候得到相反的結果。
	for _, tt := range []struct {
		v      uint16
		jumped bool
	}{{0, true}, {1, false}, {2, true}, {3, false}} {
		s := newState(0xd4, 0x05)
		s.push(tt.v)
		s.run(t, 1)
		if got := s.IPC != 2; got != tt.jumped {
			t.Errorf("FJP 遇到 %04X：跳了 %v，該是 %v", tt.v, got, tt.jumped)
		}
	}
}

func TestComparisonsTakeTOSAsTheRightOperand(t *testing.T) {
	// 原版是 `pop ax; pop bp; cmp bp,ax`——TOS 是右邊那個。
	// 顛倒過來 EQUI 看不出來，LEQI 會整個反過來。
	s := newState(0xb2) // LEQI
	s.push(3)           // 左
	s.push(5)           // 右
	s.run(t, 1)
	if got := s.pop(); got != 1 {
		t.Errorf("3 <= 5 推了 %d", got)
	}
}

func TestPackedFieldRoundTrip(t *testing.T) {
	// LDP／STP 的三元組次序：位址、位元數、最右 bit 編號。
	const addr, width, right = 0x0800, 5, 3
	s := newState(0xca, 0xc9) // STP 然後 LDP
	s.Store(addr, 0xFFFF)
	s.push(addr)
	s.push(width)
	s.push(right)
	s.push(0x0A) // 5 bit 裝得下
	s.run(t, 1)
	// 欄位外的位元不能被動到。
	if got := s.Load(addr); got != 0xFF57 {
		t.Errorf("STP 之後那個 word 是 %04X，該是 FF57", got)
	}
	s.push(addr)
	s.push(width)
	s.push(right)
	s.run(t, 1)
	if got := s.pop(); got != 0x0A {
		t.Errorf("LDP 讀回 %02X", got)
	}
}

func TestCallBuildsFigureFiveLayout(t *testing.T) {
	// MSCW 從 MP 往上是 MSSTAT、MSDYN、MSIPC、MSENV、MSPROC，
	// 而區域變數從 MP+10 開始——順序錯了，被呼叫者的區域變數會蓋到控制欄位。
	s := newState(0x90, 0x02) // CPL 2
	s.Code = append(s.Code, make([]byte, 0x100)...)
	s.ProcDict = 0x40
	// 字典往回長：程序 2 的字典項在 ProcDict − 4，值是 DATASIZE 的 word 偏移。
	binary.LittleEndian.PutUint16(s.Code[0x40-4:], 0x10)
	binary.LittleEndian.PutUint16(s.Code[0x20:], 3) // DATASIZE = 3 個 word
	s.Proc, s.Env = 7, 0x1234
	oldSP, oldLocal := s.SP, s.Local

	s.run(t, 1)

	mp := s.Local - 8
	if s.Local != s.SP+8 {
		t.Errorf("區域基底 %04X 不是 SP+8", s.Local)
	}
	if got := s.Load(mp + 0); got != oldLocal-8 {
		t.Errorf("MSSTAT = %04X，該是呼叫者的 MP %04X", got, oldLocal-8)
	}
	if got := s.Load(mp + 2); got != oldLocal-8 {
		t.Errorf("MSDYN = %04X", got)
	}
	if got := s.Load(mp + 4); got != 2 {
		t.Errorf("MSIPC = %04X，該是呼叫指令之後的 0002", got)
	}
	if got := s.Load(mp + 6); got != 0x1234 {
		t.Errorf("MSENV = %04X", got)
	}
	if got := s.Load(mp + 8); got != 7 {
		t.Errorf("MSPROC = %04X，該是舊的程序號 7", got)
	}
	// DATASIZE 個 word 要真的配置出來。
	if want := oldSP - 2*3 - 10; s.SP != want {
		t.Errorf("SP = %04X，該是 %04X（扣掉 3 個 word 的區域資料與 5 個 word 的 MSCW）", s.SP, want)
	}
	if s.IPC != 0x22 {
		t.Errorf("IPC = %04X，該是 DATASIZE 之後的 0022", s.IPC)
	}
}

func TestUnimplementedNamesTheInstruction(t *testing.T) {
	// 對拍的價值就在這裡：它要準確指出下一個要做的是哪一支，
	// 而且**不能**回「執行成功」。
	s := newState(0xc0) // ADR，浮點還沒做
	op, err := s.Step()
	var u *Unimplemented
	if !errors.As(err, &u) {
		t.Fatalf("回的是 %v", err)
	}
	if u.Op != 0xc0 || op != 0xc0 {
		t.Errorf("回報的 opcode 是 %02X／%02X", u.Op, op)
	}
	if s.IPC != 0 {
		t.Errorf("沒執行成功卻把 IPC 推進到 %04X", s.IPC)
	}
}
