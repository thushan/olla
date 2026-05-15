#!/usr/bin/env bash
# auth-headers-only.sh — proves Olla injects arbitrary static headers on
# outbound requests when only the headers: map is configured (no auth: block).
#
# The mock backend enforces a custom header, confirming that custom headers
# travel to the backend even when no structured auth block is present.
#
# Requires: go, curl, bash 4+
# Does NOT require Docker / AIMock.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

OLLA_PORT="${OLLA_PORT:-40118}"
BACKEND_PORT="${BACKEND_PORT:-19913}"
OLLA_URL="http://127.0.0.1:${OLLA_PORT}"
BACKEND_URL="http://127.0.0.1:${BACKEND_PORT}"
OLLA_LOG="${TMPDIR:-/tmp}/olla-auth-headers.log"

CUSTOM_HEADER="X-Tenant-ID"
CUSTOM_VALUE="tenant-abc"

OLLA_PID=""
BACKEND_PID=""

cleanup() {
    kill_proc "$OLLA_PID"
    kill_proc "$BACKEND_PID"
}
trap cleanup EXIT INT TERM

echo "=== auth-headers-only: static headers injection (no auth block) ==="
echo "Backend: ${BACKEND_URL}  Olla: ${OLLA_URL}"
echo

free_port "$BACKEND_PORT"
go run "$REPO_ROOT/test/cmd/mockbackend" \
    --addr "127.0.0.1:${BACKEND_PORT}" \
    --require-header "${CUSTOM_HEADER}" \
    --require-value "${CUSTOM_VALUE}" \
    >"${TMPDIR:-/tmp}/mockbackend-headers.log" 2>&1 &
BACKEND_PID=$!
wait_for_mockbackend "$BACKEND_URL" 15

CONFIG=$(mktemp "${TMPDIR:-/tmp}/olla-auth-headers-XXXXXX.yaml")
cat >"$CONFIG" <<YAML
server:
  host: "127.0.0.1"
  port: ${OLLA_PORT}
  read_timeout: 10s
  write_timeout: 0s
  shutdown_timeout: 2s
  request_logging: false
  request_limits:
    max_body_size: 10485760
    max_header_size: 524288
  rate_limits:
    global_requests_per_minute: 0
    per_ip_requests_per_minute: 0
    health_requests_per_minute: 0
    burst_size: 50
proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "priority"
  connection_timeout: 5s
  response_timeout: 10s
  read_timeout: 10s
  retry:
    enabled: false
discovery:
  type: "static"
  refresh_interval: 10s
  health_check:
    initial_delay: 1s
  static:
    endpoints:
      - url: "${BACKEND_URL}"
        name: "mock-headers"
        type: "openai-compatible"
        priority: 100
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 5s
        check_timeout: 2s
        headers:
          ${CUSTOM_HEADER}: "${CUSTOM_VALUE}"
  model_discovery:
    enabled: false
model_registry:
  type: "memory"
  enable_unifier: false
  unification:
    enabled: false
  routing_strategy:
    type: "optimistic"
    options:
      fallback_behavior: "all"
logging:
  level: "warn"
  format: "text"
  output: "stdout"
YAML

free_port "$OLLA_PORT"
go run "$REPO_ROOT" --config "$CONFIG" >"$OLLA_LOG" 2>&1 &
OLLA_PID=$!
wait_for_url "${OLLA_URL}/internal/health" 20

# Test 1: custom header present → 200
status=$(http_status_for "${OLLA_URL}/olla/openai-compatible/v1/chat/completions")
if [[ "$status" == "200" ]]; then
    pass "custom header injected → backend accepted (200)"
else
    fail "custom header injected → backend accepted (200)" "got HTTP ${status}"
fi

# ── restart without the custom header ────────────────────────────────────────
kill_proc "$OLLA_PID"
OLLA_PID=""
free_port "$OLLA_PORT"

# Remove the headers: block entirely by generating a config without it
CONFIG_NO_HDR=$(mktemp "${TMPDIR:-/tmp}/olla-auth-headers-none-XXXXXX.yaml")
# Strip the headers block (both lines)
grep -v "headers:" "$CONFIG" | grep -v "${CUSTOM_HEADER}" >"$CONFIG_NO_HDR"

go run "$REPO_ROOT" --config "$CONFIG_NO_HDR" >>"$OLLA_LOG" 2>&1 &
OLLA_PID=$!
wait_for_url "${OLLA_URL}/internal/health" 20

# Test 2: header absent → backend rejects → non-200
status=$(http_status_for "${OLLA_URL}/olla/openai-compatible/v1/chat/completions")
if [[ "$status" != "200" ]]; then
    pass "missing header propagates non-200 (got ${status})"
else
    fail "missing header propagates non-200" "expected non-200 but got 200"
fi

rm -f "$CONFIG" "$CONFIG_NO_HDR"
summarise "auth-headers-only"
