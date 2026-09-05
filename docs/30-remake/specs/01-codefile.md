# spec 01：codefile 與 code segment 的靜態結構

| | |
|---|---|
| 狀態 | `CONFORMED` |
| 日期 | 2026-09-05（`READY`）→ 2026-09-05（`CONFORMED`）|
| 涵蓋 | segment dictionary 鏈、code segment 表頭、routine dictionary、byte sex |
| 不涵蓋 | 常數池內部結構、relocation list、segment reference list、linker info、interface text |
| 驗證樣本 | `SYSTEM.PASCAL`（69632 B，136 blocks，psys21 磁碟），28 個 segment 全數解析 |

出處標記：**[手冊]** 指 IV.0 Internal Architecture Guide 的印刷頁碼；
**[PME]** 指 `SYSTEM.PME.86` 的檔案偏移；**[樣本]** 指對 `SYSTEM.PASCAL` 實測；
**[remake]** 指這個實作自己的決定。

## 1. 檔案分塊

1.1 codefile 以 512 位元組的 block 為單位。segment 的碼一律從 block 邊界開始。**[手冊 p.30]**

1.2 block 0 是 segment dictionary 的第一筆記錄。**[手冊 p.27]**

## 2. segment dictionary 記錄（512 位元組）

2.1 一筆記錄描述最多 16 個 segment，同一個 segment 的資訊分散在六個平行陣列裡，
用同一個索引取值。**[手冊 p.27]**

2.2 記錄的位元組版面（由 p.28–29 的 Pascal 宣告逐欄推算，總長剛好 512）：

| 位元組 | 欄位 | 每項大小 |
|---|---|---|
| `0x000`–`0x03f` | `Disk_Info[16]`：`Code_Addr`（起始 block）、`Code_Leng`（word 數） | 4 |
| `0x040`–`0x0bf` | `Seg_Name[16]`：8 個字元，未用項填空白 | 8 |
| `0x0c0`–`0x0df` | `Seg_Misc[16]` | 2 |
| `0x0e0`–`0x0ff` | `Seg_Text[16]`：interface text 起始 block | 2 |
| `0x100`–`0x11f` | `Seg_Info[16]` | 2 |
| `0x120`–`0x19f` | `Seg_Famly[16]` | 8 |
| `0x1a0`–`0x1a1` | `Next_Dict`：下一筆記錄的 block 號，0 表示沒有 | 2 |
| `0x1a2`–`0x1af` | 保留 7 個 word | — |
| `0x1b0`–`0x1fd` | `Copy_Note`：`string[77]`（長度位元組後接字元） | — |
| `0x1fe`–`0x1ff` | `Sex`：固定 1 | 2 |

**[手冊 p.28–32]**，版面由宣告推算。
**[樣本]** `Copy_Note` 讀出 `"Copyright 1979 U.C. Regents; Copyright 1985 SofTech Microsystems"`，
`Sex` 讀出 1——兩個位於記錄最尾端的欄位都對，表示中間所有欄位的大小加總正確。

2.3 `Code_Leng` 是 16-bit word 數，**含 relocation list、不含 segment reference list**。**[手冊 p.30]**

2.4 dictionary 是鏈結串列，後續記錄各佔一個 block、夾在 code segment 之間。**[手冊 p.27]**
**[樣本]** `SYSTEM.PASCAL` 有兩筆：block 0（16 項）與 block 133（12 項），共 28 個 segment。

2.5 `Seg_Misc` 的位元配置（`Seg_Type` 3 bits、`Filler` 5 bits、`Has_Link_Info` 1 bit、
`Relocatable` 1 bit，依宣告順序由低位往高位）：

| bit | 欄位 |
|---|---|
| 0–2 | `Seg_Type`：0 `No_Seg`、1 `Prog_Seg`、2 `Unit_Seg`、3 `Proc_Seg`、4 `Seprt_Seg` |
| 3–7 | `Filler` |
| 8 | `Has_Link_Info` |
| 9 | `Relocatable` |

**[手冊 p.28]** 的宣告順序；「由低位往高位」由 2.6 的 `Seg_Info` 獨立確認。
**[樣本]** 低三位讀出 `Unit_Seg` 或 `Proc_Seg`，與 2.7 由 `Seg_Famly` 判定的型別完全一致。

> ⚠ **bit 8 與 bit 9 的語意未經驗證。** 樣本裡 28 個 segment 的 `Seg_Misc`
> 只有 `0x0202` 與 `0x0203` 兩個值，bit 9 恆為 1、bit 8 恆為 0——**沒有變異就沒有鑑別力**。
> 而且 28 個 segment 的 relocation list 指標全部是 0，與「bit 9 = `Relocatable`」
> 對不太起來。實作照宣告順序解，但不依賴這兩個 bit 做任何判斷。

2.6 `Seg_Info` 的位元配置（合計剛好 16 bit）：

| bit | 欄位 |
|---|---|
| 0–7 | `Seg_Num`：本地 segment number |
| 8–11 | `M_Type`：0 `M_Psuedo`、2 `M_PDP_11`、3 `M_8080`、9 `M_8086`、11 `M_68000` … |
| 12 | `Filler` |
| 13–15 | `Major_Version`：4 = `IV` |

**[手冊 p.28]**。
**[樣本]** `KERNEL` 讀出 `0x8001` → `Seg_Num` 1、`M_Type` `M_Psuedo`、版本 `IV`。
若改成由高位往低位配置，`Seg_Num` 會是 128 而不是 1——**這一項同時確認了整份宣告的
packed record 打包方向**，2.5 依賴這個結論。

2.7 `Seg_Famly` 是變體記錄：`Prog_Seg`／`Unit_Seg` 是四個整數
（`Data_Size`、`Seg_Refs`、`Max_Seg_Num`、`Text_Size`）；
`Proc_Seg`／`Seprt_Seg` 是外層 program／unit 的 8 字元名字。**[手冊 p.29]**
**[樣本]** `ASSOCQUI`（`Proc_Seg`）讀出 `"ASSOCIAT"`、`SEGFOPEN` 讀出 `"FILEOPS "`、
`USERPROG` 讀出 `"KERNEL  "`——全部對應到同一個檔案裡存在的 segment。

## 3. code segment 表頭（11 個 word）

3.1 表頭在 segment 的最低位址：**[手冊 p.8]**

| word | 位元組 | 欄位 |
|---|---|---|
| 0 | `0x00` | routine dictionary 指標（段內 word 偏移） |
| 1 | `0x02` | relocation list 指標 |
| 2–5 | `0x04` | 段名，8 個字元 |
| 6 | `0x0c` | byte sex 指示字 |
| 7 | `0x0e` | 常數池指標（段內 word 偏移），0 表示沒有常數池 |
| 8 | `0x10` | `REALSIZE`：2 或 4 個 word |
| 9–10 | `0x12` | 保留 |

3.2 三個指標都是**段內相對位移**，因為 Codepool 會把整段搬走。**[手冊 p.9]**

3.3 **[PME]** 8086 直譯器切段時讀的正是這三格：段內 `+0x00` 乘二後存進 `ss:36h`
（程序字典基底）、`+0x0c` 存進 `ss:44h`（byte sex）、`+0x0e` 乘二後存進 `ss:42h`
（常數池基底）。**手冊與實作在三個獨立的偏移上互相印證。**
證據見 `docs/10-interpreter/segment-switching.md` 的助手 `0x0fba`。

3.4 **[樣本]** 28 個 segment 的表頭全部解析成功：段名可讀、`REALSIZE` 一律是 4
（對應 `$R4`，也就是 64-bit 實數，與 `LDCRL` 在堆疊上開 8 個位元組一致）。

3.5 **[樣本]** 28 個 segment 的 routine dictionary 指標**恆等於 `Code_Leng − 1`**。
這是觀察到的不變量，不是手冊寫的；實作**不得**依賴它，但可以拿它當健全性檢查。

> **保留的兩個 word 不是零。** 樣本裡它們隨 segment 變化（例如 `KERNEL` 是
> `(4184, 38466)`、`GOTOXY` 是 `(4199, 5733)`），同一個編譯單元的幾個 segment
> 常有相同的值。用途未知，列為開放項目。

## 4. byte sex

4.1 表頭的 byte sex 指示字在產生這一段的機器上**恆為 1**；以相反的位元組序讀會變成 256。
**[手冊 p.10]**

4.2 受影響的 word 導向資訊分兩類：**superstructure**（例如 routine dictionary）由作業系統
在載入時翻轉；**embedded**（`LDC` 取的常數、`XJP` 的 case 表）由直譯器在執行時翻轉。
**[手冊 p.10]**

4.3 **[PME]** 直譯器那一半確實存在：`ss:44h` 不等於 1 時，`MOV`、`LDC`、`XJP`
走逐 word 交換位元組的路徑。

4.4 **[樣本]** `SYSTEM.PASCAL` 的 28 個 segment 裡有 **6 個**（`STRINGOP`、`HEAPOPS`、
`LOCK`、`COMMANDI`、`PERMHEAP`、`SMALLCOM`）的指示字讀出 256。
**同一個 codefile 裡混著兩種位元組序，而且是作業系統本身。**
所以讀取器從第一天就要處理 byte sex，不能當成之後再補的邊緣情形。

4.5 **[remake]** 這個實作**不在原地翻轉 segment**。表頭與 routine dictionary 在解析時
依 byte sex 解碼成 Go 的值，常數池與碼保持原樣，byte sex 旗標隨 segment 保存下來，
留給執行期依 4.2 的分工處理。

原版是把載入到記憶體的那一份就地改掉；不照做的理由是 remake 沒有「就地」這個概念，
而且保留原始位元組才有辦法對拍。**這是表示法的選擇，不改變任何可觀察行為。**

## 5. routine dictionary

5.1 段的第一個 word 指向 routine dictionary。字典是一串指向常式碼的指標，
每一個都是段內 word 偏移。字典的 word 0 是段內常式的個數。
常式編號 1..255，編號就是索引。**[手冊 p.10]**

5.2 **字典往位址低的方向長**：常式 n 的指標在段內 word `dict − n`。

- **[PME]** 呼叫助手 `0x101b` 算的是 `word_36 − 2×程序號`，而 `word_36` 是
  `2 × 表頭第 0 個 word`。
- **[樣本]** 28 個 segment 用這個方向解析，全部 465 個字典項沒有一個越界；
  反方向會立刻越過段尾。

5.3 `EXTERNAL` 與 `FORWARD` 常式的字典項是 0。**[手冊 p.10]**
**[樣本]** 28 個 segment 裡有 35 個空項，其中 `KERNEL` 一段就佔 26 個。
**[PME]** 直譯器對字典項 0 的處理是跳執行期錯誤（`0x101b` 的 `test di, di`）。

5.4 一支常式在段內的版面是 `EXITIC`、`DATASIZE`、第一條指令三者相鄰，
**字典項指的是中間的 `DATASIZE`**：

| 段內 word | 內容 |
|---|---|
| `ptr − 1` | `EXITIC`：離開這支常式時要執行的碼，段內位元組偏移 |
| `ptr` | `DATASIZE`：區域資料 word 數，不含參數 |
| `ptr + 1` | 第一條可執行指令 |

- **[手冊 p.11]** 說常式的碼「由兩個 word 開頭——`DATASIZE` 與 `EXITIC`」，
  同一段又說「第一條可執行指令緊接在 `DATASIZE` 之後」。
  前一句讀起來像 `DATASIZE` 在前，後一句只有在 `EXITIC` 在前時才成立——
  **手冊自己沒有把順序講死。**
- **[PME]** 呼叫助手 `0x103a` 講死了：`shl di, 1` 把字典項換成位元組偏移之後
  `mov cx, es:[di]` 取的是 `DATASIZE`，接著 `inc di; inc di` 才指到要執行的碼。
  下一條 `or cx, cx; js` 就是原生碼的正負判斷。
- **[樣本]** 430 支有碼的常式，把 `w(ptr)` 當 `DATASIZE` 讀：中位數 3、p90 76、
  最大 5121，沒有一支是負的；把 `w(ptr−1)` 當 `EXITIC` 讀：**沒有一支越界，
  而且 418 支直接指到位元組 `0x96`，也就是 `RPU`**。
  改成手冊字面順序（`EXITIC` 在 `ptr+1`）會有 **299 支越界、0 支指到 `RPU`**。

5.5 `DATASIZE` 為負表示第一條指令是原生碼，值是 one's complement，
此時 `EXITIC` 未定義、不應該去讀。**[手冊 p.11]**
**[樣本]** `SYSTEM.PASCAL` 的 430 支常式沒有一支是負的，
與 dictionary 把 28 個 segment 全部標成 `M_Psuedo` 一致。

## 6. 這份 spec 授權實作什麼

讀取器可以做到：列出 codefile 的所有 segment、每一段的名稱／編號／型別／機器型別／
byte sex／常數池位置／`REALSIZE`、以及每一支常式的碼起點與 `DATASIZE`／`EXITIC`。

**不授權**：解讀常數池內容、relocation list、segment reference list，
以及任何執行語意。那些各自需要自己的 spec。

## 7. 同狀態驗證

`CONFORMED` 的門檻不是「測試過了」，是**在與原版相同的狀態下比對過**。
做法是把 1984 年 DOS 版的 p-System 跑起來（見 `oracle/`），
在它執行 `SYSTEM.PASCAL` 的 p-code 時逐條記錄，再與讀取器解出來的東西對拍。
測試在 `oracle/conform_test.go`。

**驗到的：**

- 原版在跑的 code segment，表頭第 4–11 個位元組讀出來是 `"USERPROG"`——
  讀取器從同一份檔案解出來的段名裡有它（3.1 的段頭版面）。
- 那個段在記憶體裡的內容與檔案裡的 `Code_Leng` 個 word **99% 以上相同**
  （2.3 的長度定義、3.2 的段內相對位移）。
- **400 條 p-code 的每一個 IPC，記憶體裡那個位元組與檔案裡同一個位移的位元組
  逐一相同**。這一條把「讀取器算出來的段內位移」與「原版取指令的位址」綁在一起——
  差一個 word 就會整片對不上。
- 每一個 IPC 都落在表頭之後、routine dictionary 之前（3.1、5.1）。

**沒驗到的**（同樣要寫出來，沒驗 ≠ 錯）：

- `Seg_Misc` 的 bit 8／bit 9 語意（2.5 已經標成未驗證，樣本沒有變異）。
- `Seg_Famly` 的變體記錄（2.7 只有靜態的自洽佐證）。
- byte sex 相反的段，作業系統翻轉 routine dictionary 的細節（4.2）——
  軌跡目前只碰到 `USERPROG` 一個段，而它與主機同序。
- `DATASIZE`／`EXITIC`（5.4）是靠直譯器的碼與 430 支常式的統計定下來的，
  不是在執行中攔到呼叫序列量的。

## 8. 這一輪留下的問題

- 段頭第 9、10 個 word（手冊說「保留」）在樣本裡不是零，而且隨 segment 變化。
  第一支常式的 `EXITIC` 通常落在 word 11、`DATASIZE` 在 word 12，所以這兩個 word
  確實不屬於任何常式。用途未知。
- 430 支常式裡有 12 支的 `EXITIC` 不是直接指到 `RPU`（指到 `0x30`、`0x84`、`0x60` 這些）。
  合理的解釋是離開序列不只一條指令，但沒有逐支讀過。
