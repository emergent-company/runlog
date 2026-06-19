#!/usr/bin/env bash
# templ-generate.sh — ensure generated _templ.go files are up to date.
#
# Pre-commit behaviour (when LEFTHOOK=1 or called with staged .templ files):
#   Runs templ generate, then git-adds any updated _templ.go files so they
#   are included in the commit automatically.  This is the "auto-fix" mode.
#
# Lint group behaviour (no staged files / called standalone):
#   Runs templ generate and reports any files that changed, but does NOT
#   auto-stage them.  Exits 1 if any _templ.go was stale.
#
# Usage:
#   scripts/lint/templ-generate.sh              # lint mode (check all)
#   scripts/lint/templ-generate.sh [file ...]   # pre-commit mode (staged files)
#
# Exit codes:
#   0 — generated files were already up to date (or have been updated + staged)
#   1 — stale generated files found (lint mode only)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPL_BIN="${TEMPL_BIN:-templ}"
VIEWS_DIR="$REPO_ROOT/cmd/runlog"

# Detect whether any staged .templ files were passed (pre-commit mode).
PRECOMMIT=0
for f in "$@"; do
  [[ "$f" == *.templ ]] && PRECOMMIT=1 && break
done
[ "${LEFTHOOK:-0}" = "1" ] && [ $# -gt 0 ] && PRECOMMIT=1

# ── Snapshot checksums of _templ.go files before generate ────────────────────
declare -A BEFORE
while IFS= read -r -d '' f; do
  BEFORE["$f"]=$(md5sum "$f" 2>/dev/null | awk '{print $1}')
done < <(find "$VIEWS_DIR" -name "*_templ.go" -print0 2>/dev/null)

# ── Run templ generate ────────────────────────────────────────────────────────
if ! "$TEMPL_BIN" generate "$VIEWS_DIR" >/dev/null 2>&1; then
  echo ""
  echo "  [templ-generate] templ generate failed — fix the errors above."
  echo ""
  exit 1
fi

# ── Compare checksums after generate ─────────────────────────────────────────
CHANGED=()
while IFS= read -r -d '' f; do
  AFTER=$(md5sum "$f" 2>/dev/null | awk '{print $1}')
  if [ "${BEFORE[$f]:-}" != "$AFTER" ]; then
    CHANGED+=("$f")
  fi
done < <(find "$VIEWS_DIR" -name "*_templ.go" -print0 2>/dev/null)

if [ ${#CHANGED[@]} -eq 0 ]; then
  exit 0
fi

if [ "$PRECOMMIT" -eq 1 ]; then
  # Auto-fix mode: stage the updated generated files and continue.
  echo ""
  echo "  [templ-generate] auto-staging updated generated files:"
  for f in "${CHANGED[@]}"; do
    rel="${f#"$REPO_ROOT/"}"
    echo "    git add $rel"
    git -C "$REPO_ROOT" add "$f"
  done
  echo ""
  exit 0
else
  # Lint mode: report stale files and fail.
  echo ""
  echo "  [templ-generate] the following generated files are stale:"
  for f in "${CHANGED[@]}"; do
    rel="${f#"$REPO_ROOT/"}"
    echo "    $rel"
  done
  echo ""
  echo "  Run:  templ generate ./cmd/runlog/    (or:  task templ)"
  echo ""
  exit 1
fi
