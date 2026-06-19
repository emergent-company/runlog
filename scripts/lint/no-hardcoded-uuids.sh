#!/usr/bin/env bash
# scripts/lint/no-hardcoded-uuids.sh — detect hardcoded UUID literals in Go files.
#
# UUIDs in test files are fine; UUIDs in non-test Go code must be generated
# dynamically via uuid.New() or similar.
#
# Usage:
#   scripts/lint/no-hardcoded-uuids.sh           — scan all Go files
#   scripts/lint/no-hardcoded-uuids.sh --staged  — scan only git-staged files
#
# Escape hatch: add a  //nolint:hardcoded-uuid  comment on the same line.
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
    *) echo "no-hardcoded-uuids: unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# UUID pattern: 8-4-4-4-12 hex digits
UUID_PATTERN='[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}'

if $STAGED_ONLY; then
  SOURCES="$(git -C "$REPO" diff --cached --name-only --diff-filter=ACMR \
    | grep -E '\.go$' | grep -vE '.*_test\.go$' \
    || true)"
  [[ -z "$SOURCES" ]] && exit 0
  SOURCES="$(echo "$SOURCES" | sed "s|^|$REPO/|")"
else
  SOURCES="$(find "$REPO" -name '*.go' ! -name '*_test.go' \
    -not -path '*/vendor/*' 2>/dev/null | sort)"
fi

[[ -z "$SOURCES" ]] && exit 0

FOUND=""
while IFS= read -r file; do
  while IFS= read -r line; do
    lineno="${line%%:*}"
    content="${line#*:}"
    if echo "$content" | grep -qE '//nolint:hardcoded-uuid'; then
      continue
    fi
    rel="${file#$REPO/}"
    FOUND="${FOUND}  ${rel}:${lineno} — hardcoded UUID literal"$'\n'
  done < <(grep -nE "$UUID_PATTERN" "$file" 2>/dev/null || true)
done <<< "$SOURCES"

if [[ -z "$FOUND" ]]; then
  exit 0
fi

echo "no-hardcoded-uuids: UUID literals found in non-test Go files:"
echo "$FOUND"
echo "Generate UUIDs dynamically with uuid.New() instead."
echo "To suppress: add  //nolint:hardcoded-uuid  on the same line."
exit 1
