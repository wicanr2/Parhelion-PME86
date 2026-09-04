# 進度與待辦

## 分輪紀錄

### R1（2026-09-05）建立 repo，第一批九篇

- 用擴充後的 `tools/dump-routines-86.py` 重跑 IDA：**169 支 dispatch 目標全部反組譯成功，
  另外追出 25 支助手常式**（追 `call` 目標）。輸入 sha256 `fe427aa66ca8…`，
  截斷上限 80 條，只有 `MPR`（`0x24e1`）與 `DVR`（`0x262a`）觸頂。
- 寫成四篇：[分派](docs/10-interpreter/dispatch-and-threading.md)、
  [狀態](docs/10-interpreter/machine-state.md)、
  [定址](docs/10-interpreter/addressing.md)、
  [段切換](docs/10-interpreter/segment-switching.md)，
  加一份工具產生的 [256 格對照表](docs/10-interpreter/opcode-map.md)。
- 格式兩篇：[`.VOL`](docs/20-formats/vol-image.md)、
  [檔頭](docs/20-formats/pme86-header.md)。
- 重做兩篇：[可行性評估](docs/30-remake/feasibility.md)、
  [spec 閘門](docs/30-remake/spec-workflow.md)。

這一輪新解出來的東西：

| 結論 | 證據 |
|---|---|
| 分派碼 8 個位元組內嵌在 129 支常式結尾；另有省一個位元組的變體用了 50 支 | 比對結尾五條指令的位元組 |
| `bx` = 局部資料基底 = `MP + 8`；`dx` = 全域資料基底 = `Env_Data + 8` | 助手 `0x093e`、`LAO`、`RPU` `0x114e`、`DVI`／`STP` 的 `mov dx, ss:28h` |
| 變數編號 1 落在 `MP + 10`，正好是 MSCW 之後第一個 word | `RPU` 逐格彈出 MSCW 五個欄位 |
| `ss:42h` 常數池基底、`ss:44h` byte sex、`ss:24h` 存起來的 IPC | `LCO`／`LDC`／`XJP`；`MOV` 的交換位元組路徑對上手冊對 `MOV` 的定義 |
| `E_Rec` 實際欄位偏移是 `Env_Data`(+0)、`E_Vec`(+2)、`SIB`(+4) | 助手 `0x0fab` 用 `+2` 當 segment number 索引的陣列；`+4` 的 `+4`／`+6` 行為對上 `Ref_Count`／`Activity` |
| 程式碼段表頭：`+0x00` 程序字典 word 偏移、`+0x0c` byte sex、`+0x0e` 常數池 word 偏移 | 助手 `0x0fba` 尾段 |
| 程序字典往回長：`字典基底 − 2×程序號` | 呼叫助手 `0x101b` |
| `CXG` 的內嵌程序表 `0x1f56` 起 48 格，**33 格非零**，其中七對指同一位址 | 直接讀檔案位元組 |
| 遮罩表 `0x1fb6` 起 17 格，第 n 項 `(1 << n) − 1` | `LDP`／`STP` 的索引算法加表本身的內容 |
| big operand 解碼段（9 個位元組）在檔案裡出現 **19 次**，字面完全相同 | 位元組序列搜尋 |

## 勘誤：被這一輪推翻的舊斷言

都在姊妹 repo [`ucsd-pascal-notes`](https://github.com/wicanr2/ucsd-pascal-notes)
的 `docs/30-opcode-tables/iv21-two-cpus.md` 與
`docs/50-iv-internals/instruction-set-details.md`，**同一輪已經回去改掉**
（commit `7891ccd`），並對該 repo 全部文件的「十進位（0x十六）」做過一次
一致性掃描，其餘 0 個不一致。

| 舊斷言 | 實際 | 證據 |
|---|---|---|
| 遮罩表是 `0x007f, 0x00ff, 0x01ff, 0x03ff…` | 表從 `0x1fb6` 起是 `0x0000, 0x0001, 0x0003…`；那串值要到 `0x1fc4` 才開始 | 直接讀檔案 |
| 「E_Rec 偏移 4 → `Env_Vect`，與手冊 p.37 宣告順序一致」 | `+2` 才是以 segment number 索引的指標陣列，`+4` 是 SIB | 助手 `0x0fab`、`0x15d2`、`0x10db` |
| `CXG` 節錄的 `cmp di, 2Fh` 之後直接 `mov di, cs:[di+1F56h]` | 中間還有 `and di, 0FFh` 與 `shl di, 1` | `0x13f9`–`0x1406` |
| 「8086 版只讀了 8 個常式，169 個相異目標是表的統計」 | 169 支全部反組譯到手，其中約三十支讀過並寫進文件 | `tools/routines-86.json` |
| 摘譯把 `RPU` 寫成「150（0x97）」 | 150 = `0x96`；dispatch 表 `0x96` 指向的就是 `RPU` | 十進位與十六進位換算 |

## 開放項目

每項都配一個消除條件——沒有消除條件的待辦會永遠留著。

| # | 問題 | 消除條件 |
|---|---|---|
| 1 | `jmp word ptr cs:[di]` 沒有位移，表卻在檔案偏移 `0x1d56`。執行期 `cs` 基底與檔案載入位址的差從哪來 | 找到設定 `cs` 的那段碼，或從載入器（`SYSTEM.PASCAL`）確認 |
| 2 | SIB 的確切版面。`Seg_Base` 佔兩個 word 是推論；`[SIB+2]` 的零檢查、`0x1bee` 呼叫前把 `ds` 換成 `es` 都還沒解釋 | 追出 `0x0fe1`–`0x0ff5` 的完整算術，或在 DOSBox 裡看實際的 SIB 內容 |
| 3 | 檔頭 18 個 word 的用途。三格對上狀態變數偏移，其餘是程式碼位址 | 找到讀這張表的碼（在載入器裡，不在本檔） |
| 4 | 內嵌的 33 支機器碼程序分別做什麼 | 逐支反組譯並對上 IV.0 的 intrinsic 清單 |
| 5 | 8 位元組實數的位元格式 | 讀完 `MPR`／`DVR`／`FLT`，或用已知數值在 DOSBox 裡對拍 |
| 6 | `MPR`（`0x24e1`）與 `DVR`（`0x262a`）被 80 條上限截斷 | 提高上限重跑，或改成追控制流而不是線性反組譯 |
| 7 | `ss:3Ch` 指向的記錄版面（堆疊檢查只用了 `+4`） | 找出誰設定 `ss:3Ch` |
| 8 | DOSBox 裡的 psys21 能不能自動送輸入、取輸出 | 跑起來、送一行指令、把畫面抓出來比對 |

## 下一輪候選

- **#8 先做。** 沒有 oracle，後面的實作只能靠讀碼的信心。
- #4 與 #6 是純反組譯工作，可以並行。
- M0（codefile 讀取器）不依賴任何開放項目，可以立刻開始。
