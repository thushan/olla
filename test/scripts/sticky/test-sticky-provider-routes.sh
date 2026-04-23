#!/usr/bin/env bash
# test-sticky-provider-routes.sh
#
# Verifies sticky session affinity across every provider-scoped route that
# AIMock can serve. Routes are defined as a data table at the top; a single
# run_sticky_test() function handles miss→hit→diversity assertions for each.
#
# Routes whose backend speaks a non-OpenAI protocol (Ollama /api/*, Lemonade
# /api/v1/chat/completions) are explicitly skipped with a printed reason — they
# require a dedicated mock server that is not included in this harness.
#
# The /olla/openai/ and /olla/openai-compatible/ routes were affected by the
# sticky session context injection bug in providerProxyHandler — they are the
# primary regression targets here.
#
# Usage:
#   OLLA_URL=http://localhost:40114 bash test/scripts/sticky/test-sticky-provider-routes.sh
#
# Prerequisites:
#   - AIMock running (make mock-up)
#   - Olla running with test/manual/config.sticky.yaml (sticky sessions enabled)
#   - curl, jq available

set -euo pipefail

OLLA_URL="${OLLA_URL:-http://localhost:40114}"
CURL_TIMEOUT=30
TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0

# Colour codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
WHITE='\033[1;37m'
GREY='\033[0;37m'
RESET='\033[0m'

# ── helpers ───────────────────────────────────────────────────────────────────

pass() { echo -e "  ${GREEN}✓ PASS${RESET} — $*"; TOTAL=$((TOTAL+1)); PASSED=$((PASSED+1)); }
fail() { echo -e "  ${RED}✗ FAIL${RESET} — $*"; TOTAL=$((TOTAL+1)); FAILED=$((FAILED+1)); }

skip() {
    local label="$1"
    local reason="$2"
    echo -e "${YELLOW}SKIP${RESET} ${WHITE}${label}${RESET} — ${reason}"
    SKIPPED=$((SKIPPED+1))
    echo
}

banner() {
    echo
    echo -e "${PURPLE}╔══════════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}  ${CYAN}Olla Sticky Session — All Provider Routes Regression Test${RESET}  ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╚══════════════════════════════════════════════════════════════╝${RESET}"
    echo
}

wait_for_olla() {
    echo -e "${YELLOW}Waiting for Olla at ${OLLA_URL}...${RESET}"
    local attempts=0
    until curl -sf --max-time 2 "${OLLA_URL}/internal/health" > /dev/null 2>&1; do
        attempts=$((attempts+1))
        if [ "$attempts" -ge 30 ]; then
            echo -e "${RED}ERROR: Olla did not become ready after 30s${RESET}"
            exit 1
        fi
        sleep 1
    done
    echo -e "${GREEN}✓ Olla is ready${RESET}"
    echo
}

# Extract a response header value (case-insensitive).
# Returns empty string when header is absent — || true prevents set -e from
# triggering when grep finds no match.
extract_header() {
    local file=$1
    local header=$2
    grep -i "^${header}:" "$file" | head -1 | cut -d' ' -f2- | tr -d '\r\n' || true
}

# Extract BACKEND:instance-X from a response body (handles both OpenAI and
# Anthropic response shapes). Returns empty string when marker is absent.
extract_backend_marker() {
    local body=$1
    echo "$body" | grep -oE 'BACKEND:instance-[a-z]+' | head -1 || true
}

# ── per-path test ─────────────────────────────────────────────────────────────
#
# run_sticky_test <label> <url_path> <body_json> [check_passthrough] [skip_turn3_reason]
#
#   Three-turn sticky session verification:
#     Turn 1: miss  — new session is pinned to a backend
#     Turn 2: hit   — same session lands on the same backend
#     Turn 3: diversity — across 30 fresh sessions at least one hits elsewhere
#              (skipped when skip_turn3_reason is non-empty)
#   Optionally asserts X-Olla-Mode: passthrough on turn 1 (Anthropic path).

run_sticky_test() {
    local label="$1"
    local url_path="$2"
    local body_json="$3"
    local check_passthrough="${4:-false}"
    local skip_turn3_reason="${5:-}"

    local ts
    ts=$(date +%s%3N)
    local session_id="sess-${label}-${ts}"
    local headers_file
    headers_file=$(mktemp)
    local body_file
    body_file=$(mktemp)

    echo -e "${WHITE}── ${label} ──${RESET}"
    echo -e "  ${GREY}Path: ${url_path}${RESET}"

    # ── Turn 1: expect miss ───────────────────────────────────────────────────
    local http_code
    http_code=$(curl -s -w "%{http_code}" -o "$body_file" -D "$headers_file" \
        --max-time "$CURL_TIMEOUT" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "X-Olla-Session-ID: ${session_id}" \
        -d "$body_json" \
        "${OLLA_URL}${url_path}" 2>/dev/null)

    if [[ ! "$http_code" =~ ^2 ]]; then
        fail "Turn 1 HTTP ${http_code} (expected 2xx) — body: $(head -c 200 "$body_file")"
        rm -f "$headers_file" "$body_file"
        return
    fi
    pass "Turn 1 HTTP ${http_code}"

    local sticky1 ep1 mode1 marker1
    sticky1=$(extract_header "$headers_file" "X-Olla-Sticky-Session")
    ep1=$(extract_header "$headers_file" "X-Olla-Endpoint")
    mode1=$(extract_header "$headers_file" "X-Olla-Mode")
    marker1=$(extract_backend_marker "$(cat "$body_file")")

    [[ "$sticky1" == "miss" ]] && pass "Turn 1 sticky=miss" || fail "Turn 1 sticky='${sticky1}' (expected miss)"

    local key_src1
    key_src1=$(extract_header "$headers_file" "X-Olla-Sticky-Key-Source")
    [[ "$key_src1" == "session_header" ]] && pass "Turn 1 key-source=session_header" || fail "Turn 1 key-source='${key_src1}' (expected session_header)"

    if [[ "$check_passthrough" == "true" ]]; then
        [[ "$mode1" == "passthrough" ]] && pass "Turn 1 X-Olla-Mode=passthrough" || fail "Turn 1 X-Olla-Mode='${mode1}' (expected passthrough)"
    fi

    echo -e "  ${GREY}Pinned to: ${ep1} (${marker1})${RESET}"

    # ── Turn 2: expect hit ────────────────────────────────────────────────────
    : > "$headers_file"
    : > "$body_file"
    http_code=$(curl -s -w "%{http_code}" -o "$body_file" -D "$headers_file" \
        --max-time "$CURL_TIMEOUT" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "X-Olla-Session-ID: ${session_id}" \
        -d "$body_json" \
        "${OLLA_URL}${url_path}" 2>/dev/null)

    [[ "$http_code" =~ ^2 ]] && pass "Turn 2 HTTP ${http_code}" || fail "Turn 2 HTTP ${http_code} (expected 2xx)"

    local sticky2 ep2 marker2
    sticky2=$(extract_header "$headers_file" "X-Olla-Sticky-Session")
    ep2=$(extract_header "$headers_file" "X-Olla-Endpoint")
    marker2=$(extract_backend_marker "$(cat "$body_file")")

    [[ "$sticky2" == "hit" ]] && pass "Turn 2 sticky=hit" || fail "Turn 2 sticky='${sticky2}' (expected hit)"
    [[ "$ep2" == "$ep1" ]] && pass "Turn 2 same endpoint (${ep1})" || fail "Turn 2 endpoint changed: '${ep1}' → '${ep2}'"
    [[ "$marker2" == "$marker1" ]] && pass "Turn 2 same backend marker (${marker1})" || fail "Turn 2 backend marker changed: '${marker1}' → '${marker2}'"

    # ── Turn 3: diversity — at least one new session lands elsewhere ──────────
    # Without diversity validation, a single-instance deploy could trivially pass
    # turns 1+2, masking a broken balancer.
    # Skipped for routes whose pool is so large (e.g. main-proxy) that LCB
    # tie-breaks deterministically at zero connections, making spread meaningless.
    if [[ -n "$skip_turn3_reason" ]]; then
        echo -e "  ${YELLOW}SKIP${RESET} Turn 3 diversity — ${skip_turn3_reason}"
        SKIPPED=$((SKIPPED+1))
    else
        local seen_other=false
        local attempt
        for attempt in $(seq 1 30); do
            local new_session="sess-diversity-${label}-${ts}-${attempt}"
            : > "$headers_file"; : > "$body_file"
            http_code=$(curl -s -w "%{http_code}" -o "$body_file" -D "$headers_file" \
                --max-time "$CURL_TIMEOUT" \
                -X POST \
                -H "Content-Type: application/json" \
                -H "X-Olla-Session-ID: ${new_session}" \
                -d "$body_json" \
                "${OLLA_URL}${url_path}" 2>/dev/null)
            local ep_div
            ep_div=$(extract_header "$headers_file" "X-Olla-Endpoint")
            if [[ "$ep_div" != "$ep1" ]]; then
                seen_other=true
                break
            fi
        done
        $seen_other && pass "Turn 3 load balancing reaches multiple backends" || fail "Turn 3 all 30 attempts hit '${ep1}' only — balancer may be stuck"
    fi

    rm -f "$headers_file" "$body_file"
    echo
}

# ── stats assertion ────────────────────────────────────────────────────────────

check_sticky_stats() {
    echo -e "${WHITE}── Sticky Session Stats ──${RESET}"
    local body
    body=$(curl -sf --max-time 10 "${OLLA_URL}/internal/stats/sticky" 2>/dev/null || true)

    if [[ -z "$body" ]]; then
        fail "Could not reach /internal/stats/sticky"
        return
    fi

    pass "Stats endpoint responded"

    local insertions hits active
    if command -v jq >/dev/null 2>&1; then
        insertions=$(echo "$body" | jq -r '.insertions // 0')
        hits=$(echo "$body" | jq -r '.hits // 0')
        active=$(echo "$body" | jq -r '.active_sessions // 0')
    else
        insertions=$(echo "$body" | grep -oE '"insertions"[[:space:]]*:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' || echo 0)
        hits=$(echo "$body" | grep -oE '"hits"[[:space:]]*:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' || echo 0)
        active=$(echo "$body" | grep -oE '"active_sessions"[[:space:]]*:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' || echo 0)
    fi

    echo -e "  ${GREY}insertions=${insertions}  hits=${hits}  active_sessions=${active}${RESET}"

    [[ "${insertions:-0}" -gt 0 ]] && pass "stats.insertions > 0 (${insertions})" || fail "stats.insertions=0 (sticky sessions may not be recording)"
    [[ "${hits:-0}" -gt 0 ]]       && pass "stats.hits > 0 (${hits})"             || fail "stats.hits=0 (no hits recorded)"
    [[ "${active:-0}" -gt 0 ]]     && pass "stats.active_sessions > 0 (${active})" || fail "stats.active_sessions=0"
    echo
}

# ── route table ───────────────────────────────────────────────────────────────
#
# Format per entry:
#   LABEL|URL_PATH|BODY_TEMPLATE|CHECK_PASSTHROUGH|SKIP_REASON|SKIP_TURN3_REASON
#
# SKIP_REASON is non-empty when AIMock cannot serve the route's native protocol
# (the entire route is skipped).
# SKIP_TURN3_REASON is non-empty when the turn-3 diversity assertion is not
# meaningful for this route (turns 1 and 2 still run).
# BODY_TEMPLATE is a key selecting a pre-defined body below.

OPENAI_BODY='{"model":"test-model","messages":[{"role":"user","content":"ping"}],"max_tokens":50}'
ANTHROPIC_BODY='{"model":"claude-3-haiku-20240307","max_tokens":50,"messages":[{"role":"user","content":"ping"}]}'

# Each row: label|path|body_key|check_passthrough|skip_reason|skip_turn3_reason
# body_key: "openai" or "anthropic"
ROUTES=(
    # ── main proxy (backward-compat baseline) ──────────────────────────────────
    # Turn-3 diversity is skipped: the main-proxy pool spans all ~24 registered
    # endpoints across every provider type; LCB tie-breaks deterministically at
    # zero connections, so 30 fresh sessions consistently land on the same
    # first-ranked instance — spread is not meaningful here.
    "main-proxy|/olla/proxy/v1/chat/completions|openai|false||main-proxy pool is huge and LCB tie-break is deterministic at zero connections — spread not meaningful here"

    # ── openai-compatible provider route (primary regression target) ───────────
    # createProviderProfile("openai-compatible") widens to all OpenAI-compat types,
    # so all three mock-instance-{a,b,c} endpoints are reachable.
    "openai-compatible|/olla/openai-compatible/v1/chat/completions|openai|false||"

    # ── openai provider route ──────────────────────────────────────────────────
    # /olla/openai/ is registered via the openai-compatible profile (prefixes: openai, openai-compatible).
    # createProviderProfile("openai") also widens to all OpenAI-compat backends.
    "openai|/olla/openai/v1/chat/completions|openai|false||"

    # ── vllm provider route ────────────────────────────────────────────────────
    # Requires type: vllm endpoints. Config has dedicated vllm endpoints.
    "vllm|/olla/vllm/v1/chat/completions|openai|false||"

    # ── sglang provider route ──────────────────────────────────────────────────
    # Requires type: sglang endpoints.
    "sglang|/olla/sglang/v1/chat/completions|openai|false||"

    # ── llamacpp provider route ────────────────────────────────────────────────
    # Requires type: llamacpp endpoints.
    "llamacpp|/olla/llamacpp/v1/chat/completions|openai|false||"

    # ── lmstudio provider route ────────────────────────────────────────────────
    # Registered under three prefixes; test the canonical lmstudio one.
    # Requires type: lm-studio endpoints.
    "lmstudio|/olla/lmstudio/v1/chat/completions|openai|false||"

    # ── lm-studio alternate prefix ─────────────────────────────────────────────
    "lm-studio|/olla/lm-studio/v1/chat/completions|openai|false||"

    # ── litellm provider route ─────────────────────────────────────────────────
    # Requires type: litellm endpoints.
    "litellm|/olla/litellm/v1/chat/completions|openai|false||"

    # ── dmr (Docker Model Runner) provider route ───────────────────────────────
    # Requires type: docker-model-runner endpoints.
    "dmr|/olla/dmr/v1/chat/completions|openai|false||"

    # ── vllm-mlx provider route ────────────────────────────────────────────────
    # Requires type: vllm-mlx endpoints.
    "vllm-mlx|/olla/vllm-mlx/v1/chat/completions|openai|false||"

    # ── lemonade provider route ────────────────────────────────────────────────
    # Lemonade uses /api/v1/chat/completions, NOT /v1/chat/completions.
    # AIMock does not serve that path prefix, so this route must be skipped.
    "lemonade|/olla/lemonade/api/v1/chat/completions|openai|false|AIMock does not serve /api/v1/* — Lemonade uses a non-standard path prefix|"

    # ── ollama provider route ──────────────────────────────────────────────────
    # Ollama speaks /api/chat or /api/generate, not OpenAI /v1/chat/completions.
    # AIMock does not implement the Ollama protocol.
    "ollama|/olla/ollama/api/chat|openai|false|AIMock does not speak the Ollama /api/* protocol|"

    # ── anthropic translator route ─────────────────────────────────────────────
    # Registered via the translator layer, not the provider proxy.
    # Exercises sticky session injection in translationHandler.
    # Passthrough mode applies because openai-compatible endpoints declare
    # anthropic_support.enabled=true in their profile.
    "anthropic-translator|/olla/anthropic/v1/messages|anthropic|true||"
)

# ── main ──────────────────────────────────────────────────────────────────────

main() {
    banner
    wait_for_olla

    for entry in "${ROUTES[@]}"; do
        # Parse the pipe-delimited row (6 fields)
        IFS='|' read -r label url_path body_key check_passthrough skip_reason skip_turn3_reason <<< "$entry"

        if [[ -n "$skip_reason" ]]; then
            skip "$label ($url_path)" "$skip_reason"
            continue
        fi

        # Select the appropriate request body
        local body
        case "$body_key" in
            anthropic) body="$ANTHROPIC_BODY" ;;
            *)         body="$OPENAI_BODY" ;;
        esac

        run_sticky_test "$label" "$url_path" "$body" "$check_passthrough" "${skip_turn3_reason:-}"
    done

    check_sticky_stats

    # ── summary ───────────────────────────────────────────────────────────────
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Results:${RESET}  ${GREEN}${PASSED} passed${RESET}  ${RED}${FAILED} failed${RESET}  ${YELLOW}${SKIPPED} skipped${RESET}  (${TOTAL} total assertions)"

    if [[ "$FAILED" -eq 0 ]]; then
        echo -e "${GREEN}✓ All sticky session assertions passed.${RESET}"
        echo
        exit 0
    else
        echo -e "${RED}✗ ${FAILED} assertion(s) failed — review output above.${RESET}"
        echo
        exit 1
    fi
}

main "$@"
