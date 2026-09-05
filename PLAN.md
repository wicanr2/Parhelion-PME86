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

### R2（2026-09-05）定名，M0 完成

- 專案定名 **Parhelion PME**，repo `Parhelion-PME86`，名字的由來寫在 `README.md`。
- 寫 [spec 01：codefile 靜態結構](docs/30-remake/specs/01-codefile.md)（狀態 `READY`），
  逐條配證據，並拿 `SYSTEM.PASCAL` 的實際位元組驗過。
- 實作 `internal/codefile` 與 `cmd/parhelion codefile`，**M0 通過**：
  28 個 segment、465 支常式、0 個字典項越界，兩筆 dictionary 記錄的鏈接得起來。
- 單元測試用合成的 codefile，不含原版資料；真檔驗收走 `PARHELION_CODEFILE` 環境變數。

這一輪解出來的東西：

| 結論 | 證據 |
|---|---|
| segment dictionary 記錄的 512 位元組版面（六個平行陣列＋`Next_Dict`＋`Copy_Note`＋`Sex`） | 最尾端的 `Copy_Note` 與 `Sex` 都讀得對，表示中間所有欄位大小加總正確 |
| `Seg_Info` 的 packed record 由低位往高位打包 | `KERNEL` 讀出 `0x8001` → 段號 1、`M_Psuedo`、版本 `IV`；反方向段號會是 128 |
| `Seg_Famly` 的變體記錄在 `Proc_Seg` 時是外層 unit 名 | `SEGFOPEN` 讀出 `"FILEOPS "`、`USERPROG` 讀出 `"KERNEL  "`，都對得到同檔案裡的段 |
| **`SYSTEM.PASCAL` 自己就混著兩種位元組序**：28 段裡 6 段的 byte sex 指示字讀出 256 | 直接讀檔案 |
| 常式版面是 `EXITIC`、`DATASIZE`、第一條指令，**字典項指的是中間的 `DATASIZE`** | 直譯器 `0x103a` 的 `mov cx, es:[di]` 後 `inc di; inc di`；430 支常式裡 418 支的 `EXITIC` 直接指到 `RPU`，0 支越界 |
| 段內所有段的 `REALSIZE` 都是 4 | 與 `LDCRL` 在堆疊上開 8 個位元組一致 |

spec 01 停在 `READY` 沒有升 `CONFORMED`。升級條件是開放項目 #8——
把原版放進 DOSBox 當 oracle 之前，「同狀態驗證」這個門檻沒有真的跨過。

### R3（2026-09-05）用 dosgolem 把原版跑起來

`dosgolem` 是同一個作者的無頭 DOS 執行器（為《大富翁2》的 remake 而寫），
可以當 Go 套件 import、決定性、看得到記憶體。拿它當 oracle 比 DOSBox 直接得多：
**問得到記憶體，不必隔著畫面反推。**

在 `dosgolem` 的一份副本上加了三樣東西：`.COM` 載入器、`int 16h` 的按鍵佇列、
IRQ1 與埠 `60h`。跑起來之後：

- p-System 開機到命令列，畫面上是 `[IV.2.1 R3.3]`——**版本從系統自己的畫面確認**，
  不再只是磁碟標示的推論。
- `PSYSTEM.VOL` 開得起來，`SYSTEM.PME.86` 定位在實體 `0x01400`（段 `0x0140`）。
- **開放項目 #1 消除**：映像偏移 `0x000`–`0x1ff` 與磁碟 `0x1d56` 起的 512 個位元組
  逐位元組相同。載入器把 dispatch 表搬到映像最前面，所以 `jmp word ptr cs:[di]`
  不必帶位移，而表項當成 `cs:offset` 剛好正確。
- 順帶把 #3 縮小：檔頭那張位址表就在被蓋掉的範圍內，所以讀它的一定是載入器。

這些量測**做成了會跑的測試**，不是一次性的觀察：`oracle/psys_test.go` 的
`TestLoaderMovesTheDispatchTableToOffsetZero` 每次都重驗 512／512。
`tools/ci.sh` 是本機 CI，需要素材的部分缺檔就跳過並且說明跳過了。

分工定了：**dosgolem 只放任何 DOS 程式都用得到的能力**（`.COM` 載入器、
`int 16h` 佇列、IRQ1 與埠 `60h`、讀文字畫面、用指紋在記憶體裡找映像），
它另外開了一個 `package dosgolem` 當對外介面；**認得 p-machine 的知識全部在
這個 repo 的 `oracle/`**。第一版把 p-System 專屬的東西寫進 dosgolem，
與那邊正在拆分的 `oracle/rich2` 是同一個問題，已經改掉。

沒通的：按鍵送進去之後，p-System 自己裝的 `int 09h`（`9CF0:01CA`）沒有讀埠 `60h`，
系統仍停在環狀緩衝區的空轉迴圈。**p-code 軌跡因此還取不到，
spec 01 也還升不上 `CONFORMED`。**

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
| ~~1~~ | ~~`jmp word ptr cs:[di]` 沒有位移，表卻在檔案偏移 `0x1d56`~~ | **已消除（R3）**：載入器把磁碟 `0x1d56` 起的 512 個位元組搬到映像偏移 0，512/512 相同；`cs` 指著映像基底，所以表就在 `cs:0000` |
| 2 | SIB 的確切版面。`Seg_Base` 佔兩個 word 是推論；`[SIB+2]` 的零檢查、`0x1bee` 呼叫前把 `ds` 換成 `es` 都還沒解釋 | 追出 `0x0fe1`–`0x0ff5` 的完整算術，或在 DOSBox 裡看實際的 SIB 內容 |
| 3 | 檔頭 18 個 word 的用途。三格對上狀態變數偏移，其餘是程式碼位址 | 找到讀這張表的碼。**已知它在載入後被 dispatch 表蓋掉**（R3），所以讀它的一定是載入器 |
| 4 | 內嵌的 33 支機器碼程序分別做什麼 | 逐支反組譯並對上 IV.0 的 intrinsic 清單 |
| 5 | 8 位元組實數的位元格式 | 讀完 `MPR`／`DVR`／`FLT`，或用已知數值在 DOSBox 裡對拍 |
| 6 | `MPR`（`0x24e1`）與 `DVR`（`0x262a`）被 80 條上限截斷 | 提高上限重跑，或改成追控制流而不是線性反組譯 |
| 7 | `ss:3Ch` 指向的記錄版面（堆疊檢查只用了 `+4`） | 找出誰設定 `ss:3Ch` |
| 8 | 把原版當 oracle：跑起來、送輸入、取得 p-code 軌跡 | **改用 `dosgolem`**（見 R3）。已經跑到命令列；還差「按鍵送得進去」與「p-code 軌跡取得出來」 |
| 9 | 段頭第 9、10 個 word（手冊說「保留」）在樣本裡不是零，隨 segment 變化 | 找到寫它的東西（編譯器或 Linker），或在多份不同來源的 codefile 上看出規律 |
| 10 | 430 支常式裡有 12 支的 `EXITIC` 不是直接指到 `RPU` | 逐支反組譯那 12 個離開序列 |

## 下一輪候選

- **#8 先做。** 沒有 oracle，M1 的「兩邊輸出相同」沒有辦法驗，
  spec 01 也升不到 `CONFORMED`。
- 接著是 spec 02：常數池的內部結構（real pool 與 main pool 的夾層版面），
  M0 已經把常數池的位置解出來，只差內容。
- #4（33 支內嵌程序）與 #6（`MPR`／`DVR` 被截斷）是純反組譯工作，可以並行。
