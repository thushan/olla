#!/bin/bash

# Olla Stream Load Test Script
# Simulates partial-stream readers to stress proxy with early disconnects
####
# Usage: ./test-load-stream.sh [duration] [concurrency]
# Example: ./test-load-stream.sh 60 10 # Defaults: 60 seconds duration, 10 concurrent workers

set -euo pipefail

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

function jitter_sleep() {
  local min_delay=${1:-0.1}
  local max_delay=${2:-0.5}
  local jitter=$(awk -v min=$min_delay -v max=$max_delay 'BEGIN{srand(); print min + rand() * (max - min)}')
  sleep "$jitter"
}

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

    jitter_sleep 0.2 1.0
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
echo "Total workers:            $CONCURRENCY"
echo "Duration:                 $DURATION seconds"
echo "Model:                    $MODEL_NAME"
echo "Target:                   $TARGET"
echo "Read bytes per stream:    $READ_BYTES"
echo "Test finished at '$(date)'"
echo "All workers completed."