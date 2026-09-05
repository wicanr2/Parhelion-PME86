# Interpreter state: four registers plus a dozen variables

**English** ｜ [日本語](machine-state.ja.md) ｜ [繁體中文](machine-state.md)

The p-machine is a stack machine. The registers listed on manual p.13 are `SP`, `MP`,
`BASE`, `IPC`, `SEG`, `JTAB` and `NP`. The 8086 has only eight 16-bit registers, and
the dispatch sequence itself consumes two of them. This page asks:
**which p-machine registers are worth keeping resident in 8086 registers, and where
the rest live.**

> See also: [fetch and dispatch](dispatch-and-threading.en.md)｜
> [addressing and activation records](addressing.en.md)｜
> [segment switching](segment-switching.en.md)

## Resident registers

| 8086 | Holds | Evidence |
|---|---|---|
| `si` | **IPC**, pointing at the next opcode | the dispatch code uses `lodsb`; every handler that calls a helper saves it first with `mov ss:24h, si` |
| `sp` | **SP**, top of the evaluation stack | `push`/`pop` *are* the p-code stack operations |
| `bx` | **local data base**, equal to `MP + 8` | helper `0x093e` walks the static chain starting from `bx − 8`; on return `RPU` writes `word_26 = new MP + 8` |
| `dx` | **global data base**, equal to `Env_Data + 8` | `LAO` is a plain `add ax, dx`; handlers that dirty `dx` (`DVI`, `STP`, …) end with `mov dx, ss:28h` |
| `ds` | the current **code segment** base | `lodsb` fetches p-code; `CSP` restores `ds` from `ss:2Ah` when done |
| `ss` | the interpreter's own data segment | every state variable is addressed via `ss:` |

`ax`, `cx`, `bp`, `di` and `es` are scratch. `di` occurs 571 times — second only to
`ax` — because the dispatch code needs it every round to compute the table address.

**That `dx` repair line captures the design trade-off best.** The 8086's `div` and `mul`
commandeer `dx`, so integer division and packed-field stores cannot preserve the global
base. The answer was not "don't keep it in `dx`" but "restore it from memory after
dirtying it" — a one-byte `add ax, dx` appears at every global access, while
`mov dx, ss:28h` appears in only a handful of handlers.

## State variables in memory

All offsets below are `ss:`-relative. The "used by" column lists the users actually
observed, not all of them.

| Offset | Holds | Used by |
|---|---|---|
| `0x24` | saved IPC (`si`) | 32 handlers — anything that calls a helper or switches segment |
| `0x26` | local data base = `MP + 8` (memory copy of `bx`) | `RPU` |
| `0x28` | global data base = `Env_Data + 8` (copy of `dx`) | `DVI`, `MPI`, `MODI`, `IXA`, `IXP`, `STP` |
| `0x2a` | 8086 segment value of the current code segment | `CSP`, the string-compare helper |
| `0x2e` | **MP**, the current activation record | `CPL`, `CPI`, `CXL`, `CXI`, `RPU`, `LSL` |
| `0x30` | **BASE**, the original `Env_Data` | `CPG` |
| `0x32` | current `MSPROC` (procedure number) | `RPU` |
| `0x34` | SIB of the current segment | the segment-switch helper |
| `0x36` | procedure dictionary base (in-segment offset ×2) | call helper `0x101b` |
| `0x3a` | **E_Vec**, the array of E_Rec pointers indexed by segment number | `LAE`, the segment-switch helper |
| `0x3c` | stack/heap boundary record | stack-check helper `0x02da` |
| `0x3e` | the current **E_Rec** | `RPU`, the segment-switch helper |
| `0x40` | "old E_Rec" saved across a segment switch | `CPL`, `CPG`, `CPI`, `RPU` |
| `0x42` | **constant-pool base** (in-segment byte offset) | `LCO`, `LDC`, `LDCRL`, `XJP` |
| `0x44` | **byte-sex flag** | `LDC`, `MOV`, `XJP` |
| `0x46` | word count computed by the stack check | the stack-check helper |
| `0xce`, `0xd0`, `0xd2`, `0xd4`–`0xda` | assorted scratch | `LDC`, `SRS`, set operations, call sequences |
| `0x110` | monotonically increasing counter written into the SIB's `Activity` | segment-switch helper `0x10f5` |
| `0xe6` | **SYSCOM base, and simultaneously `IORESULT`** | the device family, task switching |

The seven words at `0x2270`–`0x227c` are used only by `ADR`, `SBR`, `MPR` and `DVR`:
a register save area for the floating-point routines, which `xchg` `si`, `ax`, `bx`
and `di` out in a batch and swap them back afterwards.

## One slot, two jobs: `ss:0E6h`

`0x00E6` is the base address of SYSCOM (the system communication area); the device
layer treats it as the start of a structure (`mov bp,0E6h` at @0x2B3B, then
`[bp+1Ah]`, `[bp+2Ch]` throughout).
**The same word is also `IORESULT`** — a device call clears it to 0 on entry
(the first instruction at @0x2B3B) and writes the result back on the way out.

Its high byte has a third job: on a task switch it is combined with `MSPROC` into one
word and stored at `TIB+12h` (`mov ch, ss:0E6h` at @0x160F).

**This bites**: the interpreter writes that slot only during a task switch. Write it on
every call and the device result gets overwritten by the next piece of bookkeeping —
with the symptom surfacing thousands of instructions later, wearing a completely
different face.

## Local variables are not blank

Before allocating local data, the original `call`s into the stack-check helper:

```
1045: mov ax, 3Ch
1048: cmp byte ptr ds:48h, 1
104d: je  1057
104f: call 02DAh          ; return address 1052h is pushed below the old SP
1052: jae 105Ah
1057: call 02DAh          ; this one's return address is 105Ah
105a: mov sp, ax          ; the new stack top computed by the helper
```

That `call` pushes the return address **two bytes below the old SP** — and once the
local data has been allocated, that location is exactly **the highest-numbered local
variable**.

So a procedure's last local variable starts life holding `1052h` or `105Ah`. It sounds
like a detail nobody could care about, except that during OS boot a procedure with
`DATASIZE` of 1 really does pass it out as a parameter — **fail to reproduce that value
and p-code instruction 40,619 diverges.**

## Byte sex is not guesswork

Slot `0x44` maps straight onto the manual. IV.0's definition of `MOV` (197) says: "if
UB is 2 and the segment's byte sex differs from the host's, swap the bytes of each word
as it is moved." The 8086 `MOV`:

```
0a13: 0bed        or   bp, bp            ; UB
0a15: 7421        jz   plain move
0a17: 83fd02      cmp  bp, 2
0a1a: 7524        jnz  plain move
0a1c: 36833e440001 cmp word ptr ss:44h, 1
0a22: 741c        jz   plain move
0a24: ad          lodsw                  ; move word by word, swapping bytes
0a25: 86c4        xchg al, ah
0a27: ab          stosw
0a28: e2fa        loop
```

`LDC` (131, operands `UB_1, B, UB_2`) and `XJP` (214) use the same test.
**`ss:44h` of 1 means the segment's byte sex matches the host's.** The value is read on
a segment switch from byte `0x0c` of the code segment's header
(see [segment switching](segment-switching.en.md)).

## How errors are raised

Every runtime error takes the same road: put the code in `bp`, jump to `0x020f`.

```
025a: bd0b00  mov bp, 11   ; unimplemented instruction; 44 cells point here
0269: bd0e00  mov bp, 14   ; only 0xff
026e: bd1000  mov bp, 16   ; BPT
```

The 44 cells `0x40`–`0x5f`, `0xaa`, `0xaf` and `0xf5`–`0xfe` all point at `0x025a`.
The IV.0 manual labels `0xfa`–`0xff` as `RESERVE1`–`RESERVE6`, but this interpreter
spends the last cell `0xff` on error 14 — **same version, same table, and the last
cell is allocated differently from the 68000 version.**

## Limits

- The "used by" column was counted from the disassembly of 169 dispatch targets and
  25 helpers; it does not cover code past the disassembly cut-off.
- `0x3c` points at the current task's TIB; for the layout see
  [tasking and task switching](tasking.en.md). `+04`/`+06` (the stack bounds) are
  filled by the bootstrap and how they are computed is still open.
- The purpose of the slots after `0xce` is inferred from individual handlers; there is
  no consistent naming.
