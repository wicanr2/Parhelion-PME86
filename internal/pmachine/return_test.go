package pmachine

import (
	"errors"
	"testing"
)

// 這一份釘的是「返回」的三條分支：正常返回、EXIT 拆框（走 EXITIC）、
// 返回原生碼。三者只差 MSCW 裡兩個值的符號與零，錯了不會當場報錯——
// 只會跳到別的地方去執行，然後在很遠的後面才看起來像別的 bug。

// padded 把 Code 補到 256 個位元組，好讓測試在裡面擺字典與表。
func padded(s *State) *State {
	if len(s.Code) < 0x100 {
		s.Code = append(s.Code, make([]byte, 0x100-len(s.Code))...)
	}
	return s
}

// negProc 是「這一格正在被 EXIT 拆掉」的 MSPROC 寫法：程序號取負。
func negProc(n uint16) uint16 { return -n }

// frame 在 sp 底下擺一個活動記錄，回傳擺好之後的 State。
//
// 版面是 MSSTAT 0、MSDYN 2、MSIPC 4、MSENV 6、MSPROC 8（Figure 5），
// 而 MP 就指著 MSSTAT。
func frame(s *State, mp, ipc, env, proc uint16) {
	s.Store(mp+0, 0x0000) // MSSTAT
	s.Store(mp+2, mp+0x20)
	s.Store(mp+4, ipc)
	s.Store(mp+6, env)
	s.Store(mp+8, proc)
	s.Local = mp + 8
	s.ERec = env
}

func TestReturnGoesBackToTheCaller(t *testing.T) {
	s := padded(newState(0x96, 0x00)) // RPU 0
	frame(s, 0x0800, 0x1234, 0x0042, 7)
	s.run(t, 1)

	if s.IPC != 0x1234 {
		t.Errorf("IPC ＝ %04X，該是 MSIPC 1234", s.IPC)
	}
	if s.Proc != 7 {
		t.Errorf("MSPROC ＝ %d，該還原成 7", s.Proc)
	}
	if s.Local != 0x0820+8 {
		t.Errorf("MP ＝ %04X，該跟著 MSDYN 走", s.Local-8)
	}
}

// EXIT 把要拆掉的那一格的 MSPROC 變成負數。返回時就不回 MSIPC，
// 而是跳到那支程序自己的離場碼（@0x1160 的 `js`）。
func TestExitedFrameJumpsToTheExitCode(t *testing.T) {
	s := padded(newState(0x96, 0x00))
	// 程序字典：字典基底在 0x40，程序 3 的字典項在 0x40−6 ＝ 0x3A。
	// 字典項是 DATASIZE 的 word 偏移，EXITIC 在它前面一個 word。
	s.ProcDict = 0x40
	putWord(s.Code, 0x3A, 0x0030) // 程序 3 → DATASIZE 在 word 0x30
	putWord(s.Code, 0x5E, 0x2000) // word 0x2F ＝ byte 0x5E：EXITIC
	frame(s, 0x0800, 0x1000, 0x0042, negProc(3))
	s.run(t, 1)

	if s.IPC != 0x2000 {
		t.Errorf("IPC ＝ %04X，該跳到 EXITIC 2000", s.IPC)
	}
	if s.Proc != 3 {
		t.Errorf("MSPROC ＝ %d，該把負號去掉變 3", s.Proc)
	}
}

// 兩個位址取比較後面的那個（@0x118E）：已經走過離場碼就不要再跳回去。
func TestExitKeepsTheLaterAddress(t *testing.T) {
	s := padded(newState(0x96, 0x00))
	s.ProcDict = 0x40
	putWord(s.Code, 0x3A, 0x0030)
	putWord(s.Code, 0x5E, 0x0100) // EXITIC 比返回點前面
	frame(s, 0x0800, 0x0200, 0x0042, negProc(3))
	s.run(t, 1)

	if s.IPC != 0x0200 {
		t.Errorf("IPC ＝ %04X，返回點已經在離場碼後面，該留著 0200", s.IPC)
	}
}

// MSIPC 為 0 表示呼叫這一格的是原生碼。p-machine 執行不了那個，
// 但它與「指令沒實作」是兩件事，要分得出來。
func TestReturnToNativeCodeSaysSo(t *testing.T) {
	s := padded(newState(0x96, 0x00))
	frame(s, 0x0800, 0x0000, 0x0042, 7)
	if _, err := s.Step(); err == nil {
		t.Fatal("MSIPC 是 0 卻沒有回報要返回原生碼")
	} else {
		var nc *NativeCall
		if !errors.As(err, &nc) {
			t.Fatalf("錯誤型別是 %T，該是 *NativeCall", err)
		}
	}
}
