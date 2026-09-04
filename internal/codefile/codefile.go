// Package codefile 讀 UCSD p-System IV.x 的 codefile 靜態結構。
//
// 涵蓋範圍與每一條斷言的出處寫在 docs/30-remake/specs/01-codefile.md（狀態 READY）。
// 這裡只做「讀得出來」的事：segment dictionary 鏈、code segment 表頭、
// routine dictionary。常數池內容、relocation list、segment reference list
// 與任何執行語意都不在這一層。
//
// byte sex 不在原地翻轉：表頭與 routine dictionary 在解析時依旗標解碼，
// 碼與常數池保持原始位元組（spec 4.5）。
package codefile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// BlockSize 是 codefile 的分塊大小（spec 1.1）。
const BlockSize = 512

// dictEntries 是一筆 segment dictionary 記錄能描述的 segment 數（spec 2.1）。
const dictEntries = 16

// segment dictionary 記錄裡各陣列的起始位元組（spec 2.2）。
const (
	offDiskInfo = 0x000
	offSegName  = 0x040
	offSegMisc  = 0x0c0
	offSegInfo  = 0x100
	offSegFamly = 0x120
	offNextDict = 0x1a0
	offCopyNote = 0x1b0
	offSex      = 0x1fe
)

// code segment 表頭的欄位位移，單位是位元組（spec 3.1）。
const (
	hdrRoutineDict = 0x00
	hdrRelocList   = 0x02
	hdrName        = 0x04
	hdrByteSex     = 0x0c
	hdrConstPool   = 0x0e
	hdrRealSize    = 0x10
	hdrReserved    = 0x12
	hdrWords       = 11
)

// Kind 是 segment 的種類（spec 2.5）。
type Kind uint8

const (
	NoSeg Kind = iota
	ProgSeg
	UnitSeg
	ProcSeg
	SeprtSeg
)

func (k Kind) String() string {
	switch k {
	case NoSeg:
		return "No_Seg"
	case ProgSeg:
		return "Prog_Seg"
	case UnitSeg:
		return "Unit_Seg"
	case ProcSeg:
		return "Proc_Seg"
	case SeprtSeg:
		return "Seprt_Seg"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Machine 是 segment 內物件碼的目標機器（spec 2.6）。M_Psuedo 表示 p-code。
type Machine uint8

var machineNames = [...]string{
	"M_Psuedo", "M_6809", "M_PDP_11", "M_8080", "M_Z_80", "M_GA_440",
	"M_6502", "M_6800", "M_9900", "M_8086", "M_Z8000", "M_68000",
}

func (m Machine) String() string {
	if int(m) < len(machineNames) {
		return machineNames[m]
	}
	return fmt.Sprintf("Machine(%d)", uint8(m))
}

// Version 是段被標記的 p-machine 版本（spec 2.6）。
type Version uint8

var versionNames = [...]string{"Unknown", "II", "II.1", "III", "IV", "V", "VI", "VII"}

func (v Version) String() string {
	if int(v) < len(versionNames) {
		return versionNames[v]
	}
	return fmt.Sprintf("Version(%d)", uint8(v))
}

// Routine 是 routine dictionary 裡的一項。
//
// 段內一支常式的版面是 EXITIC、DATASIZE、第一條指令三者相鄰，
// 而字典項指的是中間的 DATASIZE（spec 5.4）。
type Routine struct {
	Number     int  // 1..255，就是字典索引
	HeaderWord int  // 字典項的值：DATASIZE 的段內 word 偏移；0 表示沒有碼
	CodeWord   int  // 第一條指令的段內 word 偏移，等於 HeaderWord+1
	DataSize   int  // 區域資料 word 數，不含參數
	ExitIC     int  // 離開時要執行的碼，段內位元組偏移；Native 時未定義
	Native     bool // 第一條指令是原生碼（DATASIZE 以 one's complement 存放）
}

// Present 回報這一項有沒有碼。
func (r Routine) Present() bool { return r.HeaderWord != 0 }

// Segment 是一個 code segment，含它在 dictionary 裡的描述與段頭解出來的內容。
type Segment struct {
	Name    string // dictionary 記的段名，已去掉尾端空白
	Number  int    // 本地 segment number
	Kind    Kind
	Machine Machine
	Version Version
	Family  string // Proc_Seg／Seprt_Seg 的外層 program／unit 名；其餘為空
	Misc    uint16 // Seg_Misc 原值；bit 8、bit 9 的語意未驗證，不要拿來判斷（spec 2.5）

	Block int // 起始 block
	Words int // Code_Leng，含 relocation list、不含 segment reference list

	Flipped    bool      // byte sex 與主機相反
	HeaderName string    // 段頭裡的段名，正常情況與 Name 相同
	RealSize   int       // $R2 或 $R4
	ConstPool  int       // 常數池的段內 word 偏移；0 表示沒有
	RelocList  int       // relocation list 的段內 word 偏移
	Reserved   [2]uint16 // 段頭保留的兩個 word；實測非零，用途未知

	Routines []Routine

	raw []byte // 這一段的原始位元組，長度 Words*2
}

// Raw 回傳這一段的原始位元組。內容沒有經過 byte sex 翻轉。
func (s *Segment) Raw() []byte { return s.raw }

// Word 讀段內第 i 個 word，需要時依 byte sex 翻轉。
func (s *Segment) Word(i int) (uint16, error) {
	if i < 0 || (i+1)*2 > len(s.raw) {
		return 0, fmt.Errorf("段 %s：word %d 超出段長 %d", s.Name, i, len(s.raw)/2)
	}
	v := binary.LittleEndian.Uint16(s.raw[i*2:])
	if s.Flipped {
		v = v>>8 | v<<8
	}
	return v, nil
}

// Codefile 是一份解析過的 codefile。
type Codefile struct {
	Segments []*Segment
	CopyNote string // dictionary 第一筆記錄裡的版權宣告
}

var errShort = errors.New("codefile 太短")

// Parse 解析一份完整的 codefile。
//
// 遇到解不開的地方一律回傳錯誤，不會靜靜跳過——讀取器安靜地少讀一段，
// 比讀不出來難查得多。
func Parse(data []byte) (*Codefile, error) {
	if len(data) < BlockSize {
		return nil, errShort
	}
	cf := &Codefile{}
	cf.CopyNote = pascalString(data[offCopyNote:BlockSize])

	seen := map[int]bool{}
	for blk := 0; ; {
		if seen[blk] {
			return nil, fmt.Errorf("segment dictionary 在 block %d 形成迴圈", blk)
		}
		seen[blk] = true

		rec := block(data, blk)
		if rec == nil {
			return nil, fmt.Errorf("segment dictionary 記錄落在 block %d，超出檔案", blk)
		}
		segs, err := parseDictRecord(data, rec)
		if err != nil {
			return nil, fmt.Errorf("block %d 的 dictionary 記錄：%w", blk, err)
		}
		cf.Segments = append(cf.Segments, segs...)

		next := int(int16(binary.LittleEndian.Uint16(rec[offNextDict:])))
		if next == 0 {
			break
		}
		if next < 0 {
			return nil, fmt.Errorf("block %d 的 Next_Dict 是負數 %d", blk, next)
		}
		blk = next
	}
	return cf, nil
}

// parseDictRecord 解一筆 512 位元組的 dictionary 記錄。
func parseDictRecord(data, rec []byte) ([]*Segment, error) {
	var out []*Segment
	for i := 0; i < dictEntries; i++ {
		addr := int(int16(binary.LittleEndian.Uint16(rec[offDiskInfo+i*4:])))
		leng := int(int16(binary.LittleEndian.Uint16(rec[offDiskInfo+i*4+2:])))
		// 手冊說未用項的段名填空白（spec 2.2）；實務上也遇得到填零的，兩種都當空。
		name := strings.Trim(string(rec[offSegName+i*8:offSegName+i*8+8]), " \x00")
		if leng == 0 && name == "" {
			continue // 未用項
		}
		if addr < 0 || leng < 0 {
			return nil, fmt.Errorf("第 %d 項 %q 的 Disk_Info 是負數（block %d、%d words）", i, name, addr, leng)
		}

		misc := binary.LittleEndian.Uint16(rec[offSegMisc+i*2:])
		info := binary.LittleEndian.Uint16(rec[offSegInfo+i*2:])
		s := &Segment{
			Name:    name,
			Number:  int(info & 0xff),
			Kind:    Kind(misc & 7),
			Machine: Machine(info >> 8 & 0xf),
			Version: Version(info >> 13 & 7),
			Misc:    misc,
			Block:   addr,
			Words:   leng,
		}
		if s.Kind == ProcSeg || s.Kind == SeprtSeg {
			s.Family = strings.Trim(string(rec[offSegFamly+i*8:offSegFamly+i*8+8]), " \x00")
		}
		if err := s.load(data); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// load 讀段頭與 routine dictionary。
func (s *Segment) load(data []byte) error {
	start := s.Block * BlockSize
	end := start + s.Words*2
	if start < 0 || end > len(data) {
		return fmt.Errorf("段 %s：block %d、%d words 超出檔案（%d 位元組）", s.Name, s.Block, s.Words, len(data))
	}
	s.raw = data[start:end]
	if s.Words < hdrWords {
		return fmt.Errorf("段 %s：只有 %d 個 word，裝不下 %d 個 word 的表頭", s.Name, s.Words, hdrWords)
	}

	// byte sex 指示字在產生這一段的機器上恆為 1；相反的位元組序讀出來是 256（spec 4.1）。
	switch binary.LittleEndian.Uint16(s.raw[hdrByteSex:]) {
	case 1:
		s.Flipped = false
	case 256:
		s.Flipped = true
	default:
		return fmt.Errorf("段 %s：byte sex 指示字是 %d，不是 1 也不是 256",
			s.Name, binary.LittleEndian.Uint16(s.raw[hdrByteSex:]))
	}

	w := func(off int) int { return int(s.wordAt(off)) }
	dict := w(hdrRoutineDict)
	s.RelocList = w(hdrRelocList)
	s.ConstPool = w(hdrConstPool)
	s.RealSize = w(hdrRealSize)
	s.Reserved = [2]uint16{s.wordAt(hdrReserved), s.wordAt(hdrReserved + 2)}
	s.HeaderName = strings.Trim(string(s.raw[hdrName:hdrName+8]), " \x00")

	return s.loadRoutines(dict)
}

// loadRoutines 讀 routine dictionary。字典往位址低的方向長：
// 常式 n 的指標在段內 word dict−n（spec 5.2）。
func (s *Segment) loadRoutines(dict int) error {
	count, err := s.Word(dict)
	if err != nil {
		return fmt.Errorf("段 %s：routine dictionary 指標 %d 指到段外", s.Name, dict)
	}
	n := int(count)
	if n > 255 {
		return fmt.Errorf("段 %s：常式數 %d 超過 255", s.Name, n)
	}
	if dict-n < 0 {
		return fmt.Errorf("段 %s：%d 個字典項從 word %d 往回長會越過段頭", s.Name, n, dict)
	}

	s.Routines = make([]Routine, 0, n)
	for i := 1; i <= n; i++ {
		p, err := s.Word(dict - i)
		if err != nil {
			return fmt.Errorf("段 %s：常式 %d 的字典項讀不到：%w", s.Name, i, err)
		}
		r := Routine{Number: i, HeaderWord: int(p), CodeWord: int(p) + 1}
		if r.HeaderWord != 0 {
			if err := s.loadRoutineHeader(&r); err != nil {
				return err
			}
		}
		s.Routines = append(s.Routines, r)
	}
	return nil
}

// loadRoutineHeader 讀 DATASIZE 與 EXITIC（spec 5.4）。
// 字典項指向 DATASIZE，EXITIC 在它前面一個 word，第一條指令在它後面一個 word。
func (s *Segment) loadRoutineHeader(r *Routine) error {
	ds, err := s.Word(r.HeaderWord)
	if err != nil {
		return fmt.Errorf("段 %s：常式 %d 的字典項指到 word %d，超出段長", s.Name, r.Number, r.HeaderWord)
	}
	// 第一條指令是原生碼時，DATASIZE 存的是 one's complement，用正負當旗標；
	// 這種情形下 EXITIC 未定義，不去讀它。
	if v := int16(ds); v < 0 {
		r.Native = true
		r.DataSize = int(^v)
		return nil
	} else {
		r.DataSize = int(v)
	}
	ex, err := s.Word(r.HeaderWord - 1)
	if err != nil {
		return fmt.Errorf("段 %s：常式 %d 的 EXITIC 在 word %d，超出段長", s.Name, r.Number, r.HeaderWord-1)
	}
	if int(ex) >= len(s.raw) {
		return fmt.Errorf("段 %s：常式 %d 的 EXITIC 是位元組 %d，超出段長 %d", s.Name, r.Number, ex, len(s.raw))
	}
	r.ExitIC = int(ex)
	return nil
}

func (s *Segment) wordAt(off int) uint16 {
	v := binary.LittleEndian.Uint16(s.raw[off:])
	if s.Flipped {
		v = v>>8 | v<<8
	}
	return v
}

func block(data []byte, n int) []byte {
	if n < 0 || (n+1)*BlockSize > len(data) {
		return nil
	}
	return data[n*BlockSize : (n+1)*BlockSize]
}

// pascalString 讀 UCSD 形式的字串：一個長度位元組後接字元。
func pascalString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n := int(b[0])
	if n+1 > len(b) {
		n = len(b) - 1
	}
	return string(b[1 : 1+n])
}
