#!/usr/bin/env bash
# Go 走 docker，不裝到系統環境。
#
#   tools/go.sh test ./...
#   tools/go.sh vet ./...
#   DOSGOLEM_GO_CMD=gofmt tools/go.sh -l .
#
# oracle 那一層要 dosgolem（同作者的無頭 DOS 執行器）。它不在 go.mod 裡——
# 對一個還沒發版的模組寫 require，會讓每個 package 都去抓 proxy，
# 而建置容器是 --network none，結果是整個專案編不過。
# 改用 Go workspace 接**本機副本**：
#
#   PARHELION_DOSGOLEM=~/cht/dosgolem-psys tools/go.sh test -tags oracle ./oracle/
#
# 原版素材（PSYSTEM.COM、.VOL、抽出來的 SYSTEM.PME.86）由使用者自備，
# 用 PARHELION_ORIG 指過去，唯讀掛到容器裡的 /orig。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${PARHELION_GO_IMAGE:-golang:1.26.7-bookworm}"
mkdir -p "$ROOT/workplace/gocache" "$ROOT/workplace/gomodcache"

MOUNTS=()
PASS=()
for v in GOOS GOARCH CGO_ENABLED PARHELION_CODEFILE PARHELION_ORIG PARHELION_PME; do
  [[ -n "${!v:-}" ]] && PASS+=(-e "$v=${!v}")
done
[[ -n "${PARHELION_ORIG:-}" ]] && MOUNTS+=(-v "$(cd "$PARHELION_ORIG" && pwd):/orig:ro")

# dosgolem 在旁邊就接起來。go.work 是容器裡才成立的東西，不進版控。
if [[ -n "${PARHELION_DOSGOLEM:-}" ]]; then
  MOUNTS+=(-v "$(cd "$PARHELION_DOSGOLEM" && pwd):/dosgolem:ro")
  cat > "$ROOT/go.work" <<'WORK'
go 1.24

use .
use /dosgolem
WORK
else
  rm -f "$ROOT/go.work"
fi

exec timeout "${PARHELION_TIMEOUT:-30m}" docker run --rm --network none \
  --memory "${PARHELION_MEM:-4g}" --cpus "${PARHELION_CPUS:-4}" --pids-limit 256 \
  --log-opt max-size=10m --log-opt max-file=3 \
  -u "$(id -u):$(id -g)" \
  -v "$ROOT:/src" \
  -v "$ROOT/workplace/gocache:/gocache" \
  -v "$ROOT/workplace/gomodcache:/gomodcache" \
  -e GOCACHE=/gocache -e GOMODCACHE=/gomodcache \
  -e HOME=/tmp \
  "${PASS[@]}" "${MOUNTS[@]}" -w /src "$IMAGE" "${PARHELION_GO_CMD:-go}" "$@"
