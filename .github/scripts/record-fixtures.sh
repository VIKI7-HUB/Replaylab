#!/usr/bin/env bash
# Start the real backends with no API keys (Replay Lab against its mock
# provider, the Stale API in heuristic mode), drive Replay Lab through a full
# capture -> experiment -> verdict cycle, and save every response the console
# reads into $1. The console's contract test then parses them.
#
# Usage: bash .github/scripts/record-fixtures.sh <out-dir>
set -euo pipefail

out="${1:?usage: record-fixtures.sh <out-dir>}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$(mktemp -d)"
mkdir -p "$out"
pids=()
MP="${MOCK_PORT:-8081}" RP="${REPLAYLAB_PORT:-8086}" SP="${STALE_PORT:-8001}"
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; }
trap cleanup EXIT

wait_for() {
  for _ in $(seq 1 120); do curl -sf -o /dev/null "$1" && return 0; sleep 1; done
  echo "::error::$1 never came up"; return 1
}

cd "$root/backend/replaylab"
go build -o "$work/mockprovider" ./cmd/mockprovider
go build -o "$work/replaylab" ./cmd/replaylab
PORT="$MP" "$work/mockprovider" > "$work/mock.log" 2>&1 & pids+=($!)
wait_for "http://localhost:$MP/health" || { cat "$work/mock.log"; exit 1; }
"$work/replaylab" -port="$RP" -gateway="http://localhost:$MP" -db="$work/replaylab.db" > "$work/replaylab.log" 2>&1 & pids+=($!)
wait_for "http://localhost:$RP/replaylab/health" || { cat "$work/replaylab.log"; exit 1; }

cd "$root/backend/stale"
DATABASE_PATH="$work/stale.db" python -m uvicorn app.main:app --port "$SP" > "$work/stale.log" 2>&1 & pids+=($!)
wait_for "http://localhost:$SP/v1/health" || { cat "$work/stale.log"; exit 1; }

cd "$root/backend/replaylab"
go run ./demo/generate_traffic -proxy "http://localhost:$RP" -n 24 -c 4

B="http://localhost:$RP/replaylab"
exp=$(curl -sf -X POST "$B/experiments" -H 'content-type: application/json' \
  -d '{"name":"contract","original_model":"gpt-4o","candidate_models":["gpt-4o-mini","claude-3-haiku"],"sample_size":8}')
id=$(python -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$exp")
for _ in $(seq 1 120); do
  status=$(curl -sf "$B/experiments/$id" | python -c 'import json,sys; print(json.load(sys.stdin)["status"])')
  [ "$status" = completed ] && break
  case "$status" in failed|cancelled) echo "::error::experiment $status"; exit 1;; esac
  sleep 1
done
[ "$status" = completed ] || { echo "::error::experiment did not finish"; exit 1; }

capture_id=$(curl -sf "$B/captures?limit=1" | python -c 'import json,sys; print(json.load(sys.stdin)[0]["id"])')

curl -sf "$B/health" > "$out/replaylab.health.json"
curl -sf "$B/config" > "$out/replaylab.config.json"
curl -sf "$B/models" > "$out/replaylab.models.json"
curl -sf "$B/captures?limit=3" > "$out/replaylab.captures.json"
curl -sf "$B/captures/$capture_id" > "$out/replaylab.capture.json"
curl -sf "$B/experiments?limit=3" > "$out/replaylab.experiments.json"
curl -sf "$B/experiments/$id" > "$out/replaylab.experiment.json"
curl -sf -X POST "$B/experiments" -H 'content-type: application/json' \
  -d '{"name":"contract dry run","original_model":"gpt-4o","candidate_models":["gpt-4o-mini"],"sample_size":5,"dry_run":true}' \
  > "$out/replaylab.estimate.json"
curl -sf "http://localhost:$SP/v1/health" > "$out/stale.health.json"

echo "Recorded:"; ls -l "$out"
