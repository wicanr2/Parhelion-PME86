# spec 01: the static structure of codefiles and code segments

**English** ｜ [日本語](01-codefile.ja.md) ｜ [繁體中文](01-codefile.md)

| | |
|---|---|
| State | `CONFORMED` |
| Date | 2026-09-05 (`READY`) → 2026-09-05 (`CONFORMED`) |
| Covers | the segment dictionary chain, code segment header, routine dictionary, byte sex |
| Does not cover | the internal structure of the constant pool, relocation list, segment reference list, linker info, interface text |
| Verification sample | `SYSTEM.PASCAL` (69,632 B, 136 blocks, psys21 disk), all 28 segments parsed |

Source markers: **[manual]** is a printed page number in the IV.0 Internal Architecture
Guide; **[PME]** is a file offset in `SYSTEM.PME.86`; **[sample]** is a measurement on
`SYSTEM.PASCAL`; **[remake]** is a decision made by this implementation.

## 1. File blocking

1.1 A codefile is blocked in 512 bytes. A segment's code always starts on a block
boundary. **[manual p.30]**

1.2 Block 0 is the first record of the segment dictionary. **[manual p.27]**

## 2. Segment dictionary record (512 bytes)

2.1 One record describes up to 16 segments, and the information about one segment is
spread across six parallel arrays addressed by the same index. **[manual p.27]**

2.2 Byte layout of the record (derived field by field from the Pascal declarations on
p.28–29; the total comes to exactly 512):

| Bytes | Field | Item size |
|---|---|---|
| `0x000`–`0x03f` | `Disk_Info[16]`: `Code_Addr` (starting block), `Code_Leng` (word count) | 4 |
| `0x040`–`0x0bf` | `Seg_Name[16]`: 8 characters, unused entries blank-filled | 8 |
| `0x0c0`–`0x0df` | `Seg_Misc[16]` | 2 |
| `0x0e0`–`0x0ff` | `Seg_Text[16]`: starting block of the interface text | 2 |
| `0x100`–`0x11f` | `Seg_Info[16]` | 2 |
| `0x120`–`0x19f` | `Seg_Famly[16]` | 8 |
| `0x1a0`–`0x1a1` | `Next_Dict`: block number of the next record, 0 for none | 2 |
| `0x1a2`–`0x1af` | 7 reserved words | — |
| `0x1b0`–`0x1fd` | `Copy_Note`: `string[77]` (length byte then characters) | — |
| `0x1fe`–`0x1ff` | `Sex`: always 1 | 2 |

**[manual p.28–32]**, with the layout derived from the declarations.
**[sample]** `Copy_Note` reads
`"Copyright 1979 U.C. Regents; Copyright 1985 SofTech Microsystems"` and `Sex` reads 1 —
two fields at the very end of the record both come out right, which means the sizes of
everything in between add up correctly.

2.3 `Code_Leng` is a count of 16-bit words, **including the relocation list and
excluding the segment reference list**. **[manual p.30]**

2.4 The dictionary is a linked list; subsequent records take one block each, sandwiched
between code segments. **[manual p.27]**
**[sample]** `SYSTEM.PASCAL` has two: block 0 (16 entries) and block 133 (12 entries),
28 segments in all.

2.5 Bit layout of `Seg_Misc` (`Seg_Type` 3 bits, `Filler` 5 bits, `Has_Link_Info` 1 bit,
`Relocatable` 1 bit, in declaration order from low bits upward):

| bit | Field |
|---|---|
| 0–2 | `Seg_Type`: 0 `No_Seg`, 1 `Prog_Seg`, 2 `Unit_Seg`, 3 `Proc_Seg`, 4 `Seprt_Seg` |
| 3–7 | `Filler` |
| 8 | `Has_Link_Info` |
| 9 | `Relocatable` |

Declaration order from **[manual p.28]**; "low bits upward" is confirmed independently by
`Seg_Info` in 2.6.
**[sample]** The low three bits read as `Unit_Seg` or `Proc_Seg`, in complete agreement
with the type determined from `Seg_Famly` in 2.7.

> ⚠ **The semantics of bit 8 and bit 9 are unverified.** Across the 28 sample segments
> `Seg_Misc` takes only two values, `0x0202` and `0x0203`; bit 9 is always 1 and bit 8
> always 0 — **no variation means no discriminating power**. Besides, the relocation
> list pointer of all 28 segments is 0, which does not sit well with "bit 9 =
> `Relocatable`". The implementation decodes them in declaration order but relies on
> neither bit for any decision.

2.6 Bit layout of `Seg_Info` (exactly 16 bits in total):

| bit | Field |
|---|---|
| 0–7 | `Seg_Num`: local segment number |
| 8–11 | `M_Type`: 0 `M_Psuedo`, 2 `M_PDP_11`, 3 `M_8080`, 9 `M_8086`, 11 `M_68000` … |
| 12 | `Filler` |
| 13–15 | `Major_Version`: 4 = `IV` |

**[manual p.28]**.
**[sample]** `KERNEL` reads `0x8001` → `Seg_Num` 1, `M_Type` `M_Psuedo`, version `IV`.
Laid out from the high bits downward instead, `Seg_Num` would be 128 rather than 1 —
**this item simultaneously establishes the packing direction of every packed record in
the declaration**, on which 2.5 depends.

2.7 `Seg_Famly` is a variant record: `Prog_Seg`/`Unit_Seg` gives four integers
(`Data_Size`, `Seg_Refs`, `Max_Seg_Num`, `Text_Size`); `Proc_Seg`/`Seprt_Seg` gives the
8-character name of the enclosing program or unit. **[manual p.29]**
**[sample]** `ASSOCQUI` (a `Proc_Seg`) reads `"ASSOCIAT"`, `SEGFOPEN` reads
`"FILEOPS "`, `USERPROG` reads `"KERNEL  "` — all of them corresponding to segments that
exist in the same file.

## 3. Code segment header (11 words)

3.1 The header sits at the segment's lowest address: **[manual p.8]**

| word | Byte | Field |
|---|---|---|
| 0 | `0x00` | routine dictionary pointer (in-segment word offset) |
| 1 | `0x02` | relocation list pointer |
| 2–5 | `0x04` | segment name, 8 characters |
| 6 | `0x0c` | byte sex indicator |
| 7 | `0x0e` | constant pool pointer (in-segment word offset), 0 for no pool |
| 8 | `0x10` | `REALSIZE`: 2 or 4 words |
| 9–10 | `0x12` | reserved |

3.2 All three pointers are **relative to the segment**, because the Codepool moves whole
segments around. **[manual p.9]**

3.3 **[PME]** On a segment switch the 8086 interpreter reads exactly those three slots:
in-segment `+0x00` doubled into `ss:36h` (procedure dictionary base), `+0x0c` into
`ss:44h` (byte sex), `+0x0e` doubled into `ss:42h` (constant pool base).
**Manual and implementation corroborate each other at three independent offsets.**
The evidence is helper `0x0fba` in `docs/10-interpreter/segment-switching.en.md`.

3.4 **[sample]** All 28 segment headers parse: the names are readable and `REALSIZE` is
always 4 (matching `$R4`, i.e. 64-bit reals, consistent with `LDCRL` making room for
8 bytes on the stack).

3.5 **[sample]** The routine dictionary pointer of all 28 segments is **always equal to
`Code_Leng − 1`**. This is an observed invariant, not something the manual states; the
implementation **must not** depend on it, but may use it as a sanity check.

> **The two reserved words are not zero.** In the sample they vary by segment (for
> instance `KERNEL` gives `(4184, 38466)` and `GOTOXY` gives `(4199, 5733)`), and
> segments from the same compilation unit often share a value. Purpose unknown; listed
> as an open item.

## 4. Byte sex

4.1 The header's byte-sex indicator is **always 1** on the machine that produced the
segment; read with the opposite byte order it becomes 256. **[manual p.10]**

4.2 The affected word-oriented information falls into two classes: **superstructure**
(the routine dictionary, for instance) is flipped by the operating system at load time;
**embedded** (constants fetched by `LDC`, the case table of `XJP`) is flipped by the
interpreter at run time. **[manual p.10]**

4.3 **[PME]** The interpreter's half really is there: when `ss:44h` is not 1, `MOV`,
`LDC` and `XJP` take the byte-swapping path word by word.

4.4 **[sample]** Of `SYSTEM.PASCAL`'s 28 segments, **six** (`STRINGOP`, `HEAPOPS`,
`LOCK`, `COMMANDI`, `PERMHEAP`, `SMALLCOM`) have an indicator that reads 256.
**One codefile carries both byte orders, and it is the operating system itself.**
So the reader has to handle byte sex from day one; it cannot be treated as an edge case
to be added later.

4.5 **[remake]** This implementation **does not flip segments in place**. The header and
routine dictionary are decoded into Go values according to byte sex at parse time, the
constant pool and code are left as they are, and the byte-sex flag is kept with the
segment for the run time to handle per the division in 4.2.

The original modifies the copy loaded into memory in place; the reason for not following
suit is that the remake has no notion of "in place", and keeping the original bytes is
what makes co-simulation possible. **This is a choice of representation and changes no
observable behaviour.**

## 5. Routine dictionary

5.1 The segment's first word points at the routine dictionary. The dictionary is a run
of pointers to routine code, each an in-segment word offset. Word 0 of the dictionary is
the number of routines in the segment. Routines are numbered 1..255, and the number is
the index. **[manual p.10]**

5.2 **The dictionary grows toward lower addresses**: the pointer for routine n is at
in-segment word `dict − n`.

- **[PME]** Call helper `0x101b` computes `word_36 − 2×procedure number`, and `word_36`
  is `2 × header word 0`.
- **[sample]** Parsing all 28 segments in that direction, not one of the 465 dictionary
  entries goes out of bounds; the other direction runs off the end of the segment
  immediately.

5.3 The dictionary entry for `EXTERNAL` and `FORWARD` routines is 0. **[manual p.10]**
**[sample]** The 28 segments hold 35 empty entries, 26 of them in `KERNEL` alone.
**[PME]** The interpreter's response to a zero entry is a jump to a runtime error
(`test di, di` in `0x101b`).

5.4 A routine's layout within the segment puts `EXITIC`, `DATASIZE` and the first
instruction adjacent, and **the dictionary entry points at the middle one,
`DATASIZE`**:

| In-segment word | Contents |
|---|---|
| `ptr − 1` | `EXITIC`: the code to run when leaving this routine, as an in-segment byte offset |
| `ptr` | `DATASIZE`: word count of local data, excluding parameters |
| `ptr + 1` | the first executable instruction |

- **[manual p.11]** says a routine's code "begins with two words — `DATASIZE` and
  `EXITIC`", and in the same passage that "the first executable instruction immediately
  follows `DATASIZE`". The first sentence reads as if `DATASIZE` comes first; the second
  only holds if `EXITIC` comes first — **the manual does not pin the order down.**
- **[PME]** Call helper `0x103a` does pin it down: after `shl di, 1` turns the dictionary
  entry into a byte offset, `mov cx, es:[di]` fetches `DATASIZE`, and only then does
  `inc di; inc di` point at the code to execute. The next line, `or cx, cx; js`, is the
  sign test for native code.
- **[sample]** Across the 430 routines that have code, reading `w(ptr)` as `DATASIZE`
  gives a median of 3, p90 of 76 and a maximum of 5121, with none negative; reading
  `w(ptr−1)` as `EXITIC` puts **not one out of bounds, and 418 of them straight at byte
  `0x96`, which is `RPU`**. Taking the manual's literal order instead (`EXITIC` at
  `ptr+1`) gives **299 out of bounds and 0 pointing at `RPU`**.

5.5 A negative `DATASIZE` means the first instruction is native code, with the value in
one's complement; `EXITIC` is then undefined and must not be read. **[manual p.11]**
**[sample]** None of `SYSTEM.PASCAL`'s 430 routines is negative, consistent with the
dictionary marking all 28 segments `M_Psuedo`.

## 6. What this spec authorises

The reader may: list every segment of a codefile; give each segment's name, number,
type, machine type, byte sex, constant pool location and `REALSIZE`; and give each
routine's code start along with `DATASIZE` and `EXITIC`.

**Not authorised**: interpreting the contents of the constant pool, the relocation list
or the segment reference list, nor any execution semantics. Each of those needs a spec
of its own.

## 7. Same-state verification

The bar for `CONFORMED` is not "tested" but **compared in the same state as the
original**. The method is to get the 1984 DOS p-System running (see `oracle/`), record
its p-code instruction by instruction while it executes `SYSTEM.PASCAL`, and compare
that against what the reader decoded. The test is `oracle/conform_test.go`.

**Verified:**

- Bytes 4–11 of the header of the code segment the original is running read
  `"USERPROG"` — and the reader's segment names, decoded from the same file, include it
  (the header layout of 3.1).
- That segment's contents in memory agree with `Code_Leng` words in the file for
  **over 99%** (the length definition of 2.3, the segment-relative offsets of 3.2).
- **For every IPC across 400 p-code instructions, the byte in memory and the byte at the
  same offset in the file are identical.** This binds "the in-segment offset the reader
  computed" to "the address the original fetches from" — one word out and the whole
  thing would mismatch.
- Every IPC falls after the header and before the routine dictionary (3.1, 5.1).

**Not verified** (written out just the same; not verified ≠ wrong):

- The semantics of `Seg_Misc` bit 8/bit 9 (2.5 already marks them unverified; the sample
  has no variation).
- The variant record of `Seg_Famly` (2.7 has only static self-consistency in support).
- For segments with the opposite byte sex, the details of the operating system flipping
  the routine dictionary (4.2) — the trace so far only touches `USERPROG`, which matches
  the host's order.
- `DATASIZE`/`EXITIC` (5.4) were pinned down from the interpreter's code plus statistics
  over 430 routines, not by intercepting a call sequence during execution.

## 8. Questions this round leaves open

- Header words 9 and 10 ("reserved" per the manual) are not zero in the sample and vary
  by segment. The first routine's `EXITIC` usually lands at word 11 and its `DATASIZE`
  at word 12, so those two words really do belong to no routine. Purpose unknown.
- Twelve of the 430 routines have an `EXITIC` that does not point directly at `RPU`
  (pointing at `0x30`, `0x84`, `0x60` and the like). The plausible reading is that the
  exit sequence is more than one instruction, but they were not read one by one.
