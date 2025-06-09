#!/bin/bash

# Olla Chaos Load Test Script (The Benny KaosTheory Edition)
# Simulates partial-stream readers to stress proxy with early disconnects.
# Useful for stress testing connection handling, stream cancellation and 
# robustness under mid-stream disconnects.
#
# Supports a --chaos flag that introduces randomised read sizes, malformed 
# JSON payloads, and jitter between requests.
# 
# Run this alone or along with test-load-limits.sh to simulate real-world
# scenarios where clients may disconnect mid-stream or send malformed data.
####
# Requirements: jq, curl
####
# Usage: ./test-load-stream.sh [duration] [concurrency] [--chaos]
# - duration: Duration of the test in seconds (default: 60)
# - concurrency: Number of concurrent workers (default: 10)
# - --chaos: Enable chaos mode with random read bytes and malformed data
#
# Example: ./test-load-stream.sh 60 10 # Defaults: 60 seconds duration, 10 concurrent workers

set -euo pipefail

if ! command -v jq &> /dev/null; then
  echo "jq is required but not installed. Please install jq."
  exit 1
fi
if ! command -v curl &> /dev/null; then
  echo "curl is required but not installed. Please install curl."
  exit 1
fi

if [ ! -f .env ]; then
  echo "Missing .env file"
  exit 1
fi
set -a; source .env; set +a
DURATION=${1:-60}
CONCURRENCY=${2:-10}
CHAOS_MODE=false

# Check for --chaos flag
if [[ "$*" == *"--chaos"* ]]; then
  CHAOS_MODE=true
fi

READ_BYTES=${READ_BYTES:-4096}  # Default read limit
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

    # Chaos: randomise read bytes, malformed JSON, or large payload to trigger size limits
    local chaos_bytes=$READ_BYTES
    if [ "$CHAOS_MODE" = true ]; then
      rand=$((RANDOM % 10))
      if [ $rand -le 6 ]; then
        chaos_bytes=$((RANDOM % 8192 + 1024))  # Random read window
      elif [ $rand -le 8 ]; then
        # monkey around with invalid Jason Derulo
        payload='{"model": "'$MODEL_NAME'", "messages": [INVALID_JSON]'
      else
        # monkey around with the payload size to trigger size limits
        payload=$(jq -nc --arg q "$question" --arg m "$MODEL_NAME" '
          {model: $m, messages: [{role: "user", content: $q}], stream: true, temperature: 0.2, max_tokens: 999999}
        ')
      fi
    fi

    curl -s -X POST "$TARGET" \
      -H "Content-Type: application/json" \
      -H "User-Agent: OllaChaosMonkey/1.0" \
      -d "$payload" \
      --no-buffer | head -c "$chaos_bytes" > /dev/null &

    jitter_sleep 0.2 1.0
  done
}

trap 'echo Exiting...; kill 0' SIGINT SIGTERM

echo "Starting stream test: duration=${DURATION}s, concurrency=${CONCURRENCY}, read_bytes=${READ_BYTES}, chaos_mode=${CHAOS_MODE}" && echo

for i in $(seq 1 $CONCURRENCY); do
  worker "$i" &
  sleep 0.05
done

wait

echo "\nStream test complete."
echo "Target:                   $TARGET"
echo "Model:                    $MODEL_NAME"
echo "Total workers:            $CONCURRENCY"
echo "Duration:                 $DURATION seconds"
echo "Read bytes per stream:    $READ_BYTES"
echo "Test finished             '$(date)'"