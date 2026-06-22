#!/usr/bin/env bash
# scripts/lint/check-htmx-events.sh — detect HTMX v3 event names in v4 codebase.
#
# HTMX 4 uses namespaced event names like `htmx:before:request` instead of
# v3's camelCase `htmx:beforeRequest`.  This linter catches the old pattern.
#
# Usage:
#   scripts/lint/check-htmx-events.sh              — scan all files
#   scripts/lint/check-htmx-events.sh --staged     — scan only git-staged files
#
# Exit codes:
#   0  no violations
#   1  one or more violations found

set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
STAGED_ONLY=false

for arg in "$@"; do
  case "$arg" in
    --staged) STAGED_ONLY=true ;;
    *) echo "check-htmx-events: unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# ── Known v3→v4 mappings for suggestions ────────────────────────────────────
declare -A V3_TO_V4=(
  [beforeRequest]="htmx:before:request"
  [afterRequest]="htmx:after:request"
  [responseError]="htmx:error"
  [sendError]="htmx:error"
  [beforeSwap]="htmx:before:swap"
  [afterSwap]="htmx:after:swap"
  [beforeSettle]="htmx:before:settle"
  [afterSettle]="htmx:after:settle"
  [beforeProcess]="htmx:before:process"
  [afterProcess]="htmx:after:process"
  [historyRestore]="htmx:before:history:restore"
  [historyPush]="htmx:after:history:push"
  [historyReplace]="htmx:after:history:replace"
  [beforeCleanup]="htmx:before:cleanup"
  [afterCleanup]="htmx:after:cleanup"
  [beforeOnLoad]="htmx:before:onLoad"
  [afterOnLoad]="htmx:after:onLoad"
  [load]="htmx:after:process"
  [configRequest]="htmx:config:request"
  [beforeHistorySave]="htmx:before:history:save"
  [afterHistorySave]="htmx:after:history:save"
  [beforeHistoryRestore]="htmx:before:history:restore"
  [afterHistoryRestore]="htmx:after:history:restore"
  [beforeRequest]="htmx:before:request"
  [afterRequest]="htmx:after:request"
  [beforeSwap]="htmx:before:swap"
  [afterSwap]="htmx:after:swap"
  [beforeSettle]="htmx:before:settle"
  [afterSettle]="htmx:after:settle"
)

if $STAGED_ONLY; then
  SOURCES="$(git -C "$REPO" diff --cached --name-only --diff-filter=ACMR \
    | grep -E '\.(templ|go|js)$' \
    || true)"
  if [[ -z "$SOURCES" ]]; then
    exit 0
  fi
  SOURCES="$(echo "$SOURCES" | sed "s|^|$REPO/|")"
else
  SOURCES="$(find "$REPO" -name '*.templ' -o -name '*.go' -o -name '*.js' \
    -not -path '*/vendor/*' \
    -not -path '*/node_modules/*' \
    -not -name '*_templ.go' \
    2>/dev/null | sort)"
fi

if [[ -z "$SOURCES" ]]; then
  exit 0
fi

FOUND=""
while IFS= read -r file; do
  while IFS= read -r match; do
    lineno=$(echo "$match" | cut -d: -f1)
    event=$(echo "$match" | grep -oP "htmx:\K[a-zA-Z]+" || true)
    [ -z "$event" ] && continue

    # Check if the matched word is camelCase (v3 style) - has lowercase then uppercase
    if [[ "$event" =~ ^[a-z]+[A-Z] ]]; then
      rel="${file#$REPO/}"
      suggestion="${V3_TO_V4[$event]:-}"
      if [[ -n "$suggestion" ]]; then
        FOUND="${FOUND}  ${rel}:${lineno} — HTMX v3 event 'htmx:${event}' — use '${suggestion}'"$'\n'
      else
        FOUND="${FOUND}  ${rel}:${lineno} — HTMX v3 event 'htmx:${event}' — unknown v4 equivalent"$'\n'
      fi
    fi
  done < <(grep -nP "addEventListener\(['\"]htmx:[a-zA-Z]+" "$file" 2>/dev/null || true)
done <<< "$SOURCES"

if [[ -z "$FOUND" ]]; then
  exit 0
fi

echo "check-htmx-events: found HTMX v3 event names (use v4 namespaced format):"
echo "$FOUND"
exit 1
