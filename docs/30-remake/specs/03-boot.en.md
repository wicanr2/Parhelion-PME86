# spec 03: self-boot

**English** ｜ [日本語](03-boot.ja.md) ｜ [繁體中文](03-boot.md)

State: **`CONFORMED`** (see [the spec gate](../spec-workflow.en.md)) — **it boots to the
command line and it responds.** Feed it one `.VOL` and the p-System brings itself up;
226,623 p-code instructions match the original one for one — the whole boot, right up to
the moment the original itself sits waiting for a key.

```
Startup Utility - [1R1.0]
PSYSTEM:SYSTEM.MISCINFO ---> RAMDISK:SYSTEM.MISCINFO
PSYSTEM:SYSTEM.PASCAL   ---> RAMDISK:SYSTEM.PASCAL
...
Root is RAMDISK
Prefix is RAMDISK
SYSTEM.PASCAL is on RAMDISK
Command: E(dit, R(un, F(ile, C(omp, L(ink, X(ecute, A(ssem,? [IV.2.1 R3.3]
```

Press `F` for the Filer and `L` to list; what comes out are the five files the operating
system copied over itself during boot:

```
Filer: L(dir, R(em, C(hng, T(rans, D(ate, Q(uit, B(ad-blks, E(xt-dir,? [6R4.0]
Dir listing of what vol ? RAMDISK:

RAMDISK:
SYSTEM.MISCINFO    2  2-Sep-90           SYSTEM.PASCAL    136 14-Jan-85
SYSTEM.EDITOR    106  9-Dec-85           SYSTEM.FILER      45 20-Dec-84
SYSTEM.LIBRARY   102 27-Dec-84
5/5 files<listed/in-dir>, 397 blocks used, 353 unused, 353 in largest
```

Those dates verify the reader in passing: `2-Sep-90`, `14-Jan-85` and `20-Dec-84` match
the `daccess` fields on disk (`B429`, `AAE1`, `A94C`) one by one —
**the directory format we decoded agrees exactly with what the p-System's own Filer
displays.**

This document is about "how to bring the p-System up from a `.VOL` without the original
and without DOS". The p-machine itself is [spec 02](02-pmachine-core.en.md); this is
only about the host.

## 1. Why this document is needed

Co-simulation already walks the whole boot, but that machine was **strapped alongside
the original**: segment 1's native procedures, the disk and task switching were all
handed to the original to perform. To run independently, the host side has to be built.

The border is clear, and the original drew it itself:

| What the host must provide | Where that is visible |
|---|---|
| the memory layout at the moment of boot | what the bootstrap arranges before the first p-code instruction |
| segment 1's 33 embedded native procedures | the non-zero cells of the 48-entry table at interpreter @0x1F56 |
| what to do when a segment is not loaded | the zero check at segment-switch helper @0x0FDF |
| task switching | when `SIGNAL`/`WAIT` really have to queue |

**Everything else is p-code.** The operating system itself is written in p-code.

## 2. The moment of boot

The original spends 7,218 8086 instructions reaching the first p-code instruction, all
of it bootstrap. Measured, the state at that moment (data segment paragraph `04D1`) is:

```
1251:1AB8 90 CPL   sp=D7C2 mp=D7C2 E_Rec=D7D2 MSPROC=1
```

The first instruction is `CPL` — the operating system's outer program calling its own
first procedure.

### Which segment is loaded

The one that comes **first** in `SYSTEM.PASCAL`: `USERPROG`, block 1, 3,831 words,
41 routines, segment number 22, enclosed by `KERNEL`.
The linker put it in the first block after the dictionary, so the bootstrap can read it
without consulting the dictionary at all.

### Laid downward from the code

```
D800  code (USERPROG, 3831 words)
D7DC  SIB           36 bytes
D7D2  E_Rec         10 bytes (6 used, 4 padding)
D7CC  E_Vec          6 bytes
D7C2  first MSCW    10 bytes  ← both MP and SP point here
      ↓ stack grows downward
0178  global data
0170  BASE: the outermost frame
0154  TIB
00E6  SYSCOM
0024  the interpreter's state area
```

The SIB's `Seg_Base` is `{0000, D800}` — a zero pointer means "right here in the data
segment", and the second word is the in-segment offset. `E_Vec` has only three slots,
with slot 2 pointing at that `E_Rec`.

The first MSCW's `MSSTAT` and `MSDYN` both point at the global frame itself: **there is
nobody further out.**

### The top three blocks of the data segment

Cut downward from `0xFFFE`, all three read in by the bootstrap beforehand:

```
FFFE  ← stack upper bound (what TIB +6 records)
F7FE  disk directory, 4 blocks
F5FE  segment dictionary of the OS codefile, 1 block
      ↑ (Code_Addr, Code_Leng) is looked up here when another segment is needed
```

The directory copy **is not a copy of the one on disk**: the bootstrap replaces `+14h`
of entry 0 with **this boot's date** (UCSD packed format: month 4 bits, day 5, year 7),
leaving the on-disk copy alone. `psystem.Options.Date` is that value.

The segment dictionary block is 512 bytes moved across verbatim, its contents just
`(Code_Addr, Code_Leng)` pairs in dictionary order.

### There are two directories

One at the top of the data segment (`0xF7FE`), pointed at by `SYSCOM+8`; another at
`0x3AF0` for the operating system's own use. **Both need the boot date stamped on
them.** The second one's location was measured; how it is computed is still open.

## 3. Not yet decoded: the bootstrap itself

**The initial values the bootstrap leaves in the low part of the data segment**
(`bootWords` in `internal/psystem/machine.go`) fall into three blocks with different
origins:

### That whole block is the interpreter's own initialised data

**The first 512 bytes of `SYSTEM.PME.86` are moved verbatim into the same offset in the
data segment** — the 37 non-zero words were checked one by one and not one differs
(`TestInterpreterDataComesFromItsOwnHeader`).

That stretch is invisible in the code segment, because the loader moves the dispatch
table from `0x1D56` to image offset 0 and covers it. **It lives only in the data
segment** — which also explains why those 512 bytes look like a pile of meaningless
constants in the file.

The contents are the interpreter introducing itself to the operating system:

| Location | What it is |
|---|---|
| `0x08` | address of SYSCOM (`0x00E6`) |
| `0x0A`/`0x0C`/`0x0E` | offsets, within the state area, of the local base, global base and current E_Rec (`0x26`/`0x28`/`0x3E`) |
| `0x12`–`0x18` | entry points of four native callbacks |
| `0x1A`–`0x22` | other interpreter code addresses |

We have our own equivalents of the first four — **those offsets are the constants in
`internal/psystem`.** The ones after `0x12` are 8086 machine-code addresses with no Go
counterpart: the operating system only needs them on a `NAT` call, and `NAT` is out of
reach anyway. They are copied verbatim so that the p-code reading those slots takes the
same road.

The same block also holds SYSCOM (device count and parameters), the initial byte-sex
value, the outermost frame's `MSSTAT`/`MSDYN`, and global variable 1 pointing at
SYSCOM — all of it in the file.

### The bootstrap computes only a few slots itself

| Location | What it is |
|---|---|
| `0x06` | address of the boot parameter area |
| `0x150` | physical address of the data segment |
| `0x14E`/`0x152` | not decoded |
| TIB `+4`/`+6` | this task's stack bounds (`04A6`/`FFFE`); how they are computed is not decoded |

**Those four slots are all that is left undecoded.** Everything else has a source.

And there is more than that. Comparing the whole "data segment at the moment of boot"
(`TestSelfBootInitialStateDiff`) started at **1,982 differing regions**.

The largest block was tracked down: **`SYSTEM.CONFIG`**. Blocks 38–67 on disk, 14,848
bytes, read in whole by the bootstrap at `0x5020`, with the first 5 blocks copied again
at `0x06F0`. Inside is this machine's device configuration — driver names (`DOSVV.`,
`SERPAR`, `BRIDGE`) and one 40-byte record per unit (layout in §4). The operating system
builds the unit table pointed at by `ss:112h` from it. Adding it dropped the difference
from 1,982 regions to **540**; getting the remaining blocks right (the second and third
directory copies, the working copy of the configuration) brings it to **409**.

How it was found: search memory for the first 32 bytes of every file on disk
(`cmd/bootdump -findfile`). **Faster than disassembling the bootstrap, and the answer is
what actually happened.**

### This number cannot reach zero

Of the remaining 409 regions, **397 are 8086 machine code**: `0x0192`–`0x06EF` is the
native driver the bootstrap moved in from interpreter image `0x3AA2`, and
`0x42F0`–`0x5010` is the PSP plus `PSYSTEM.COM` itself. The operating system calls the
former through `NAT`. **Our host is Go, so neither block was ever going to be there** —
the job is to implement their function in Go and intercept those `NAT` calls, not to
copy the bytes across.

So the metric has to be read in two halves: `409 regions (397 of them host machine
code)`.

A counter-intuitive fact: **without those 1,982 regions, the first 40,619 p-code
instructions still match one for one.** The operating system only reads them much later.
So "it runs" is not the same as "it was built right", and the two metrics must be kept
apart.

## 4. Segment 1's native procedures

48 cells from @0x1F56, 33 non-zero, 26 distinct. Seventeen are used during boot.

Implemented:

| Proc | Address | Does |
|---:|---|---|
| 4 | `0x1B2A` | relocate the addresses of a just-loaded segment per its relocation list |
| 14 | `0x1AE4` | move a whole segment between two codepool positions |
| 15/16 | `0x191E`/`0x1970` | `MOVELEFT`/`MOVERIGHT` |
| 18/19 | `0x2C5A`/`0x2C5F` | `UNITREAD`/`UNITWRITE` |
| 21 | `0x18DD` | `FILLCHAR` |
| 23/30 | `0x2B0D`/`0x2B02` | `IOCHECK`/`IORESULT` |
| 24/25 | `0x1A6E`/`0x1A94` | move bytes codepool ↔ data segment |
| 26 | `0x1ABA` | byte-swap a just-loaded segment word by word |
| 27/28 | `0x19F4`/`0x1A00` | disable / enable interrupts |
| 39/46 | `0x1BAF` | read a segment from disk into the codepool |
| 20 | `0x2CB8` | `TIME(var hi, lo)`: the system clock, in 1/60 s |
| 22 | `0x1992` | `SCAN`: mode 0 looks for equal, non-zero for unequal; the count is signed |
| 29 | `0x1841` | `ATTACH`: hook a semaphore into the interrupt vector table at `ss:4Eh` |
| 34/44 | `0x2C36` | wait for a device to finish (mode 4) |
| 36/45 | `0x2C1B` | ask a device for its status (mode 8) |
| 38 | `0x1CF6` | look up a binary tree with an 8-byte name (`+8` and `+0Ah` are the two branches) |

Not implemented: `31`/`33` (mode 3), `47` (swapping in a different set of floating-point
routines — **it rewrites the @0x1F56 table itself**), and the few whose code has not been
read. A complete boot needs none of them.

The device family is really one routine, distinguished by the mode byte at `0x2AA0`:
1 read, 2 write, 3, 4 wait, 8 status. The parameter block is at `0x2AA0`–`0x2AA8` in the
interpreter image.

Parameter order is always read off that routine's `pop` sequence. `MOVELEFT`, for
instance, pops the count first, then the destination's (base, offset) pair, and the
source last — matching `movs`'s `es:di ← ds:si`.

### When there is no key, do not answer anyway

On a real machine, reading the console **blocks**. Stepping one instruction at a time we
cannot block, so with no key available we back up to this instruction and hand
`NeedInput` back; the caller supplies `Machine.Keys` and `Run` resumes from the same
instruction.

What happens if you answer "read 0 bytes, IORESULT 0" instead: **the operating system
takes the stale characters in the buffer for new keystrokes**, and the same instruction
repeats indefinitely (in practice the Filer's memlock/swappable flickers back and forth).
This kind of error raises nothing; it just makes the machine look insane.

### The device table is decoded from `SYSTEM.CONFIG`

Those 14,848 bytes are no longer merely copied. The layout is **40 bytes per record**:

	+0  which instance within its class
	+1  class (1 console, 2 serial/parallel, 3 disk, 4/5 other)
	+3  0 means this record loads a driver, non-zero means it shares the previous one
	+4  length of the driver file name, then up to 11 characters

The disk class decodes to:

| unit | Instance | Driver |
|---:|---:|---|
| 4/5 | 0/1 | `FLOPPY.DRV` |
| 9–12 | 2–5 | `FDISK.DRV` |
| **13** | 6 | **`RAMDSK.DRV`** |
| **14** | 7 | **`DOSVV.DRV`** |
| 15–19 | 8–12 | `DOSVV.DRV` (shared driver) |

The unit numbering order for the disk class comes from the manual (4 and 5 are the first
two drives, 9–12 the third through sixth, and upward from there).
**That order agrees with measurement**: the units the original answers "present" for are
exactly 13 and 14. So where the RAM disk sits and which drive is the boot disk are now
looked up, not two hard-coded numbers.

The unit numbering rules for the other classes are unverified, so those records' numbers
are left blank — writing a guessed number in would make it impossible later to tell
which numbers were measured.

### No disk mounted is not an error

During boot the operating system asks each unit in turn. **An unmounted disk must answer
9 (no such volume), not 2 (bad unit number), and certainly not 0.** Answering 0 makes it
believe there is something there, and it then goes wrong thousands of instructions later
wearing a completely different face.

`IORESULT` also gets saved into the TIB: the high byte of `+12h` is whatever `ss:0E6h`
holds at that moment (`mov ch, ss:0E6h` at @0x160F). Saving task state must read the
copy in memory; taking a stale one swallows the device result.

### `UNITREAD`/`UNITWRITE`

The machine code of the two differs only by a mode byte (`mov al,1` at @0x2C5A,
`mov al,2` at @0x2C5F); parameter handling is shared entirely. Top-down on the stack:
mode, block, length, the buffer's (base, offset), unit — that is, Pascal's
`UNITREAD(unit, buf, length, block, mode)` pushed left to right. The result goes into
`ss:0E6h` (`IORESULT`, cleared to 0 by the first instruction at @0x2B3B).

Measured during boot: unit 1 is the console (`<ESC>H`, `<ESC>E` and the other cursor
controls come out through it), and **unit 14 is the boot disk** — an assignment of this
DOS host, not something the manual prescribes.

`ss:0E6h` is also the high byte of `MSPROC` (written at @0x1636 on a task switch).
**One byte with two jobs**, so the interpreter's state is written back there only during
a task switch; writing it on every call would overwrite `IORESULT`.

### The console is a terminal, and the control codes come from `SYSTEM.MISCINFO`

What unit 1 receives is not plain text but a byte stream with escape sequences in it.
**Those sequences are not hard-coded**: at boot the operating system reads
`SYSTEM.MISCINFO`, where from `0x3E` there is a set of `(length, bytes…)` control-code
definitions, and `0x4A`/`0x4C` give the screen height and width. This disk decodes to:

| Purpose | Sequence |
|---|---|
| lead-in | `1B` |
| cursor home | `1B H` |
| erase to end of screen / end of line | `1B J` / `1B K` |
| cursor right / left / up / down | `1B C` / `08` / `1B A` / `1B B` |
| erase whole screen / clear one line | `1B E` / `1B L` |
| absolute positioning | `1B Y` then `row+32`, `column+32` |

The screen is 25 rows × 80 columns. **`CR` implies `LF`** — with carriage return alone,
the Filer's directory listing piles up on a single row.

The host side runs a character matrix from exactly that table:
`internal/psystem/screen.go`. Unrecognised sequences are recorded in `Unknown`, never
guessed at and never swallowed — the symptom of a wrong guess is a screen that looks
"about right", and that is harder to chase than one that is plainly broken.

### Where control resumes afterwards

`SCXG` at @0x0545 saves `si` into `ss:24h` on entry, and at that moment si points at
**the procedure number**; the `inc si` at @0x1410 comes only afterwards, before the jump
into native code. Most native procedures save it again themselves (`MOVELEFT` at
@0x192C), so they resume after the procedure number.

**The few that do not are different.** Procedure 4 is one: its exit is @0x0200 (which
restores si from `ss:24h`), so it comes back to the procedure-number byte, and that byte
is executed as the next instruction — `04` is `SLDC 4`. This is not a detail that can be
rounded away; one byte out and everything after it is wrong.

## 5. Acceptance

Two metrics, kept apart:

| Test | What it pins | Now |
|---|---|---|
| `TestBootReachesTheCommandLine` | **whether it boots** (no original needed) | reaches the command line, waiting for a key |
| `TestFilerListsTheRAMDisk` | **whether it responds** (no original needed) | `F`→`L`→lists five files |
| `TestSelfBootMatchesTheOriginal` | whether the p-code it runs is the same | 226,623 instructions, the whole boot |
| `TestSelfBootInitialStateDiff` | whether the state at boot was built right | 409 regions left, 397 of them host machine code |

The first is a floor (may only rise), the second a ceiling (may only fall).

`TestSelfBootMatchesTheOriginal`: boot ourselves, and compare IPC, SP, TOS and E_Rec
against the original instruction by instruction.

**The floor pins "the boot state must not be built wrong".** Not walking to the end is
not a failure of this test — it means the host still has something unimplemented. The
floor is currently 226,000 and the actual walk is 226,623, which is to say right up to
the moment the original itself sits waiting for a key.

## 6. The scheduler

The blocking paths of `SIGNAL`/`WAIT` are the p-machine's own business, not the host's:
they touch only three lists in the data segment.

| Location | What it is |
|---|---|
| `ss:38h` | head of the ready queue. **The running task stays in it, at the front** |
| `ss:3Ch` | which task is current |
| semaphore `+2` | the list of TIBs waiting on that semaphore |

`WAIT` with no credit: remove itself from the ready queue (@0x1722), insert itself into
the semaphore's wait queue by priority (@0x16E9), record which semaphore it waits on in
`TIB+14h`, and switch.
`SIGNAL` with someone waiting: wake the head of the queue, clear its `+14h`, put it in
the ready queue, and **switch only if the woken task's priority is not lower than its
own** (the `jb` at @0x17C7).
Priority is the **low byte** of `TIB+2`; the high byte is flags.

The switch itself (@0x17E8): save state into the current TIB, count one activity tick,
`ss:3Ch` ← `ss:38h`, then restore the lot.

> The original **defers the switch to the next instruction boundary**: it fills the whole
> dispatch table with one address (@0x18A6) so that whatever the next instruction is it
> lands in the switching code, then copies the table back afterwards (@0x1892).
> That `dec si` is there to hand back the byte `lodsb` already took.
> Stepping one instruction at a time, switching at the end of this instruction is the
> same thing.

The `psystem` side **imports nothing from the oracle**: the original appears only in
tests.

## 7. Where it got stuck along the way

Each of these is worth recording, because each is the kind of thing you would never
guess at but can see the moment you measure:

**Local variables are not blank.** Before allocating local data the original `call`s into
the stack-check helper (@0x104F), and that `call` pushes the return address below the old
SP — which is exactly where **the highest-numbered local variable** ends up. During boot
a procedure with `DATASIZE` of 1 passes it out as a parameter. Fail to reproduce that
value and instruction 40,619 diverges.

**`ss:0E6h` does two jobs.** It is both `IORESULT` and the high byte of `MSPROC`. Saving
task state has to read the copy in memory at that moment; taking the stale value from a
struct swallows the device result.

**No disk mounted, device unsupported and no such file are three different answers**
(9/3/10). The operating system asks units 2 through 22 during boot, and one wrong answer
sends it down a different road.

**Unit 13 is an empty RAM disk.** 750 blocks, named `RAMDISK`, and **no files at all at
boot** — the operating system copies `SYSTEM.MISCINFO` and four other files over from the
boot disk itself, then switches the root to it. Without that disk the boot flow is
entirely different.

**Unit 128 is a gateway to the DOS host's file system.** That is specific to the DOS
version — it lets the p-System read and write the files of the DOS underneath.
**There is no DOS under a Go host, so the answer down this road is always "no"**, which
is the same kind of border as `NAT`: not "not done yet" but "that thing does not exist on
this machine".

The shape of the exchange was measured: write a seven-byte request block
`{command, buffer address, length, flags}`, and the driver replaces the first word with
status code 3; then write the data in that buffer (two bytes), and get IORESULT 10 (no
such file). The operating system asks once during boot, gets "no", and moves on.
