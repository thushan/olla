---
name: test-sticky-sessions
description: >
  Runs the Olla sticky session integration test end-to-end across all
  provider-scoped routes. Trigger when the user asks to: verify sticky sessions
  work, run the sticky session integration test, test provider-route affinity,
  or check whether the providerProxyHandler bug fix is holding.
  Delegable to Sonnet - does not require Opus.
argument-hint: "[--olla-port PORT]"
model: sonnet
---

# /test-sticky-sessions - Sticky Session Integration Test

Verifies KV-cache affinity routing across every provider-scoped route that
AIMock can serve, including the issue #178 log-field assertions that confirm
routing decisions reach the structured log.

## Route table

| Route | Request path | Status |
|---|---|---|
| Main proxy | `/olla/proxy/v1/chat/completions` | tested |
| openai-compatible | `/olla/openai-compatible/v1/chat/completions` | tested (primary regression target) |
| openai | `/olla/openai/v1/chat/completions` | tested |
| vllm | `/olla/vllm/v1/chat/completions` | tested |
| sglang | `/olla/sglang/v1/chat/completions` | tested |
| llamacpp | `/olla/llamacpp/v1/chat/completions` | tested |
| lmstudio | `/olla/lmstudio/v1/chat/completions` | tested |
| lm-studio (alt prefix) | `/olla/lm-studio/v1/chat/completions` | tested |
| litellm | `/olla/litellm/v1/chat/completions` | tested |
| dmr | `/olla/dmr/v1/chat/completions` | tested |
| vllm-mlx | `/olla/vllm-mlx/v1/chat/completions` | tested |
| anthropic translator | `/olla/anthropic/v1/messages` | tested + passthrough assertion + log-field check |
| lemonade | `/olla/lemonade/api/v1/chat/completions` | **skipped** - AIMock does not serve `/api/v1/*` |
| ollama | `/olla/ollama/api/chat` | **skipped** - AIMock does not speak Ollama `/api/*` protocol |

## Constants

```bash
OLLA_PORT="${OLLA_PORT:-40114}"
OLLA_URL="http://localhost:${OLLA_PORT}"
LOG="${TMPDIR:-/tmp}/olla-sticky.log"
OPENAI_BODY='{"model":"test-model","messages":[{"role":"user","content":"ping"}],"max_tokens":50}'
ANTHROPIC_BODY='{"model":"claude-3-haiku-20240307","max_tokens":50,"messages":[{"role":"user","content":"ping"}]}'
CURL_TIMEOUT=30
```

## Phase 0 - Pre-flight

Check Docker is available:

```bash
docker info > /dev/null 2>&1 || { echo "Docker not running - start Docker Desktop first"; exit 1; }
```

Check port `$OLLA_PORT` is free before starting:

```bash
curl -sf --max-time 2 "${OLLA_URL}/internal/health" > /dev/null 2>&1 && \
  echo "ERROR: Olla already running on ${OLLA_PORT} - stop it first" && exit 1 || true
```

## Phase 1 - Start AIMock

```bash
make mock-up
```

Waits until all three AIMock containers report healthy (ports 9300/9301/9302).
Each instance embeds a unique `BACKEND:instance-{a,b,c}` marker in its response
body so affinity tests can confirm which backend served each response.

## Phase 2 - Start Olla

```bash
go build -o build/olla-sticky .
build/olla-sticky --config test/manual/config.sticky.yaml > "$LOG" 2>&1 &
OLLA_PID=$!
echo "Olla PID: $OLLA_PID (log: $LOG)"
```

Wait for health (poll up to 30s, 1s interval; abort to teardown on timeout):

```bash
attempt=0
until curl -sf --max-time 2 "${OLLA_URL}/internal/health" > /dev/null 2>&1; do
    attempt=$((attempt+1))
    if [ "$attempt" -ge 30 ]; then
        echo "ERROR: Olla did not become healthy within 30s"
        tail -n 80 "$LOG"
        # go to teardown
        exit 1
    fi
    sleep 1
done
echo "Olla ready"
```

## Phase 3 - Per-route sticky session assertions

For each active (non-skipped) route, run three turns. Use the helper functions
below and track PASSED/FAILED/SKIPPED counts. Record every assertion result as
`PASS:`, `FAIL:`, or `SKIP:`.

### Helper: extract_header

```bash
extract_header() {
    local file=$1 header=$2
    grep -i "^${header}:" "$file" | head -1 | cut -d' ' -f2- | tr -d '\r\n' || true
}
```

### Helper: extract_backend_marker

```bash
extract_backend_marker() {
    echo "$1" | grep -oE 'BACKEND:instance-[a-z]+' | head -1 || true
}
```

### Per-route test sequence

For each route call `run_sticky_test LABEL URL_PATH BODY [check_passthrough] [skip_turn3_reason]`:

**Turn 1 - expect miss:**

```bash
http_code=$(curl -s -w "%{http_code}" -o "$BODY_FILE" -D "$HDR_FILE" \
    --max-time $CURL_TIMEOUT \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-Olla-Session-ID: ${SESSION_ID}" \
    -d "$BODY" "${OLLA_URL}${URL_PATH}" 2>/dev/null)
```

Check the status before anything else. Any non-2xx ends this route: record
`FAIL: LABEL turn 1 returned <code>`, skip turns 2 and 3 and move to the next
route. On `429` do not continue at all, see "Abort on 429" below. A failed
turn 1 has no endpoint or marker, so the remaining assertions would only add
noise.

When the status is 2xx, assert:

- `X-Olla-Sticky-Session: miss`
- `X-Olla-Sticky-Key-Source: session_header`
- Body contains a `BACKEND:instance-*` marker (record it as `marker1`)
- Record `X-Olla-Endpoint` as `ep1`
- If `check_passthrough == true`: `X-Olla-Mode: passthrough`

**Turn 2 - same session, expect hit:**

Re-use the same `X-Olla-Session-ID` value.

- HTTP 2xx. On any other status record `FAIL: LABEL turn 2 returned <code>`,
  skip turn 3 and move to the next route (`429` aborts the run).
- `X-Olla-Sticky-Session: hit`
- `X-Olla-Endpoint` equals `ep1`
- Backend marker in body equals `marker1`

**Turn 3 - load balancing diversity (unless skip_turn3_reason is set):**

Issue up to 30 requests with distinct session IDs. Assert that at least one
response has `X-Olla-Endpoint` different from `ep1`. Stop the loop on the first
non-2xx: a 404 or 503 carries no `X-Olla-Endpoint`, so looping on would burn the
rate limit and produce false failures on later routes.

```bash
seen_other=false
bad_code=""
for i in $(seq 1 30); do
    new_session="sess-div-${LABEL}-${TS}-${i}"
    resp=$(curl -s -D - -o /dev/null -w "HTTPCODE:%{http_code}" --max-time $CURL_TIMEOUT \
        -X POST -H "Content-Type: application/json" \
        -H "X-Olla-Session-ID: ${new_session}" \
        -d "$BODY" "${OLLA_URL}${URL_PATH}" 2>/dev/null)
    code=$(echo "$resp" | grep -o 'HTTPCODE:[0-9]*' | cut -d: -f2)
    case "$code" in
        2??) ;;
        *) bad_code=${code:-000}; break ;;
    esac
    ep_div=$(echo "$resp" | grep -i "^X-Olla-Endpoint:" | cut -d' ' -f2- | tr -d '\r\n')
    if [ -n "$ep_div" ] && [ "$ep_div" != "$ep1" ]; then
        seen_other=true; break
    fi
done
if [ "$bad_code" = "429" ]; then
    echo "ABORT: rate limited (429) on ${LABEL} turn 3"; exit 1
elif [ -n "$bad_code" ]; then
    echo "FAIL: ${LABEL} turn 3 returned ${bad_code} (stopped at request ${i})"
elif ! $seen_other; then
    echo "FAIL: Turn 3 all 30 attempts hit ${ep1} only"
fi
```

A `429` here aborts the run, see below.

### Abort on 429

A `429` on any turn means the rate limiter is throttling the test, not that the
route is broken, and every later route would fail the same way. Stop testing
routes immediately, print `ABORT: rate limited (429) on LABEL turn N - raise
server.rate_limits in the test config or wait a minute and rerun`, and report the
routes not yet tested as `SKIP: not run, rate limited` rather than `FAIL`. Do not
count them as route failures in the summary.

### Route list

Process these rows in order. Skip entire row when `SKIP_REASON` is non-empty
(print `SKIP: LABEL (PATH) - REASON`). Skip turn 3 when `SKIP_TURN3_REASON` is non-empty.

```
# LABEL | PATH | BODY | CHECK_PASSTHROUGH | SKIP_REASON | SKIP_TURN3_REASON
main-proxy        | /olla/proxy/v1/chat/completions               | openai     | false |  | main-proxy pool spans all endpoints - LCB tie-break is deterministic at zero connections
openai-compatible | /olla/openai-compatible/v1/chat/completions   | openai     | false |  |
openai            | /olla/openai/v1/chat/completions               | openai     | false |  |
vllm              | /olla/vllm/v1/chat/completions                 | openai     | false |  |
sglang            | /olla/sglang/v1/chat/completions               | openai     | false |  |
llamacpp          | /olla/llamacpp/v1/chat/completions             | openai     | false |  |
lmstudio          | /olla/lmstudio/v1/chat/completions             | openai     | false |  |
lm-studio         | /olla/lm-studio/v1/chat/completions            | openai     | false |  |
litellm           | /olla/litellm/v1/chat/completions              | openai     | false |  |
dmr               | /olla/dmr/v1/chat/completions                  | openai     | false |  |
vllm-mlx          | /olla/vllm-mlx/v1/chat/completions             | openai     | false |  |
lemonade          | /olla/lemonade/api/v1/chat/completions         | openai     | false | AIMock does not serve /api/v1/* - Lemonade uses a non-standard path prefix |
ollama            | /olla/ollama/api/chat                          | openai     | false | AIMock does not speak the Ollama /api/* protocol |
anthropic-translator | /olla/anthropic/v1/messages               | anthropic  | true  |  |
```

## Phase 4 - Sticky stats assertion

```bash
body=$(curl -sf --max-time 10 "${OLLA_URL}/internal/stats/sticky" 2>/dev/null || true)
```

Assert (using `jq` when available, or grep fallback):

- `insertions > 0`
- `hits > 0`
- `active_sessions > 0`

## Phase 5 - Log-field assertions (issue #178)

These assert that routing decisions reach the structured log.

**sticky_outcome must appear in at least one completed-request line:**

```bash
if grep -q "sticky_outcome=" "$LOG" 2>/dev/null; then
    echo "PASS: log contains sticky_outcome"
else
    echo "FAIL: log missing sticky_outcome - check logRequestResult in handler_proxy.go"
fi
```

**routing_strategy must appear:**

```bash
if grep -q "routing_strategy=" "$LOG" 2>/dev/null; then
    echo "PASS: log contains routing_strategy"
else
    echo "FAIL: log missing routing_strategy - check RoutingDecision wiring"
fi
```

**provider_model - non-fatal if absent (backend may not report model):**

```bash
if grep -q "provider_model=" "$LOG" 2>/dev/null; then
    echo "PASS: log contains provider_model"
else
    echo "INFO: provider_model not found - backend may not report model in response (non-fatal)"
fi
```

**session_id must appear on INFO "Request completed" lines:**

```bash
if grep "Request completed" "$LOG" 2>/dev/null | grep -q "session_id="; then
    echo "PASS: session_id present on INFO Request completed lines"
else
    echo "FAIL: session_id missing from INFO Request completed lines - check logRequestResult"
fi
```

## Phase 6 - Teardown (always run)

```bash
kill "$OLLA_PID" 2>/dev/null || taskkill //F //PID "$OLLA_PID" 2>/dev/null || true
make mock-down
```

## Phase 7 - Report verdict

Print a summary:

```
Results: N passed  N failed  N skipped  (N total assertions)
```

Exit 0 on zero failures, exit 1 otherwise.

---

## Notes

- `test/manual/config.sticky.yaml` registers three endpoints per provider type
  (all pointing at AIMock on 9300/9301/9302) so affinity checks are meaningful.
  The default `engine: "sherpa"` covers the Sherpa path; change it to `"olla"`
  to exercise the high-performance engine.
- The `openai-compatible` profile declares `anthropic_support.enabled: true`,
  enabling passthrough mode on the Anthropic translator path.
- Lemonade and Ollama are skipped because AIMock does not implement their
  native wire protocols (`/api/v1/chat/completions` and `/api/chat`).
- The log-field assertions (Phase 5) require the log file path. They grep plain
  key=value pairs from structured slog text output, not JSON.
