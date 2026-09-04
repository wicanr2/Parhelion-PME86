# -*- coding: utf-8 -*-
"""讀 UCSD p-system 的 `.VOL` 磁碟映像：列出目錄，或把檔案抽出來。

目錄版面照 IV.0 手冊 p.125 的 Figure 6：`array [0..77] of direntry`，
每筆 26 位元組，目錄從 block 2 開始（block 0–1 是 bootstrap）。
數值一律 little-endian。

    dir[0]（volume）      dir[1..77]（檔案）
    0–1  dfirstblk        0–1  dfirstblk
    2–3  dlastblk         2–3  dlastblk
    4–5  dfkind           4–5  status/dfkind
    6–13 dvid（長度+7）   6–21 dtid（長度+15）
    14–15 deovblk         22–23 dlastbyte
    16–17 dnumfiles       24–25 daccess

`dlastblk` 是「最後一個 block 的下一個」，所以檔案大小 =
(dlastblk − dfirstblk − 1) × 512 + dlastbyte。

用法：
    docker run --rm --network none -u "$(id -u):$(id -g)" \
      -v "$PWD:/work" -v /path/to/vols:/v:ro -w /work python:3.13-alpine \
      python tools/read-vol.py /v/PSYSTEM.VOL              # 列目錄
      python tools/read-vol.py /v/PSYSTEM.VOL -x OUTDIR [檔名...]   # 抽檔
"""
import os
import struct
import sys

BLOCK = 512
DIR_BLOCK = 2
ENTRY = 26
KIND = {0: 'untyped', 1: 'xdsk', 2: 'code', 3: 'text',
        4: 'info', 5: 'data', 6: 'graf', 7: 'foto', 8: 'securedir'}


def pstr(b):
    """UCSD 字串：一個長度位元組後接字元"""
    return bytes(b[1:1 + b[0]]).decode('ascii', 'replace')


def read_dir(data):
    d = data[DIR_BLOCK * BLOCK: DIR_BLOCK * BLOCK + 4 * BLOCK]
    vid = pstr(d[6:14])
    eov, nfiles = struct.unpack_from('<HH', d, 14)
    files = []
    for i in range(1, nfiles + 1):
        o = i * ENTRY
        first, last, kind = struct.unpack_from('<HHH', d, o)
        name = pstr(d[o + 6:o + 22])
        lastbyte = struct.unpack_from('<H', d, o + 22)[0]
        size = (last - first - 1) * BLOCK + lastbyte
        files.append({'name': name, 'first': first, 'last': last,
                      'kind': KIND.get(kind & 15, kind & 15), 'size': size})
    return vid, eov, files


def main(argv):
    path = argv[0]
    data = open(path, 'rb').read()
    vid, eov, files = read_dir(data)
    blocks = len(data) // BLOCK
    note = '' if eov == blocks else f'  ⚠ 目錄說 {eov} blocks，檔案是 {blocks}'
    print(f'volume "{vid}"  {eov} blocks  {len(files)} files{note}')

    if '-x' not in argv:
        for f in files:
            print(f"  {f['name']:<18} blk {f['first']:>4}–{f['last']:<4} "
                  f"{f['size']:>7} B  {f['kind']}")
        return 0

    i = argv.index('-x')
    outdir = argv[i + 1]
    want = set(argv[i + 2:])
    os.makedirs(outdir, exist_ok=True)
    for f in files:
        if want and f['name'] not in want:
            continue
        blob = data[f['first'] * BLOCK: f['first'] * BLOCK + f['size']]
        out = os.path.join(outdir, f['name'].replace('/', '_'))
        open(out, 'wb').write(blob)
        print(f"  {f['name']:<18} {f['size']:>7} B -> {out}")
    return 0


if __name__ == '__main__':
    if not sys.argv[1:]:
        sys.exit(__doc__)
    sys.exit(main(sys.argv[1:]))
