# The first 512 bytes of `SYSTEM.PME.86`

**English** ｜ [日本語](pme86-header.ja.md) ｜ [繁體中文](pme86-header.md)

The very front of the file looks like a pile of constants with no pattern: 18 non-zero
words, then zeros all the way, a few more scattered after `0x0100`, and only then code.
Those values are not instructions.

```
offset 0x00: 3a04 0000 0c9b 0000 00e6 0026 0028 003e
offset 0x10: 02fe 185a 1865 1870 1874 0306 1281 11bc
offset 0x20: 11ba 11af 0000 0000 …
```

The conclusion first: **that is the interpreter's own initialised data.** The loader
copies those 512 bytes verbatim into **the same offset in the data segment**, and then
moves the dispatch table from `0x1d56` to image offset 0, covering them up. So they are
invisible in the code segment — they live only in the data segment, and they live there
until the system is shut down.

> See also: [interpreter state](../10-interpreter/machine-state.en.md)｜
> [fetch and dispatch](../10-interpreter/dispatch-and-threading.en.md)

## How this was established

Run the system, and **at the moment the first p-code instruction executes** compare the
first 512 bytes of the data segment against the 512 bytes on disk, word by word:

```
of the non-zero words in the file's first 512 bytes, 37 out of 37 arrive in the
data segment untouched
```

Not one differs. The zeros are blanks reserved for run-time state — `IPC`, `MP`, `E_Rec`
and the like, filled in by the bootstrap before it hands over.

There is exactly one exception: `0x06` is 0 in the file and `0x0140` in memory — the
address of the boot parameter area, filled in by the bootstrap.

## What is in there

Those 37 words are **the interpreter introducing itself to the operating system**, plus
a few initial values. The OS proper is written in p-code, and it needs to know where the
interpreter keeps its state and where to jump when it wants the interpreter to do
something:

| Offset | What it is |
|---|---|
| `0x08` | address of SYSCOM (`0x00E6`) |
| `0x0A` | offset, within the state area, of the local data base (`MP + 8`) |
| `0x0C` | offset of the global data base (`Env_Data + 8`) |
| `0x0E` | offset of the current `E_Rec` |
| `0x12`–`0x18` | entry points of four native callbacks (`185A`/`1865`/`1870`/`1874`) |
| `0x1A`–`0x22` | other interpreter code addresses |
| `0x44` | initial value of the byte-sex flag (1 = same as the host) |
| from `0xE6` | SYSCOM: device count and parameters |
| from `0x140` | boot parameters, plus the `MSSTAT`/`MSDYN` of the outermost frame |
| `0x178`/`0x17A` | global variable 1, pointing at SYSCOM |

Those four callback addresses look like this in the interpreter (from `0x185A`):

```
185a: mov bp, sp
185c: mov di, [bp+4]
185f: call 0x1791      ; SIGNAL
1862: lret 2
1865: mov bp, sp
1867: mov di, [bp+4]
186a: call 0x1878      ; an interrupt arrived: SIGNAL the semaphore on that vector
186d: lret 2
1870: call 0x1837      ; disable interrupts
1873: lret
1874: call 0x183c      ; enable interrupts
1877: lret
```

The `lret` says they are **meant for far calls** — the OS comes in from the p-code side
via `NAT` and the `lret` takes it back. This is precisely the door between "the world of
p-code" and "the world of machine code".

## Why it was done this way

A piece of data at a **fixed position at the very front of the file** is something the
loader can copy mindlessly, without having to recognise a single slot inside it. When
the interpreter changes version, only its own table changes; the loader stays put.

That it shares those 512 bytes with the dispatch table is no coincidence either:
**that space is useless to the code segment after loading** (the table has to move to
offset 0 so that `jmp word ptr cs:[di]` needs no displacement), so it is used to hold a
block of data that wants copying out. Not one byte wasted.

## Limits

- The two slots `0x3a04` and `0x0c9b` are unexplained. By value one is near the end of
  the file and the other falls between the error handling and the addressing helpers,
  but neither was traced to a user.
- The addresses after `0x12` belong to **this particular interpreter**. A remade machine
  has no counterpart for them — the OS only needs them on a `NAT` call, and `NAT`
  requires an actual 8086.
- "37 out of 37 words" is measured (`TestInterpreterDataComesFromItsOwnHeader`); "so it
  is initialised data" is the most direct reading of that measurement.
