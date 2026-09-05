// Package psystem 是 p-machine 的宿主：記憶體、磁碟、開機時要擺好的資料結構，
// 以及直譯器內嵌的那些原生程序。
//
// **p-machine 不認得這裡的任何東西。** 它只知道「給我那一段的樣子」與
// 「這一支交給宿主」；SIB、Codepool、磁碟版面都是宿主這一側的事。
package psystem

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Block 是 p-system 的磁碟塊大小，整個系統都以它為單位。
const Block = 512

// File 是 .VOL 目錄裡的一筆。
type File struct {
	Name  string
	First int // 起始 block
	Last  int // 最後一個 block 的**下一個**
	Kind  int
	Size  int
}

// Volume 是一份 .VOL 磁碟映像。
type Volume struct {
	ID    string
	Files []File
	data  []byte
}

// OpenVolume 讀一份 .VOL 映像的目錄。
//
// 版面照 IV.0 手冊 p.125 的 Figure 6：目錄從 block 2 開始（block 0–1 是
// bootstrap），每筆 26 個位元組，第 0 筆描述 volume 本身。
func OpenVolume(data []byte) (*Volume, error) {
	const dirBlock, entry = 2, 26
	if len(data) < (dirBlock+4)*Block {
		return nil, fmt.Errorf("psystem: 映像只有 %d 位元組，讀不到目錄", len(data))
	}
	// **抄一份**。磁碟是可以寫的，而呼叫端給的那份位元組不該被我們改掉——
	// 寫回檔案是另一件事，要做也是明確地做。
	data = append([]byte(nil), data...)
	d := data[dirBlock*Block:]
	v := &Volume{ID: pstr(d[6:14]), data: data}
	n := int(binary.LittleEndian.Uint16(d[16:]))
	if n > 77 {
		return nil, fmt.Errorf("psystem: 目錄說有 %d 個檔案，超過上限 77", n)
	}
	for i := 1; i <= n; i++ {
		o := i * entry
		first := int(binary.LittleEndian.Uint16(d[o:]))
		last := int(binary.LittleEndian.Uint16(d[o+2:]))
		lastByte := int(binary.LittleEndian.Uint16(d[o+22:]))
		v.Files = append(v.Files, File{
			Name:  pstr(d[o+6 : o+22]),
			First: first, Last: last,
			Kind: int(binary.LittleEndian.Uint16(d[o+4:])) & 15,
			Size: (last-first-1)*Block + lastByte,
		})
	}
	return v, nil
}

// Read 取一個檔案的內容。
func (v *Volume) Read(name string) ([]byte, error) {
	for _, f := range v.Files {
		if strings.EqualFold(f.Name, name) {
			end := f.First*Block + f.Size
			if end > len(v.data) {
				return nil, fmt.Errorf("psystem: %s 說到 block %d，映像沒那麼長", name, f.Last)
			}
			return v.data[f.First*Block : end], nil
		}
	}
	return nil, fmt.Errorf("psystem: 磁碟上沒有 %s", name)
}

// Blocks 直接讀磁碟上連續幾個 block——磁碟服務用得到，不經過目錄。
func (v *Volume) Blocks(first, n int) []byte {
	lo, hi := first*Block, (first+n)*Block
	if lo < 0 || hi > len(v.data) {
		return nil
	}
	return v.data[lo:hi]
}

// pstr 是 UCSD 字串：一個長度位元組後接字元。
func pstr(b []byte) string {
	n := int(b[0])
	if n > len(b)-1 {
		n = len(b) - 1
	}
	return string(b[1 : 1+n])
}

// NewRAMDisk 造一片空的記憶體磁碟。
//
// 這台主機在 unit 13 掛了一片 `RAMDISK`，750 塊。**開機時上面一個檔案都沒有**
// ——`dnumfiles` 是 0，作業系統之後才會把 `SYSTEM.MISCINFO` 寫上去。
// 沒有這片，它會以為那台沒有磁碟，而原版是有的。
//
// 版面照 IV.0 手冊 p.125 的目錄格式，與 `.VOL` 完全相同，
// 所以造出來之後就是一片普通的磁碟。
func NewRAMDisk(name string, blocks int) *Volume {
	img := make([]byte, blocks*Block)
	d := img[2*Block:]
	put := func(o int, v uint16) { binary.LittleEndian.PutUint16(d[o:], v) }

	put(2, 6) // dlastblk：目錄佔到 block 6
	d[6] = byte(len(name))
	copy(d[7:], name)
	put(14, uint16(blocks)) // deovblk
	put(16, 0)              // dnumfiles：空的

	v, err := OpenVolume(img)
	if err != nil {
		return nil
	}
	return v
}

// Write 把位元組寫進磁碟映像。只改記憶體裡這一份，不碰原始檔案。
func (v *Volume) Write(off int, b []byte) bool {
	if off < 0 || off+len(b) > len(v.data) {
		return false
	}
	copy(v.data[off:], b)
	return true
}
