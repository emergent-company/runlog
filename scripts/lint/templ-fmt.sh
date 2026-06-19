#!/usr/bin/env bash
# templ-fmt.sh — verify that all .templ source files are formatted with templ fmt.
#
# Usage:
#   scripts/lint/templ-fmt.sh [file ...]   # check specific files
#   scripts/lint/templ-fmt.sh              # check all *.templ files
#
# Exit codes:
#   0 — all files are correctly formatted
#   1 — one or more files have formatting drift
#
# To fix: run  templ fmt .  (or  task templ:fmt)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPL_BIN="${TEMPL_BIN:-templ}"

# ── File list ─────────────────────────────────────────────────────────────────
if [ $# -gt 0 ]; then
  FILES=("$@")
else
  mapfile -t FILES < <(find "$REPO_ROOT" \
    -name "*.templ" \
    -not -name "*_templ.go" \
    -not -path "*/vendor/*" \
    -not -path "*/node_modules/*")
fi

if [ ${#FILES[@]} -eq 0 ]; then
  exit 0
fi

# ── Check ─────────────────────────────────────────────────────────────────────
UNFORMATTED=()

for file in "${FILES[@]}"; do
  [[ "$file" == *.templ ]] || continue
  [[ "$file" == *_templ.go ]] && continue
  [ -f "$file" ] || continue

  if ! "$TEMPL_BIN" fmt -stdout "$file" 2>/dev/null | diff -q - "$file" >/dev/null 2>&1; then
    rel="${file#"$REPO_ROOT/"}"
    UNFORMATTED+=("$rel")
  fi
done

if [ ${#UNFORMATTED[@]} -gt 0 ]; then
  echo ""
  echo "  [templ-fmt] the following .templ files need formatting:"
  for f in "${UNFORMATTED[@]}"; do
    echo "    $f"
  done
  echo ""
  echo "  Run:  templ fmt .    (or:  task templ:fmt)"
  echo ""
  exit 1
fi

exit 0
