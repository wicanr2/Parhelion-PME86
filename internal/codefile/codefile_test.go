package codefile

import (
	"encoding/binary"
	"strings"
	"testing"
)

// 測試用的 codefile 是合成的，不含任何原版資料。版面照
// docs/30-remake/specs/01-codefile.md 組出來。

type segSpec struct {
	name     string
	block    int
	words    int
	flipped  bool
	realSize int
	routines []Routine // 只用 HeaderWord、DataSize、ExitIC、Native
}

// builder 組一份 codefile。
type builder struct {
	data []byte
}

func newBuilder(blocks int) *builder { return &builder{data: make([]byte, blocks*BlockSize)} }

func (b *builder) put(off int, v uint16, flip bool) {
	if flip {
		v = v>>8 | v<<8
	}
	binary.LittleEndian.PutUint16(b.data[off:], v)
}

// dictRecord 寫一筆 segment dictionary 記錄。
func (b *builder) dictRecord(block int, segs []segSpec, next int, note string) {
	base := block * BlockSize
	for i, s := range segs {
		b.put(base+offDiskInfo+i*4, uint16(s.block), false)
		b.put(base+offDiskInfo+i*4+2, uint16(s.words), false)
		copy(b.data[base+offSegName+i*8:], []byte(s.name+"        ")[:8])
		b.put(base+offSegMisc+i*2, uint16(UnitSeg), false)
		// Seg_Num = i+1、M_Psuedo、版本 IV
		b.put(base+offSegInfo+i*2, uint16(i+1)|uint16(4)<<13, false)
	}
	b.put(base+offNextDict, uint16(next), false)
	b.data[base+offCopyNote] = byte(len(note))
	copy(b.data[base+offCopyNote+1:], note)
	b.put(base+offSex, 1, false)
}

// segment 寫一個 code segment，含表頭與 routine dictionary。
func (b *builder) segment(s segSpec) {
	base := s.block * BlockSize
	w := func(i int, v uint16) { b.put(base+i*2, v, s.flipped) }

	dict := s.words - 1
	w(hdrRoutineDict/2, uint16(dict))
	w(hdrRelocList/2, 0)
	copy(b.data[base+hdrName:], []byte(s.name+"        ")[:8])
	w(hdrByteSex/2, 1)
	w(hdrConstPool/2, 0)
	w(hdrRealSize/2, uint16(s.realSize))

	w(dict, uint16(len(s.routines)))
	for i, r := range s.routines {
		w(dict-(i+1), uint16(r.HeaderWord))
		if r.HeaderWord == 0 {
			continue
		}
		ds := uint16(r.DataSize)
		if r.Native {
			ds = uint16(^int16(r.DataSize))
		}
		w(r.HeaderWord, ds)
		if !r.Native {
			w(r.HeaderWord-1, uint16(r.ExitIC))
		}
	}
}

// fixture 是兩個 dictionary 記錄、三個 segment 的合成 codefile。
func fixture() []byte {
	b := newBuilder(8)
	plain := segSpec{name: "PLAIN", block: 1, words: 64, realSize: 4, routines: []Routine{
		{HeaderWord: 12, DataSize: 3, ExitIC: 40},
		{HeaderWord: 0}, // EXTERNAL／FORWARD
		{HeaderWord: 30, DataSize: 7, Native: true},
	}}
	flipped := segSpec{name: "FLIPPED", block: 2, words: 48, flipped: true, realSize: 2,
		routines: []Routine{{HeaderWord: 12, DataSize: 300, ExitIC: 60}}}
	tail := segSpec{name: "TAIL", block: 4, words: 32, realSize: 4,
		routines: []Routine{{HeaderWord: 12, DataSize: 0, ExitIC: 26}}}

	b.dictRecord(0, []segSpec{plain, flipped}, 3, "合成測資")
	b.dictRecord(3, []segSpec{tail}, 0, "")
	b.segment(plain)
	b.segment(flipped)
	b.segment(tail)
	return b.data
}

func TestParseFixture(t *testing.T) {
	cf, err := Parse(fixture())
	if err != nil {
		t.Fatalf("Parse：%v", err)
	}
	if got, want := len(cf.Segments), 3; got != want {
		t.Fatalf("segment 數 = %d，想要 %d", got, want)
	}
	if cf.CopyNote != "合成測資" {
		t.Errorf("CopyNote = %q", cf.CopyNote)
	}

	p := cf.Segments[0]
	if p.Name != "PLAIN" || p.HeaderName != "PLAIN" {
		t.Errorf("段名 = %q / %q", p.Name, p.HeaderName)
	}
	if p.Flipped {
		t.Error("PLAIN 不該被判成反 byte sex")
	}
	if p.Kind != UnitSeg || p.Machine != 0 || p.Version != 4 {
		t.Errorf("種類／機器／版本 = %v / %v / %v", p.Kind, p.Machine, p.Version)
	}
	if p.RealSize != 4 {
		t.Errorf("RealSize = %d", p.RealSize)
	}
	if len(p.Routines) != 3 {
		t.Fatalf("常式數 = %d", len(p.Routines))
	}

	// 字典項指向 DATASIZE，第一條指令在它後面一個 word（spec 5.4）。
	r := p.Routines[0]
	if r.HeaderWord != 12 || r.CodeWord != 13 || r.DataSize != 3 || r.ExitIC != 40 || r.Native {
		t.Errorf("常式 1 = %+v", r)
	}
	if p.Routines[1].Present() {
		t.Error("常式 2 應該沒有碼")
	}
	// 原生碼：DATASIZE 以 one's complement 存放，EXITIC 未定義（spec 5.5）。
	if n := p.Routines[2]; !n.Native || n.DataSize != 7 || n.ExitIC != 0 {
		t.Errorf("常式 3 = %+v", n)
	}

	f := cf.Segments[1]
	if !f.Flipped {
		t.Fatal("FLIPPED 應該被判成反 byte sex")
	}
	if f.RealSize != 2 || f.Routines[0].DataSize != 300 || f.Routines[0].ExitIC != 60 {
		t.Errorf("FLIPPED = RealSize %d, 常式 %+v", f.RealSize, f.Routines[0])
	}

	// 第二筆 dictionary 記錄要接上。
	if cf.Segments[2].Name != "TAIL" {
		t.Errorf("第三個 segment = %q", cf.Segments[2].Name)
	}
}

func TestSegmentWordRespectsByteSex(t *testing.T) {
	cf, err := Parse(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range cf.Segments {
		got, err := s.Word(hdrByteSex / 2)
		if err != nil {
			t.Fatal(err)
		}
		// 依旗標解碼之後，byte sex 指示字一律讀成 1（spec 4.1）。
		if got != 1 {
			t.Errorf("段 %s 的 byte sex 指示字解出來是 %d", s.Name, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name   string
		mangle func(b []byte)
		want   string
	}{
		{"太短", func(b []byte) {}, ""}, // 由下面另外處理
		{"byte sex 不合法", func(b []byte) {
			binary.LittleEndian.PutUint16(b[1*BlockSize+hdrByteSex:], 7)
		}, "byte sex"},
		{"segment 超出檔案", func(b []byte) {
			binary.LittleEndian.PutUint16(b[offDiskInfo:], 900)
		}, "超出檔案"},
		{"dictionary 迴圈", func(b []byte) {
			binary.LittleEndian.PutUint16(b[3*BlockSize+offNextDict:], 0)
			binary.LittleEndian.PutUint16(b[offNextDict:], 0)
			binary.LittleEndian.PutUint16(b[offNextDict:], 3)
			binary.LittleEndian.PutUint16(b[3*BlockSize+offNextDict:], 3)
		}, "迴圈"},
		{"字典項指到段外", func(b []byte) {
			// PLAIN 的常式 1 指到遠超段長的 word
			binary.LittleEndian.PutUint16(b[1*BlockSize+(64-1-1)*2:], 5000)
		}, "超出段長"},
	}
	for _, tt := range tests {
		if tt.want == "" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			b := fixture()
			tt.mangle(b)
			_, err := Parse(b)
			if err == nil {
				t.Fatal("預期要有錯誤，卻成功了")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("錯誤訊息 %q 沒有提到 %q", err, tt.want)
			}
		})
	}

	if _, err := Parse(make([]byte, 100)); err == nil {
		t.Error("太短的檔案應該回錯誤")
	}
}
