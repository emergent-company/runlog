#!/usr/bin/env bash
# scripts/lint/no-panic-in-lib.sh — block panic() calls in library packages.
#
# panic() is only acceptable in main (unrecoverable startup) and in test files.
# Library code must return errors.
#
# Usage:
#   scripts/lint/no-panic-in-lib.sh              — scan all library Go files
#   scripts/lint/no-panic-in-lib.sh --staged     — scan only git-staged files
#
# Escape hatch: append  //nolint:panic — <reason>  on the same line as the
# panic() call.
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
    *) echo "no-panic-in-lib: unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# Packages considered "library" — everything except cmd/ and test files.
LIB_DIRS=( . )

if $STAGED_ONLY; then
  SOURCES="$(git -C "$REPO" diff --cached --name-only --diff-filter=ACMR \
    | grep -E '\.go$' \
    | grep -v '^cmd/' \
    | grep -vE '.*_test\.go$' \
    || true)"
  if [[ -z "$SOURCES" ]]; then
    exit 0
  fi
  SOURCES="$(echo "$SOURCES" | sed "s|^|$REPO/|")"
else
  SOURCES="$(find "$REPO" -name '*.go' ! -name '*_test.go' \
    -not -path '*/cmd/*' \
    -not -path '*/vendor/*' \
    2>/dev/null | sort)"
fi

if [[ -z "$SOURCES" ]]; then
  exit 0
fi

FOUND=""
while IFS= read -r file; do
  while IFS= read -r line; do
    lineno="${line%%:*}"
    content="${line#*:}"
    if echo "$content" | grep -qE '//nolint:panic'; then
      continue
    fi
    rel="${file#$REPO/}"
    FOUND="${FOUND}  ${rel}:${lineno} — panic() in library code"$'\n'
  done < <(grep -nE '\bpanic\(' "$file" 2>/dev/null || true)
done <<< "$SOURCES"

if [[ -z "$FOUND" ]]; then
  exit 0
fi

echo "no-panic-in-lib: panic() calls found in library packages:"
echo "$FOUND"
echo "Library code must return errors instead of panicking."
echo "If this is truly unrecoverable (e.g. invariant violation), add:"
echo "  //nolint:panic — <reason>"
exit 1
