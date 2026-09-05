# Fetch and dispatch: an interpreter with no main loop

**English** ｜ [日本語](dispatch-and-threading.ja.md) ｜ [繁體中文](dispatch-and-threading.md)

`SYSTEM.PME.86` is the p-machine interpreter of the 1984 DOS UCSD p-system,
16,384 bytes. What it does is simple enough — fetch a byte, look it up, jump
there, and fetch the next one when done. What is unusual is that
**there is no place in it called "the main loop".**

> See also: [interpreter state](machine-state.en.md)｜
> [addressing and activation records](addressing.en.md)｜
> [the 256-entry map](opcode-map.en.md)

## What the dispatch sequence looks like

Every handler ends with the same eight bytes:

```
32e4      xor  ah, ah
ac        lodsb                  ; al ← [ds:si], si++
97        xchg ax, di            ; di ← opcode
d1e7      shl  di, 1             ; ×2 → byte index into the table
2eff25    jmp  word ptr cs:[di]  ; jump to the handler
```

`si` is the IPC (instruction pointer counter), and `lodsb` does both the read
and the advance in one instruction. Of the 169 handlers, 129 carry **a verbatim
copy of this sequence at their end**; the rest `jmp` into another handler's
ending or take an error path.

This is direct threading: **the dispatch code is copied into every handler**,
saving the unconditional jump back to a main loop. On a 1984 8088 a `jmp` costs
around 15 clocks, and a typical handler is only eight instructions — the
proportion saved is not small. The price is eight extra bytes per handler; 169
of them is 1,352 bytes, 8% of the whole interpreter.
**Space traded for time, paid per routine.**

The 68000 interpreter of the same version (SunDog) chose the other side: every
handler ends with `jmp (a5)`, with `a5` permanently pointing at the one main
loop. Which is why "find the main loop by counting jump targets" hits
immediately on the 68000 and fails completely on the 8086 — no target stands out.

<p align="center"><img src="../../img/inline-threading.svg" width="960" alt="On the left, the 68000 version's shared main loop with every handler jumping back to one place; on the right, the 8086 version with the dispatch code copied into the end of each handler"></p>

## A variant that saves one byte

Fifty handlers drop the `xor ah, ah` and do this instead:

```
97        xchg ax, di
ac        lodsb
97        xchg ax, di
d1e7      shl  di, 1
2eff25    jmp  word ptr cs:[di]
```

The first `xchg` moves `di` into `ax`. That is only equivalent when
**`di`'s high byte happens to be zero** right then — the zero moved in serves as
the `ah = 0` that `lodsb` needs.

`SLDC 0` is the cleanest example. When the opcode is 0, `di` is exactly 0 after
dispatch:

```
0319: 57        push di          ; di = 0×2 = 0, so what gets pushed is the constant 0
      97        xchg ax, di      ; ax ← 0
      ac        lodsb            ; ah is already 0
      97 d1e7 2eff25
```

`SLDC` occupies the thirty-two cells `0x00`–`0x1f`, encoding "the constant to
push" into the opcode itself. `0x00` gets its own routine because `di` happens
to be zero there; `0x01`–`0x1f` share `0x0322`.

## Where the table is

`jmp word ptr cs:[di]` carries no displacement, which literally says the table
is at `cs:0000` — but nothing in the file looks like a table. So work backwards
from something known: IV.0 defines `BPT` (`0x9e`) as raising a runtime error,
and the 8086 version puts the error code in `bp` and jumps to shared handling:

```
026e: bd1000    mov bp, 16
0271: eb9c      jmp 0x020f
```

The value `0x026e` occurs exactly once in the whole file, at offset `0x1e92`.
If that is the entry for `0x9e`, the table starts at
`0x1e92 − 0x9e×2 = 0x1d56`. Reading 256 entries from that base, all of them land
in plausible code, and disassembling each one agrees with the IV.0 manual
(see [the map](opcode-map.en.md)).

**The entries are file offsets** — and at run time they are also `cs`-relative
addresses, because **the loader moves those 512 bytes to the front of the image.**

Measured on the running system: `SYSTEM.PME.86` is loaded at physical `0x01400`
(segment `0x0140`), and image offsets `0x000`–`0x1ff` are **byte for byte
identical** to the 512 bytes starting at `0x1d56` on disk, while the address
table that was originally there is overwritten.

Three things then hold at once:

- `cs` points at the image base, so `cs:0000` is the start of the table —
  `jmp word ptr cs:[di]` needs no displacement.
- Entries are image-relative offsets, so taking one as `cs:offset` gives the
  handler address directly.
- The table at the front of the file is only useful during loading; afterwards
  it is gone (see [the first 512 bytes](../20-formats/pme86-header.en.md)).

The measurement runs `PSYSTEM.COM`, finds the image base in memory using the
mask table (`0x1fb6` onwards — pure constants, never relocated) as a
fingerprint, and then compares byte by byte.

## How the 256 cells are used

| Kind | Cells | Where |
|---|---|---|
| Own handler | 163 | — |
| `SLDC` shared (`0x00` has its own) | 31 | `0x01`–`0x1f` |
| `SLLA` shared | 8 | `0x60`–`0x67` |
| `SCXG` shared | 8 | `0x70`–`0x77` |
| Unimplemented (error 11) | 44 | `0x40`–`0x5f`, `0xaa`, `0xaf`, `0xf5`–`0xfe` |
| Error 14 | 1 | `0xff` |
| Error 16 | 1 | `0x9e` (`BPT`) |

169 distinct targets. All disassembled successfully, median eight instructions;
only `MPR` (`0x24e1`) and `DVR` (`0x262a`) exceed the 80-instruction cut-off.

## Statistics along the way

Across 2,026 instructions (excluding the truncated parts):

| Register | Occurrences | | Instruction | Count |
|---|---|---|---|---|
| `ax` | 746 | | `mov` | 258 |
| `di` | 571 | | `xchg` | 241 |
| `bp` | 347 | | `lodsb` | 177 |
| `cs` | 135 | | `jmp` | 166 |
| `cx` | 130 | | `shl` | 160 |
| `si` | 129 | | `pop` | 151 |

`xchg` coming second is no accident. The 8086's `xchg ax, reg` is one byte where
`mov` needs two; in an eight-instruction handler that one-byte difference gets
copied 169 times. `shl` in fifth place is structural too — p-code addresses are
in words and the 8086's are in bytes, so **every address computation multiplies
by two.**

## Limits

- Disassembly stops at "any unconditional transfer", so code past a `jmp` is not
  collected. `MPR` and `DVR` were cut off by the 80-instruction limit, so the
  instruction statistics here run low.
- "129 handlers with inline dispatch" comes from comparing the bytes of the last
  five instructions; it does not handle endings that branch partway through.
