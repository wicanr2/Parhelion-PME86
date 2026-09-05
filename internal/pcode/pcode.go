// Package pcode 是 p-code 這一層的共用小東西：目前只有 IV.0 的官方助記符表。
//
// 表本身是 JSON，因為它也要給 tools/ 底下的 Python 用；
// Go 這邊用 go:embed 讀同一份檔案，避免兩份會走鐘的副本。
package pcode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

//go:embed iv0-opcodes.json
var tableJSON []byte

var (
	once    sync.Once
	names   [256]string
	loadErr error
)

func load() {
	var doc struct {
		Source  string            `json:"_source"`
		Opcodes map[string]string `json:"opcodes"`
	}
	if err := json.Unmarshal(tableJSON, &doc); err != nil {
		loadErr = fmt.Errorf("pcode: 助記符表解不開：%w", err)
		return
	}
	for k, v := range doc.Opcodes {
		n, err := strconv.Atoi(k)
		if err != nil || n < 0 || n > 255 {
			loadErr = fmt.Errorf("pcode: 助記符表有不合法的 opcode %q", k)
			return
		}
		names[n] = v
	}
}

// Mnemonic 回 IV.0 官方表裡這個 opcode 的助記符。
//
// 沒有對應指令的格回空字串——那 44 格在這份直譯器裡全部指向錯誤 11，
// 硬給一個名字只會讓「執行到沒有的指令」看起來像正常的一條。
func Mnemonic(op uint8) string {
	once.Do(load)
	return names[op]
}

// Err 回報助記符表有沒有讀壞。表壞掉的話 Mnemonic 會整排回空字串，
// 而那看起來就像「這段 p-code 全是保留格」。
func Err() error {
	once.Do(load)
	return loadErr
}
