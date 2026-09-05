package psystem

import "fmt"

// Device 是 `SYSTEM.CONFIG` 裡的一筆裝置設定。
type Device struct {
	Instance int    // 同一類裡的第幾個
	Class    int    // 裝置類別：1 主控台、2 序列／並列、3 磁碟、4／5 其他
	Driver   string // 驅動的檔名
	Shared   bool   // 與前一筆共用已經載入的驅動
	Unit     uint16 // 對應的 unit 編號；0 表示還沒解出來
}

func (d Device) String() string {
	s := fmt.Sprintf("unit %-3d class %d 第 %d 個  %s", d.Unit, d.Class, d.Instance, d.Driver)
	if d.Shared {
		s += "（共用驅動）"
	}
	return s
}

// 一筆設定的版面。40 個位元組一筆，一路排到檔尾。
//
//	+0  同一類裡的第幾個
//	+1  類別
//	+3  0 表示這一筆要載入驅動，非 0 表示與前面共用
//	+4  驅動檔名的長度，接著最多 11 個字元
//
// 其餘的位元組是驅動自己的參數，還沒解。
const (
	configRecord  = 40
	configNameLen = 11
)

// diskUnits 是磁碟類的 unit 編號順序。前六個是手冊定的
// （4、5 是前兩台，9–12 是第三到第六台），再往後就依序往上加。
//
// **這個順序是對得上實測的**：`SYSTEM.CONFIG` 裡磁碟類的第 6 個是
// `RAMDSK.DRV`、第 7 個是 `DOSVV.DRV`，而原版回答「這台在」的正是
// unit 13 與 unit 14。
var diskUnits = []uint16{4, 5, 9, 10, 11, 12}

func diskUnit(instance int) uint16 {
	if instance < len(diskUnits) {
		return diskUnits[instance]
	}
	return uint16(13 + instance - len(diskUnits))
}

// ParseConfig 讀 `SYSTEM.CONFIG`：這台機器上有哪些裝置、各自用哪個驅動。
//
// **只解得出磁碟類的 unit 編號。** 其餘類別的編號規則還沒查證，
// 那幾筆的 Unit 留 0——寫一個猜的號碼進去，之後查起來會分不出哪個是量到的。
func ParseConfig(b []byte) []Device {
	var out []Device
	for off := 0; off+configRecord <= len(b); off += configRecord {
		r := b[off : off+configRecord]
		n := int(r[4])
		if n == 0 || n > configNameLen {
			continue
		}
		name := r[5 : 5+n]
		ok := true
		for _, c := range name {
			if c < 0x20 || c >= 0x7f {
				ok = false
			}
		}
		if !ok {
			continue
		}
		d := Device{
			Instance: int(r[0]),
			Class:    int(r[1]),
			Driver:   string(name),
			Shared:   r[3] != 0,
		}
		if d.Class == classDisk {
			d.Unit = diskUnit(d.Instance)
		}
		out = append(out, d)
	}
	return out
}

// 裝置類別。目前只用得到磁碟那一類。
const (
	classConsole = 1
	classSerial  = 2
	classDisk    = 3
)

// 這一台主機把哪個驅動當成什麼用。
const (
	driverRAMDisk = "RAMDSK.DRV"
	driverDOSVol  = "DOSVV.DRV"
)

// FindUnit 找出第一個用某個驅動、而且自己會載入它的 unit。
func FindUnit(devs []Device, driver string) (uint16, bool) {
	for _, d := range devs {
		if d.Driver == driver && !d.Shared && d.Unit != 0 {
			return d.Unit, true
		}
	}
	return 0, false
}
