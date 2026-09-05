# `.VOL` ディスク・イメージ：ファイルを取り出す

[English](vol-image.en.md) ｜ **日本語** ｜ [繁體中文](vol-image.md)

`psys21`（1984 年の DOS ホスト版 p-system）の 3 枚のディスクは `.VOL` イメージとして
配布されている。`SYSTEM.PME.86` は 1 枚目にある。フォーマットはマニュアル p.125 の
Figure 6、`tools/read-vol.py` がその版面の実装である。

## ディレクトリの版面

ブロック長は 512 バイト、ブロック 0–1 は bootstrap、**ディレクトリはブロック 2 から**
始まり 4 ブロックを占める——`array [0..77] of direntry`、1 件 26 バイト、
数値はすべて little-endian。

第 0 件はボリューム自身を、第 1 件以降はファイルを記述する：

| バイト | ボリューム（第 0 件） | ファイル（第 1 件以降） |
|---|---|---|
| 0–1 | `dfirstblk` | `dfirstblk` |
| 2–3 | `dlastblk` | `dlastblk` |
| 4–5 | `dfkind` | status ＋ `dfkind` |
| 6–13 | `dvid`（長さバイト ＋ 7 文字） | — |
| 6–21 | — | `dtid`（長さバイト ＋ 15 文字） |
| 14–15 | `deovblk` | — |
| 16–17 | `dnumfiles` | — |
| 22–23 | — | `dlastbyte` |
| 24–25 | — | `daccess` |

文字列は UCSD 形式——長さバイト 1 個に文字が続く。

`dlastblk` は「最後のブロックの次」なので、

```
ファイル長 = (dlastblk − dfirstblk − 1) × 512 + dlastbyte
```

`dfkind` の下位 4 ビットがファイル種別：0 untyped、1 xdsk、2 code、3 text、4 info、
5 data、6 graf、7 foto、8 securedir。

`daccess` はパックされた日付である。下位 4 ビットが月、続く 5 ビットが日、
上位 7 ビットが西暦下 2 桁の年。

```
daccess = month | day << 4 | year << 9
```

ディスク上の `B429`、`AAE1`、`A94C` の 3 値をこの式で解くと `2-Sep-90`、`14-Jan-85`、
`20-Dec-84` になり、Filer 自身が並べる文字列と一字一句一致する。

## 走らせる

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

`-x OUTDIR [ファイル名…]` を付けると抽出できる。`SYSTEM.PME.86` は 16,384 バイト、
sha256 `fe427aa66ca8…`——この repo のすべての結論はこのハッシュの上に立っている。

`PME` は P-Machine Emulator、`.86` は 8086 の意。`SYSTEM.PASCAL` が OS 本体
（p-code で書かれている）、`SYSTEM.COMPILER` がコンパイラである。

## 境界

- status ビットは未解。この repo でも使わない。
- 「ディレクトリが 4 ブロックを占める」はマニュアルの記述どおりで、
  ファイル数が 77 件を超える場合は検証していない。
- この repo は**オリジナルのディスク・イメージや実行ファイルを一切含まない**。
  バイト水準の結論とハッシュだけを残している。
