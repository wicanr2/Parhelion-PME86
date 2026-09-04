# 工具

都不依賴其他 repo。除了 IDA 那支之外，都在 `python:3.13-alpine` 裡跑，
沒有第三方套件。

| 檔案 | 做什麼 |
|---|---|
| `read-vol.py` | 讀 `.VOL` 磁碟映像，列目錄或抽檔 |
| `dump-routines-86.py` | 在 IDA 裡把 dispatch 表與每支常式反組譯成 JSON，並追 `call` 目標 |
| `analyze-dispatch.py` | 從那份 JSON 產統計或 256 格 markdown 表 |
| `routines-86.json` | `dump-routines-86.py` 的產出：169 支常式加 25 支助手 |
| `iv0-opcodes.json` | IV.0 官方助記符表，來源見檔內 `_source` |

## 反組譯

```
docker run --rm --network none -u "$(id -u):$(id -g)" \
  --memory 4g --cpus 2 --pids-limit 256 \
  --log-opt max-size=10m --log-opt max-file=3 \
  -v "$WORK:/work" -w /work ida-pro-9.4-idapython:locked-v1 \
  idat -A -S"/work/dump-routines-86.py /work/routines-86.json" /work/PME86.BIN
```

`$WORK` 要放 `PME86.BIN`（從 `.VOL` 抽出來的 `SYSTEM.PME.86`）與腳本本身。
headless 的 `print` 看不到，**exit code 也不能當證據**——收工要驗輸出檔存在、
`sha256` 欄位是 `fe427aa66ca8…`、`routines` 有 169 筆。

## 產表

```
docker run --rm --network none -u "$(id -u):$(id -g)" \
  --memory 1g --cpus 1 --pids-limit 64 \
  -v "$PWD:/work:ro" -w /work python:3.13-alpine \
  python tools/analyze-dispatch.py tools/routines-86.json          # 統計
  python tools/analyze-dispatch.py tools/routines-86.json --table  # markdown 表
```

`--table` 的輸出直接貼進 `docs/10-interpreter/opcode-map.md` 的表格區。
