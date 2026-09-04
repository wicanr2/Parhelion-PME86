# -*- coding: utf-8 -*-
"""分析 PME.86 的 dispatch 表：分類統計，或輸出完整的 256 格對照表。

輸入是 `dump-routines-86.py` 在 IDA 裡產生的 JSON，以及本目錄的 `iv0-opcodes.json`。

用法：
    python tools/analyze-dispatch.py routines-86.json           # 統計
    python tools/analyze-dispatch.py routines-86.json --table   # markdown 表
"""
import collections
import json
import os
import sys

ERR11 = 0x025a          # mov bp,11 → 未實作指令
ERR14 = 0x0269          # mov bp,14
ERR16 = 0x026e          # mov bp,16（BPT）


def load(path):
    d = json.load(open(path, encoding='utf-8'))
    return d, [int(x, 16) for x in d['dispatch']]


def names():
    p = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'iv0-opcodes.json')
    return {int(k): v for k, v in json.load(open(p, encoding='utf-8'))['opcodes'].items()}


def fmt(ops):
    xs = sorted(ops)
    if not xs:
        return '—'
    out, s, p = [], xs[0], xs[0]
    for x in xs[1:]:
        if x == p + 1:
            p = x
        else:
            out.append((s, p)); s = p = x
    out.append((s, p))
    return '、'.join(f'0x{a:02x}' if a == z else f'0x{a:02x}–0x{z:02x}' for a, z in out)


def main(path, mode):
    d, tbl = load(path)
    iv0 = names()
    cnt = collections.Counter(tbl)
    routines = d['routines']

    if mode == 'table':
        print('| opcode | IV.0 助記符 | 常式 | 指令數 | 備註 |')
        print('|---|---|---|---|---|')
        seen = set()
        for op in range(256):
            t = tbl[op]
            code = routines.get(f'{t:04x}', [])
            note = ''
            if t == ERR11:
                note = '未實作'
            elif t == ERR14:
                note = '錯誤 14'
            elif t == ERR16:
                note = '錯誤 16'
            elif cnt[t] > 1:
                note = f'與另外 {cnt[t]-1} 格共用'
            print(f"| `0x{op:02x}` | `{iv0.get(op, '—')}` | `0x{t:04x}` | {len(code)} | {note} |")
        return 0

    print(f"輸入 {d['input']}  sha256 {d['sha256'][:12]}…")
    print(f"dispatch 表 @0x{d['table_off']:04x}，256 項，{len(set(tbl))} 個相異目標")
    print()
    groups = {
        '未實作（錯誤 11）': [i for i in range(256) if tbl[i] == ERR11],
        '錯誤 14': [i for i in range(256) if tbl[i] == ERR14],
        '錯誤 16（BPT）': [i for i in range(256) if tbl[i] == ERR16],
    }
    for k, v in groups.items():
        if v:
            print(f'{k:<18} {len(v):>3} 格  {fmt(v)}')
    shared = [(a, n) for a, n in cnt.items() if n > 1 and a not in (ERR11, ERR14, ERR16)]
    print(f'{"多格共用的常式":<18} {len(shared):>3} 個  涵蓋 {sum(n for _, n in shared)} 格')
    for a, n in sorted(shared, key=lambda x: -x[1]):
        ops = [i for i in range(256) if tbl[i] == a]
        print(f'    0x{a:04x} ×{n:<3} {fmt(ops)}  {iv0.get(ops[0], "?")}…')
    solo = [a for a, n in cnt.items() if n == 1 and a not in (ERR11, ERR14, ERR16)]
    print(f'{"專屬常式":<18} {len(solo):>3} 個')
    lens = sorted(len(routines[f'{a:04x}']) for a in solo)
    print(f'    指令數 中位數 {lens[len(lens)//2]}、最長 {lens[-1]}')
    return 0


if __name__ == '__main__':
    if not sys.argv[1:]:
        sys.exit(__doc__)
    sys.exit(main(sys.argv[1], 'table' if '--table' in sys.argv else 'stats'))
