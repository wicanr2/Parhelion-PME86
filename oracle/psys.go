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
)

// System 是一台跑著 p-System 的機器。
type System struct {
	M *dosgolem.Machine
	D *dosgolem.DOS

	img     []byte // 磁碟上那份 SYSTEM.PME.86
	base    uint32 // 直譯器映像在記憶體裡的實體位址
	seg     uint16
	targets map[uint16]bool // dispatch 表裡出現過的常式位址
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
	return base, nil
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

// PCode 是軌跡裡的一條。
type PCode struct {
	Seg, IPC uint16 // p-code 所在的 code segment 與段內位元組偏移
	Op       uint8  // 剛開始執行的 opcode
	SP, TOS  uint16 // 求值堆疊頂的位置與內容
}

func (p PCode) String() string {
	return fmt.Sprintf("%04X:%04X %02X sp=%04X tos=%04X", p.Seg, p.IPC, p.Op, p.SP, p.TOS)
}

// Trace 記錄原版直譯器實際執行的 p-code，最多 want 條，最多花 budget 條機器指令。
//
// 判準是「控制權剛落到某個 dispatch 目標」。載入器把 dispatch 表搬到映像偏移 0，
// 所以表項就是 cs 相對的常式位址；進到常式時 lodsb 已經走過，
// 剛執行的 opcode 因此在 ds:si−1，而 si 就是 IPC。
func (s *System) Trace(want int, budget uint64) ([]PCode, error) {
	if s.targets == nil {
		return nil, fmt.Errorf("oracle: 還沒定位 PME，先呼叫 LocatePME")
	}
	out := make([]PCode, 0, want)
	limit := s.M.Steps + budget
	for len(out) < want && s.M.Steps < limit && !s.D.Exited {
		if err := s.M.Step(); err != nil {
			return out, err
		}
		c := s.M.CPU
		if c.Seg[dosgolem.CS] != s.seg || !s.targets[c.IP] {
			continue
		}
		ds, si, sp := c.Seg[dosgolem.DS], c.R[dosgolem.SI], c.R[dosgolem.SP]
		out = append(out, PCode{
			Seg: ds, IPC: si - 1,
			Op:  s.M.Read8(uint32(ds)*16 + uint32(si-1)),
			SP:  sp,
			TOS: s.M.Read16(uint32(c.Seg[dosgolem.SS])*16 + uint32(sp)),
		})
	}
	return out, nil
}
