#!/usr/bin/env bash
# prune-templ-url-imports — remove stale net/url imports from templ-generated files
# that no longer reference url.* methods (all replaced by urls.go functions).
# Run after `templ generate` to keep air builds clean.
set -euo pipefail

FILES=(
  cmd/runlog/catalog_templ.go
  cmd/runlog/environments_templ.go
  cmd/runlog/launch_templ.go
  cmd/runlog/linters_templ.go
  cmd/runlog/run_detail_templ.go
  cmd/runlog/test_detail_templ.go
)

for f in "${FILES[@]}"; do
  [ -f "$f" ] || continue
  sed -i '/^import "net\/url"$/d; /^\t"net\/url"$/d' "$f"
done
gofmt -w "${FILES[@]}" 2>/dev/null
