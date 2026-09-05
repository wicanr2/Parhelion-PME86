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

// SYSTEM.CONFIG 決定哪個 unit 是什麼。認錯的話，作業系統會在一台不存在的
// 磁碟上找檔案，而症狀會出現在很遠的地方。
func TestParseConfigMapsDiskUnits(t *testing.T) {
	// 一筆 40 個位元組：第幾個、類別、（保留）、共用旗標、名字長度、名字。
	rec := func(instance, class, shared int, name string) []byte {
		r := make([]byte, configRecord)
		r[0], r[1], r[3] = byte(instance), byte(class), byte(shared)
		r[4] = byte(len(name))
		copy(r[5:], name)
		return r
	}
	var cfg []byte
	for _, r := range [][]byte{
		rec(0, classConsole, 0, "CONSOL.DRV"),
		rec(0, classDisk, 0, "FLOPPY.DRV"),
		rec(1, classDisk, 3, "FLOPPY.DRV"),
		rec(6, classDisk, 0, driverRAMDisk),
		rec(7, classDisk, 0, driverDOSVol),
	} {
		cfg = append(cfg, r...)
	}

	devs := ParseConfig(cfg)
	if len(devs) != 5 {
		t.Fatalf("解出 %d 筆", len(devs))
	}
	// 磁碟類的第 0、1 個是 unit 4、5；第 6、7 個是 13、14。
	for i, want := range map[int]uint16{1: 4, 2: 5, 3: 13, 4: 14} {
		if devs[i].Unit != want {
			t.Errorf("第 %d 筆是 unit %d，該是 %d", i, devs[i].Unit, want)
		}
	}
	if devs[0].Unit != 0 {
		t.Errorf("主控台那一類的編號還沒解出來，不該亂填：%d", devs[0].Unit)
	}
	if !devs[2].Shared {
		t.Error("第二台軟碟該標成共用驅動")
	}
	if u, ok := FindUnit(devs, driverRAMDisk); !ok || u != 13 {
		t.Errorf("記憶體磁碟找到 unit %d（%v）", u, ok)
	}
	if u, ok := FindUnit(devs, driverDOSVol); !ok || u != 14 {
		t.Errorf("開機磁碟找到 unit %d（%v）", u, ok)
	}
}
