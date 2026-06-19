#!/usr/bin/env bash
# no-dead-code.sh — detect unreachable functions in Go programs using deadcode.
#
# Uses golang.org/x/tools/cmd/deadcode (Rapid Type Analysis) to build a call
# graph from all known entry points and reports any function that is unreachable.
#
# Usage:
#   scripts/lint/no-dead-code.sh   # always analyzes the full module
#
# Exit codes:
#   0 — no unreachable functions found
#   1 — one or more unreachable functions found
#
# Escape hatch:
#   Add a  //nolint:deadcode  comment on the same line as the func declaration
#   to suppress a specific finding.
#
# Install deadcode (once per machine):
#   go install golang.org/x/tools/cmd/deadcode@latest

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEADCODE_BIN="${DEADCODE_BIN:-deadcode}"

if ! command -v "$DEADCODE_BIN" >/dev/null 2>&1; then
  echo ""
  echo "  [no-dead-code] deadcode not found."
  echo "  Install: go install golang.org/x/tools/cmd/deadcode@latest"
  echo ""
  exit 1
fi

# ── Entry points ──────────────────────────────────────────────────────────────
ENTRYPOINTS=(
  "./cmd/runlog"
)

# ── Run analysis ──────────────────────────────────────────────────────────────
MODULE="github.com/emergent-company/runlog"

RAW=$("$DEADCODE_BIN" \
  -test \
  -filter "$MODULE" \
  "${ENTRYPOINTS[@]}" 2>&1) || true

if [ -z "$RAW" ]; then
  exit 0
fi

# ── Filter out //nolint:deadcode suppressions ─────────────────────────────────
FAIL=0
VIOLATIONS=()

while IFS= read -r report_line; do
  [ -z "$report_line" ] && continue

  file_part="${report_line%%:*}"
  rest="${report_line#*:}"
  lineno="${rest%%:*}"

  abs_file="$REPO_ROOT/$file_part"
  [ -f "$abs_file" ] || abs_file="$file_part"

  if [ -f "$abs_file" ] && [ -n "$lineno" ]; then
    src_line=$(sed -n "${lineno}p" "$abs_file" 2>/dev/null || true)
    if echo "$src_line" | grep -q '//nolint:deadcode'; then
      continue
    fi
  fi

  VIOLATIONS+=("$report_line")
  FAIL=1
done <<< "$RAW"

if [ "$FAIL" -eq 1 ]; then
  echo ""
  echo "  [no-dead-code] unreachable functions found:"
  for v in "${VIOLATIONS[@]}"; do
    echo "    $v"
  done
  echo ""
  echo "  Either delete the dead code or add  //nolint:deadcode — <reason>  on the func line."
  echo ""
  exit 1
fi

exit 0
