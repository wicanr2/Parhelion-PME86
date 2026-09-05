package pmachine

import "testing"

// 排程器只碰資料段裡的三條串列：ready queue（ss:38h）、目前的 task（ss:3Ch）、
// 號誌的等待佇列（+2）。這一份釘的是那三條的維護——錯了不會當場報錯，
// 只會讓兩個 task 同時進臨界區，或讓某個 task 再也不被叫起來。

// tib 在 at 擺一個 TIB：優先權 prio，狀態指向 ipc。
func tib(s *State, at uint16, prio uint8, ipc, erec uint16) {
	s.Store(at, 0) // 串列的下一個
	s.Store(at+2, uint16(prio))
	s.Store(at+8, 0x0700)    // SP
	s.Store(at+0x0a, 0x0700) // MP
	s.Store(at+0x0e, ipc)
	s.Store(at+0x10, erec)
	s.Store(at+0x12, 1)
}

// oneSegEnv 只認得一個 E_Rec——換 task 不換段的情境用。
type oneSegEnv struct{ seg *Segment }

func (e *oneSegEnv) ByNumber(uint16) (*Segment, error) { return e.seg, nil }
func (e *oneSegEnv) ByERec(uint16) (*Segment, error)   { return e.seg, nil }
func (e *oneSegEnv) Globals(uint16) (uint16, error)    { return e.seg.Global, nil }
func (e *oneSegEnv) Intrinsic(uint16) bool             { return false }

func schedState(op byte) *State {
	s := newState(op)
	s.Code = append(s.Code, make([]byte, 0x100)...)
	s.Env = &oneSegEnv{&Segment{Code: s.Code, Global: 0x0400, ERec: 0x0100}}
	s.ERec = 0x0100
	return s
}

func TestWaitBlocksAndHandsOver(t *testing.T) {
	const sem, me, other = 0x0900, 0x0800, 0x0820
	s := schedState(0xdf) // WAIT
	tib(s, me, 128, 0x0050, 0x0100)
	tib(s, other, 100, 0x0060, 0x0100)
	s.Store(me, other) // ready queue：自己在前，另一個在後
	s.Store(readyQueue, me)
	s.Store(tibPtr, me)
	s.TIB = me
	s.Store(sem, 0) // 沒餘額
	s.push(sem)

	if _, err := s.Step(); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(readyQueue); got != other {
		t.Errorf("ready queue 的頭是 %04X，自己該被拿掉才對", got)
	}
	if got := s.Load(sem + 2); got != me {
		t.Errorf("號誌的等待佇列是 %04X，該是 %04X", got, me)
	}
	if got := s.Load(me + 0x14); got != sem {
		t.Errorf("TIB+14h 是 %04X，該記著在等哪個號誌", got)
	}
	if s.Load(tibPtr) != other || s.IPC != 0x0060 {
		t.Errorf("換人之後是 %04X／IPC %04X，該換到 %04X／0060",
			s.Load(tibPtr), s.IPC, other)
	}
}

// 叫起來的那個優先權**比自己低**就不換人（@0x17C7 的 `jb`）。
func TestSignalOnlyYieldsToEqualOrHigherPriority(t *testing.T) {
	const sem, me, other = 0x0900, 0x0800, 0x0820
	for _, tt := range []struct {
		name   string
		prio   uint8
		yields bool
	}{
		{"優先權比較低就不換", 100, false},
		// 一樣高也會走換人那條路，但插隊是插在自己**後面**，
		// 所以 ready queue 的頭還是自己——換完等於沒換。
		{"一樣高就排在自己後面", 128, false},
		{"比自己高就真的換過去", 200, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := schedState(0xde) // SIGNAL
			tib(s, me, 128, 0x0050, 0x0100)
			tib(s, other, tt.prio, 0x0060, 0x0100)
			s.Store(readyQueue, me)
			s.Store(tibPtr, me)
			s.TIB = me
			s.Store(sem, 1)
			s.Store(sem+2, other) // 有人在等
			s.push(sem)

			if _, err := s.Step(); err != nil {
				t.Fatal(err)
			}
			if got := s.Load(sem + 2); got != 0 {
				t.Errorf("等待佇列還剩 %04X，該被取走", got)
			}
			if got := s.Load(other + 0x14); got != 0 {
				t.Errorf("被叫起來的 TIB+14h 是 %04X，該清成 0", got)
			}
			if got := s.Load(readyQueue); got == 0 {
				t.Fatal("被叫起來的沒有進 ready queue")
			}
			switched := s.Load(tibPtr) == other
			if switched != tt.yields {
				t.Errorf("換人＝%v，該是 %v", switched, tt.yields)
			}
		})
	}
}

// 同優先權要排在後面——先等的先被叫到。
func TestQueueInsertKeepsArrivalOrderWithinAPriority(t *testing.T) {
	s := newState()
	const a, b, c = 0x0800, 0x0820, 0x0840
	tib(s, a, 128, 0, 0)
	tib(s, b, 128, 0, 0)
	tib(s, c, 200, 0, 0)

	head := s.queueInsert(0, a)
	head = s.queueInsert(head, b)
	if head != a || s.Load(a) != b {
		t.Errorf("同優先權排成 %04X→%04X，該是 %04X→%04X", head, s.Load(head), a, b)
	}
	head = s.queueInsert(head, c)
	if head != c || s.Load(c) != a {
		t.Errorf("優先權高的沒排到最前面：頭是 %04X", head)
	}
}
