package psystem

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/wicanr2/Parhelion-PME86/internal/codefile"
	"github.com/wicanr2/Parhelion-PME86/internal/pmachine"
)

// MemSize 是宿主給整個系統的實體記憶體。原版跑在 640 KB 的 DOS 機器上。
const MemSize = 640 * 1024

// 資料段裡幾個固定位置。偏移的來源是原版執行時量出來的版面
// （docs/10-interpreter/machine-state.md 與 docs/30-remake/specs/03-boot.md）。
const (
	stateSysCom   = 0x0140 // 系統通訊區
	stateTIB      = 0x0154 // 開機那個 task 的 TIB
	stateBase     = 0x0170 // 最外層的 BASE：全域資料的框
	dataSeg       = 0x04D1 // 資料段的 paragraph，沿用原版的值好逐條對照
	codeBase      = 0xD800 // 開機那一段程式碼在資料段裡的位置
	sysCom        = 0x00E6 // SYSCOM：單元表與 I/O 狀態
	ramDiskBlocks = 750    // 量原版量到的大小
	// 畫面大小寫在 SYSTEM.MISCINFO 裡（0x4A 是高、0x4C 是寬）。
	screenWidth    = 80
	screenHeight   = 25
	stackTop       = 0xFFFE             // 資料段的上緣，也是 TIB 記的堆疊上界
	dirBase        = stackTop - 4*Block // 磁碟目錄：4 塊
	dictBase       = dirBase - Block    // 作業系統 codefile 的 segment dictionary：1 塊
	dirLastBoot    = 0x14               // 目錄第 0 筆裡「最後一次開機」的日期
	configBase     = 0x5020             // SYSTEM.CONFIG 整份
	configWorkBase = 0x06F0             // 它的前 5 塊，另一份
	configWork     = 5 * Block
	dirCache       = 0x3AF0 // 目錄的第二份，作業系統自己用。位置是量到的
	dirCache2      = 0xBEEE // 第三份，同上
)

// Machine 是一台自己跑得起來的 p-System：平坦記憶體 ＋ p-machine ＋ 磁碟。
type Machine struct {
	Mem  []byte
	S    *pmachine.State
	Vol  *Volume
	Boot *codefile.Segment // 開機載入的那一段

	Steps uint64 // 走過幾條 p-code
	Traps map[uint16]int

	// Units 是 unit 編號對磁碟。哪個編號是什麼，由 SYSTEM.CONFIG 決定。
	Units map[uint16]*Volume

	// Devices 是 SYSTEM.CONFIG 解出來的裝置表。
	Devices []Device

	// Clock 是系統時鐘，1／60 秒為單位，`TIME` 讀的就是它。
	Clock uint32

	// IOLog 記下每一次裝置呼叫。診斷用——I/O 的結果會透過 IORESULT 傳很遠，
	// 出錯的地方與看到症狀的地方通常隔了幾千條指令。
	IOLog []string

	Faults  int     // 發生過幾次 segment fault
	Console []byte  // 寫到主控台的原始位元組
	Screen  *Screen // 同一份位元組渲染出來的畫面
	Keys    []byte  // 還沒被讀走的鍵盤輸入
}

// Word 讀資料段裡的一個 word。診斷用。
func (m *Machine) Word(off uint16) uint16 { return m.word(off) }

// word／setWord 讀寫資料段裡的一個 word。
func (m *Machine) word(off uint16) uint16 {
	return binary.LittleEndian.Uint16(m.S.Data[off:])
}

func (m *Machine) setWord(off, v uint16) {
	binary.LittleEndian.PutUint16(m.S.Data[off:], v)
}

// Options 是開機時可以換掉的東西。
type Options struct {
	// Date 是開機日期，UCSD 的打包格式：月 4 位、日 5 位、年 7 位。
	// bootstrap 會把它蓋進**記憶體裡那份目錄**的第 0 筆 +14h，
	// 磁碟上那份不動。作業系統之後就拿它當「這次開機的日期」。
	Date uint16

	// Clock 是開機那一刻的系統時鐘。
	Clock uint32
}

// PackDate 把年月日打包成 UCSD 的日期 word。年是西元後兩位。
func PackDate(year, month, day int) uint16 {
	return uint16(month&0xF) | uint16(day&0x1F)<<4 | uint16(year&0x7F)<<9
}

// Boot 從一份 .VOL 映像開機，日期用預設值。
func Boot(volume []byte, osFile string) (*Machine, error) {
	return BootWith(volume, osFile, Options{Date: PackDate(85, 1, 1)})
}

// BootWith 從一份 .VOL 映像開機。
//
// 原版的 bootstrap 做的事，這裡照著做一次：把作業系統的起始段從磁碟載進來、
// 在它底下擺好 SIB／E_Rec／E_Vec／第一個活動記錄、填好直譯器的狀態區，
// 然後從那一段的程序 1 開始跑。**做完就不需要原版了。**
func BootWith(volume []byte, osFile string, opt Options) (*Machine, error) {
	v, err := OpenVolume(volume)
	if err != nil {
		return nil, err
	}
	raw, err := v.Read(osFile)
	if err != nil {
		return nil, err
	}
	osFirst := 0
	for _, f := range v.Files {
		if strings.EqualFold(f.Name, osFile) {
			osFirst = f.First
		}
	}
	cf, err := codefile.Parse(raw)
	if err != nil {
		return nil, err
	}

	// 起始段是檔案裡**排在最前面**的那一段（block 1，緊接著 dictionary）。
	// 連結器把作業系統的外層程式擺在這裡，bootstrap 不必查 dictionary 就讀得到。
	var boot *codefile.Segment
	bootIdx := 0
	for i, s := range cf.Segments {
		if boot == nil || s.Block < boot.Block {
			boot, bootIdx = s, i
		}
	}
	if boot == nil {
		return nil, fmt.Errorf("psystem: %s 裡一段都沒有", osFile)
	}

	m := &Machine{
		Mem: make([]byte, MemSize), Vol: v, Boot: boot,
		Traps: map[uint16]int{},
		Units: map[uint16]*Volume{},
		Clock: opt.Clock,
	}
	m.Screen = NewScreen(screenWidth, screenHeight)
	data := m.Mem[dataSeg*16 : dataSeg*16+0x10000]
	m.S = &pmachine.State{Data: data, Env: m}

	code := boot.Raw()
	if codeBase+len(code) > len(data) {
		return nil, fmt.Errorf("psystem: 起始段 %d 位元組，放不進資料段", len(code))
	}
	copy(data[codeBase:], code)

	// 由程式碼底端往下擺：SIB、E_Rec、E_Vec，再下面就是堆疊。
	const sibLen, erecLen, evecLen, mscwLen = 36, 10, 6, 10
	sib := uint16(codeBase - sibLen)
	erec := sib - erecLen
	evec := erec - evecLen
	mp := evec - mscwLen

	m.setWord(sib+0, 0)        // Seg_Base：指標為 0 ＝ 就在資料段裡
	m.setWord(sib+2, codeBase) //           池內偏移
	m.setWord(sib+4, 1)        // Ref_Count
	m.setWord(sib+6, 0)        // Activity
	m.setWord(sib+20, uint16(boot.Words)+uint16(boot.Words&1))
	// +16h 是這一段在**磁碟上**的塊號：codefile 的起點加上段內塊號。
	// 之後要重新載入這一段就靠它（原生程序 39 讀的就是這一格）。
	m.setWord(sib+0x16, uint16(osFirst+boot.Block))
	m.setWord(erec+0, stateBase) // Env_Data
	m.setWord(erec+2, evec)      // E_Vec
	m.setWord(erec+4, sib)       // SIB
	m.setWord(evec+0, 2)         // 這份迷你 E_Vec 只有一格有東西
	m.setWord(evec+4, erec)

	// 最外層的框。MSSTAT 與 MSDYN 都指向全域框自己——外面沒有別人了。
	m.setWord(stateBase+0, stateBase)
	m.setWord(stateBase+2, stateBase)
	m.setWord(mp+0, stateBase) // MSSTAT
	m.setWord(mp+2, stateBase) // MSDYN
	m.setWord(mp+6, erec)      // MSENV
	m.setWord(mp+8, 1)         // MSPROC

	// TIB。SP／MP／IPC／E_Rec 這幾格要與待會兒設的狀態一致，
	// 不然第一次 LPR 或換 task 就會把機器帶回一個不存在的過去。
	m.setWord(stateTIB+2, 0x0080)
	// +4／+6 是這個 task 的堆疊上下界。**目前是量原版量到的值**，
	// 怎麼算出來的還沒解（PLAN.md 的開放項目）。
	m.setWord(stateTIB+4, 0x04A6)
	m.setWord(stateTIB+6, 0xFFFE)
	m.setWord(stateTIB+8, mp)
	m.setWord(stateTIB+0x0a, mp)
	m.setWord(stateTIB+0x10, erec)
	m.setWord(stateTIB+0x12, 1)
	// +18／+1A 也是量到的：一個小整數與指回 BASE 的指標，用途還沒解。
	m.setWord(stateTIB+0x18, 3)
	m.setWord(stateTIB+0x1a, stateBase)
	m.setWord(0x38, stateTIB)
	m.setWord(0x3c, stateTIB)
	for off, v := range bootWords {
		m.setWord(off, v)
	}
	// 開機磁碟的目錄先讀進來，擺在資料段最上面（`0xFFFE` 往下 2048 個位元組），
	// SYSCOM+8 指著它。作業系統一開始就在讀 `dnumfiles`。
	if dir := v.Blocks(2, 4); dir != nil {
		// **目錄有三份。** 一份在資料段最上面，SYSCOM+8 指著它；
		// 另外兩份是作業系統自己用的。三份都要蓋上開機日期。
		for _, at := range []uint16{dirBase, dirCache, dirCache2} {
			copy(data[at:], dir)
			m.setWord(at+dirLastBoot, opt.Date)
		}
		m.setWord(sysCom+8, dirBase)
	}
	// SYSTEM.CONFIG 是這台機器的裝置組態：驅動的名字、每個 unit 的記錄。
	// bootstrap 把它整份讀進 0x5020，前 5 塊另外再放一份在 0x06F0。
	// **這是那 1,982 段差異裡最大的一塊。**
	if cfg, err := v.Read("SYSTEM.CONFIG"); err == nil {
		copy(data[configBase:], cfg)
		n := configWork
		if len(cfg) < n {
			n = len(cfg)
		}
		copy(data[configWorkBase:], cfg[:n])
		m.Devices = ParseConfig(cfg)
	}

	// 哪個 unit 是開機磁碟、哪個是記憶體磁碟，由設定檔決定，不是寫死的。
	if u, ok := FindUnit(m.Devices, driverDOSVol); ok {
		m.Units[u] = v
	}
	if u, ok := FindUnit(m.Devices, driverRAMDisk); ok {
		if rd := NewRAMDisk("RAMDISK", ramDiskBlocks); rd != nil {
			m.Units[u] = rd
		}
	}

	// 作業系統 codefile 的 segment dictionary 原封不動搬一塊進來，
	// 就放在目錄下面一塊。要載別的段時，(Code_Addr, Code_Leng) 就從這裡查。
	if len(raw) >= codefile.BlockSize {
		copy(data[dictBase:], raw[:codefile.BlockSize])
		// 起始段那一格的 `Code_Leng` 會被進位成偶數（3831 → 3832）。
		// 量到的，理由還沒解——但差一個 word 後面的算式就全錯。
		at := dictBase + uint16(4*bootIdx) + 2
		m.setWord(at, uint16(boot.Words)+uint16(boot.Words&1))
	}
	m.setWord(stateSysCom+0x10, dataSeg*16) // 資料段的實體位址
	// 全域變數 1 指向 SYSCOM（系統通訊區）。作業系統一開口就要它。
	m.setWord(stateBase+8, 1)
	m.setWord(stateBase+10, sysCom)

	seg, err := m.ByERec(erec)
	if err != nil {
		return nil, err
	}
	entry, err := m.procEntry(seg, 1)
	if err != nil {
		return nil, err
	}
	m.S.SP = mp
	m.S.TIB = stateTIB
	m.S.Local = mp + 8
	m.S.Proc = 1
	m.S.IPC = entry
	m.setWord(mp+4, entry)        // MSIPC：外層程式返回時回到自己
	m.setWord(stateBase+4, entry) // 全域框記的也是同一個位址
	m.setWord(stateTIB+0x0e, entry)
	m.S.Enter(seg)
	// 目前的程式碼段值（`ss:2Ah`）。8086 上是段暫存器的值，
	// 我們沒有段暫存器，但作業系統讀得到這一格，所以照樣算出來擺著。
	m.setWord(0x24, 0) // 存起來的 IPC：bootstrap 交棒時是 0
	return m, nil
}

// procEntry 是一支程序第一條指令的段內位元組偏移。
func (m *Machine) procEntry(seg *pmachine.Segment, proc uint16) (uint16, error) {
	off := seg.ProcDict - 2*proc
	if int(off)+1 >= len(seg.Code) {
		return 0, fmt.Errorf("psystem: 程序 %d 的字典項落在段外", proc)
	}
	entry := binary.LittleEndian.Uint16(seg.Code[off:])
	if entry == 0 {
		return 0, fmt.Errorf("psystem: 程序 %d 沒有碼", proc)
	}
	return entry*2 + 2, nil
}

// --- pmachine.Environment ---

// ByNumber 用 segment number 查 E_Vec。
func (m *Machine) ByNumber(seg uint16) (*pmachine.Segment, error) {
	evec := m.word(0x3a)
	erec := m.word(evec + 2*seg)
	if erec == 0 {
		return nil, fmt.Errorf("psystem: E_Vec 裡沒有段 %d", seg)
	}
	return m.ByERec(erec)
}

// Globals 只走兩層就到全域資料：E_Vec → E_Rec → Env_Data（@0x15D2）。
// 不查 SIB——跨段讀全域變數不需要那一段的程式碼在記憶體裡。
func (m *Machine) Globals(seg uint16) (uint16, error) {
	erec := m.word(m.word(0x3a) + 2*seg)
	if erec == 0 {
		return 0, fmt.Errorf("psystem: E_Vec 裡沒有段 %d", seg)
	}
	return m.word(erec) + 8, nil
}

// ByERec 把一個 E_Rec 換算成執行期的段（照原版 @0x0FBA 的做法）。
func (m *Machine) ByERec(erec uint16) (*pmachine.Segment, error) {
	sib := m.word(erec + 4)
	if sib == 0 {
		return nil, fmt.Errorf("psystem: E_Rec %04Xh 沒有 SIB", erec)
	}
	ptr, off := m.word(sib), m.word(sib+2)
	if off == 0 {
		return nil, &pmachine.NotResident{ERec: erec}
	}
	pool := uint16(dataSeg)
	if ptr != 0 {
		lo, hi := m.word(ptr), m.word(ptr+2)
		pool = hi>>4 | (lo&0xF)<<12
	}
	para := pool + off>>4
	base := uint32(para) * 16
	if int(base)+22 > len(m.Mem) {
		return nil, fmt.Errorf("psystem: 段落在 %05Xh，超出記憶體", base)
	}
	code := m.Mem[base:]
	if len(code) > 0x10000 {
		code = code[:0x10000]
	}
	return &pmachine.Segment{
		Code:      code,
		Global:    m.word(erec) + 8,
		ProcDict:  binary.LittleEndian.Uint16(code[0:]) * 2,
		ConstPool: binary.LittleEndian.Uint16(code[0x0e:]) * 2,
		ERec:      erec,
		EVec:      m.word(erec + 2),
		SIB:       sib,
		Para:      para,
		Flipped:   binary.LittleEndian.Uint16(code[0x0c:]) != 1,
	}, nil
}

// Intrinsic 回報段 1 的這支程序是不是直譯器內嵌的原生碼。
//
// 清單是原版 @0x1F56 那張 48 格表的非零格，直接讀映像數出來的。
func (m *Machine) Intrinsic(proc uint16) bool {
	return proc < uint16(len(nativeProc)) && nativeProc[proc]
}

// bootWords 是資料段低位址處的初值。
//
// **它是直譯器自己的初始資料。** `SYSTEM.PME.86` 檔案開頭那 512 個位元組
// 會被原封不動搬進資料段的同一個偏移——非零的 37 個 word 逐一對過，
// 一個不差（`TestInterpreterDataComesFromItsOwnHeader`）。
// 那一段在程式碼段裡看不到，因為載入器把 `0x1D56` 起的 dispatch 表
// 搬到映像偏移 0 蓋掉了它；它只在資料段裡活著。
//
// 內容是直譯器對作業系統的自我介紹加上幾個初值：
//
//	0x08        SYSCOM 的位址
//	0x0A/0C/0E  區域基底、全域基底、目前 E_Rec 在狀態區的偏移
//	0x12–0x18   四個原生回呼的進入點
//	0x1A–0x22   其他直譯器碼位址
//	0x44        byte sex 旗標的初值
//	0xE6 起     SYSCOM：裝置數與參數
//	0x140 起    開機參數與最外層那個框
//
// 前面那幾個偏移我們自己也有（就是這個檔案裡的常數）；
// **`0x12` 之後那些是 8086 機器碼的位址，Go 宿主沒有對應的東西**——
// 作業系統只在 `NAT` 呼叫時用得到，而 `NAT` 本來就做不到。
// 照抄是為了讓讀到那幾格的 p-code 走同一條路。
//
// bootstrap 自己算出來的只有幾格：`0x06`（開機參數區的位址）、
// `0x14E`、`0x150`（資料段的實體位址）、`0x152`，以及 TIB 的堆疊上下界。
var bootWords = map[uint16]uint16{
	0x0000: 0x3A04, 0x0004: 0x0C9B,
	0x0008: sysCom, 0x000A: 0x0026, 0x000C: 0x0028, 0x000E: 0x003E,
	0x0010: 0x02FE,
	0x0012: 0x185A, 0x0014: 0x1865, 0x0016: 0x1870, 0x0018: 0x1874,
	0x001A: 0x0306, 0x001C: 0x1281, 0x001E: 0x11BC, 0x0020: 0x11BA,
	0x0022: 0x11AF,
	0x0044: 0x0001, // byte sex：與主機相同

	// SYSCOM。
	0x00EA: 0x000E, 0x00EE: dirBase, 0x0100: 0x0080,
	0x0118: 0x0009, 0x011A: 0x0146, 0x011C: 0x0002, 0x011E: 0x0004,
	0x0132: 0x0050, 0x0138: 0x0603, 0x013A: 0x1300, 0x013C: 0x3F7F,
	0x013E: 0x1B7F,

	// 開機參數與最外層的框。
	0x0140: 0x031B, 0x0142: 0x127F, 0x0146: 0x0006,
	0x0156: 0x0080,                    // TIB+2：優先權
	0x0166: 0x0001,                    // TIB+12h：MSPROC
	0x016C: 0x0003, 0x016E: stateBase, // TIB+18h／+1Ah
	0x0170: stateBase, 0x0172: stateBase, // 全域框的 MSSTAT 與 MSDYN
	0x0178: 0x0001, 0x017A: sysCom, // 全域變數 1 指向 SYSCOM

	// bootstrap 自己算的。
	0x0006: 0x0140, 0x014E: 0x0001, 0x0152: 0x043C,
}

// nativeProc 是 @0x1F56 表裡有實作的 33 格。
var nativeProc = [48]bool{
	4: true, 14: true, 15: true, 16: true,
	18: true, 19: true, 20: true, 21: true, 22: true, 23: true, 24: true,
	25: true, 26: true, 27: true, 28: true, 29: true, 30: true, 31: true,
	32: true, 33: true, 34: true, 36: true, 37: true, 38: true, 39: true,
	40: true, 41: true, 42: true, 43: true, 44: true, 45: true, 46: true, 47: true,
}
