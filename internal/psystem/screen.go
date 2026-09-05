package psystem

import "strings"

// Screen 是一塊字元畫面，把作業系統送出來的位元組串渲染成看得懂的樣子。
//
// 控制碼不是猜的：`SYSTEM.MISCINFO` 第 `0x3E` 個位元組起就是這台終端機的
// 控制字元表，第一個是 lead-in（`1B`／ESC），後面依序是各個動作的字元。
// 畫面大小也在那個檔裡（`0x4A` 是高、`0x4C` 是寬）。
//
// **認不得的序列會被記下來**（`Unknown`），不會靜靜吃掉——
// 安靜地忽略控制碼，畫面會慢慢歪掉而且沒有人知道為什麼。
type Screen struct {
	W, H     int
	cells    []byte
	row, col int
	esc      []byte // 正在收的控制序列；空的表示不在序列裡
	Unknown  map[string]int
	ctl      terminal
}

// terminal 是這台終端機的控制字元。名字沿用 UCSD SETUP 的說法。
type terminal struct {
	leadIn                   byte
	home, eraseEOS, eraseEOL byte
	right, up, left, down    byte
	eraseScreen, clearLine   byte
	moveCursor               byte // 絕對定位：lead-in ＋ 這個字元 ＋ 列 ＋ 行（各加 32）
}

// defaultTerminal 是 `SYSTEM.MISCINFO` 裡那一組。字元從檔案讀出來，
// 但**哪個欄位是哪個動作是推的**——手冊沒有給 IV.2.1 的欄位順序。
// 推的依據是實際送出來的序列：`ESC H` 之後游標回左上、`ESC E` 之後畫面清空。
var defaultTerminal = terminal{
	leadIn: 0x1B, home: 'H', eraseEOS: 'J', eraseEOL: 'K',
	right: 'C', up: 'A', left: 0x08, down: 'B',
	eraseScreen: 'E', clearLine: 'L', moveCursor: 'Y',
}

// NewScreen 造一塊空白畫面。
func NewScreen(w, h int) *Screen {
	s := &Screen{W: w, H: h, ctl: defaultTerminal, Unknown: map[string]int{}}
	s.cells = make([]byte, w*h)
	s.Clear()
	return s
}

// Clear 把畫面清成空白，游標回左上。
func (s *Screen) Clear() {
	for i := range s.cells {
		s.cells[i] = ' '
	}
	s.row, s.col = 0, 0
}

// Write 把一串位元組送進畫面。
func (s *Screen) Write(p []byte) {
	for _, c := range p {
		s.put(c)
	}
}

func (s *Screen) put(c byte) {
	// 絕對定位要吃兩個參數，先收滿再動作。
	if len(s.esc) == 2 && s.esc[1] == s.ctl.moveCursor {
		s.esc = append(s.esc, c)
		return
	}
	if len(s.esc) == 3 {
		s.row = int(s.esc[2]) - 32
		s.col = int(c) - 32
		s.clamp()
		s.esc = nil
		return
	}
	if len(s.esc) == 1 {
		s.control(c)
		return
	}
	switch {
	case c == s.ctl.leadIn:
		s.esc = []byte{c}
	case c == '\r':
		// **CR 要連著換行。** p-System 的行尾只送 CR，補 LF 是驅動的事
		// （手冊 p.85：textfile 行尾是 CR，之後要補送 LF）。
		s.col = 0
		s.newline()
	case c == '\n':
		s.newline()
	case c == s.ctl.left:
		if s.col > 0 {
			s.col--
		}
	case c == 0x07: // 響鈴，畫面上不留東西
	case c < 0x20:
		s.Unknown[string([]byte{c})]++
	default:
		s.cells[s.row*s.W+s.col] = c
		if s.col++; s.col >= s.W {
			s.col = 0
			s.newline()
		}
	}
}

func (s *Screen) control(c byte) {
	defer func() {
		if len(s.esc) < 2 {
			s.esc = nil
		}
	}()
	switch c {
	case s.ctl.moveCursor:
		s.esc = append(s.esc, c) // 還要兩個參數
	case s.ctl.home:
		s.row, s.col = 0, 0
	case s.ctl.eraseScreen:
		s.Clear()
	case s.ctl.eraseEOL, s.ctl.clearLine:
		for i := s.col; i < s.W; i++ {
			s.cells[s.row*s.W+i] = ' '
		}
	case s.ctl.eraseEOS:
		for i := s.row*s.W + s.col; i < len(s.cells); i++ {
			s.cells[i] = ' '
		}
	case s.ctl.up:
		if s.row > 0 {
			s.row--
		}
	case s.ctl.down:
		s.newline()
	case s.ctl.right:
		if s.col < s.W-1 {
			s.col++
		}
	default:
		s.Unknown[string([]byte{s.ctl.leadIn, c})]++
	}
}

func (s *Screen) newline() {
	if s.row++; s.row >= s.H {
		copy(s.cells, s.cells[s.W:])
		for i := (s.H - 1) * s.W; i < len(s.cells); i++ {
			s.cells[i] = ' '
		}
		s.row = s.H - 1
	}
}

func (s *Screen) clamp() {
	if s.row < 0 {
		s.row = 0
	}
	if s.row >= s.H {
		s.row = s.H - 1
	}
	if s.col < 0 {
		s.col = 0
	}
	if s.col >= s.W {
		s.col = s.W - 1
	}
}

// Lines 是畫面每一列的內容，右邊的空白已經切掉。
func (s *Screen) Lines() []string {
	out := make([]string, s.H)
	for r := 0; r < s.H; r++ {
		out[r] = strings.TrimRight(string(s.cells[r*s.W:(r+1)*s.W]), " ")
	}
	return out
}

// String 把整塊畫面接成一段文字。
func (s *Screen) String() string { return strings.Join(s.Lines(), "\n") }

// Cursor 是游標現在的位置。
func (s *Screen) Cursor() (row, col int) { return s.row, s.col }
