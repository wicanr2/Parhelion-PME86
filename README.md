# Parhelion PME

幻日 —— UCSD p-System p-machine 的重製，以及它的知識庫。

`SYSTEM.PME.86` 是 1984 年 DOS 版 UCSD p-system 的 p-machine 直譯器，16384 個位元組。
這個 repo 做兩件事：**把它逐支拆開寫清楚**，然後**照著這些結論用 Go 重做一台**。

知識庫在前，實作在後。理由很直接——沒有寫下來的理解會在寫程式的過程中
悄悄變成猜測，而猜測看起來和知識長得一模一樣。

## 名字

要從一款 1985 年的 Atari ST 遊戲說起。**SunDog: Frozen Legacy** 的遊戲邏輯
不是 68000 機器碼，是 p-code——逆向它的過程就是這整件事的起點。

**sun dog 的正式名稱是 parhelion（幻日）**：大氣中的冰晶把陽光折射出去，
在真太陽旁邊二十二度的位置形成第二顆太陽。它不是太陽，但亮到會被誤認，
而且**只在真太陽也在的時候出現**。

模擬器就是這樣的東西：一個逼真到會被誤認的假像，而且它的正確性只能靠真品來定義。
這個專案的驗證方式正是把兩顆太陽擺在一起——同一份 codefile 餵給原版與 Go 版，
輸出必須一致。

`PME` 沿用原版自己的檔名（`SYSTEM.PME.86`，P-Machine Emulator），
`86` 是 8086。名字紀念的是起點；**題目是 1984 年 DOS 版的 8086 直譯器，
不是 SunDog 那份 68000 直譯器。**

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
- [spec 01：codefile 靜態結構](docs/30-remake/specs/01-codefile.md) — `CONFORMED`。
- [spec 02：p-machine 執行核心](docs/30-remake/specs/02-pmachine-core.md) — `CONFORMED`。
- [spec 03：自己開機](docs/30-remake/specs/03-boot.md) — `DRAFT`。

**M0 做完。M1：210 個指令全部實作，對拍走完整段開機——224,987 條 p-code
與原版逐條一致，0 個分歧。M2 開工：不靠原版也不靠 DOS，
[自己從磁碟映像開機](docs/30-remake/specs/03-boot.md)。
跨段呼叫已完成並驗過**——45 次換段逐一與原版對過，對拍實際走過一次換段。

```
$ go run ./cmd/parhelion codefile -r SYSTEM.PASCAL
69632 位元組（136 blocks），28 個 segment
Copyright 1979 U.C. Regents; Copyright 1985 SofTech Microsystems

段名        blk  words  段號  種類        機器        版本  sex  常數池   R  常式  無碼  外層
KERNEL    16   2150   1   Unit_Seg  M_Psuedo  IV  同    1918  4  66  26
GOTOXY    25   63     2   Unit_Seg  M_Psuedo  IV  同    58    4  2   0
...
合計 465 支常式，其中 35 支沒有碼；6 個 segment 的 byte sex 與主機相反
```

最後那一行是這一輪最值得記的發現：**作業系統自己的 codefile 裡就混著兩種位元組序。**

**原版跑得起來了**，可以當差分測試的 oracle。底下是
[`dosgolem`](https://github.com/wicanr2/dosgolem)——同一個作者的無頭決定性
DOS 執行器。`oracle/` 這一層認得 p-machine（怎麼定位直譯器、怎麼判斷
dispatch 目標、怎麼讀 p-code 軌跡），dosgolem 只提供通用能力。

```
=== oracle：把原版跑起來
--- PASS: TestLoaderMovesTheDispatchTableToOffsetZero
      映像基底 01400h，dispatch 表 512／512 byte 相同
--- PASS: TestExecutedCodeMatchesWhatTheReaderParses
      段 "USERPROG"：記憶體 1251h，檔案 block 1、3831 words、41 支常式
      400 條 p-code 的 opcode 與 codefile 逐位元組相同
```

對拍逐條比 `IPC`、`SP`、`TOS`：

```
$ tools/go.sh run -tags oracle ./cmd/parity -pme .../SYSTEM.PME.86
兩邊一致地走了 224987 條 p-code
另有 1636 條交給原版自己走（宿主的工作，**沒有驗證**）： 70 SCXG1×1617 DE SIGNAL×5 DF WAIT×7 94 CXG×7
停下來的原因： oracle: 原版沒有再執行 p-code（多半停在等輸入的迴圈）

用到 173 種 opcode：…
```

停下來是因為**原版沒事做了**——開機跑完，系統停在等鍵盤的迴圈。

交給原版走的 1,636 條是 p-machine 依定義就要交出去的四種：段 1 的內嵌原生程序、
`NAT`（跳進 8086 機器碼）、段還沒載入、換 task。
**p-code 指令沒實作不算**，不然「還沒做」會看起來像「做完了」。

軌跡讀起來就是 Pascal：

```
1251:0B08 23 SLDL4     1251:0AFA 21 SLDL2
1251:0B09 26 SLDL7     1251:0AFB 98 LDCN
1251:0B0A B2 LEQI      1251:0AFC B1 NEQI
1251:0B0B D5 FJPL      1251:0AFD D5 FJPL
```

左邊是迴圈條件，右邊是 `while p <> nil`。

## 自評：這台機器現在到什麼程度

結論先講：**能逐條跑 p-code，還不能自己開機。**
它目前是掛在原版旁邊的第二顆太陽——正確性由原版定義，也還離不開原版。

### 這個數字證明了什麼

12,924 條不是為了跑分寫的測試程式，是**作業系統自己的開機碼**。
一路上有跨段呼叫、packed 欄位、集合、字串比較、`XJP` 跳表，
每一條都比對 `IPC`、`SP`、`TOS` 三項，45 次換段另外比對四項。
沒有預設哪些指令會出現，出現什麼就得對什麼。

### 這個數字沒有證明什麼

**用到的是 130 種 opcode，剩下 80 種對拍一次都沒碰到。**
開機路徑不需要它們——作業系統開機不算實數：

| 沒被對拍走過的 | 幾支 | 現在靠什麼撐著 |
|---|---:|---|
| 浮點 | 16 | 單元測試 + 讀原版的浮點助手 |
| 跨段呼叫的其餘形式（`CXL`／`CXG`／`CXI`／`CPF`／`CPI`） | 5 | 單元測試；只有 `SCXG1`／`SCXG2` 被實際走過 |
| 集合與字串比較（`EQPWR`／`LEPWR`／`GEPWR`／`INN`／`LESTR`／`GESTR`） | 6 | 單元測試 |
| `DVI`／`MODI`／`CHK`／`ABI`／`SWAP`／`NOP`／`BPT` 等零星 | 其餘 | 單元測試 |

**單元測試不是獨立證據**——它和實作出自同一份理解，我讀錯原版的話兩邊會一起錯。
真正的證據是對拍，而對拍對這 80 支還沒說過話。

另外，對拍比的是三個純量，不是整個堆疊與資料段。
**寫錯了堆疊深處、而後來沒有人讀它**，這種錯抓不到。

### 還差多少才算「一台能用的 p-machine」

| 缺的東西 | 現況 |
|---|---|
| 段 1 的 26 支內嵌原生程序 | 對拍時交給原版走；自己跑就沒有 |
| `CSP`（檔案系統、主控台、磁碟） | 同上 |
| 從磁碟把 segment 載進 codepool | 沒做。碰到不在記憶體的段就回 `ErrNotResident` |
| 換 task 的排程器 | 只做了號誌的非阻塞路徑 |
| `NAT` | 要一台 8086 才做得到，不是寫 p-code 能補的 |

前三項是同一件事的三個面向：**這台 p-machine 還沒有作業系統。**
`SYSTEM.PASCAL` 是 p-code 寫的，原則上跑得起來，
但它一開口就要檔案系統，而檔案系統在直譯器的機器碼裡。

### 最花力氣的不是那 210 支指令

是建立一條**能逐條問「你錯在哪」的迴路**。指令本身多半半小時一支，
而迴路一旦建好，錯誤會自己報上位置。

一個具體例子值得記：Go **不定義 `a + b` 的求值順序**，
`s.IPC + 1 + int16(s.fetch())` 裡的 `s.fetch()` 可能先跑，`s.IPC` 就多讀了一個位元組。
所有跳躍指令因此差一格。單元測試抓不到——我寫測試時用的是同一個錯誤理解，
期望值跟著錯。**對拍在第 101 條就把它指出來了**，因為原版不會跟著我一起錯。

這是差分測試相對單元測試的價值：**它的期望值不是我寫的。**

## 怎麼跑

`tools/go.sh` 把 Go 包在 docker 裡，`tools/ci.sh` 是本機 CI。
需要外部東西的檢查會跳過，**而且會說它跳過了**。

```sh
tools/ci.sh                                   # 只跑不需要素材的部分

PARHELION_CODEFILE=/src/workplace/SYSTEM.PASCAL \
PARHELION_ORIG=~/cht/p-code/psys21/"psystem 1984" \
PARHELION_PME=/src/workplace/SYSTEM.PME.86 \
PARHELION_DOSGOLEM=~/cht/dosgolem-psys \
  tools/ci.sh                                 # 全部
```

單元測試用的是合成的 codefile，不含任何原版資料。原版素材（`.VOL`、
`PSYSTEM.COM`、抽出來的 `SYSTEM.PME.86`）由使用者自備，缺檔就跳過——
**不自製代用品**，安靜的替代品會讓「還沒驗」看起來像「驗過了」。

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
- Go 實作 210 支指令全有，但**對拍只走過其中 130 支**；其餘只有單元測試，
  而單元測試與實作同源。見上面的自評。

## 授權

文件與圖採 CC BY 4.0。反組譯產出（`tools/routines-86.json`）是對
SofTech Microsystems 及其後續權利人所有之程式的分析結果，收錄為技術研究之用。
Go 實作的授權在開工時另行決定。
