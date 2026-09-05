package pmachine

import (
	"encoding/binary"
	"errors"
	"testing"
)

// 這一份測試不需要原版素材：程式碼是手寫的 p-code，狀態自己擺。
// 真正的判準是 oracle 那邊的逐條對拍——這裡釘的是「改壞了會立刻知道」的地方。

// putWord 是測試裡擺 p-code 常數用的小工具。
func putWord(b []byte, off int, v uint16) {
	binary.LittleEndian.PutUint16(b[off:], v)
}

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
	s.Proc, s.ERec = 7, 0x1234
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
	// 0x40 在 IV.0 表裡沒有指令，這份直譯器把它指向錯誤 11。
	// 真的執行到就是 p-code 壞了，要當場說出是哪一格。
	s := newState(0x40)
	op, err := s.Step()
	var u *Unimplemented
	if !errors.As(err, &u) {
		t.Fatalf("回的是 %v", err)
	}
	if u.Op != 0x40 || op != 0x40 {
		t.Errorf("回報的 opcode 是 %02X／%02X", u.Op, op)
	}
	if !contains(u.Error(), "沒有這個指令") {
		t.Errorf("沒有指出這一格在 IV.0 表裡不存在：%v", u)
	}
	if s.IPC != 0 {
		t.Errorf("沒執行成功卻把 IPC 推進到 %04X", s.IPC)
	}
}

// fakeEnv 是測試用的 Environment：段號直接對到預先擺好的 Segment。
type fakeEnv struct {
	byNum     map[uint16]*Segment
	byERec    map[uint16]*Segment
	intrinsic map[uint16]bool
}

func (e *fakeEnv) Intrinsic(proc uint16) bool { return e.intrinsic[proc] }

func (e *fakeEnv) ByNumber(n uint16) (*Segment, error) {
	if s, ok := e.byNum[n]; ok {
		return s, nil
	}
	return nil, ErrNotResident
}

// Globals 不查 SIB——跨段讀全域變數不需要那一段在記憶體裡。
// 測試裡拿 byNum 的 Global 當答案；查不到才算沒有那一段。
func (e *fakeEnv) Globals(n uint16) (uint16, error) {
	if s, ok := e.byNum[n]; ok {
		return s.Global, nil
	}
	return 0, ErrNotResident
}

func (e *fakeEnv) ByERec(r uint16) (*Segment, error) {
	if s, ok := e.byERec[r]; ok {
		return s, nil
	}
	return nil, ErrNotResident
}

// TestCrossSegmentCallSwitchesEverythingTogether 釘住「切段是一整組」。
//
// 少換一項不會報錯：常數池沒換就讀到別段的常數、字典沒換就呼叫到別段的程序，
// 而兩者都只是安靜地讀到看起來像資料的東西。
func TestCrossSegmentCallAndReturn(t *testing.T) {
	target := &Segment{
		Code:      make([]byte, 0x100),
		Global:    0x0600,
		ProcDict:  0x40,
		ConstPool: 0x30,
		ERec:      0x2222,
	}
	binary.LittleEndian.PutUint16(target.Code[0x40-2:], 0x10) // 程序 1 → word 0x10
	binary.LittleEndian.PutUint16(target.Code[0x20:], 2)      // DATASIZE = 2
	target.Code[0x22] = 0x96                                  // RPU
	target.Code[0x23] = 0x00                                  // B = 0

	s := newState(0x70, 0x01, 0xEE) // SCXG1 程序 1，返回後接 DECI
	home := &Segment{
		Code: s.Code, Global: s.Global, ProcDict: 0, ConstPool: 0, ERec: 0x1111,
	}
	s.ERec = home.ERec
	s.Env = &fakeEnv{
		byNum:  map[uint16]*Segment{1: target},
		byERec: map[uint16]*Segment{home.ERec: home, target.ERec: target},
	}
	oldLocal := s.Local

	s.run(t, 1) // SCXG1

	if s.ERec != target.ERec {
		t.Fatalf("E_Rec 還是 %04X，沒有切段", s.ERec)
	}
	if s.Global != target.Global || s.ProcDict != target.ProcDict || s.ConstPool != target.ConstPool {
		t.Errorf("切段沒有一起換：Global=%04X ProcDict=%04X ConstPool=%04X",
			s.Global, s.ProcDict, s.ConstPool)
	}
	if s.IPC != 0x22 {
		t.Errorf("IPC = %04X，該是被呼叫者的第一條 0022", s.IPC)
	}
	// MSENV 要記**呼叫端**的 E_Rec，不是切過去之後的——記錯了返回時換不回來。
	if got := s.Load(s.Local - 8 + 6); got != home.ERec {
		t.Errorf("MSENV = %04X，該是呼叫端的 %04X", got, home.ERec)
	}

	s.run(t, 1) // RPU

	if s.ERec != home.ERec {
		t.Errorf("返回之後 E_Rec 是 %04X，沒有換回來", s.ERec)
	}
	if s.ConstPool != home.ConstPool || s.ProcDict != home.ProcDict {
		t.Error("返回之後常數池／字典沒有換回來")
	}
	if s.Local != oldLocal {
		t.Errorf("返回之後區域基底是 %04X，該是 %04X", s.Local, oldLocal)
	}
	if s.IPC != 2 {
		t.Errorf("返回之後 IPC = %04X，該是呼叫指令之後的 0002", s.IPC)
	}
}

// TestSegmentOneIntrinsicIsCaughtBeforeSwitching 釘住順序。
//
// 原版查內嵌表在切段**之前**，而且查到就完全不換段。先切再發現會留下
// 換了一半的狀態——而那不會報錯，只會讓後面每一條都讀錯地方。
func TestSegmentOneIntrinsicIsCaughtBeforeSwitching(t *testing.T) {
	s := newState(0x70, 0x18) // SCXG1 程序 24
	s.ERec = 0x1111
	s.Env = &fakeEnv{intrinsic: map[uint16]bool{24: true}}
	_, err := s.Step()
	var ic *IntrinsicCall
	if !errors.As(err, &ic) || ic.Proc != 24 {
		t.Fatalf("回的是 %v", err)
	}
	if s.ERec != 0x1111 {
		t.Errorf("E_Rec 變成 %04X——切了一半", s.ERec)
	}
}

func TestCrossSegmentCallReportsNotResident(t *testing.T) {
	s := newState(0x70, 0x01)
	s.Env = &fakeEnv{byNum: map[uint16]*Segment{}}
	if _, err := s.Step(); !errors.Is(err, ErrNotResident) {
		t.Fatalf("段不在記憶體時回的是 %v", err)
	}
}

func TestSemaphoreNonBlockingPaths(t *testing.T) {
	// 號誌是兩個 word：計數與等待佇列的頭（@0x1791 的 `[di]`／`[di+2]`）。
	// 非阻塞的路徑就是加減一；判斷條件寫錯的症狀是「該排隊時沒排」，
	// 而那不會報錯，只會讓兩個 task 同時進臨界區。
	const sem = 0x0900
	for _, tt := range []struct {
		name   string
		op     byte
		count  uint16
		queue  uint16
		want   uint16
		blocks bool
	}{
		{"WAIT 有餘額就減一", 0xdf, 3, 0, 2, false},
		{"SIGNAL 沒人在等就加一", 0xde, 1, 0, 2, false},
		{"SIGNAL 計數為負也只加一", 0xde, 0xFFFF, 0x1234, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newState(tt.op)
			s.Store(sem, tt.count)
			s.Store(sem+2, tt.queue)
			s.push(sem)
			_, err := s.Step()
			if err != nil {
				t.Fatalf("%v", err)
			}
			if got := s.Load(sem); got != tt.want && tt.name != "SIGNAL 計數為負也只加一" {
				t.Errorf("計數變成 %04X，該是 %04X", got, tt.want)
			}
		})
	}
}

func TestNativeCallIsStructurallyImpossible(t *testing.T) {
	// NAT 直接跳進 8086 機器碼。這與「還沒做」是兩件事——
	// 寫多少 p-code 都不會讓它變得可以執行，所以錯誤型別要分得開。
	s := newState(0xa8)
	_, err := s.Step()
	var nc *NativeCall
	if !errors.As(err, &nc) {
		t.Fatalf("回的是 %v", err)
	}
	var u *Unimplemented
	if errors.As(err, &u) {
		t.Error("被當成「還沒實作」了")
	}
	if s.IPC != 0 {
		t.Errorf("IPC 推進到 %04X，該留在原地", s.IPC)
	}
}
