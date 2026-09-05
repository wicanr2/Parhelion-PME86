# Native procedures embedded in segment 1: the border between p-code and machine code

**English** ｜ [日本語](native-intrinsics.ja.md) ｜ [繁體中文](native-intrinsics.md)

The operating system proper is written in p-code, but it still has to move memory, touch
disks and disable interrupts. The IV.0 manual says one sentence about this on p.66: the
procedures of segment 1 **may** be implemented directly inside the interpreter. This
page unpacks that sentence — **it is the complete list of "what is not p-code" on this
machine**.

> See also: [segment switching](segment-switching.en.md)｜
> [interpreter state](machine-state.en.md)｜
> [the header of `SYSTEM.PME.86`](../20-formats/pme86-header.en.md)

## The interpreter draws the border itself

On a cross-segment call into segment 1, `CXG` first consults a table (@0x13F4; `SCXG1`
takes the same road):

```
13f4: cmp bp, 1            ; is the segment number 1?
13f7: jnz general path
13f9: mov di, [si]         ; peek at the procedure number (without advancing)
13fb: and di, 0FFh
13ff: cmp di, 2Fh
1402: jg  general path     ; > 47 → no embedded implementation
1404: shl di, 1
1406: mov di, cs:[di+1F56h]
140b: cmp di, 0
140e: jz  general path      ; this cell is 0 → no embedded implementation
1410: inc si
1411: jmp di                ; jump straight into machine code
```

48 cells from `0x1F56`, indexed by procedure number. **33 are non-zero, giving 26
distinct addresses** (seven pairs point at the same routine). A miss falls back to the
ordinary cross-segment call and goes looking for p-code inside segment 1.

So the table *is* the border: **what is on it is the host's business; everything else is
p-code.**

## What the 26 do

Parameter order is always read off from that routine's `pop` sequence, not guessed. The
"read" column says whether the routine's code was read instruction by instruction.

| Proc | Address | Does | Read |
|---:|---|---|:--:|
| 4 | `1B2A` | relocate the addresses of a just-loaded segment per its relocation list | ✓ |
| 14 | `1AE4` | move a whole segment between two codepool positions, handling overlap | ✓ |
| 15 | `191E` | `MOVELEFT`: move from low addresses to high | ✓ |
| 16 | `1970` | `MOVERIGHT`: move from high addresses to low | ✓ |
| 18/40 | `2C5A` | `UNITREAD` | ✓ |
| 19/41 | `2C5F` | `UNITWRITE` | ✓ |
| 20 | `2CB8` | `TIME(var hi, lo)`: the system clock, in 1/60 s | ✓ |
| 21 | `18DD` | `FILLCHAR` | ✓ |
| 22 | `1992` | `SCAN` | ✓ |
| 23 | `2B0D` | `IOCHECK`: raise a runtime error if `IORESULT` is non-zero | ✓ |
| 24 | `1A6E` | move from the codepool into the data segment | ✓ |
| 25 | `1A94` | move from the data segment into the codepool | ✓ |
| 26 | `1ABA` | byte-swap a just-loaded segment word by word | ✓ |
| 27/28 | `19F4`/`1A00` | disable / enable interrupts | ✓ |
| 29 | `1841` | `ATTACH`: hook a semaphore into the interrupt vector table at `ss:4Eh` | ✓ |
| 30 | `2B02` | `IORESULT` | ✓ |
| 34/44 | `2C36` | wait for a device to finish (device mode 4) | ✓ |
| 36/45 | `2C1B` | ask a device for its status (mode 8) | ✓ |
| 38 | `1CF6` | look up a binary tree with an 8-byte name | ✓ |
| 39/46 | `1BAF` | read a segment from disk into the codepool | ✓ |
| 31/42 | `2BF1` | device mode 3 | |
| 33/43 | `2BF7` | device mode 3 | |
| 32 | `2826` | | |
| 37 | `1C0B` | | |
| 47 | `1A0C` | swap in a different set of floating-point routines — **it rewrites the `0x1F56` table itself** | ✓ |

Seventeen of them are used during a complete boot.

## The device family is really one routine

18/19/31/33/34/36 all land in the same code, differing only in a mode byte written to
`0x2AA0`:

| Mode | Who |
|---:|---|
| 1 | `UNITREAD` |
| 2 | `UNITWRITE` |
| 3 | procedures 31/33 |
| 4 | wait for the device |
| 8 | ask for device status |

The parameter block sits at `0x2AA0`–`0x2AA8` in the interpreter image: mode, unit,
buffer address, length, block number, mode. Pushed top-down on the stack the order is
`mode, block, length, buffer (base, offset), unit` — that is, Pascal's
`UNITREAD(unit, buf, length, block, mode)` pushed left to right.

On completion the result goes into `IORESULT` (`ss:0E6h`, cleared to 0 by the first
instruction at @0x2B3B).

## Two details that bite

**`ss:0E6h` does two jobs.** It is both `IORESULT` and the high byte of `MSPROC`
(@0x160F folds it into `TIB+12h` when saving task state). So the interpreter writes that
slot only during a task switch — writing it on every call would overwrite the device
result.

**Where control resumes is not always after the procedure number.** `SCXG` at @0x0545
saves `si` into `ss:24h` on entry, and at that moment it points at **the procedure
number**; the `inc si` at @0x1410 happens only afterwards, just before the jump into
machine code. Most native procedures save it again themselves (`MOVELEFT` at @0x192C),
so they resume after the procedure number; **the few that do not resume at the procedure
number itself**, and that byte then gets executed as the next instruction. Procedure 4's
number is `04`, which happens to be `SLDC 4`.

## Limits

- "33 non-zero cells, 26 distinct addresses" was counted straight off the image bytes
  and is solid.
- In the "does" column, the rows marked "read" were read instruction by instruction and
  verified by actual co-simulation; the unmarked ones are only known to be on the table.
- All parameter orders were read off the `pop` sequences and co-simulated against the
  original in [self-boot](../30-remake/specs/03-boot.md).
