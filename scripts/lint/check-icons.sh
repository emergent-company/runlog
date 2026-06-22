#!/usr/bin/env bash
# scripts/lint/check-icons.sh — validate Lucide icon references in .templ and .go files.
#
# Checks that every `lucide--<name>` reference matches a known Lucide icon name.
# The known list is pulled from the lucide-static NPM package (or a bundled list).
#
# Usage:
#   scripts/lint/check-icons.sh              — scan all files
#   scripts/lint/check-icons.sh --staged     — scan only git-staged files
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
    *) echo "check-icons: unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# ── Known Lucide icon names (subset of most common) ──────────────────────────
# Full list: https://lucide.dev/icons
KNOWN_ICONS=(
  layout-dashboard flask-conical play square-play-circle
  check x circle alert-triangle alert-circle circle-check
  info settings user users search filter
  arrow-left arrow-right arrow-up arrow-down chevron-left chevron-right chevron-up chevron-down
  plus minus more-horizontal more-vertical ellipsis-vertical edit trash trash-2
  external-link link copy clipboard clock
  refresh loader spinner download upload
  home menu sidebar book file file-text text list
  code terminal github twitter shield layers
  sun moon monitor-dot panel-left-close folder
  paperclip image-plus mic cpu zap
  pencil eye building-2 bot
  thumbs-up thumbs-down send-horizontal briefcase lock
)

if $STAGED_ONLY; then
  SOURCES="$(git -C "$REPO" diff --cached --name-only --diff-filter=ACMR \
    | grep -E '\.(templ|go)$' \
    || true)"
  if [[ -z "$SOURCES" ]]; then
    exit 0
  fi
  SOURCES="$(echo "$SOURCES" | sed "s|^|$REPO/|")"
else
  SOURCES="$(find "$REPO" -name '*.templ' -o -name '*.go' \
    -not -path '*/vendor/*' \
    -not -name '*_test.go' \
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
    while IFS= read -r icon; do
      [ -z "$icon" ] && continue
      known=false
      for k in "${KNOWN_ICONS[@]}"; do
        [[ "$icon" == "$k" ]] && known=true && break
      done
      if ! $known; then
        rel="${file#$REPO/}"
        FOUND="${FOUND}  ${rel}:${lineno} — unknown icon: ${icon}"$'\n'
      fi
    done < <(echo "$match" | grep -oP 'lucide--\K[a-zA-Z0-9_-]+' || true)
  done < <(grep -n 'lucide--' "$file" 2>/dev/null || true)
done <<< "$SOURCES"

if [[ -z "$FOUND" ]]; then
  exit 0
fi

echo "check-icons: unknown Lucide icon references:"
echo "$FOUND"
echo "Check https://lucide.dev/icons for valid names."
exit 1
