package psystem

// 段 1 的內嵌原生程序。**這是宿主與 p-machine 之間唯一的介面**：
// 作業系統要搬記憶體、碰磁碟、開關中斷，都是從這裡出去的。
//
// 每一支的參數次序都由原版那一支的 pop 順序讀出來，位址記在各自的註解裡。

// intrinsic 執行段 1 的程序 proc。呼叫者已經把參數推在堆疊上。
func (m *Machine) intrinsic(proc uint16) error {
	s := m.S
	switch proc {
	case 15: // MOVELEFT @0x191E：由低位址往高位址搬
		n := int16(s.Pop())
		dst := s.Pop() + s.Pop()
		src := s.Pop() + s.Pop()
		if n > 0 {
			copy(s.Data[dst:dst+uint16(n)], s.Data[src:src+uint16(n)])
		}
	case 16: // MOVERIGHT @0x1970：由高位址往低位址搬（重疊時方向相反）
		n := int16(s.Pop())
		dst := s.Pop() + s.Pop()
		src := s.Pop() + s.Pop()
		for i := int16(n) - 1; i >= 0; i-- {
			s.Data[dst+uint16(i)] = s.Data[src+uint16(i)]
		}
	case 21: // FILLCHAR @0x18DD
		ch := byte(s.Pop())
		n := int16(s.Pop())
		dst := s.Pop() + s.Pop()
		for i := int16(0); i < n; i++ {
			s.Data[dst+uint16(i)] = ch
		}
	case 27, 28: // @0x19F4／@0x1A00：關中斷／開中斷
		// 我們是單執行緒，沒有真的中斷可關。
	default:
		return &Trap{Proc: proc, IPC: s.IPC, Why: nativeName(proc)}
	}
	return nil
}

// nativeName 是目前對那一支的認識。**沒讀過碼的就寫「還沒讀」**——
// 猜一個好聽的名字會讓「還沒查證」看起來像「已經知道」。
func nativeName(proc uint16) string {
	switch proc {
	case 4:
		return "把載入的段照 relocation list 修位址 @0x1B2A"
	case 14:
		return "在兩個池內段之間搬 word @0x1AE4"
	case 22:
		return "SCAN @0x1992"
	case 24:
		return "從池內段搬進資料段 @0x1A6E"
	case 25:
		return "從資料段搬進池內段 @0x1A94"
	case 26:
		return "把載入的段逐 word 交換位元組 @0x1ABA"
	case 29:
		return "ATTACH：把號誌掛到中斷向量 @0x1841"
	case 39, 46:
		return "磁碟／裝置 I/O @0x1BAF"
	case 47:
		return "換一組浮點常式 @0x1A0C"
	}
	return "還沒讀"
}
