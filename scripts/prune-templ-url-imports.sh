#!/usr/bin/env bash
# prune-templ-url-imports — remove stale net/url imports from templ-generated files
# that no longer reference url.* methods (all replaced by urls.go functions).
# Run after `templ generate` to keep air builds clean.
set -euo pipefail

RUNLOG_DIR="cmd/runlog"

FILES=(
  environments_templ.go
  launch_templ.go
  linters_templ.go
  run_detail_templ.go
  test_detail_templ.go
)

for f in "${FILES[@]}"; do
  pf="$RUNLOG_DIR/$f"
  if [[ -f "$pf" ]]; then
    if grep -q 'import "net/url"' "$pf" 2>/dev/null || grep -q $'\t"net/url"' "$pf" 2>/dev/null; then
      sed -i '/^import "net\/url"$/d; /^\t"net\/url"$/d' "$pf"
      gofmt -w "$pf" 2>/dev/null || true
    fi
  fi
done
