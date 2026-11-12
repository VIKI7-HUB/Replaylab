#!/bin/sh
# seed.sh: sends demo traffic through the bifrost-http gateway, waits until
# Replay Lab has stored it, then starts an experiment over it. Runs as the
# "seed" service in loadable/compose.yml.
set -eu
GATEWAY=${GATEWAY:-http://gateway:8080}
API=${API:-http://gateway:8086}
N=${N:-120}

stored() {
	wget -qO- "$API/replaylab/health" | sed -n 's/.*"stored_captures":\([0-9]*\).*/\1/p'
}

before=$(stored)
echo "seed: sending $N chat requests through bifrost-http at $GATEWAY"
# Streaming is off: the demo shows the paths the gateway smoke test covers.
/app/generate_traffic -proxy "$GATEWAY" -model openai/gpt-4o -n "$N" \
	-stream-every 0 -prompts /app/sample_prompts.json

want=$((before + N))
i=0
while [ "$(stored)" -lt "$want" ]; do
	i=$((i + 1))
	if [ "$i" -gt 60 ]; then
		echo "seed: only $(stored) of $want captures stored after 30s" >&2
		exit 1
	fi
	sleep 0.5
done
echo "seed: Replay Lab has $(stored) captures"

body='{"name":"gateway demo","original_model":"gpt-4o","candidate_models":["openai/gpt-4o-mini","openai/meta-llama-3.1-8b"],"sample_size":50}'
exp=$(wget -qO- --header 'Content-Type: application/json' --post-data "$body" "$API/replaylab/experiments")
id=$(echo "$exp" | sed -n 's/^{"id":"\([^"]*\)".*/\1/p')
if [ -z "$id" ]; then
	echo "seed: experiment not started: $exp" >&2
	exit 1
fi

cat <<EOF
seed: experiment $id started; its replays go back through the gateway.

  Captures:  http://localhost:8086/replaylab/captures
  Progress:  http://localhost:8086/replaylab/experiments/$id
  Report:    http://localhost:8086/replaylab/experiments/$id/report?format=markdown
  Health:    http://localhost:8086/replaylab/health  (target_gateway_version)
  Gateway:   http://localhost:8080/v1/chat/completions  (model "openai/gpt-4o")
EOF
