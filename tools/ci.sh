#!/usr/bin/env bash
# 本機 CI。**push 之前這一支要全綠。**
#
#   tools/ci.sh
#
# 需要外部東西的檢查會自己跳過，但**會說它跳過了**——
# 安靜的跳過會讓「還沒驗」看起來像「驗過了」。三個開關：
#
#   PARHELION_CODEFILE   一份真的 codefile（例如 SYSTEM.PASCAL），驗 M0
#   PARHELION_ORIG       psys21 磁碟目錄（含 PSYSTEM.COM）
#   PARHELION_PME        抽出來的 SYSTEM.PME.86，路徑是**容器裡**的
#   PARHELION_DOSGOLEM   本機 dosgolem 副本，oracle 那一層要它
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

fail=0
step() { printf '\n\033[1m=== %s\033[0m\n' "$1"; }
bad() { echo "  ✗ $1"; fail=1; }

step "gofmt"
out=$(PARHELION_GO_CMD=gofmt tools/go.sh -l . 2>&1)
if [[ -n "$out" ]]; then
  bad "沒有格式化："
  echo "$out" | sed 's/^/      /'
else
  echo "  ✓"
fi

step "go vet"
tools/go.sh vet ./... && echo "  ✓" || bad "go vet 有問題"

step "go test（不需要原版素材）"
tools/go.sh test ./... || bad "測試沒過"

step "M0：對一份真的 codefile"
if [[ -n "${PARHELION_CODEFILE:-}" ]]; then
  tools/go.sh test ./internal/codefile -run TestRealCodefile -v || bad "codefile 驗收沒過"
else
  echo "  跳過：沒設 PARHELION_CODEFILE"
fi

step "oracle：把原版跑起來"
if [[ -n "${PARHELION_DOSGOLEM:-}" && -n "${PARHELION_ORIG:-}" && -n "${PARHELION_PME:-}" ]]; then
  tools/go.sh test -tags oracle ./oracle/ -v || bad "oracle 沒過"
else
  echo "  跳過：要同時設 PARHELION_DOSGOLEM、PARHELION_ORIG、PARHELION_PME"
  echo "  ⚠ 跳過不等於通過。spec 01 能不能升 CONFORMED 就看這一段。"
fi

printf '\n'
if [[ $fail -eq 0 ]]; then echo "全部通過。"; else echo "有項目沒過，見上面的 ✗。"; fi
exit $fail
