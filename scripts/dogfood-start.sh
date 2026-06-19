#!/usr/bin/env bash
set -euo pipefail

DOGFOOD_PORT="${1:-17433}"
DOGFOOD_DB="${2:-/tmp/runlog-dogfood.db}"
DOGFOOD_BIN="${3:-/tmp/runlog-dogfood-bin}"
DOGFOOD_LOG="${4:-/tmp/runlog-dogfood.log}"

fuser "$DOGFOOD_PORT"/tcp 2>/dev/null && fuser -k "$DOGFOOD_PORT"/tcp 2>/dev/null || true

go build -mod=mod -o "$DOGFOOD_BIN" ./cmd/runlog
# Don't rm the DB here — caller seeds it before calling this script
export RUNLOG_APP_TITLE="runlog (dogfood)"

# Run with --daemon so it re-execs into a new session (Setsid).
# Since the daemon forks itself into a new session, we can safely
# background this invocation and let it become its own leader.
"$DOGFOOD_BIN" --daemon --db "$DOGFOOD_DB" --port="$DOGFOOD_PORT" > "$DOGFOOD_LOG" 2>&1 &

# Wait for /health
for i in $(seq 1 10); do
  if curl -sf "http://localhost:$DOGFOOD_PORT/health" > /dev/null 2>&1; then
    echo "Dogfood daemon started on port $DOGFOOD_PORT"
    exit 0
  fi
  sleep 0.5
done

echo "Dogfood daemon failed to start"
cat "$DOGFOOD_LOG"
exit 1
