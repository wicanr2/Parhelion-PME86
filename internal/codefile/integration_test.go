package codefile_test

import (
	"os"
	"testing"

	"github.com/wicanr2/Parhelion-PME86/internal/codefile"
)

// TestRealCodefile 是 M0 的驗收：拿一份真的 codefile 跑一次，看解出來的東西自洽。
//
// 原版檔案不在版控裡，所以這個測試預設跳過。要跑就指到本機的一份：
//
//	PARHELION_CODEFILE=/path/to/SYSTEM.PASCAL go test ./internal/codefile/
//
// 驗收條件不是「跟某組數字一樣」——那會把測試綁死在特定的一份檔案上——
// 而是結構自洽：每一段的表頭讀得出來、每一個字典項都落在段內、
// EXITIC 指到的位元組在段內。
func TestRealCodefile(t *testing.T) {
	path := os.Getenv("PARHELION_CODEFILE")
	if path == "" {
		t.Skip("沒有設 PARHELION_CODEFILE，跳過")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := codefile.Parse(data)
	if err != nil {
		t.Fatalf("Parse：%v", err)
	}
	if len(cf.Segments) == 0 {
		t.Fatal("一個 segment 都沒有")
	}

	var routines, absent, flipped int
	for _, s := range cf.Segments {
		if s.Name == "" {
			t.Errorf("block %d 的 segment 沒有名字", s.Block)
		}
		if s.HeaderName != "" && s.HeaderName != s.Name {
			t.Errorf("段 %s：表頭裡的名字是 %q", s.Name, s.HeaderName)
		}
		if s.RealSize != 2 && s.RealSize != 4 {
			t.Errorf("段 %s：REALSIZE = %d，既不是 2 也不是 4", s.Name, s.RealSize)
		}
		if s.ConstPool >= s.Words {
			t.Errorf("段 %s：常數池在 word %d，超出段長 %d", s.Name, s.ConstPool, s.Words)
		}
		if s.Flipped {
			flipped++
		}
		for _, r := range s.Routines {
			routines++
			if !r.Present() {
				absent++
				continue
			}
			if r.CodeWord >= s.Words {
				t.Errorf("段 %s 常式 %d：碼在 word %d，超出段長 %d", s.Name, r.Number, r.CodeWord, s.Words)
			}
			if r.DataSize < 0 {
				t.Errorf("段 %s 常式 %d：DATASIZE = %d", s.Name, r.Number, r.DataSize)
			}
		}
	}
	t.Logf("%d 個 segment、%d 支常式（%d 支沒有碼）、%d 個 segment 的 byte sex 與主機相反",
		len(cf.Segments), routines, absent, flipped)
}
