# `.VOL` disk images: getting the files out

**English** ｜ [日本語](vol-image.ja.md) ｜ [繁體中文](vol-image.md)

The three disks of `psys21` (the 1984 DOS-hosted p-system) are distributed as `.VOL`
images. `SYSTEM.PME.86` is on the first one. The format is Figure 6 on manual p.125, and
`tools/read-vol.py` is an implementation of that layout.

## Directory layout

Blocks are 512 bytes, blocks 0–1 are the bootstrap, and **the directory starts at block
2**, occupying four blocks: `array [0..77] of direntry`, 26 bytes each, all values
little-endian.

Entry 0 describes the volume itself; entry 1 onward are files:

| Bytes | volume (entry 0) | file (entry 1 onward) |
|---|---|---|
| 0–1 | `dfirstblk` | `dfirstblk` |
| 2–3 | `dlastblk` | `dlastblk` |
| 4–5 | `dfkind` | status + `dfkind` |
| 6–13 | `dvid` (length byte + 7 chars) | — |
| 6–21 | — | `dtid` (length byte + 15 chars) |
| 14–15 | `deovblk` | — |
| 16–17 | `dnumfiles` | — |
| 22–23 | — | `dlastbyte` |
| 24–25 | — | `daccess` |

Strings are in UCSD form: a length byte followed by the characters.

`dlastblk` is "one past the last block", so

```
file size = (dlastblk − dfirstblk − 1) × 512 + dlastbyte
```

The low four bits of `dfkind` give the file kind: 0 untyped, 1 xdsk, 2 code, 3 text,
4 info, 5 data, 6 graf, 7 foto, 8 securedir.

`daccess` is a packed date: the low four bits are the month, the next five the day, and
the top seven the year as two digits after 1900.

```
daccess = month | day << 4 | year << 9
```

Decoding the three on-disk values `B429`, `AAE1` and `A94C` with that formula gives
`2-Sep-90`, `14-Jan-85` and `20-Dec-84`, character for character the same strings the
Filer lists by itself.

## Running it

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

Add `-x OUTDIR [names…]` to extract. `SYSTEM.PME.86` comes out as 16,384 bytes, sha256
`fe427aa66ca8…` — every conclusion in this repo rests on that hash.

`PME` is P-Machine Emulator and `.86` is 8086. `SYSTEM.PASCAL` is the operating system
proper (written in p-code); `SYSTEM.COMPILER` is the compiler.

## Limits

- The status bits are unexplained, and this repo has no use for them.
- "The directory occupies four blocks" is taken from the manual; the case of more than
  77 files was never verified.
- This repo **contains no original disk images or executables** — only byte-level
  conclusions and hashes.
