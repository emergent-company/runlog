#!/usr/bin/env bash
# scripts/lint/require-testid.sh — enforce data-testid on interactive elements
# in .templ files so that e2e browser tests can reliably select them.
#
# Rules enforced:
#   1. <button ...> must have data-testid="..."
#   2. <form ...> must have data-testid="..."
#   3. Any element with hx-get/hx-post/hx-put/hx-delete must have data-testid="..."
#
# Usage:
#   scripts/lint/require-testid.sh [file ...]   # check specific files
#   scripts/lint/require-testid.sh              # check all .templ files
#
# Exit codes:
#   0 — all interactive elements have data-testid
#   1 — one or more elements are missing data-testid
#
# Escape hatch:
#   Append  <!-- lint:allow-no-testid: reason -->  within 5 lines of the element.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

TEMPL_FILES=()
if [ $# -gt 0 ]; then
  for f in "$@"; do
    [[ "$f" == *.templ ]] && TEMPL_FILES+=("$f")
  done
else
  mapfile -t TEMPL_FILES < <(find "$REPO_ROOT" \
    -name "*.templ" \
    -not -name "*_templ.go" \
    -not -path "*/vendor/*" \
    -not -path "*/node_modules/*")
fi

FAIL=0

check_pattern() {
  local file="$1"
  local pattern="$2"
  local label="$3"
  local rel="${file#"$REPO_ROOT/"}"

  while IFS= read -r match_line; do
    [ -z "$match_line" ] && continue
    local lineno="${match_line%%:*}"

    local window_start=$(( lineno > 5 ? lineno - 5 : 1 ))
    local window_end=$(( lineno + 5 ))
    local window
    window=$(sed -n "${window_start},${window_end}p" "$file" 2>/dev/null || true)

    if echo "$window" | grep -qE 'data-testid=|lint:allow-no-testid'; then
      continue
    fi

    echo "  [require-testid] $rel:$lineno — $label missing data-testid"
    FAIL=1
  done < <(grep -nE "$pattern" "$file" || true)
}

for file in "${TEMPL_FILES[@]}"; do
  [ -f "$file" ] || continue
  [[ "$file" == *_templ.go ]] && continue

  check_pattern "$file" '<button[[:space:]>]' '<button>'
  check_pattern "$file" '<form[[:space:]>]'   '<form>'
  check_pattern "$file" 'hx-(get|post|put|delete)=' 'hx-* element'
done

if [ "$FAIL" -eq 1 ]; then
  echo ""
  echo "  Add data-testid=\"...\" to each flagged element so tests can target it."
  echo "  If the element is inside a dynamic list already identified by a parent"
  echo "  testid, use <!-- lint:allow-no-testid: reason --> within 5 lines."
  echo ""
  exit 1
fi

exit 0
