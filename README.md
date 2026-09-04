# PME.86：8086 版 p-machine 的解剖，以及一台 Go 重做的機器

`SYSTEM.PME.86` 是 1984 年 DOS 版 UCSD p-system 的 p-machine 直譯器，16384 個位元組。
這個 repo 做兩件事：**把它逐支拆開寫清楚**，然後**照著這些結論用 Go 重做一台**。

知識庫在前，實作在後。理由很直接——沒有寫下來的理解會在寫程式的過程中
悄悄變成猜測，而猜測看起來和知識長得一模一樣。

## 從哪開始讀

1. [取指令與分派](docs/10-interpreter/dispatch-and-threading.md)
   — 一台沒有主迴圈的直譯器。dispatch 表怎麼在沒有位移的間接跳躍下被找出來。
2. [直譯器的狀態](docs/10-interpreter/machine-state.md)
   — 哪幾個 p-machine 暫存器值得常駐在 8086 暫存器裡，其餘的放哪裡。
3. [定址與活動記錄](docs/10-interpreter/addressing.md)
   — word 世界換到位元組世界的那一層，以及變數編號為什麼從 1 開始。
4. [段的切換](docs/10-interpreter/segment-switching.md)
   — `E_Rec`、`SIB`、程式碼段表頭、程序字典，還有內嵌在直譯器裡的 33 支程序。
5. [256 格對照表](docs/10-interpreter/opcode-map.md)
   — 逐格：IV.0 助記符、常式偏移、指令數。

## 格式

- [`.VOL` 磁碟映像](docs/20-formats/vol-image.md) — 目錄版面與抽檔。
- [`SYSTEM.PME.86` 的檔頭](docs/20-formats/pme86-header.md) — 前 18 個 word，部分已解。

## 重做

- [可行性評估](docs/30-remake/feasibility.md) — 六層盤點、oracle 迴路、四個里程碑。
- [spec 閘門](docs/30-remake/spec-workflow.md) — `DRAFT` → `READY` → `CONFORMED`。

實作還沒有開始。開始之前要先有 `READY` 的 spec。

## 工具

`tools/` 裡的東西都是自足的，不依賴其他 repo：

| 檔案 | 做什麼 |
|---|---|
| `read-vol.py` | 讀 `.VOL` 映像，列目錄或抽檔 |
| `dump-routines-86.py` | 在 IDA 裡把 dispatch 表與每支常式反組譯成 JSON，並追 `call` 目標 |
| `analyze-dispatch.py` | 從那份 JSON 產統計或 256 格 markdown 表 |
| `routines-86.json` | 上面那支的產出，169 支常式加 25 支助手 |
| `iv0-opcodes.json` | IV.0 官方助記符表 |

## 資料來源

**SofTech《UCSD p-System and UCSD Pascal Version IV.0 Internal Architecture Guide》**
（1981）。手冊的逐節摘譯在
[`ucsd-pascal-notes`](https://github.com/wicanr2/ucsd-pascal-notes) 的
`docs/50-iv-internals/`，掃描檔也在那個 repo。

實測對象是 `psys21`（1984 年 DOS hosted p-system）磁碟映像裡的 `SYSTEM.PME.86`，
sha256 `fe427aa66ca8…`。**本 repo 不含任何原版磁碟映像或可執行檔**，
只保留位元組層級的結論與反組譯產出。

p-machine 的通則、指令編碼、程序呼叫的第一性原理推導在
[`ucsd-pascal-notes`](https://github.com/wicanr2/ucsd-pascal-notes)；
這個 repo 只處理 8086 這一份實作，以及重做。

## 邊界

- 「IV.2.1」這個版本身分來自磁碟的發行標示，**沒有從檔案本身讀到版號**。
- 助記符來自 IV.0 官方手冊的附錄。編號已經逐格對過，語意逐條相同沒有驗證。
- 169 支常式全部反組譯到手，但**不是每一支都讀過**。讀過並寫進文件的約三十支，
  其餘只做過表層級的統計。
- 反組譯的結束判準是第一個無條件轉移，所以每支只涵蓋主路徑，
  跳過去之後的碼要另外追。

## 授權

文件與圖採 CC BY 4.0。反組譯產出（`tools/routines-86.json`）是對
SofTech Microsystems 及其後續權利人所有之程式的分析結果，收錄為技術研究之用。
Go 實作的授權在開工時另行決定。
