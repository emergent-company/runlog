#!/usr/bin/env bash
# no-raw-components.sh — forbid raw HTML elements in .templ files when a
# go-daisy component covers the use case.
#
# Usage:
#   scripts/lint/no-raw-components.sh [file ...]   # check specific files
#   scripts/lint/no-raw-components.sh              # check all *.templ files
#
# Exit codes:
#   0 — all clean
#   1 — one or more violations found
#
# To suppress a legitimate exception, add a trailing comment on the same line:
#   <dialog ...>  <!-- lint:allow-raw -->

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# ── Forbidden patterns ────────────────────────────────────────────────────────
declare -a RULES=(
  '<dialog[ >	]|Use modal.Modal / modal.FormModal from components/modal'
  '<table[ >	]|Use table.TableWithProps from components/table'
  '<thead[ >	]|Use table.TableHead from components/table'
  '<tbody[ >	]|Use table.TableBody from components/table'
  '<tr[ >	]|Use table.TableRow from components/table'
  '<th[ >	]|Use table.TableHeader from components/table'
  '<td[ >	]|Use table.TableCell / table.TableActionCell from components/table'
)

# ── File list ─────────────────────────────────────────────────────────────────
if [ $# -gt 0 ]; then
  FILES=("$@")
else
  mapfile -t FILES < <(find "$REPO_ROOT" -name "*.templ" -not -name "*_templ.go" -not -path "*/vendor/*")
fi

if [ ${#FILES[@]} -eq 0 ]; then
  exit 0
fi

# ── Check ─────────────────────────────────────────────────────────────────────
FAIL=0

for rule in "${RULES[@]}"; do
  PATTERN="${rule%%|*}"
  rest="${rule#*|}"
  REASON="${rest%%|*}"

  for file in "${FILES[@]}"; do
    [[ "$file" == *.templ ]] || continue
    [[ "$file" == *_templ.go ]] && continue
    [ -f "$file" ] || continue

    VIOLATIONS=()
    while IFS= read -r match_line; do
      [ -z "$match_line" ] && continue
      lineno="${match_line%%:*}"
      window_start=$(( lineno > 2 ? lineno - 2 : 1 ))
      window_end=$(( lineno + 2 ))
      window=$(sed -n "${window_start},${window_end}p" "$file" 2>/dev/null || true)
      if echo "$window" | grep -q 'lint:allow-raw'; then
        continue
      fi
      VIOLATIONS+=("$match_line")
    done < <(grep -nE "$PATTERN" "$file" || true)

    if [ ${#VIOLATIONS[@]} -gt 0 ]; then
      rel="${file#"$REPO_ROOT/"}"
      echo ""
      echo "  [no-raw-components] $rel"
      echo "  Reason : $REASON"
      for v in "${VIOLATIONS[@]}"; do echo "    $v"; done
      FAIL=1
    fi
  done
done

if [ "$FAIL" -eq 1 ]; then
  echo ""
  echo "  Use go-daisy components instead of raw HTML elements."
  echo "  To allow a specific line: append  <!-- lint:allow-raw -->  on the same line."
  echo ""
  exit 1
fi

exit 0
