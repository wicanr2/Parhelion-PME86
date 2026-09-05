# Segment switching: E_Rec, SIB and the code segment's header

**English** ｜ [日本語](segment-switching.ja.md) ｜ [繁體中文](segment-switching.md)

The p-machine has exactly one "current segment" at a time. A cross-segment call has to
replace the code base, the constant-pool base and the global data base, and also update
two counters so the operating system knows which segment may be swapped out. This page
takes apart the 43 instructions with which the 8086 version does it.

> See also: [interpreter state](machine-state.en.md)｜[addressing](addressing.en.md)

## Three levels of indirection

The `E_Rec` on manual p.37 is a segment's run-time descriptor. The field offsets this
version actually uses:

| Offset | Holds | Evidence |
|---|---|---|
| `+0` | `Env_Data`, the segment's global data base | the `LAE` helper adds 8 to it and then the variable offset; on a switch it is written to `word_30` and, plus 8, becomes `dx` |
| `+2` | `E_Vec`, **the array of `E_Rec` pointers indexed by segment number** | the `CXL` helper does `bp = segnum×2; bp = [bp + ss:3Ah]`, and `ss:3Ah` is this very slot |
| `+4` | `SIB` | written to `word_34` on a switch, then counted on at `+4`/`+6`, matching the manual's `Ref_Count` and `Activity` |

The manual's Pascal declaration reads `Env_Data`, `Env_SIB`, `Env_Vect`.
**In this implementation the second and third fields are the other way round.** Two
independent pieces of evidence point at `+2` being the pointer array: it is indexed by
segment number times two, and what comes out is then dereferenced as an `E_Rec`.

<p align="center"><img src="../../img/segment-chain.svg" width="960" alt="A segment number goes through E_Vec to an E_Rec; the E_Rec's three fields point at global data, E_Vec and the SIB; the code segment's first three fields feed three state variables"></p>

## The switching code

`0x0fab` has three entry points, one for each kind of caller depending on how much it
already knows:

```
0fab: d1e5      shl bp, 1            ; entry 1: bp = segment number
0fad: 032e3a00  add bp, word_3A      ;   look up E_Vec
0fb1: 8b6e00    mov bp, [bp+0]
0fb4: a13e00    mov ax, word_3E      ; entry 2: bp = target E_Rec
0fb7: a34000    mov word_40, ax      ;   remember the old E_Rec
0fba: 892e3e00  mov word_3E, bp      ; entry 3: don't remember it
0fbe: 8b5600    mov dx, [bp+0]       ; Env_Data
0fc1: 89163000  mov word_30, dx      ;   → BASE
0fc5: 83c208    add dx, 8            ;   → global data base (kept in dx)
0fc8: 89162800  mov word_28, dx
0fcc: 8b4602    mov ax, [bp+2]
0fcf: a33a00    mov word_3A, ax      ; E_Vec
0fd2: 8b7e04    mov di, [bp+4]
0fd5: 893e3400  mov word_34, di      ; SIB
0fd9: 8b4502    mov ax, [di+2]
0fdc: 3d0000    cmp ax, 0
0fdf: 7439      jz  return           ; ← ZF = segment not resident
      ...                            ; compute the code segment's 8086 segment value
0ff7: 8ec0      mov es, ax
0ff9: a32a00    mov word_2A, ax
0ffc: 33ed      xor bp, bp
0ffe: 268b460c  mov ax, es:[bp+0Ch]
1002: a34400    mov word_44, ax      ; byte sex
1005: 268b4600  mov ax, es:[bp+0]
1009: 268b6e0e  mov bp, es:[bp+0Eh]
100d: d1e0      shl ax, 1
100f: a33600    mov word_36, ax      ; procedure dictionary base
1012: d1e5      shl bp, 1
1014: 892e4200  mov word_42, bp      ; constant pool base
1018: 85ed      test bp, bp
101a: c3        retn
```

`CXL` comes in at entry 1 (it only knows the segment number), `CPF` at entry 2, `RPU` at
entry 3. **All three share the same tail** — writing "switch segment" as one routine and
letting different callers skip the first few instructions, the same trick as `SLOD1`/
`SLOD2` sharing the addressing helper.

The return value rides on ZF: the `jz` at `0x0fdf` goes straight to `retn`, so the
caller's `jz` means "segment not resident, raise a segment fault". `CXL`'s
`call 0fab; jz 0x143d` is exactly that.

## The code segment's header

The last few instructions above read three values from **the beginning of the loaded
code segment**:

| In-segment byte offset | Holds |
|---|---|
| `0x00` | word offset of the procedure dictionary; the interpreter doubles it into `word_36` |
| `0x0c` | byte-sex flag; 1 means the same as the host |
| `0x0e` | word offset of the constant pool; doubled into `word_42` |

The procedure dictionary **grows backwards**. Call helper `0x101b`:

```
101b: 8f06d600  pop word_D6          ; the caller's return address
101f: 8f06da00  pop word_DA          ; the static-chain value the caller pushed
1023: 892ed800  mov word_D8, bp      ; bp = procedure number
1027: d1e5      shl bp, 1
1029: f7dd      neg bp
102b: 032e3600  add bp, word_36      ; dictionary base − 2×procedure number
102f: 268b7e00  mov di, es:[bp+0]
1033: 85ff      test di, di
1035: 7503      jnz  continue
1037: e9f9f1    jmp  error
```

A zero dictionary entry is an error. This explains why `CPL` and `CPG` differ by one
line:

```
CPL: ff362e00  push word_2E      ; MP   — the caller's own frame as the static link
CPG: ff363000  push word_30      ; BASE — the global frame as the static link
```

Everything else in the two is identical, and both go on to `call 0x101b`.

## The two counters

After a successful cross-segment call (helper `0x10db`):

```
10db: 8b2e2e00  mov bp, word_2E      ; MP
10df: 8b3e4000  mov di, word_40      ; the old E_Rec
10e3: 897e06    mov [bp+6], di       ; MSENV ← the old E_Rec
10e6: 368b6d04  mov bp, ss:[di+4]    ; the old segment's SIB
10ea: e80800    call 0x10f5          ;   [SIB+6] ← ++counter
10ed: 8b2e3400  mov bp, word_34      ; the new segment's SIB
10f1: ff4604    inc word ptr [bp+4]  ;   Ref_Count++
```

`RPU` does the reverse: `dec word ptr [bp+4]`. `0x10f5` is only five instructions:

```
10f5: 36a11001  mov ax, ss:110h
10f9: 40        inc ax
10fa: 36a31001  mov ss:110h, ax
10fe: 894606    mov [bp+6], ax
1101: c3        retn
```

The manual says `Ref_Count` is "the number of outstanding cross-segment calls" and
`Activity` is "accumulated by use, increasing over time". The behaviour of `+4` and `+6`
matches those two descriptions respectively. Working from the manual's declared field
order, `Ref_Count` should be at `+2` and `Activity` at `+4` — **both are two bytes off,
which says the 8086 version's `Seg_Base` occupies two words, not one.**

That fits this machine's predicament too: a 16-bit address cannot reach beyond 64 KB,
and the code pool does not fit in 64 KB. Helper `0x1bee` folds those two words into an
8086 segment value:

```
1bee: 85ed      test bp, bp
1bf0: 7416      jz  → ax = ss
1bf2: 8b4602    mov ax, [bp+2]
1bf5: 8b6e00    mov bp, [bp+0]
1bf8: 51        push cx
1bf9: b104      mov cl, 4
1bfb: d3e8      shr ax, cl           ; low word >> 4
1bfd: 81e50f00  and bp, 0Fh          ; only 4 bits of the high word are meaningful
1c01: d3cd      ror bp, cl           ;   << 12
1c03: 59        pop cx
1c04: 0bc5      or  ax, bp
1c06: 7502      jnz →
1c08: 8cd0      mov ax, ss           ; a computed zero falls back to the interpreter's own segment
1c0a: c3        retn
```

That is, a 20-bit byte address stored as `{word of the high 4 bits, word of the low 16
bits}`, shifted right by four at use time to give a paragraph. Zero means "inside the
interpreter's own segment".

## How `Seg_Base` actually becomes a segment value

The two words at the head of the `SIB` are not one address but **a pointer plus an
offset**:

| word | Holds |
|---|---|
| `Seg_Base+0` | pointer to the Codepool base, **relative to the interpreter's data segment** (`ss`) |
| `Seg_Base+2` | that segment's **byte offset** within the Codepool |

```
segment value = paragraph(Codepool base) + (offset in pool ÷ 16)
```

A zero pointer means the segment lives in the interpreter's own segment
(`test bp,bp; jz` in `0x1bee`).

One set of measurements from the running system (`ss` = `0x04d1`, `ss:11ac` =
`{0001, 4d10}`, so the Codepool base is paragraph `0x14d1`):

| Segment | `Seg_Base` | Computed | Actually loaded at |
|---|---|---|---|
| `KERNEL` | `11ac 0020` | `0x14d1 + 0x002` | `0x14d3` |
| `CUPOPS` | `11ac 1420` | `0x14d1 + 0x142` | `0x1613` |
| `GETCMD` | `11ac 2a20` | `0x14d1 + 0x2a2` | `0x1773` |
| `MSGOPS` | `11ac 4620` | `0x14d1 + 0x462` | `0x1933` |
| `COMMANDI` | `11ac b420` | `0x14d1 + 0xb42` | `0x2013` |
| `SMALLCOM` | `11ac ce20` | `0x14d1 + 0xce2` | `0x21b3` |
| `REALOPS` | `11ac d820` | `0x14d1 + 0xd82` | `0x2253` |
| `USERPROG` | `0000 d800` | `ss + 0xd80` | `0x1251` |

All eight hit. The last row is the zero-pointer case.

**The zero check at `0x0fdf` looks at the second word** (the offset within the pool); an
offset of 0 means the segment is not resident — `SCREENOP`, `LOCK`, `FILEOPS` and the
other segments not loaded at that moment have zero in both words.

The rest of the `SIB` fields line up too. `Seg_Name` reads `"KERNEL  "` (byte `+12`) and
`Seg_Leng` reads 2152 (`+20`), in exactly the order declared on manual p.34 — provided
`Seg_Base` is counted as two words.

## Procedures embedded in the interpreter

Before actually switching, `CXG` asks one question first: "is this one the host's
machine code?" For segment 1 it consults the 48-entry table at `0x1F56`, and on a hit it
jumps straight into machine code without switching segments at all.

That table is the complete list of "things that are not p-code"; for the entries one by
one see [native procedures embedded in segment 1](native-intrinsics.en.md).

## Limits

- The `Seg_Base` computation was measured on the running system (all eight segments
  hit), not taken from the manual. The manual only says `Seg_Base` is a `Mem_Ptr`.
- The pointer `0x1bee` reads is relative to `ss`. From the code alone it looks like it
  should be relative to `es`; measurement says otherwise — **on this point the
  measurement wins.**
- The entry-by-entry content of the native-procedure table has moved to
  [native procedures embedded in segment 1](native-intrinsics.en.md).
