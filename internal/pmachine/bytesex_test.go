package pmachine

import "testing"

// XJP 的 case 表是**資料**，所以跟著段的 byte sex 走；指令流不用。
// 同一個 codefile 裡就混著兩種 sex，這一格錯了會跳到看起來合理的錯位址。
func TestCaseTableFollowsTheSegmentByteSex(t *testing.T) {
	// XJP 運算元 0（表就在常數池起點），索引 1。
	build := func(flipped bool) *State {
		s := padded(newState(0x01, 0xD6, 0x00)) // SLDC 1；XJP 0
		s.ConstPool = 0x20
		lo, hi, jump := uint16(0), uint16(3), uint16(0x0040)
		if flipped {
			swap := func(v uint16) uint16 { return v>>8 | v<<8 }
			lo, hi, jump = swap(lo), swap(hi), swap(jump)
		}
		putWord(s.Code, 0x20, lo)
		putWord(s.Code, 0x22, hi)
		putWord(s.Code, 0x26, jump) // 索引 1 的表項
		s.Flipped = flipped
		return s
	}

	for _, flipped := range []bool{false, true} {
		s := build(flipped)
		s.run(t, 2)
		if want := uint16(0x0003 + 0x0040); s.IPC != want {
			t.Errorf("flipped=%v：IPC ＝ %04X，該是 %04X", flipped, s.IPC, want)
		}
	}
}
