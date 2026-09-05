# Remaking a p-machine in Go: a feasibility assessment

**English** ｜ [日本語](feasibility.ja.md) ｜ [繁體中文](feasibility.md)

This assessment answers one concrete question: **is the evidence in hand enough to
support a Go interpreter that can run the 1984 DOS p-system?**

The conclusion first: **the core interpreter is doable, and it is the part of the whole
thing we are surest about; the risk sits in the standard procedures and I/O, not in the
instruction set.** The decisive factor is that this job has an oracle — the original
interpreter and the original disks are both on hand and can be co-simulated against.

> See also: [the spec gate](spec-workflow.en.md)｜
> [the 256-cell map](../10-interpreter/opcode-map.en.md)

## Six layers, taken one at a time

| Layer | Needs | Evidence in hand | Verdict |
|---|---|---|---|
| 1 instruction execution | 256 cells of semantics, the stack model, activation-record layout | IV.0 manual, instruction by instruction; all 169 routines of the 8086 version disassembled; the 68000 version available for cross-checking | **doable** |
| 2 codefile format | segment dictionary, segment header, procedure dictionary, constant pool, jump table | manual chapter 2; the actual offsets of three header fields read out of the 8086 version | **doable, needs another round of decoding** |
| 3 standard procedures (CSP) | the parameters and behaviour of each | 33 embedded as machine code inside the interpreter; the rest live in `SYSTEM.PASCAL` | **the biggest unknown** |
| 4 I/O and file system | unit read/write, screen, keyboard, `.VOL` directory | the manual's three-layer I/O, in translation; the `.VOL` format already implemented | **a lot of work, but mechanical** |
| 5 memory management | Codepool, segment swap-in/out, `Ref_Count`/`Activity` | the switching code is decoded | **the remake can skip it** |
| 6 floating point | the bit format of 8-byte reals and the four operations | 16 dedicated routines; the format resolves to IEEE 754 binary64 | **doable** |

Layer 5 deserves a word. The original swaps segments out of memory because a 1984
machine could not hold them all; the Go version has no such constraint and can keep
every segment resident. **This is deliberate non-conformance, not something left
undone** — the difference has to go into the spec, or in three months nobody will be
able to tell the two apart.

## Why the core instruction set is the surest part

Three independent sources corroborate each other:

1. **The official IV.0 manual** states semantics and stack effects instruction by
   instruction.
2. **The 8086 interpreter**'s 169 routines are all disassembled, with a median of 8
   instructions — short enough to read end to end.
3. **The 68000 interpreter** (SunDog, 1985) is another port of the same version, with 98
   routines already verified one by one. Two spellings of the same instruction on two
   instruction sets are each other's check.

On top of that, **the source of UCSD Pascal I.5 is public** — including the PDP-11
interpreter and the operating system source. It is an older version, but the semantics
of many instructions did not change, so it serves as a fourth source.

When three implementations and a manual all point at the same meaning, there is little
room left to be wrong.

## Having an oracle is what makes this controllable

The full differential-testing loop is ready-made:

```
Pascal source
   │  psys21's SYSTEM.COMPILER (running in DOSBox)
   ▼
codefile (p-code)
   ├─────────────────────► the original SYSTEM.PME.86 (DOSBox) ──► output A
   └─────────────────────► the Go p-machine ────────────────────► output B
                                                    compare A and B
```

What that buys: **"my understanding of this instruction is wrong" becomes a failing
test, rather than a discrepancy found three months later.** A remake with no oracle can
only lean on the confidence of whoever read the code; with an oracle, confidence can be
converted into a signal.

The price is getting psys21 running inside DOSBox first, with input fed and output
captured automatically. That had never been done, so it is listed as a prerequisite of
the first milestone.

## Staged

| Milestone | Deliverable | What counts as passing |
|---|---|---|
| **M0** | codefile reader | prints segment names and procedure dictionary for `SYSTEM.PASCAL`, consistent with the manual and self-consistent |
| **M1** | interpreter for the pure-computation subset | hand-written p-code sequences give the same output on the Go version and the original under DOSBox |
| **M2** | multi-segment calls and some CSPs | a small program compiled by `SYSTEM.COMPILER` runs to completion |
| **M3** | runs `SYSTEM.PASCAL` | a prompt appears and accepts one command |

M0 is entirely static with no unknowns; M1 needs the oracle in place; M2 starts touching
layer 3, and the work depends on how many CSPs a small program actually uses; M3 is the
answer to "can it really run", and should not be claimed before it is reached.

All four milestones have been reached. M3's actual result goes one step beyond the bar
written here: not only does the prompt appear, the Filer can be entered and a directory
listed, and the 226,623 p-code instructions of the whole boot match the original one for
one (see [self-boot](specs/03-boot.en.md)).

## Known obstacles

- **The manual is IV.0, the disks are IV.2.1.** The opcode numbering of the two has been
  compared cell by cell, but identical semantics instruction by instruction has not been
  verified, and one cell is already known to be allocated differently (`0xff`). Any
  semantics supported only by the manual, with no interpreter to corroborate it, has to
  be treated as an assumption.
- **`SYSTEM.PASCAL` has no source.** The IV.x operating system can only be a black box.
  If M3 gets stuck, debugging can only lean on oracle co-simulation, not on reading
  source.
- **The 33 embedded machine-code procedures** must each have their semantics read out;
  there is no shortcut. They are what `SYSTEM.PASCAL` depends on directly.
- **The real format was unresolved at the time.** It was later decoded from the helper at
  @0x22a8 as IEEE 754 binary64 (the manual's BCD claim does not hold for this version),
  and the four operations plus comparison have all been co-simulated.

## Where this judgement could be wrong

The easiest thing to overestimate is layer 3. "33" was counted off a table, but that is
only the ones in segment 1 that **have an embedded implementation**; how many other CSPs
`SYSTEM.PASCAL` reaches for is speculation until a program is actually compiled and its
references inspected.

The easiest thing to underestimate is the distance between M2 and M3. The things the
operating system uses — task switching, segment faults, interrupts, the clock — appear
in no small program at all.
