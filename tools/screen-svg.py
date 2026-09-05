# -*- coding: utf-8 -*-
"""把一段終端機輸出畫成 SVG。

README 上的「執行結果」要看得出是真的跑出來的，所以圖是**從實際輸出產生**的，
不是手畫的。改了行為重跑一次就好。

用法：
    tools/screen-svg.py 標題 < 輸出.txt > img/x.svg

或在容器裡：
    docker run --rm --network none -u "$(id -u):$(id -g)" \
      -v "$PWD:/w" -w /w python:3.13-alpine \
      python tools/screen-svg.py "命令" < out.txt > img/x.svg
"""
import html
import sys

CW, LH = 7.7, 16.5          # 字元寬、行高
PAD, TITLE = 14, 30         # 內距、標題列高
BG, FG, DIM = '#12141a', '#d7dae0', '#7f8794'
BAR, ACCENT = '#20242e', '#e08b3a'


def render(title, lines):
    cols = max([len(l) for l in lines] + [len(title) + 4, 40])
    w = cols * CW + 2 * PAD
    h = TITLE + len(lines) * LH + 2 * PAD
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w:.0f}" height="{h:.0f}" '
        f'viewBox="0 0 {w:.0f} {h:.0f}" font-family="\'DejaVu Sans Mono\','
        f'\'Menlo\',\'Consolas\',monospace">',
        f'<rect width="{w:.0f}" height="{h:.0f}" rx="8" fill="{BG}"/>',
        f'<path d="M0 8a8 8 0 0 1 8-8h{w - 16:.0f}a8 8 0 0 1 8 8v{TITLE - 8}H0z" fill="{BAR}"/>',
    ]
    for i, (cx, c) in enumerate(((16, '#e05c5c'), (34, '#e0b23a'), (52, '#5ca85c'))):
        out.append(f'<circle cx="{cx}" cy="15" r="5" fill="{c}"/>')
    out.append(f'<text x="70" y="20" fill="{DIM}" font-size="12">{html.escape(title)}</text>')

    y = TITLE + PAD + 12
    for line in lines:
        if line.strip():
            fill = ACCENT if line.startswith('$') else FG
            out.append(f'<text x="{PAD}" y="{y:.0f}" fill="{fill}" font-size="13" '
                       f'xml:space="preserve">{html.escape(line)}</text>')
        y += LH
    out.append('</svg>')
    return '\n'.join(out)


if __name__ == '__main__':
    title = sys.argv[1] if len(sys.argv) > 1 else 'terminal'
    body = [l.rstrip('\n') for l in sys.stdin.read().splitlines()]
    sys.stdout.write(render(title, body))
