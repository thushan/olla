#!/usr/bin/env bash
# auth-env-fatal.sh — proves Olla refuses to start when a config references an
# environment variable that is not set.
#
# This is the most important safety test: a missing credential must produce a
# loud startup failure, not a silent zero-value that sends unauthenticated
# requests to production backends.
#
# The script asserts:
#   1. Olla exits non-zero within a few seconds (fatal startup error).
#   2. The error output mentions the endpoint name so the operator knows which
#      endpoint's config is broken.
#
# Requires: go, bash 4+
# Does NOT require Docker / AIMock or a running backend.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

OLLA_PORT="${OLLA_PORT:-40119}"
# Port chosen to avoid collision with other auth test scripts; a backend is
# not actually required because Olla must fail before it ever connects.
BACKEND_PORT="${BACKEND_PORT:-19914}"
OLLA_LOG="${TMPDIR:-/tmp}/olla-auth-env-fatal.log"

# Deliberately unset — must NOT be present when Olla starts.
MISSING_VAR="OLLA_TEST_MISSING_TOKEN_$(date +%s%N)"
unset "$MISSING_VAR" 2>/dev/null || true

ENDPOINT_NAME="mock-env-fatal"

echo "=== auth-env-fatal: missing env var must abort startup ==="
echo "Missing var: \${${MISSING_VAR}}"
echo "Endpoint name in config: ${ENDPOINT_NAME}"
echo

CONFIG=$(mktemp "${TMPDIR:-/tmp}/olla-auth-env-fatal-XXXXXX.yaml")
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
      - url: "http://127.0.0.1:${BACKEND_PORT}"
        name: "${ENDPOINT_NAME}"
        type: "openai-compatible"
        priority: 100
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 5s
        check_timeout: 2s
        auth:
          type: bearer
          token: "\${${MISSING_VAR}}"
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
  level: "debug"
  format: "text"
  output: "stdout"
YAML

# Run Olla and capture combined output; it must exit non-zero quickly.
# We give it 15s — it should fail in under 1s but allow for slow CI machines.
set +e
timeout 15 go run "$REPO_ROOT" --config "$CONFIG" >"$OLLA_LOG" 2>&1
EXIT_CODE=$?
set -e

# Test 1: non-zero exit (fatal startup failure)
if [[ $EXIT_CODE -ne 0 ]]; then
    pass "Olla exited non-zero on missing env var (exit ${EXIT_CODE})"
else
    fail "Olla exited non-zero on missing env var" "got exit 0 — startup should have aborted"
fi

# Test 2: error output mentions the endpoint name
# This lets operators know which endpoint has the broken config, not just
# that some env var somewhere is missing.
if grep -qF "${ENDPOINT_NAME}" "$OLLA_LOG"; then
    pass "error mentions endpoint name (${ENDPOINT_NAME})"
else
    fail "error mentions endpoint name (${ENDPOINT_NAME})" "endpoint name not found in output"
    echo "--- Olla output ---" >&2
    cat "$OLLA_LOG" >&2
    echo "-------------------" >&2
fi

# Test 3: error output mentions the missing variable name
if grep -qF "$MISSING_VAR" "$OLLA_LOG"; then
    pass "error mentions missing variable name (${MISSING_VAR})"
else
    fail "error mentions missing variable name (${MISSING_VAR})" "variable name not found in output"
fi

rm -f "$CONFIG"
summarise "auth-env-fatal"
