# Parhelion PME

幻日 —— 用 Go 重做的 UCSD p-System，以及支撐它的知識庫。

一片 1984 年的 `.VOL` 磁碟映像丟進去，p-System 自己開到命令列，
按鍵有反應，Filer 列得出目錄。**沒有原版直譯器，沒有 DOS，沒有 8086 模擬器。**

```
$ go run ./cmd/boot -vol PSYSTEM.VOL
Copyright 1979 U.C. Regents; Copyright 1985 SofTech Microsystems
Startup Utility - [1R1.0]
PSYSTEM:SYSTEM.MISCINFO ---> RAMDISK:SYSTEM.MISCINFO
PSYSTEM:SYSTEM.PASCAL   ---> RAMDISK:SYSTEM.PASCAL
PSYSTEM:SYSTEM.EDITOR   ---> RAMDISK:SYSTEM.EDITOR
PSYSTEM:SYSTEM.FILER    ---> RAMDISK:SYSTEM.FILER
PSYSTEM:SYSTEM.LIBRARY  ---> RAMDISK:SYSTEM.LIBRARY
Root is RAMDISK
Prefix is RAMDISK
SYSTEM.PASCAL is on RAMDISK
Command: E(dit, R(un, F(ile, C(omp, L(ink, X(ecute, A(ssem,? [IV.2.1 R3.3]

走了 226624 條，停在等鍵盤
```

那五行 `--->` 是作業系統**自己**把系統檔複製到記憶體磁碟上。
打字進去它會動：

```
$ go run ./cmd/boot -vol PSYSTEM.VOL -keys $'FLRAMDISK:\r'
Filer: L(dir, R(em, C(hng, T(rans, D(ate, Q(uit, B(ad-blks, E(xt-dir,? [6R4.0]
Dir listing of what vol ? RAMDISK:

RAMDISK:
SYSTEM.MISCINFO    2  2-Sep-90           SYSTEM.PASCAL    136 14-Jan-85
SYSTEM.EDITOR    106  9-Dec-85           SYSTEM.FILER      45 20-Dec-84
SYSTEM.LIBRARY   102 27-Dec-84
5/5 files<listed/in-dir>, 397 blocks used, 353 unused, 353 in largest
```

## 這台機器現在做得到什麼

| | |
|---|---|
| 執行 p-code | **210 個指令全部實作**，包含浮點、集合、packed 欄位、字串 |
| 跨段呼叫與返回 | 九族呼叫指令、`EXIT` 拆框走 EXITIC、activity 與參考計數 |
| 多工 | 號誌、ready queue、依優先權換 task |
| 段載入 | 段不在記憶體就發 segment fault，叫醒作業系統的載入 task 去磁碟讀 |
| 磁碟 | `.VOL` 映像讀寫、目錄解析、記憶體磁碟 |
| 主控台 | 80×25 的終端機模擬（游標定位、清行、清畫面、捲動）；輸入沒東西就交回去等，補上字再從同一條指令繼續 |
| 裝置組態 | 解 `SYSTEM.CONFIG`，unit 編號與驅動由設定檔決定 |
| 開機 | 從磁碟把作業系統的起始段載進來，擺好 SIB／E_Rec／E_Vec／TIB／第一個活動記錄 |

跑得動的東西：p-System 的 **Startup Utility** 與 **Filer**（列目錄、換磁碟區）。

## 界線在哪裡

界線不是我畫的，是原版自己畫的——直譯器有一張 48 格的表
（映像 `0x1F56`），列出「這一支不是 p-code，是宿主的機器碼」。

| 這一側 | 誰做 | 狀態 |
|---|---|---|
| 210 個 p-code 指令 | `internal/pmachine` | 全部 |
| 段 1 的內嵌原生程序 | `internal/psystem` | 相異 26 支裡做了 21 支 |
| 磁碟、主控台、時鐘、記憶體磁碟 | `internal/psystem` | 開機與 Filer 用得到的都有 |
| `NAT`（程式碼段裡的 8086 機器碼） | — | **結構上做不到**，要一台 8086 |

還沒做的原生程序：`31`／`33`（裝置模式碼 3）、`32`、`37`、
`47`（換一組浮點常式——它會改寫 `0x1F56` 那張表本身）。整段開機不需要它們。

**unit 128 是 DOS 主機的檔案系統閘道**，讓 p-System 直接讀寫底下那台 DOS 的
檔案。Go 宿主底下沒有 DOS，所以答案永遠是「沒有」——與 `NAT` 同一種界線，
不是還沒做。往來的形狀量出來寫進 [spec 03](docs/30-remake/specs/03-boot.md) 了。

資料段低位址那一塊的來源查清楚了：**它是直譯器自己的初始資料**——
`SYSTEM.PME.86` 檔案開頭那 512 個位元組原封不動搬進資料段的同一個偏移，
非零的 37 個 word 一個不差。內容是直譯器對作業系統的自我介紹
（SYSCOM 位址、幾個狀態偏移、四個原生回呼的進入點）。
**bootstrap 自己算的只剩四格**：開機參數區的位址、資料段的實體位址，
以及 TIB 的堆疊上下界。

## 怎麼確定它是對的

四個指標，各問不同的問題。**前兩個不需要原版**：

| 測試 | 問什麼 | 現在 |
|---|---|---|
| `TestBootReachesTheCommandLine` | 有沒有開起來 | 開到命令列，停在等鍵盤 |
| `TestFilerListsTheRAMDisk` | 用不用得動 | `F` → `L` → 畫面上五個檔案排在對的位置 |
| `TestSelfBootMatchesTheOriginal` | 跑出來的 p-code 一不一樣 | **226,623 條逐條相同**，整段開機 |
| `TestSelfBootInitialStateDiff` | 開機那一刻的狀態建對了沒 | 409 段不同，其中 397 段是宿主的機器碼 |

後兩個把原版擺在旁邊逐條對——這是這個專案的方法，也是名字的由來。
另外還有一組**共生對拍**：讓原版驅動，我們跟著走，
每一條比 `IPC`、`SP`、`TOS`、`E_Rec` 四項：

```
$ tools/go.sh run -tags oracle ./cmd/parity -pme .../SYSTEM.PME.86
兩邊一致地走了 224999 條 p-code
另有 1624 條交給原版自己走（宿主的工作，**沒有驗證**）： 70 SCXG1×1617 94 CXG×7
停下來的原因： oracle: 原版沒有再執行 p-code（多半停在等輸入的迴圈）

用到 173 種 opcode：…
```

交出去的那 1,624 條是 p-machine 依定義就該交出去的兩種：段 1 的內嵌原生程序、
段還沒載入。**p-code 指令沒實作不算**——那種也放行的話，
「還沒做」會看起來像「做完了」。

一個順帶的獨立交叉驗證：Filer 列出來的日期 `2-Sep-90`、`14-Jan-85`、`20-Dec-84`，
與磁碟上 `daccess` 欄位（`B429`、`AAE1`、`A94C`）用我們自己的解碼器算出來的
逐一對得上——**兩邊獨立，答案相同。**

### 走不到的指令另外對拍

開機那條路只用得到 173 種 opcode——浮點 16 支一次都沒碰到，
因為作業系統開機不算實數。那些指令改用**單條對拍**驗：
指令位元組與運算元由我們指定，寫進原版的程式碼段與堆疊，兩邊各走一條逐 word 比。

```
ADR ＝ 4.75      MPR ＝ 1.0000000000000001e-305     DVR ＝ 0.3333333333333333
MODI(-17,3) ＝ 1        RND(-3.5) ＝ -4        LESTR("AB","ABC") ＝ 1
```

55 個案例全部相同，**對拍覆蓋率因此到 205／211**。剩下 6 種：
`CPI`／`CXL`／`CPF`（真的會跳進另一支程序）、`BPT`（故意觸發執行期錯誤）、
`NAT`（結構上做不到）、`LDCRL`（運算元指常數池，擺進去會踩壞它）。

### 對拍沒說到的地方

對拍比的是四個純量，不是整個堆疊。**寫錯了堆疊深處而後來沒有人讀它**，
這種錯抓不到。

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

## 怎麼跑

`tools/go.sh` 把 Go 包在 docker 裡，`tools/ci.sh` 是本機 CI。
需要外部素材的檢查會跳過，**而且會說它跳過了**。

```sh
tools/ci.sh                                   # 只跑不需要素材的部分

PARHELION_CODEFILE=/src/workplace/SYSTEM.PASCAL \
PARHELION_ORIG=~/cht/p-code/psys21/"psystem 1984" \
PARHELION_PME=/src/workplace/SYSTEM.PME.86 \
PARHELION_DOSGOLEM=~/cht/dosgolem-psys \
  tools/ci.sh                                 # 全部
```

原版素材（`.VOL`、`PSYSTEM.COM`、抽出來的 `SYSTEM.PME.86`）由使用者自備，
缺檔就跳過——**不自製代用品**，安靜的替代品會讓「還沒驗」看起來像「驗過了」。

| 指令 | 做什麼 |
|---|---|
| `cmd/boot` | 吃一片 `.VOL`，把 p-System 開起來 |
| `cmd/parhelion` | 讀 codefile：段字典、常式表、byte sex |
| `cmd/pcode` | 反組譯一段 p-code |
| `cmd/parity` | 共生對拍（要 `-tags oracle`）|
| `cmd/bootdump`／`cmd/whowrote`／`cmd/ioprobe`／`cmd/hostprobe`／`cmd/segprobe` | 量原版實際行為的探針（要 `-tags oracle`）|

對拍那一層底下是 [`dosgolem`](https://github.com/wicanr2/dosgolem)——同一個作者的
無頭決定性 DOS 執行器。`oracle/` 認得 p-machine（怎麼定位直譯器、怎麼判斷
dispatch 目標、怎麼讀 p-code 軌跡），dosgolem 只提供通用能力。
**跑起來的那一側不 import 它**：原版只出現在測試裡。

## 知識庫

實作是照著這些結論寫的，不是反過來。

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

格式：[`.VOL` 磁碟映像](docs/20-formats/vol-image.md)、
[`SYSTEM.PME.86` 的檔頭](docs/20-formats/pme86-header.md)。

重做：[可行性評估](docs/30-remake/feasibility.md)、
[spec 閘門](docs/30-remake/spec-workflow.md)、
[spec 01 codefile](docs/30-remake/specs/01-codefile.md)（`CONFORMED`）、
[spec 02 執行核心](docs/30-remake/specs/02-pmachine-core.md)（`CONFORMED`）、
[spec 03 自己開機](docs/30-remake/specs/03-boot.md)（`DRAFT`）。

分輪進度、被推翻的舊斷言、開放項目清單都在 [`PLAN.md`](PLAN.md)。

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
- 開機走的是**這一片磁碟、這一套裝置組態**。換一片磁碟或換一套組態
  會走到還沒驗過的路上。

## 授權

文件與圖採 CC BY 4.0。反組譯產出（`tools/routines-86.json`）是對
SofTech Microsystems 及其後續權利人所有之程式的分析結果，收錄為技術研究之用。
Go 實作的授權在開工時另行決定。
