#!/usr/bin/env bash
# Full end-to-end sticky session manual test orchestrator.
# Starts AIMock, runs Olla with the sticky config, invokes the assertion
# script, and tears everything down on exit (success or failure).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

OLLA_PORT="${OLLA_PORT:-40114}"
OLLA_URL="${OLLA_URL:-http://localhost:${OLLA_PORT}}"
OLLA_LOG="${OLLA_LOG:-${TMPDIR:-/tmp}/olla-sticky.log}"
CONFIG="${OLLA_CONFIG:-test/manual/config.sticky.yaml}"

OLLA_PID=""

cleanup() {
    echo "Tearing down..."
    if [ -n "$OLLA_PID" ] && kill -0 "$OLLA_PID" 2>/dev/null; then
        kill "$OLLA_PID" 2>/dev/null || true
        wait "$OLLA_PID" 2>/dev/null || true
        echo "Olla stopped"
    fi
    (cd "$REPO_ROOT" && make mock-down 2>/dev/null) || true
}
trap cleanup EXIT INT TERM

cd "$REPO_ROOT"

echo "Starting AIMock containers..."
make mock-up

echo "Starting Olla with sticky session config..."
# Background go run so we can signal the parent process on teardown.
go run . --config "$CONFIG" >"$OLLA_LOG" 2>&1 &
OLLA_PID=$!
echo "Olla PID: $OLLA_PID (log: $OLLA_LOG)"

echo "Waiting for Olla to become healthy..."
attempt=0
until curl -sf --max-time 2 "${OLLA_URL}/internal/health" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 30 ]; then
        echo "ERROR: Olla did not become healthy within 30s"
        echo "--- last 80 log lines ---"
        tail -n 80 "$OLLA_LOG" || true
        exit 1
    fi
    sleep 1
done
echo "Olla is ready"

OLLA_URL="$OLLA_URL" bash "$SCRIPT_DIR/test-sticky-provider-routes.sh"
