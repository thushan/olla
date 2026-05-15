#!/usr/bin/env bash
# Shared helpers for auth test scripts.
# Source this file; do not execute it directly.

# Colour codes — match the project's ANSI palette
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RESET='\033[0m'

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=()

pass() {
    local name="$1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    PASSED_TESTS=$((PASSED_TESTS + 1))
    printf "${GREEN}PASS${RESET}: %s\n" "$name"
}

fail() {
    local name="$1"
    local reason="${2:-}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    FAILED_TESTS+=("$name")
    if [[ -n "$reason" ]]; then
        printf "${RED}FAIL${RESET}: %s (%s)\n" "$name" "$reason" >&2
    else
        printf "${RED}FAIL${RESET}: %s\n" "$name" >&2
    fi
}

# summarise prints the final PASS/FAIL summary line and exits with the
# appropriate code.  Call from a trap or at the end of each script.
summarise() {
    local script_name="${1:-test}"
    echo
    echo "Results: ${PASSED_TESTS}/${TOTAL_TESTS} passed"
    if [[ ${#FAILED_TESTS[@]} -eq 0 ]]; then
        printf "${GREEN}PASS${RESET}: %s\n" "$script_name"
        return 0
    else
        printf "${RED}FAIL${RESET}: %s\n" "$script_name"
        return 1
    fi
}

# wait_for_url polls until the URL returns HTTP 200 or the timeout is reached.
wait_for_url() {
    local url="$1"
    local timeout="${2:-15}"
    local attempt=0
    until curl -sf --max-time 2 "$url" >/dev/null 2>&1; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $timeout ]]; then
            printf "${RED}ERROR${RESET}: %s did not become available within %ss\n" "$url" "$timeout" >&2
            return 1
        fi
        sleep 1
    done
}

# wait_for_mockbackend polls the unauthenticated /health endpoint on the mock
# backend. Use this instead of wait_for_url for mockbackend instances because
# the auth-enforced /v1/models path returns 401 which curl -sf treats as failure.
wait_for_mockbackend() {
    local base_url="$1"
    local timeout="${2:-15}"
    wait_for_url "${base_url}/health" "$timeout"
}

# http_status_for issues a POST with optional bearer token and returns the
# HTTP status code.
http_status_for() {
    local url="$1"
    local token="${2:-}"
    local extra_header="${3:-}"
    local extra_value="${4:-}"

    local args=(-s -o /dev/null -w "%{http_code}" --max-time 10
        -X POST
        -H "Content-Type: application/json"
        -d '{"model":"mock-model","messages":[{"role":"user","content":"hi"}],"max_tokens":5}')

    if [[ -n "$token" ]]; then
        args+=(-H "Authorization: Bearer $token")
    fi
    if [[ -n "$extra_header" && -n "$extra_value" ]]; then
        args+=(-H "${extra_header}: ${extra_value}")
    fi

    curl "${args[@]}" "$url"
}

# kill_proc sends SIGTERM to a PID and waits for it to exit.
kill_proc() {
    local pid="$1"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    fi
}

# free_port kills any process already listening on the given port so repeated
# test runs don't collide with stale backends from previous runs.
# Works on Linux, macOS, and Windows (Git Bash / MSYS2).
free_port() {
    local port="$1"
    local pids=""

    # Linux: ss is preferred; fall back to netstat -tlnp
    if command -v ss >/dev/null 2>&1; then
        pids=$(ss -tlnp "sport = :${port}" 2>/dev/null | grep -oP '(?<=pid=)\d+' || true)
    fi

    # macOS: lsof
    if [[ -z "$pids" ]] && command -v lsof >/dev/null 2>&1; then
        pids=$(lsof -ti "tcp:${port}" 2>/dev/null || true)
    fi

    # Windows (Git Bash): netstat -ano gives PID in last column
    if [[ -z "$pids" ]] && command -v netstat >/dev/null 2>&1; then
        pids=$(netstat -ano 2>/dev/null \
            | grep -E "[:.]${port}\s+.+LISTENING" \
            | awk '{print $NF}' || true)
    fi

    if [[ -n "$pids" ]]; then
        for pid in $pids; do
            # On Windows, bash kill may fail; fall through to taskkill
            kill "$pid" 2>/dev/null || \
                { command -v taskkill >/dev/null 2>&1 && taskkill /F /PID "$pid" >/dev/null 2>&1; } || true
        done
        sleep 0.8
    fi
}
