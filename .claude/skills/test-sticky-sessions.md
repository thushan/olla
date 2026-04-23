---
name: test-sticky-sessions
description: >
  Runs the Olla sticky session integration test harness end-to-end across all
  provider-scoped routes. Trigger when the user asks to: verify sticky sessions
  work, run the sticky session integration test, test provider-route affinity,
  or check whether the providerProxyHandler bug fix is holding.
  Delegable to Sonnet — does not require Opus.
---

# Sticky Session Integration Test

This skill exercises sticky session affinity across **all** provider-scoped
routes that AIMock can serve:

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
| anthropic translator | `/olla/anthropic/v1/messages` | tested + passthrough assertion |
| lemonade | `/olla/lemonade/api/v1/chat/completions` | **skipped** — AIMock does not serve `/api/v1/*` |
| ollama | `/olla/ollama/api/chat` | **skipped** — AIMock does not speak Ollama `/api/*` protocol |

The `/olla/openai-compatible/` and `/olla/openai/` paths were affected by a bug
where `providerProxyHandler` never injected sticky session context — those
routes are the primary regression targets.

## Steps

### 1. Pre-flight: verify Docker is running

```bash
docker info > /dev/null 2>&1 || { echo "Docker is not running — start Docker Desktop first"; exit 1; }
```

### 2. Start AIMock instances

```bash
make mock-up
```

Waits until all three AIMock containers report healthy (ports 9300/9301/9302).
Each instance returns a unique `BACKEND:instance-{a,b,c}` marker so the test
can confirm which backend served each response.

### 3. Build Olla and start with sticky config

```bash
LOG="${TMPDIR:-/tmp}/olla-sticky.log"
go run . --config test/manual/config.sticky.yaml > "$LOG" 2>&1 &
OLLA_PID=$!
```

Wait until ready:
```bash
until curl -sf http://localhost:40114/internal/health > /dev/null; do sleep 1; done
echo "Olla ready (PID $OLLA_PID, log $LOG)"
```

### 4. Run the assertion script

```bash
OLLA_URL=http://localhost:40114 bash test/scripts/sticky/test-sticky-provider-routes.sh
RESULT=$?
```

For each active (non-skipped) route, the script asserts:
- Turn 1: `X-Olla-Sticky-Session: miss`, `X-Olla-Sticky-Key-Source: session_header`
- Turn 2: `X-Olla-Sticky-Session: hit`, same `X-Olla-Endpoint` as Turn 1, same backend marker
- Turn 3: across 10 fresh sessions, at least one lands on a different backend
- Anthropic path additionally asserts `X-Olla-Mode: passthrough`
- Stats endpoint: `insertions > 0`, `hits > 0`, `active_sessions > 0`

Skipped routes print clearly: `SKIP <label> — <reason>` and do not count as failures.

### 5. Teardown (bulletproof — always runs)

```bash
kill "$OLLA_PID" 2>/dev/null || true
make mock-down
exit "$RESULT"
```

### Fully automated (single command)

```bash
make test-sticky-manual
```

This target handles all five steps including the EXIT trap teardown.

---

## Expected output (passing run)

```text
╔══════════════════════════════════════════════════════════════╗
║  Olla Sticky Session — All Provider Routes Regression Test  ║
╚══════════════════════════════════════════════════════════════╝

── main-proxy ──
  ✓ PASS — Turn 1 HTTP 200
  ✓ PASS — Turn 1 sticky=miss
  ✓ PASS — Turn 1 key-source=session_header
  Pinned to: mock-compat-b (BACKEND:instance-b)
  ✓ PASS — Turn 2 HTTP 200
  ✓ PASS — Turn 2 sticky=hit
  ✓ PASS — Turn 2 same endpoint (mock-compat-b)
  ✓ PASS — Turn 2 same backend marker (BACKEND:instance-b)
  ✓ PASS — Turn 3 load balancing reaches multiple backends

── openai-compatible ──
  ... same pattern ...

  ... (vllm, sglang, llamacpp, lmstudio, lm-studio, litellm, dmr, vllm-mlx) ...

SKIP lemonade (/olla/lemonade/api/v1/chat/completions) — AIMock does not serve /api/v1/* — Lemonade uses a non-standard path prefix
SKIP ollama (/olla/ollama/api/chat) — AIMock does not speak the Ollama /api/* protocol

── anthropic-translator ──
  ✓ PASS — Turn 1 X-Olla-Mode=passthrough
  ...

── Sticky Session Stats ──
  ✓ PASS — stats.insertions > 0
  ✓ PASS — stats.hits > 0
  ✓ PASS — stats.active_sessions > 0

Results:  99 passed  0 failed  2 skipped  (99 total assertions)
✓ All sticky session assertions passed.
```

---

## Manual verification (troubleshooting)

**Health check:**
```bash
curl -s http://localhost:40114/internal/health | python3 -m json.tool
```

**Turn 1 — main proxy:**
```bash
curl -s -D - -X POST http://localhost:40114/olla/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Olla-Session-ID: debug-sess-001" \
  -d '{"model":"test","messages":[{"role":"user","content":"ping"}],"max_tokens":20}'
```

Expected response headers:
```text
X-Olla-Sticky-Session: miss
X-Olla-Sticky-Key-Source: session_header
X-Olla-Endpoint: mock-compat-{a,b,c}
```

**Turn 2 — same session, expect hit:**
```bash
curl -s -D - -X POST http://localhost:40114/olla/proxy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Olla-Session-ID: debug-sess-001" \
  -d '{"model":"test","messages":[{"role":"user","content":"ping"}],"max_tokens":20}'
```

Expected: `X-Olla-Sticky-Session: hit`

**Provider-scoped route (regression path):**
```bash
curl -s -D - -X POST http://localhost:40114/olla/openai-compatible/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Olla-Session-ID: debug-sess-002" \
  -d '{"model":"test","messages":[{"role":"user","content":"ping"}],"max_tokens":20}'
```

**vLLM-specific route:**
```bash
curl -s -D - -X POST http://localhost:40114/olla/vllm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-Olla-Session-ID: debug-sess-vllm" \
  -d '{"model":"test","messages":[{"role":"user","content":"ping"}],"max_tokens":20}'
```

**Anthropic passthrough:**
```bash
curl -s -D - -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -H "X-Olla-Session-ID: debug-sess-003" \
  -d '{"model":"claude-3-haiku-20240307","max_tokens":20,"messages":[{"role":"user","content":"ping"}]}'
```

Expected: `X-Olla-Mode: passthrough`

**Stats:**
```bash
curl -s http://localhost:40114/internal/stats/sticky | python3 -m json.tool
```

---

## Notes

- `test/manual/config.sticky.yaml` registers three endpoints per provider type
  (all pointing at AIMock on 9300/9301/9302) so affinity checks are meaningful.
- The `openai-compatible` profile declares `anthropic_support.enabled: true`,
  enabling passthrough mode on the Anthropic translator path.
- Lemonade and Ollama routes are skipped cleanly — they require a dedicated mock
  that speaks their native protocols (`/api/v1/chat/completions` and
  `/api/chat`/`/api/generate` respectively).
- To test the Olla engine (high-performance), change `engine: "sherpa"` to
  `engine: "olla"` in `test/manual/config.sticky.yaml` and re-run.
- The script is portable: `#!/usr/bin/env bash`, no absolute paths, no
  platform-specific constructs. Runs on macOS, Linux, and Git-Bash on Windows.
