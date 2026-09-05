# Tasking and task switching: three lists and one trick

**English** ｜ [日本語](tasking.ja.md) ｜ [繁體中文](tasking.md)

UCSD Pascal has `PROCESS`, `START` and `SEMAPHORE` — real concurrency on a machine from
1978. This page looks at how the 8086 version does it.

The conclusion first: **it touches only three lists in the data segment**, and at the
moment of a switch it uses a trick that looks brutal but saves the entire test —
**filling the whole dispatch table with one address**.

> See also: [interpreter state](machine-state.en.md)｜
> [fetch and dispatch](dispatch-and-threading.en.md)

## The three lists

| Location | What it is |
|---|---|
| `ss:38h` | head of the ready queue. **The running task stays in it, at the front** |
| `ss:3Ch` | which task is current (points at its TIB) |
| semaphore `+2` | the list of TIBs waiting on that semaphore |

The list's "next" lives at TIB `+0`, and the priority in the **low byte** of `+2`
(the high byte is flags; `mov al,[bx+2]` at @0x17C4 takes one byte only).

The other TIB fields in use:

| Offset | What it is |
|---|---|
| `+08` | SP |
| `+0A` | MP |
| `+0E` | IPC |
| `+10` | E_Rec |
| `+12` | MSPROC + `ss:0E6h` (high byte) |
| `+14` | which semaphore it waits on; 0 if none |

## Semaphores

A semaphore is two words: a count and the head of a wait queue.

**`WAIT` (@0x1755)**: if the count is non-zero, decrement and leave. If it is zero,
remove itself from the ready queue (@0x1722), insert itself into the semaphore's wait
queue by priority (@0x16E9), record which semaphore it waits on in `TIB+14h`, and switch.

**`SIGNAL` (@0x17D5 → helper @0x1791)**: if the count is negative or nobody is waiting,
increment and leave. If someone is waiting, wake the head of the queue, clear its `+14h`,
put it into the ready queue by priority, and **switch only if the woken task's priority
is not lower than its own** (the `jb` at @0x17C7).

The queueing routine (@0x16E9) stops in front of the first entry whose priority is
**lower** than its own — equal priorities go behind, so whoever waited first is woken
first.

## The switching trick

A task switch cannot happen in the middle of an instruction. Rather than testing "can we
switch right now?", the original **fills all 256 cells of the dispatch table with the
same address**:

```
18a6: push es; push di; push cx
18a9: xor di,di
18ac: push cs; pop es
18ae: mov cx, 0x100
18b1: rep stosw          ; all 256 words set to ax
```

So **whatever the next instruction is, it lands at that address.** After switching, the
table is copied back (@0x1892, moving 512 bytes from `0x1D56` to offset 0).

The landing point is @0x17E8:

```
17e8: dec si             ; give back the byte lodsb already took
17e9: call 0x15F5        ; save the whole state into the current TIB
17ec: call 0x1892        ; copy the dispatch table back
17ef: bp = ss:34h; call 0x10F5   ; count one activity tick for the current segment
17f7: call 0x1837        ; disable interrupts
17fa: bp = ss:38h        ; head of the ready queue
17ff: ss:3Ch = bp        ;   becomes the current task
1804: call 0x1623        ; load the whole state back from the new TIB
1807: call 0x183c        ; enable interrupts
180a: is the new segment resident? if not, raise a segment fault
```

That `dec si` is the price of the trick: the trap only takes effect **after** the
`lodsb`, so the byte has to be handed back.

**What makes the trick worth remembering is what it avoids.** "Can we switch now?" is a
question that would have to be asked on every single instruction; turning it into "let
the next fetch fall into the trap by itself" means never asking it. The cost is moving
512 bytes twice at the moment of a switch — and switches are rare, so the trade pays.

A remade machine that steps one instruction at a time needs none of this: switching at
the end of an instruction is the same thing. But **without knowing the trick, that
`dec si` looks like a bug.**

## Save and restore

`@0x15F5` saves: `[TIB+8]=SP`, `+0A=MP`, `+0E=IPC`, `+10=E_Rec`,
`+12=MSPROC|ss:0E6h<<8`.

`@0x1623` restores `MSPROC` first, then `E_Rec` (which drags `Env_Data`, `E_Vec`, `SIB`,
the code segment value, the procedure dictionary, byte sex and the constant pool along
with it), and finally `IPC`, `MP`, `SP`. **It does not check whether the segment is
resident** — that is the caller's job (@0x180A).

`SPR` (store processor register) takes the same road: save into the TIB, write the slot,
read the whole state back. If what it writes is register −1 (`ss:3Ch`), that step *is*
the task switch.

## Limits

- Ready-queue insertion and removal, both semaphore paths and the switching sequence
  were all read instruction by instruction, and actually exercised in
  [self-boot](../30-remake/specs/03-boot.en.md).
- On the interrupt side only `ATTACH` (hooking a semaphore into the vector table at
  `ss:4Eh`) and the dispatch at @0x1878 were read; the actual interrupt sources were not
  pursued.
- `TIB+04`/`+06` (the stack bounds) are filled by the bootstrap; how they are computed
  is still open.
