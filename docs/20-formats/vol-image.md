# `.VOL` 磁碟映像：把檔案抽出來

[English](vol-image.en.md) ｜ [日本語](vol-image.ja.md) ｜ **繁體中文**

`psys21`（1984 年的 DOS hosted p-system）的三個磁碟以 `.VOL` 映像散布。
`SYSTEM.PME.86` 就在第一片裡。格式在 IV.0 手冊 p.125 的 Figure 6，
`tools/read-vol.py` 是那份版面的實作。

## 目錄版面

block 大小 512 個位元組，block 0–1 是 bootstrap，**目錄從 block 2 開始**，
佔四個 block：`array [0..77] of direntry`，每筆 26 個位元組，數值一律 little-endian。

第 0 筆描述 volume 本身，第 1 筆之後是檔案：

| 位元組 | volume（第 0 筆） | 檔案（第 1 筆起） |
|---|---|---|
| 0–1 | `dfirstblk` | `dfirstblk` |
| 2–3 | `dlastblk` | `dlastblk` |
| 4–5 | `dfkind` | status ＋ `dfkind` |
| 6–13 | `dvid`（長度位元組 ＋ 7 字元） | — |
| 6–21 | — | `dtid`（長度位元組 ＋ 15 字元） |
| 14–15 | `deovblk` | — |
| 16–17 | `dnumfiles` | — |
| 22–23 | — | `dlastbyte` |
| 24–25 | — | `daccess` |

字串是 UCSD 形式：一個長度位元組後接字元。

`dlastblk` 是「最後一個 block 的下一個」，所以

```
檔案大小 = (dlastblk − dfirstblk − 1) × 512 + dlastbyte
```

`dfkind` 低四位是檔案種類：0 untyped、1 xdsk、2 code、3 text、4 info、5 data、
6 graf、7 foto、8 securedir。

`daccess` 是打包的日期：低四位是月、接著五位是日、最高七位是西元後兩位的年。

```
daccess = month | day << 4 | year << 9
```

拿磁碟上的 `B429`、`AAE1`、`A94C` 三個值用這個式子解，得到 `2-Sep-90`、
`14-Jan-85`、`20-Dec-84`，與 Filer 自己列出來的字串逐字相同。

## 跑起來

```
docker run --rm --network none -u "$(id -u):$(id -g)" \
  -v "$PWD:/work" -v /path/to/vols:/v:ro -w /work python:3.13-alpine \
  python tools/read-vol.py /v/PSYSTEM.VOL
```

```
volume "PSYSTEM"  1000 blocks  29 files
  SYSTEM.PME.86      blk    6–38     16384 B  data
  SYSTEM.PASCAL      blk   68–204    69632 B  data
  SYSTEM.COMPILER    blk  693–803    56320 B  data
  ...
```

加 `-x OUTDIR [檔名…]` 抽檔。`SYSTEM.PME.86` 抽出來是 16384 個位元組，
sha256 `fe427aa66ca8…`——本 repo 所有結論都建立在這個雜湊上。

`PME` 是 P-Machine Emulator，`.86` 是 8086。`SYSTEM.PASCAL` 是作業系統本體
（p-code 寫的），`SYSTEM.COMPILER` 是編譯器。

## 邊界

- status 位元沒有解，本 repo 也用不到。
- 目錄佔四個 block 是照手冊寫的；沒有驗證過「檔案數超過 77 筆」的情形。
- 本 repo **不含任何原版磁碟映像或可執行檔**，只保留位元組層級的結論與雜湊。
