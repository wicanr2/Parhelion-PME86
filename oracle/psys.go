//go:build oracle

// Package oracle 把 1984 年 DOS 版的 UCSD p-System 跑起來，當作 remake 的對照。
//
// 底下是 [dosgolem]——同一個作者的無頭決定性 DOS 執行器。dosgolem 只提供
// 「跑得動 DOS 程式、問得到記憶體」這種通用能力；**認得 p-machine 的知識全部在這裡**，
// 不往下滲。
//
// 這個 package 要 build tag `oracle`，因為它依賴一份本機的 dosgolem 副本。
// 沒有那份副本時 `go test ./...` 會跳過它，不會失敗。接法見 tools/go.sh。
//
// 原版素材（PSYSTEM.COM、.VOL 磁碟映像、抽出來的 SYSTEM.PME.86）由使用者自備，
// 不在這個 repo 裡。
//
// [dosgolem]: https://github.com/wicanr2/dosgolem
package oracle

import (
	"fmt"
	"os"
	"strings"

	"github.com/wicanr2/Parhelion-PME86/internal/pcode"
	"github.com/wicanr2/dosgolem"
)

// 直譯器映像裡兩個位置固定的東西。兩個都寫在
// docs/10-interpreter/dispatch-and-threading.md，這裡只是把它們變成可執行的斷言。
const (
	// MaskTableOff 是遮罩表的檔案偏移，第 n 項是 (1<<n)−1。
	// 純常數，載入時不會被改寫——**拿來當指紋最可靠**。
	// 檔頭那 18 個 word 不能當指紋，載入器會把 dispatch 表蓋上去。
	MaskTableOff = 0x1fb6

	// DispatchOff 是 dispatch 表在檔案裡的偏移。
	DispatchOff = 0x1d56

	// DispatchBytes 是 dispatch 表的長度：256 項，每項一個 word。
	DispatchBytes = 512

	// ErrorEntry 是直譯器的執行期錯誤共同入口（映像偏移）。
	// 每一支發錯誤的常式都是 `mov bp, 錯誤碼` 之後跳到這裡，
	// 所以停在這裡的時候 bp 就是錯誤碼。
	ErrorEntry = 0x020f
)

// dispatchSite 是分派那一跳的機器碼：`2E FF 25` ＝ `jmp word ptr cs:[di]`。
//
// 追蹤的判準用它，不用「IP 落在某個 dispatch 目標」。後者會多算：
// 有些常式做完事直接跳進另一支常式的入口（`SBR` 落進 `ADR` 就是），
// 那一跳不是取指令，卻長得一模一樣。
var dispatchSite = [3]uint8{0x2E, 0xFF, 0x25}

// System 是一台跑著 p-System 的機器。
type System struct {
	M *dosgolem.Machine
	D *dosgolem.DOS

	img      []byte // 磁碟上那份 SYSTEM.PME.86
	base     uint32 // 直譯器映像在記憶體裡的實體位址
	seg      uint16
	targets  map[uint16]bool // dispatch 表裡出現過的常式位址
	errBreak int             // 執行期錯誤入口的中斷點
}

// RuntimeFault 是原版自己跑進了執行期錯誤。
//
// **這與「我們對不上」是兩件事。** 拿一台已經出錯的機器當對照，
// 之後每一條都會對不上，而症狀看起來像我們的實作有問題。
type RuntimeFault struct {
	Code uint16 // 直譯器放在 bp 裡的錯誤碼
	Seg  uint16 // 出錯時在哪一段的哪裡
	IPC  uint16
}

func (e *RuntimeFault) Error() string {
	return fmt.Sprintf("oracle: 原版在 %04X:%04X 發了執行期錯誤 %d", e.Seg, e.IPC, e.Code)
}

// Boot 載入 PSYSTEM.COM 並掛好 DOS 服務層。**還沒開始跑**——要呼叫 Run。
//
// root 是 .VOL 磁碟映像所在的目錄。
func Boot(com, root string) (*System, error) {
	data, err := os.ReadFile(com)
	if err != nil {
		return nil, err
	}
	m := dosgolem.New()
	if err := m.LoadCOM(data); err != nil {
		return nil, err
	}
	d := dosgolem.NewDOS(m, root)
	d.Dir = "" // .VOL 就在 root 底下，沒有子目錄
	d.Install()
	return &System{M: m, D: d}, nil
}

// Run 最多跑 steps 條指令；程式自己結束就提前收工。
func (s *System) Run(steps uint64) error {
	limit := s.M.Steps + steps
	for s.M.Steps < limit && !s.D.Exited {
		if err := s.M.Step(); err != nil {
			return err
		}
	}
	return nil
}

// Type 把一串字元排進輸入。掃描碼走 IRQ1、ASCII 走 int 16h，兩條都排——
// 還沒確定 p-System 走哪一條。
func (s *System) Type(text string) error {
	s.D.TypeKeys(text)
	return s.M.TypeScan(text)
}

// Screen 回文字畫面的 25 列。
func (s *System) Screen() []string { return s.M.TextScreen(0) }

// ScreenHas 回報整個文字畫面裡有沒有這段字。
func (s *System) ScreenHas(want string) bool {
	for _, l := range s.Screen() {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// LocatePME 在記憶體裡找 p-machine 直譯器載到哪，並記下 dispatch 表的目標集合。
//
// pmePath 是從 .VOL 抽出來的 SYSTEM.PME.86。
func (s *System) LocatePME(pmePath string) (uint32, error) {
	img, err := os.ReadFile(pmePath)
	if err != nil {
		return 0, err
	}
	if len(img) < MaskTableOff+34 {
		return 0, fmt.Errorf("oracle: PME 映像只有 %d bytes，放不下遮罩表", len(img))
	}
	hits := s.M.Find(img[MaskTableOff : MaskTableOff+34])
	if len(hits) == 0 {
		return 0, fmt.Errorf("oracle: 記憶體裡找不到 PME 的遮罩表")
	}
	if len(hits) > 1 {
		return 0, fmt.Errorf("oracle: 遮罩表在記憶體裡出現 %d 次，指紋不夠獨特", len(hits))
	}
	base := hits[0] - MaskTableOff
	if base%16 != 0 {
		return 0, fmt.Errorf("oracle: PME 基底 %05Xh 不是段對齊", base)
	}
	s.img, s.base, s.seg = img, base, uint16(base>>4)
	s.targets = make(map[uint16]bool, 256)
	for op := 0; op < 256; op++ {
		s.targets[s.M.Read16(base+uint32(op)*2)] = true
	}
	// 原版跑進執行期錯誤時要**當場知道**。不設這個中斷點的話它會繼續跑，
	// 而我們會拿一台已經出錯的機器當對照——對不上的地方看起來像我們的問題。
	if s.errBreak != 0 {
		s.M.ClearBreak(s.errBreak)
	}
	s.errBreak = s.M.BreakAt(s.seg, ErrorEntry)
	return base, nil
}

// WaitForPME 一小段一小段地跑，直到直譯器出現在記憶體裡為止。
//
// 直譯器是開機途中從 .VOL 搬進來的，不知道確切在第幾條指令。
// 一路跑到底再定位也可以，但那時系統已經在等鍵盤，**開機期間執行的
// p-code 全部錯過了**——而那正是最容易拿來對拍的一段。
func (s *System) WaitForPME(pmePath string, maxSteps, chunk uint64) (uint32, error) {
	if chunk == 0 {
		chunk = 250_000
	}
	for s.M.Steps < maxSteps && !s.D.Exited {
		if base, err := s.LocatePME(pmePath); err == nil {
			return base, nil
		}
		if err := s.Run(chunk); err != nil {
			return 0, err
		}
	}
	return s.LocatePME(pmePath)
}

// PME 回傳直譯器的映像基底與段值。要先呼叫 LocatePME。
func (s *System) PME() (base uint32, seg uint16, ok bool) {
	return s.base, s.seg, s.targets != nil
}

// DispatchMoved 回報「載入器把 dispatch 表搬到映像偏移 0」成立到什麼程度：
// 映像前 512 個位元組裡有幾個與磁碟上 DispatchOff 起的內容相同。
//
// 這是「`jmp word ptr cs:[di]` 為什麼不必帶位移」的關鍵證據，
// 所以做成可以重跑的量測，不是一次性的觀察。
func (s *System) DispatchMoved() (same, total int, err error) {
	if s.targets == nil {
		return 0, DispatchBytes, fmt.Errorf("oracle: 還沒定位 PME")
	}
	for i := 0; i < DispatchBytes; i++ {
		if s.M.Mem[s.base+uint32(i)] == s.img[DispatchOff+i] {
			same++
		}
	}
	return same, DispatchBytes, nil
}

// ImageMatches 回報記憶體裡的映像有幾個位元組與磁碟上的一致。
// 差異應該只落在被 dispatch 表蓋掉的前 512 個位元組，以及直譯器的工作區。
func (s *System) ImageMatches() (same, total int) {
	if s.targets == nil {
		return 0, 0
	}
	for i, b := range s.img {
		if s.M.Mem[s.base+uint32(i)] == b {
			same++
		}
	}
	return same, len(s.img)
}

// CodeSegment 讀一個載入中的 code segment 的前 n 個位元組。
//
// 軌跡裡的 Seg 就是段值。拿它回來與 codefile 讀取器解出來的東西對拍，
// 才知道「我們解出來的結構」與「原版實際在跑的結構」是不是同一件事。
func (s *System) CodeSegment(seg uint16, n int) []byte {
	base := uint32(seg) * 16
	if n <= 0 || base+uint32(n) > dosgolem.MemSize {
		return nil
	}
	out := make([]byte, n)
	copy(out, s.M.Mem[base:base+uint32(n)])
	return out
}

// SegmentName 讀一個載入中的 code segment 表頭裡的 8 字元段名。
// 段頭版面見 docs/30-remake/specs/01-codefile.md §3.1。
func (s *System) SegmentName(seg uint16) string {
	b := s.CodeSegment(seg, 12)
	if b == nil {
		return ""
	}
	return strings.Trim(string(b[4:12]), " \x00")
}

// Regs 是原版此刻幾個關鍵暫存器與直譯器狀態變數的快照，給探路用。
type Regs struct {
	CS, DS, SS, SP, SI, BX, DX uint16

	ERec      uint16 // ss:3Eh
	EVec      uint16 // ss:3Ah
	SIB       uint16 // ss:34h
	EnvData   uint16 // ss:30h
	ProcDict  uint16 // ss:36h
	ConstPool uint16 // ss:42h
	CodeSeg   uint16 // ss:2Ah
	Proc      uint16 // ss:32h
}

// Regs 讀那份快照。狀態變數的偏移見 docs/10-interpreter/machine-state.md。
func (s *System) Regs() Regs {
	c := s.M.CPU
	ss := c.Seg[dosgolem.SS]
	w := func(off uint16) uint16 { return s.M.Read16(uint32(ss)*16 + uint32(off)) }
	return Regs{
		CS: c.Seg[dosgolem.CS], DS: c.Seg[dosgolem.DS], SS: ss,
		SP: c.R[dosgolem.SP], SI: c.R[dosgolem.SI],
		BX: c.R[dosgolem.BX], DX: c.R[dosgolem.DX],
		ERec: w(0x3e), EVec: w(0x3a), SIB: w(0x34), EnvData: w(0x30),
		ProcDict: w(0x36), ConstPool: w(0x42), CodeSeg: w(0x2a), Proc: w(0x32),
	}
}

// FindName 在記憶體裡找「表頭段名是這個」的 code segment，回段值。
//
// 段頭第 4–11 個位元組是段名（spec 01 §3.1），而段一定從段邊界開始，
// 所以命中的位址減 4 要是 16 的倍數。
func (s *System) FindName(name string) []uint16 {
	if len(name) == 0 {
		return nil
	}
	padded := name
	for len(padded) < 8 {
		padded += " "
	}
	var out []uint16
	for _, a := range s.M.Find([]byte(padded)) {
		if a >= 4 && (a-4)%16 == 0 {
			out = append(out, uint16((a-4)/16))
		}
	}
	return out
}

// DataWord 讀資料段（ss）的一個 word。
func (s *System) DataWord(off uint16) uint16 {
	return s.M.Read16(uint32(s.M.CPU.Seg[dosgolem.SS])*16 + uint32(off))
}

// PCode 是軌跡裡的一條。
type PCode struct {
	Seg, IPC uint16 // p-code 所在的 code segment 與段內位元組偏移
	Op       uint8  // 剛開始執行的 opcode
	SP, TOS  uint16 // 求值堆疊頂的位置與內容
	ERec     uint16 // 目前的 E_Rec（`ss:3Eh`）——在哪一段執行
}

// Mnemonic 是這個 opcode 在 IV.0 官方表裡的助記符；沒有對應指令就是空字串。
func (p PCode) Mnemonic() string { return pcode.Mnemonic(p.Op) }

func (p PCode) String() string {
	name := p.Mnemonic()
	if name == "" {
		name = "?"
	}
	return fmt.Sprintf("%04X:%04X %02X %-6s sp=%04X tos=%04X",
		p.Seg, p.IPC, p.Op, name, p.SP, p.TOS)
}

// atDispatchSite 回報這個位址上是不是 `jmp word ptr cs:[di]`。
func (s *System) atDispatchSite(seg, ip uint16) bool {
	a := uint32(seg)*16 + uint32(ip)
	for i, b := range dispatchSite {
		if s.M.Read8(a+uint32(i)) != b {
			return false
		}
	}
	return true
}

// Trace 記錄原版直譯器實際執行的 p-code，最多 want 條，最多花 budget 條機器指令。
//
// 判準是「剛剛執行的那一條是 `jmp word ptr cs:[di]`，而且落點是一個 dispatch 目標」。
// 只看落點會多算——有些常式做完事直接跳進另一支常式的入口，那一跳不是取指令。
//
// 進到常式時 lodsb 已經走過，所以剛取到的 opcode 在 ds:si−1，而 si 就是 IPC。
func (s *System) Trace(want int, budget uint64) ([]PCode, error) {
	if s.targets == nil {
		return nil, fmt.Errorf("oracle: 還沒定位 PME，先呼叫 LocatePME")
	}
	out := make([]PCode, 0, want)
	// 判準：剛執行完的那一條是分派那一跳（`2E FF 25`），而且落點是 dispatch 目標。
	// 只看落點會多算——有些常式做完事直接跳進另一支常式的入口。
	//
	// `Insn()` 回的是剛執行完那一條的起點，所以這個條件在 RunUntil 的
	// 檢查時機（每一條之前、跳過第一條）剛好成立。
	atDispatch := func(m *dosgolem.Machine) bool {
		cs, ip := m.Insn()
		if cs != s.seg || !s.atDispatchSite(cs, ip) {
			return false
		}
		return m.CPU.Seg[dosgolem.CS] == s.seg && s.targets[m.CPU.IP]
	}

	limit := s.M.Steps + budget
	for len(out) < want && s.M.Steps < limit && !s.D.Exited {
		why, err := s.M.RunUntil(atDispatch, limit-s.M.Steps)
		if err != nil {
			return out, err
		}
		switch why {
		case dosgolem.StopBreakpoint:
			return out, &RuntimeFault{
				Code: s.M.CPU.R[dosgolem.BP],
				Seg:  s.M.CPU.Seg[dosgolem.DS],
				IPC:  s.M.CPU.R[dosgolem.SI] - 1,
			}
		case dosgolem.StopBudget:
			return out, nil
		}
		c := s.M.CPU
		ds, si, sp := c.Seg[dosgolem.DS], c.R[dosgolem.SI], c.R[dosgolem.SP]
		out = append(out, PCode{
			Seg: ds, IPC: si - 1,
			Op:   s.M.Read8(uint32(ds)*16 + uint32(si-1)),
			SP:   sp,
			TOS:  s.M.Read16(uint32(c.Seg[dosgolem.SS])*16 + uint32(sp)),
			ERec: s.M.Read16(uint32(c.Seg[dosgolem.SS])*16 + erecOff),
		})
	}
	return out, nil
}
