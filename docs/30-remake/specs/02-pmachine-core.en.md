# spec 02: the p-machine execution core

**English** ｜ [日本語](02-pmachine-core.ja.md) ｜ [繁體中文](02-pmachine-core.md)

| | |
|---|---|
| State | `CONFORMED` (for the 56 opcodes in scope) |
| Date | 2026-09-05 |
| Covers | execution state, instruction fetch, short forms, addressing, arithmetic, comparison, jumps, packed fields, same-segment and **cross-segment** call and return |
| Does not cover | segment 1's embedded native procedures, floating point, sets, strings, tasking, CSP, multi-word moves |
| Verification | **306 p-code instructions** walked one by one, all three items matching; **45 segment switches each verified**, all four items matching; a real segment switch actually walked in co-simulation |

**Every semantic here was read out of the 8086 interpreter's code**; the manual serves
only as a cross-check. The addresses below are file offsets in `SYSTEM.PME.86`, so they
can be taken straight back to the bytes.

## 1. Execution state

1.1 Two segments suffice: code lives in one (`ds`, where `lodsb` fetches), and the
evaluation stack, locals and globals all live in the other (`ss`).

1.2 The resident bases (`docs/10-interpreter/machine-state.en.md`):

| Field | What it is | Original keeps it in |
|---|---|---|
| `IPC` | in-segment byte offset of the next opcode | `si` |
| `SP` | top of the evaluation stack, growing downward | `sp` |
| `Local` | local data base = `MP + 8`; variable n is at `Local + 2n` | `bx` (copy at `ss:26h`) |
| `Global` | global data base = `Env_Data + 8` | `dx` (copy at `ss:28h`) |
| `ConstPool` | byte offset of the constant pool within the code segment | `ss:42h` |
| `ProcDict` | byte offset of the procedure dictionary within the code segment | `ss:36h` |
| `Proc`/`Env` | the current `MSPROC` and `E_Rec` | `ss:32h`/`ss:3Eh` |

1.3 `Proc` and `Env` cannot be left unmodelled. They take no part in computation, but
**a call has to save them into the MSCW** — miss one and the stack contents differ from
the original by a word, and every subsequent `SP` is wrong with it.

## 2. Instruction fetch

2.1 The opcode is one byte. A variable-length operand (`big`) whose top bit is 0 is the
value itself (0–127); with the top bit 1, the low 7 bits are the high byte and one more
byte is read as the low byte (0–32767). This code is expanded inline 19 times in the
interpreter.

2.2 **A `big` almost always gets doubled**: p-code speaks in words, memory is addressed
in bytes. The one exception is `NAT-INFO` (`0xa9` @0x075f), which does `add si, ax` with
no `shl` — that is a count of native code bytes to skip.

## 3. Short forms

| opcode | Semantics | Source |
|---|---|---|
| `0x00`–`0x1f` `SLDC` | push the opcode itself | @0x0319, @0x0322 (`shr di,1; push di`) |
| `0x20`–`0x2f` `SLDL` | push `[Local + 2n]` | @0x032d (`mov bp,bx`) |
| `0x30`–`0x3f` `SLDO` | push `[Global + 2n]` | @0x03fd (`mov bp,dx`) |
| `0x60`–`0x67` `SLLA` | push `Local + 2n` (an address) | @0x04cd (`sub di,0BEh; add di,bx`) |
| `0x68`–`0x6f` `SSTL` | store into `[Local + 2n]` | @0x04dd |
| `0x78`–`0x7f` `SIND` | take TOS as an address, push `[TOS + 2n]` | @0x0554 |

## 4. Addressing and data movement

`LDCB`(@0x05b4), `LDCI`(@0x05bf), `LCO`(@0x05ca), `LLA`(@0x05e6), `LDO`(@0x05ff),
`LAO`(@0x061b), `LDL`(@0x0634), `STL`(@0x070a), `SRO`(@0x0726), `LDB`(@0x0751),
`STB`(@0x0822), `STO`(@0x0814), `IND`(@0x08d7), `LDM`(@0x09b8), `DUP1`(@0x08bc),
`LDCN`(@0x0693), `NOP`(@0x06b9), `NAT-INFO`(@0x075f).

The intermediate-level forms (`LDA` @0x0650, `LOD` @0x065d, `STR` @0x0742,
`SLOD1/2` @0x0775/@0x0787) share helper @0x093a: `bp = bx − 8` to get back to MP, then
`bp = [bp]` DB times (`MSSTAT` is at MP+0), finally `bp += 2n + 8`. `LSL` (@0x14ee)
walks the same way but pushes that level's MP.

## 5. Arithmetic and comparison

5.1 Two-operand instructions are always `pop ax; pop bp; op bp, ax` — **TOS is the right
operand**. Reversed, `EQUI` shows no difference and `LEQI` comes out entirely backwards.

`LOR`(@0x06d2), `LAND`(@0x06e0), `ADI`(@0x06ee), `SBI`(@0x06fc),
`MPI`(@0x066c, `imul` keeping the low 16 bits), `DVI`(@0x067f), `MODI`(@0x0bbc),
`NGI`(@0x08af), `ABI`(@0x089a), `LNOT`(@0x08ca, complementing the whole word),
`BNOT`(@0x06c2, complement then keep only the lowest bit), `INC`(@0x08f2),
`DECI`(@0x0918), `IXA`(@0x0864).

5.2 When `MODI`'s remainder is negative, the absolute value of the divisor is added
(the `test dx,dx; jns` part of @0x0bbc). Pascal's `MOD` is not C's `%`.

5.3 Comparisons all take the same shape: after `cmp bp,ax`, push 1 if the condition
holds and 0 otherwise. `EQUI`(@0x0799), `NEQI`(@0x07ab), `LEQI`(@0x07be),
`GEQI`(@0x07d0) are signed; `LEUSW`(@0x07e2), `GEUSW`(@0x07f4) unsigned.

## 6. Jumps

6.1 The displacement is counted **from after the operand**, because the original does
`lodsb`/`lodsw` first and `add si, ax` second.

6.2 A conditional jump looks at **the lowest bit of TOS** (`shr ax,1` into the carry),
not at whether the whole word is zero. With a value of 2 the two readings give opposite
answers.

`UJP`(@0x0c47, byte), `JPL`(@0x0c84, word), `FJP`(@0x0848), `FJPL`(@0x0c74),
`TJP`(@0x0c38), `EFJ`(@0x0c54, jump if unequal), `NFJ`(@0x0c64, jump if equal).

> **A trap was stepped in here, and it is worth writing as a rule.** Go does not specify
> the evaluation order of the two operands of `a + b`, so
> `s.IPC = uint16(int16(s.IPC+1) + int16(int8(s.fetch())))` is right sometimes and wrong
> others — `s.IPC+1` may be read **after** `fetch()` has already advanced the IPC.
> **For relative addresses, always take the operand into a variable first, then
> compute.** This bug was caught by co-simulation at instruction 101; reading the code
> statically does not reveal it.

## 7. Packed fields

`LDP`(@0x0a77), `STP`(@0x0a91), `IXP`(@0x0a50). The triple's order is address, bit
count, rightmost bit number (`docs/10-interpreter/addressing.en.md`).

The original uses the ready-made mask table at `cs:1FB6h` (entry n = `(1<<n)−1`); here
the value is computed directly, with the same result. **This is a remake-defined
technique, not a behavioural difference** — the table exists because computing a mask
costs more than a lookup on the 8086, and Go has no such problem.

`IXP` gets "which word" and "which slot in the word" from a single division
(`div bp` at @0x0a50).

## 8. Same-segment call and return

8.1 `CPL`(@0x1310), `CPG`(@0x1337), `CPI`(@0x1368), `SCPI1`/`SCPI2` all share
@0x101b → @0x103a → @0x1057. The only difference is where the static link comes from:

| Instruction | Static link |
|---|---|
| `CPL` | the caller's own MP |
| `CPG` | `BASE` (= `Global − 8`) |
| `CPI`/`SCPIn` | the MP of the level reached by walking out DB levels |

8.2 The dictionary entry points at `DATASIZE`, and the first instruction is one word
past it (spec 01 §5.4). A negative `DATASIZE` means native code, which returns an error
here.

8.3 Allocating local data: `SP −= 2 × DATASIZE` (the value computed at @0x02da comes to
this once what the call pushed on the stack is subtracted).

8.4 The MSCW push order (@0x1057) is `MSPROC`, `MSENV`, `MSIPC`, `MSDYN`, `MSSTAT`.
The stack grows downward, so reading upward from MP gives Figure 5's field order, and
locals start at `MP+10`. **Get the order wrong and the callee's locals overwrite the
control fields.**

8.5 `RPU`(@0x1102) takes it apart again: `SP = MP − 2`, then in order `[MP−2]`,
`MSSTAT`, `MSDYN` (which becomes the new MP), `MSIPC`, `MSENV`, `MSPROC`, and finally
`SP += parameter bytes`. When the MSCW's `E_Rec` differs from the current one this is a
cross-segment return; see 8.11.

## 8bis. Cross-segment calls

8.6 `SCXG`(@0x0545), `CXL`(@0x13b8), `CXG`(@0x13eb/@0x1413), `CXI`(@0x1457/@0x1475) all
share one road: **switch segment first, then build the frame the same-segment way.**
`SCXG n` is `CXG` with the segment number encoded in the opcode
(`shr di,1; sub di,6Fh`).

Where the static link comes from: `CXL` uses the caller's MP; `CXG`/`SCXG` use the
**target segment's** `BASE` (`push word_30` at @0x1420, and `word_30` has already been
replaced by the switch); `CXI` walks out DB levels. **It has to be computed before the
switch** — walking the chain is the caller's side of the business.

8.7 A segment switch is **one whole set**: code, `Env_Data`, procedure dictionary,
constant pool and `E_Rec` change together. Missing one raises no error: an unswitched
constant pool reads another segment's constants, an unswitched dictionary calls another
segment's procedures — both quietly read something that looks like data.

8.8 The MSCW's `MSENV` must record **the caller's** `E_Rec`. The original pushes the new
one at @0x1057 and changes it back to the old one at @0x10db; pushing the old one
directly has the same effect. Record the wrong one and the return cannot switch back.

8.9 When the segment is not resident the original backs up to the start of the
instruction and raises a segment fault, so that after the operating system loads it
**the same instruction runs again** (`si = ss:24h; dec si` at @0x143d). Here it returns
`ErrNotResident` — loading is the host's business.

8.10 **A segment 1 procedure may be native code embedded in the host** (manual p.66).
The original's `CXG` consults that table **before switching** (@0x13f4), and on a hit
jumps straight into machine code with no switch at all. Switching first and finding out
afterwards leaves a half-switched state, and that raises no error.

8.11 In `RPU`, an `E_Rec` in the MSCW that differs from the current one is a
cross-segment return, and the segment has to be switched back first
(`bp = [MP+6]; cmp bp, word_3E` at @0x1102).

8.12 **The `Environment` layer is deliberately kept out of the p-machine.** How a segment
number maps to a piece of memory involves `E_Vec`, `SIB` and the Codepool — operating
system data structures that are completely different under another host. The p-machine
only asks "give me what that segment looks like".

The implementation side (`liveEnv` in `oracle/parity.go`) computes it the @0x0fba way:

```
segment value = paragraph(Codepool base) + (offset in pool ÷ 16)
```

`Seg_Base` is the first two words of the `SIB`: a pointer to the Codepool base
(relative to `ss`) and a byte offset within the pool. Details and measurements are in
`docs/10-interpreter/segment-switching.en.md`.

## 9. Verification

`TestParityAgainstTheOriginal` in `oracle/conform_test.go`: expand from the original's
state at that moment, walk both sides instruction by instruction, and compare `IPC`,
`SP` and `TOS` after each. It stops as soon as any one of the three fails to match —
carrying on would only pile the error up into unreadable noise.

**The three ways of stopping are distinguishable**: walked to the end; an instruction
not yet implemented; an actual mismatch. Only the third is a failure; the second is
progress, and it says precisely which routine to do next.

Currently: **224,987 instructions matching one for one, 173 distinct opcodes used, 0
divergences.**

Four things are compared: the address of the next instruction, the position and contents
of the stack top, and the current E_Rec. Beyond that, **each p-code instruction gets a
budget of 400,000 8086 instructions from the original** — after boot the system waits on
the disk and polls the keyboard between two p-code instructions, and too small a budget
gets misread as "the original has nothing left to do" and finishes early.
It stops because **the original has nothing left to do** — the boot has run and the
system is sitting in the keyboard-wait loop. Getting there means the whole stretch was
co-simulated; that is finishing, not failing.

Another **147 instructions are handed to the original to walk itself** — all of them
segment 1's embedded native procedures. That is the interpreter's own machine code, not
p-machine semantics, so after the original executes them the state is copied over again
and the walk continues. **Those instructions are not verified**; the denominator is the
"matching one for one" number.

Only four things are let through, and each is something the p-machine has to hand off by
definition: segment 1's embedded native procedures; `NAT` (jumping into 8086 machine
code, which needs an actual 8086 to execute); a segment not yet loaded (the operating
system has to fetch it from disk); and a task switch (the scheduler swapping the whole
state for another TIB's).
**An unimplemented p-code instruction does not count.** Copying the state over for those
too would make "not done yet" look like "done", and the co-simulation would lose its
meaning. The test checks that the skipped opcodes really are only the cross-segment call
family.

Cross-segment calls get two more checks:

- `TestSegmentResolutionMatchesTheOriginal`: the code segment contents, global base,
  procedure dictionary and constant pool we compute are **identical in all four** to the
  original's after it switches. This one matters particularly because the `Seg_Base`
  conversion was measured rather than taken from the manual; the symptom of getting it
  wrong is "landing in the middle of another segment", which raises no error.

  Switches are caught with `dosgolem`'s `WatchWord` watching the interpreter's `E_Rec`,
  **not by polling**. Between two p-code instructions the machine can switch away and
  back again, and polling would miss it — and a miss raises no error, it just makes some
  switches look as though they never happened. With watching in place, **all 45 switches
  in one trace were received and each verified** (polling had sampled only 3).
- `TestParityAcrossASegmentSwitch`: push the original to a point where the next
  instruction really is a switch, then expand and **walk that switch instruction by
  instruction** (`E_Rec D7D2 → 0664`).

The test also pins a floor (`parityFloor`) so that progress cannot regress.

Under `internal/pmachine/` there are another 23 tests that need no original material,
using hand-written p-code to pin down cases **co-simulation cannot reach, or reaches
without being able to tell**:

| What it pins | Why co-simulation cannot tell |
|---|---|
| operand order in arithmetic | addition and bitwise ops give the same result reversed |
| the four sign combinations of `MODI` | with a positive dividend Pascal's and C's `%` agree |
| signed vs unsigned comparison | tested only with small positive numbers, both pass |
| `IXA`'s shortcut for index 1 | with one-word elements both paths agree |
| the push direction of `LDM` | reversed, every word is still a legal value |
| the byte index of `LDB`/`STB` | read as a word index, even positions still come out right |
| how many levels the static chain walks | the wrong level still reads a legal value |
| divide by zero, `CHK` out of range, `ABI` overflow | the normal path never reaches them |
| that a cross-segment call's embedded procedure must be caught **before** the switch | switching first leaves a half-switched state |

Each case's name carries its file offset in `SYSTEM.PME.86`, so a failure can go straight
back to the bytes.

## 10. Representation of sets

A set on the stack is **N words plus N on top**, with word k at `sp + 2k`. This was read
out of `INN`'s (@0x0dc6) `sp += 2k; pop`, not guessed.

`SRS` (@0x0d4b) builds a subrange set, with bounds required to be within 0..4079.
`ADJ` (@0x0cac) adjusts a set to a fixed UB words **and removes the length** — from then
on the set is fixed length, and keeping the length would leave every subsequent
instruction's stack off by a word.

Length rules: union takes the longer, intersection the shorter, difference the left one.
For comparison, the shorter side is treated as zero-padded.

`INN` gets its bit mask by XORing two adjacent entries of the mask table
(`cs:[1FB8h+2i] xor cs:[1FB6h+2i]` = `1<<i`) — cheaper than shifting on the 8086. Here
it is computed directly, a remake-defined technique rather than a behavioural
difference.

## 11. Floating point

**The eight bytes are IEEE 754 binary64, little-endian.**
Read out of helper @0x22a8: `and ax,0Fh` takes the top 4 bits of the mantissa,
`and cx,7FF0h; shr 4` takes the 11-bit exponent, `or al,10h` restores the hidden bit.
The sign is bit 7 of the highest byte (`and byte [bp+7],7Fh` in `ABR` @0x2a94,
`xor ... 80h` in `NGR` @0x278c).

> **Manual p.14 says reals are a BCD floating-point format; this version's are not.**
> The implementation governs. By 1984 IEEE 754 was a standard, and changing was
> reasonable.

`NGR` flips the sign only when the four words are not all zero — negating directly would
turn 0 into −0.0, which is a different bit pattern.
`LDCRL`'s operand is a **word offset** into the constant pool (`shl ax,1; add ax,
ss:42h`), not "which real constant". `STRL`'s address sits under the real
(`mov di,[si+8]`).

## 12. Tasking

A semaphore is two words: a count and the head of a wait queue (`[di]`/`[di+2]` at
@0x1791). **The non-blocking paths are just increment and decrement** — `WAIT`
decrements when there is credit, `SIGNAL` increments when the count is negative or
nobody is waiting. Only actually queueing or waking enters the scheduler, and that
returns `TaskSwitch`.

A task switch replaces the whole execution state with another TIB's; that is the
operating system's scheduler, not the semantics of one instruction. `NAT` is more direct
still: it `retf`s into 8086 machine code inside the code segment, which is
**structurally not something a p-machine can do** — no amount of p-code will make it
executable. The error types for these two are distinguishable from "not done yet".
