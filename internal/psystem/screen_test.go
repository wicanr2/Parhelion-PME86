package psystem

import "testing"

// 控制碼認錯不會報錯，只會讓畫面慢慢歪掉——所以每一條都要釘住。

func TestScreenPlacesTextWhereItIsTold(t *testing.T) {
	s := NewScreen(20, 5)
	// ESC Y 的兩個參數是列與行，各加 32。
	s.Write([]byte("\x1bY\x22\x23hi"))
	if got := s.Lines()[2]; got != "   hi" {
		t.Errorf("第 2 列是 %q", got)
	}
	if r, c := s.Cursor(); r != 2 || c != 5 {
		t.Errorf("游標在 (%d,%d)", r, c)
	}
}

// CR 要連著換行——p-System 的行尾只送 CR，補 LF 是驅動的事（手冊 p.85）。
func TestCarriageReturnAlsoMovesDown(t *testing.T) {
	s := NewScreen(20, 5)
	s.Write([]byte("one\rtwo"))
	if got := s.Lines(); got[0] != "one" || got[1] != "two" {
		t.Errorf("換行之後是 %q／%q", got[0], got[1])
	}
}

func TestEraseCommands(t *testing.T) {
	s := NewScreen(10, 3)
	s.Write([]byte("abcdefghij\rklmnop"))
	s.Write([]byte("\x1bY\x20\x23")) // 回到 (0,3)
	s.Write([]byte("\x1bK"))         // 清到行尾
	if got := s.Lines()[0]; got != "abc" {
		t.Errorf("清到行尾之後第 0 列是 %q", got)
	}
	s.Write([]byte("\x1bJ")) // 清到畫面尾
	if got := s.Lines()[1]; got != "" {
		t.Errorf("清到畫面尾之後第 1 列是 %q", got)
	}
	s.Write([]byte("\x1bE")) // 整面清掉，游標回左上
	if r, c := s.Cursor(); r != 0 || c != 0 {
		t.Errorf("清畫面之後游標在 (%d,%d)", r, c)
	}
}

// 寫到最後一列還要往下，就整面往上捲。
func TestScrollAtTheBottom(t *testing.T) {
	s := NewScreen(4, 2)
	s.Write([]byte("aa\rbb\rcc"))
	if got := s.Lines(); got[0] != "bb" || got[1] != "cc" {
		t.Errorf("捲動之後是 %q／%q", got[0], got[1])
	}
}

// **認不得的序列要留下紀錄。** 靜靜吃掉的話，畫面歪了也查不出是哪一個。
func TestUnknownSequencesAreCounted(t *testing.T) {
	s := NewScreen(10, 2)
	s.Write([]byte("\x1bZ\x1bZ\x01"))
	if s.Unknown["\x1bZ"] != 2 {
		t.Errorf("ESC Z 記了 %d 次", s.Unknown["\x1bZ"])
	}
	if s.Unknown["\x01"] != 1 {
		t.Errorf("控制字元 01 記了 %d 次", s.Unknown["\x01"])
	}
}
