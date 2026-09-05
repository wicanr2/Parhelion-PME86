package psystem

import (
	"encoding/binary"
	"testing"
)

// 造一份最小的 .VOL：block 0–1 是 bootstrap，block 2 起是目錄。
func synthVolume(t *testing.T, name string, first, blocks, lastByte int, body []byte) []byte {
	t.Helper()
	img := make([]byte, (first+blocks+1)*Block)
	d := img[2*Block:]
	put := func(o int, v uint16) { binary.LittleEndian.PutUint16(d[o:], v) }
	d[6] = 7
	copy(d[7:], "PSYSTEM")
	put(14, uint16(len(img)/Block)) // deovblk
	put(16, 1)                      // 一個檔案

	o := 26
	put(o, uint16(first))
	put(o+2, uint16(first+blocks))
	put(o+4, 2) // code
	d[o+6] = byte(len(name))
	copy(d[o+7:], name)
	put(o+22, uint16(lastByte))
	copy(img[first*Block:], body)
	return img
}

func TestVolumeDirectoryReadsAFile(t *testing.T) {
	body := []byte("HELLO")
	img := synthVolume(t, "A.CODE", 6, 1, len(body), body)

	v, err := OpenVolume(img)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "PSYSTEM" {
		t.Errorf("volume 名字 %q", v.ID)
	}
	if len(v.Files) != 1 || v.Files[0].Name != "A.CODE" {
		t.Fatalf("目錄讀出來是 %+v", v.Files)
	}
	// 大小 ＝ (last − first − 1)×512 + lastbyte。只佔一個 block 時就是 lastbyte。
	if v.Files[0].Size != len(body) {
		t.Errorf("檔案大小 %d，該是 %d", v.Files[0].Size, len(body))
	}
	got, err := v.Read("a.code") // 檔名不分大小寫
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Errorf("讀出來是 %q", got)
	}
}

func TestMissingFileSaysSo(t *testing.T) {
	img := synthVolume(t, "A.CODE", 6, 1, 1, []byte{1})
	v, err := OpenVolume(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Read("SYSTEM.PASCAL"); err == nil {
		t.Fatal("磁碟上沒有這個檔案卻讀成功了")
	}
}

// 目錄壞掉要當場說，不要靜靜回一份空的檔案清單——
// 「這片磁碟沒有檔案」與「這片磁碟讀不出來」是兩件事。
func TestTruncatedImageIsAnError(t *testing.T) {
	if _, err := OpenVolume(make([]byte, Block)); err == nil {
		t.Fatal("只有一個 block 的映像卻讀出了目錄")
	}
}
