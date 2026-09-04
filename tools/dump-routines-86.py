# -*- coding: utf-8 -*-
"""把 SYSTEM.PME.86 的 dispatch 表與每一支處理常式反組譯成 JSON。
在 IDA 裡跑：

    docker run --rm --network none -u "$(id -u):$(id -g)" \
      -v "$WORK:/work" -w /work ida-pro-9.4-idapython:locked-v1 \
      idat -A -S"/work/dump-routines-86.py /work/routines-86.json" /work/PME86.BIN

輸入是從 `.VOL` 抽出來的 SYSTEM.PME.86（16384 位元組，原始碼型 binary）。
IDA 預設把它當 64-bit，要先把 segment 改成 16-bit real mode，否則整份反組譯是垃圾。

8086 版沒有共同主迴圈——fetch-dispatch（`2e ff 25` = `jmp word ptr cs:[di]`）內嵌在
每支常式的結尾。所以逐支反組譯的結束判準是「任何無條件轉移」。
`call` 的目標會排進工作佇列一起反組譯，助手常式（堆疊檢查、切段、算中介位址）
就是這樣撈出來的。
"""
import json
import struct
import sys

import ida_auto
import ida_bytes
import ida_nalt
import ida_pro
import ida_segment
import ida_ua
import idc

TABLE = 0x1d56        # dispatch 表在檔案裡的偏移，定位方式見 docs/10-interpreter/
LIMIT = 80            # 單支常式最多反組譯幾條，避免落進資料區跑不完
STOP = ('jmp', 'retn', 'retf', 'ret', 'iret')


def disasm(base, off):
    ea, out, calls = base + off, [], []
    for _ in range(LIMIT):
        ida_bytes.del_items(ea, ida_bytes.DELIT_SIMPLE, 8)
        ln = ida_ua.create_insn(ea)
        if ln <= 0:
            out.append({'ea': ea - base, 'bytes': ida_bytes.get_bytes(ea, 2).hex(),
                        'text': '<無法反組譯>'})
            break
        text = (idc.generate_disasm_line(ea, 1) or '').split(';')[0].rstrip()
        out.append({'ea': ea - base, 'bytes': ida_bytes.get_bytes(ea, ln).hex(),
                    'text': text})
        m = text.split()[0].lower() if text else ''
        if m == 'call':
            t = idc.get_operand_value(ea, 0)
            if idc.get_operand_type(ea, 0) == idc.o_near and base <= t < base + 0x4000:
                calls.append(t - base)
        if m in STOP:
            break
        ea += ln
    return out, calls


def main(out_path):
    seg = ida_segment.getnseg(0)
    ida_segment.set_segm_addressing(seg, 0)      # 16-bit real mode
    ida_auto.auto_wait()
    base = seg.start_ea
    raw = ida_bytes.get_bytes(base, 0x4000)
    tbl = [struct.unpack_from('<H', raw, TABLE + 2 * i)[0] for i in range(256)]

    routines, helpers, seen = {}, {}, set()
    todo = [(t, False) for t in sorted(set(tbl))]
    while todo:
        off, is_helper = todo.pop(0)
        if off in seen:
            continue
        seen.add(off)
        code, calls = disasm(base, off)
        (helpers if is_helper else routines)[f'{off:04x}'] = code
        todo += [(c, True) for c in calls if c not in seen]

    json.dump({'input': ida_nalt.get_root_filename(),
               'sha256': ida_nalt.retrieve_input_file_sha256().hex(),
               'base': base, 'table_off': TABLE,
               'dispatch': [f'{v:04x}' for v in tbl],
               'routines': routines, 'helpers': helpers},
              open(out_path, 'w'), indent=1)
    ida_pro.qexit(0)


main(sys.argv[1] if len(sys.argv) > 1 else '/work/routines-86.json')
