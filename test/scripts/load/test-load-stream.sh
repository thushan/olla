#!/bin/bash

# Olla Stream Load Test Script
# Simulates partial-stream readers to stress proxy with early disconnects


if [ ! -f .env ]; then
  echo "Missing .env file"
  exit 1
fi
set -a; source .env; set +a

DURATION=${1:-60}
CONCURRENCY=${2:-10}
READ_BYTES=${READ_BYTES:-4096}  # How much of the stream to read before aborting gracefully(?)
MODEL_NAME=${MODEL_NAME:-phi4:latest}
PROXY_PATH="${PROXY_ENDPOINT:-/olla/}v1/chat/completions"
TARGET=${TARGET_URL}${PROXY_PATH}

start_time=$(date +%s)
end_time=$((start_time + DURATION))

function worker() {
  local wid=$1
  while [ $(date +%s) -lt $end_time ]; do
    question="$(date +%s) - Worker $wid asks: What is the capital of France?"
    payload=$(jq -nc --arg q "$question" --arg m "$MODEL_NAME" '
      {model: $m, messages: [{role: "user", content: $q}], stream: true, temperature: 0.2}
    ')

    curl -s -X POST "$TARGET" \
      -H "Content-Type: application/json" \
      -d "$payload" \
      --no-buffer | head -c "$READ_BYTES" > /dev/null &
  done
}

trap 'echo Exiting...; kill 0' SIGINT SIGTERM

echo "Starting stream test: duration=${DURATION}s, concurrency=${CONCURRENCY}, read_bytes=${READ_BYTES}" && echo

for i in $(seq 1 $CONCURRENCY); do
  worker "$i" &
  sleep 0.05
done

wait

echo "\nStream test complete."
