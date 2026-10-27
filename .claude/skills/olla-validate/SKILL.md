---
name: olla-validate
description: >
  Full automated validation of Olla via parallel agents against mock backends
  (no Docker, no real LLMs - CI/CD safe). Gates on make ready first, then boots
  an ollamock fleet + Olla and fans out per-area validation agents covering
  core routing, OpenAI/Anthropic surfaces, observability, health/recovery and
  failure handling. Use as the pre-release gate or after major changes.
  Trigger when the user asks to validate Olla, run the validation suite,
  run regression/nightly validation, or gate a release.
  Runs on Sonnet regardless of the session model; area agents are balanced
  between Sonnet and Haiku for token efficiency.
argument-hint: "[--quick|--nightly] [--soak-minutes=N]"
model: sonnet
---

# /olla-validate - Olla Validation Harness

One reusable suite, two depths:

| Mode | Target time | Scope |
|---|---|---|
| `--quick` | 5–10 min | `make ready` gate + live smoke of every area (trimmed checklists) |
| `--nightly` | 2–4 h | full test suite + stress + exhaustive checklists + chaos + soak + sherpa pass + translation-forced pass + bench snapshot |

**If neither flag is given, ask** with AskUserQuestion: "Which validation depth?" - options **Quick (Recommended after major changes)** (5–10 min smoke gate) and **Nightly** (multi-hour exhaustive pre-release gate). `--soak-minutes=N` overrides the nightly soak duration (default 30).

The harness never touches real backends. All traffic goes to `test/cmd/ollamock`
instances (stdlib Go mock speaking OpenAI, Ollama-native, LM Studio, Lemonade,
Anthropic and Text Embeddings Inference protocols, with a `/_mock/`
fault-injection control plane).

## Model assignment (token efficiency)

The orchestration runs on Sonnet (`model: sonnet` above) - it makes the
judgement calls, sequences nightly passes and writes the report. Subagents
cannot spawn subagents, so orchestration must stay here; never try to
delegate the whole run to a single agent.

Spawned area agents get an explicit `model` on every Agent call:

- **haiku** - mechanical curl-and-assert checklists that never mutate state:
  core-routing, openai-api, observability, limits-failures (client section),
  tei-routing (nightly only, read-only).
- **sonnet** - anything that mutates mock/Olla state or needs protocol-
  sequence judgement: resilience, anthropic (both sections - SSE event-order
  and translation validation), limits-failures (upstream section).

Escalation rule: if a Haiku agent returns malformed report JSON, dies, or its
failures look like agent error rather than product error (e.g. it asserted
the wrong endpoint set), re-run that one area once on Sonnet before trusting
a FAIL. Record the escalation in the report.

## Topology (fixed ports)

One ollamock instance per endpoint - Olla keys endpoint identity on the URL,
so endpoints sharing a URL silently collapse to one (last declared wins).

| Port | Process | Endpoint (type) | Models |
|---|---|---|---|
| 19431 | ollamock `mock-a` | mock-openai-a (openai-compatible) | test-model, shared-model |
| 19432 | ollamock `mock-b` | mock-openai-b (openai-compatible) | test-model, beta-model |
| 19433 | ollamock `mock-c` | mock-ollama-c (ollama, native `/api/*`) | llama3.1:8b, shared-model |
| 19434 | ollamock `mock-d` | mock-lmstudio-d (lm-studio) | phi-4, shared-model |
| 19435 | ollamock `mock-e` | mock-vllm-e (vllm - anthropic passthrough target) | test-model, shared-model |
| 19436 | ollamock `mock-f` | mock-litellm-f (litellm - anthropic translation target) | test-model, beta-model |
| 19437 | ollamock `mock-g` | mock-llamacpp-g (llamacpp) | phi-4, shared-model |
| 41141 | olla (main) | `test/validate/config.validate.yaml` - 7 endpoints, unifier, sticky, anthropic translator | |
| 41142 | olla (limits) | `test/validate/config.validate.limits.yaml` - 256KB body cap, 30 req/min per-IP, backed by mock-a | |

Nightly only (never booted for `--quick`): a third Olla over eight further mocks,
for the model-kind routing checks in `areas/tei-routing.md`. These sit outside the
main fleet on purpose: the main checklists count endpoints and enumerate eligible
hosts per route, and these backends would change both.

| Port | Process | Endpoint (type) | Models |
|---|---|---|---|
| 19438 | ollamock `mock-tei-embed` (`--tei-kind embedding`) | mock-tei-embed (text-embeddings-inference) | BAAI/bge-small-en-v1.5 |
| 19439 | ollamock `mock-tei-rerank` (`--tei-kind reranker`) | mock-tei-rerank (text-embeddings-inference) | BAAI/bge-reranker-base |
| 19440 | ollamock `mock-tei-class` (`--tei-kind classifier`) | mock-tei-class (text-embeddings-inference) | SamLowe/roberta-go-emotions |
| 19441 | ollamock `mock-ollama-h` (`--tags-capabilities --loaded llama3.2:3b`) | mock-ollama-h (ollama, tags carry capabilities) | llama3.2:3b, nomic-embed-text:latest, qwen3-vl:8b |
| 19442 | ollamock `mock-ollama-i` (`--loaded qwen3:8b`) | mock-ollama-i (ollama, old: tags omit capabilities) | qwen3:8b, mxbai-embed-large |
| 19443 | ollamock `mock-vllm-j` | mock-vllm-j (vllm) | intfloat/e5-base-v2, vllm-chat-model |
| 19444 | ollamock `mock-sglang-k` | mock-sglang-k (sglang) | sglang-chat-model |
| 19445 | ollamock `mock-lmdeploy-l` | mock-lmdeploy-l (lmdeploy) | lmdeploy-chat-model |
| 41143 | olla (kinds) | `test/validate/config.validate.kinds.yaml` - 8 endpoints for path/capability/model routing | |

Mock fault injection: `POST http://127.0.0.1:194NN/_mock/behaviour` with
`{"mode":"ok|error|flaky|hang|slow","error_status":503,"error_rate":0.5,"latency_ms":0,"fail_health":false,"drop_mid_stream":false,"malformed_json":false}`;
`POST /_mock/reset` restores defaults; `GET /_mock/stats` gives per-path request
counts. See `test/cmd/ollamock/README.md`.

## Phase 0 - Setup

```bash
MODE=quick|nightly                  # from args or AskUserQuestion
SOAK_MINUTES=${SOAK_MINUTES:-30}    # nightly only
RUN_TS=$(date -u +%Y%m%d-%H%M%S)
GIT_SHA=$(git rev-parse --short HEAD)
RESULTS=test/results
REPORT=$RESULTS/olla-validate-$RUN_TS.md
LOGDIR=$RESULTS/logs/olla-validate-$RUN_TS
mkdir -p "$LOGDIR"
```

Check ports 19431–19437, 41141–41142 (nightly also 19438–19445, 41143) are free (`netstat -ano | grep <port>` or
curl probe); if occupied, kill stale ollamock/olla-validate processes from a
previous run, otherwise abort with a clear message.

Track every PID you start. **Teardown (Phase 7) must always run**, including on
abort.

## Phase 1 - Static gate (fail fast)

Both modes:

```bash
make ready 2>&1 | tee "$LOGDIR/make-ready.log"
```

Nightly additionally:

```bash
make test        2>&1 | tee "$LOGDIR/make-test.log"
make test-stress 2>&1 | tee "$LOGDIR/make-test-stress.log"
```

Any failure here → **abort**: skip the live phases, go straight to Phase 8 and
write a FAILED report quoting the failing output. Do not "fix and continue" -
this skill is a gate, not a repair loop. Report failures; the user decides.

## Phase 2 - Build

```bash
EXE=$(go env GOEXE)
go build -o "build/validate/olla$EXE" .
go build -o "build/validate/ollamock$EXE" ./test/cmd/ollamock
```

## Phase 3 - Boot the fleet

Start each process with the Bash tool in background mode, logging to `$LOGDIR`:

```bash
build/validate/ollamock$EXE --addr 127.0.0.1:19431 --name mock-a --models "test-model,shared-model"   > "$LOGDIR/mock-a.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19432 --name mock-b --models "test-model,beta-model"     > "$LOGDIR/mock-b.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19433 --name mock-c --models "llama3.1:8b,shared-model"  > "$LOGDIR/mock-c.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19434 --name mock-d --models "phi-4,shared-model"        > "$LOGDIR/mock-d.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19435 --name mock-e --models "test-model,shared-model"   > "$LOGDIR/mock-e.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19436 --name mock-f --models "test-model,beta-model"     > "$LOGDIR/mock-f.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19437 --name mock-g --models "phi-4,shared-model"        > "$LOGDIR/mock-g.log" 2>&1
build/validate/olla$EXE --config test/validate/config.validate.yaml        > "$LOGDIR/olla-main.log" 2>&1
build/validate/olla$EXE --config test/validate/config.validate.limits.yaml > "$LOGDIR/olla-limits.log" 2>&1
```

Readiness gates (poll up to 60s, 1s interval; abort to teardown on timeout):

1. Each mock: `curl -sf http://127.0.0.1:194NN/health`
2. Olla main: `curl -sf http://127.0.0.1:41141/internal/health`
3. Olla limits: `curl -sf http://127.0.0.1:41142/internal/health`
4. All 7 endpoints healthy: poll `/internal/status/endpoints` until every entry reports healthy/routable
5. Model discovery done: poll `/olla/models` until it lists `shared-model`

Record boot time in the report. If any process dies during boot, capture the
tail of its log into the report.

## Phase 4 - Wave 1: parallel happy-path agents

Spawn **five agents in parallel** (single message, multiple Agent calls,
`subagent_type: general-purpose`, `model` per the table below). Each agent
prompt must contain:

- The mode (`quick` or `nightly`) and instruction to execute the matching
  checklist in its area file (path below) - read the file first.
- The topology table above (URLs, ports, which mock backs which endpoint).
- The reporting contract: *"Return ONLY a JSON object:
  `{"area":"<name>","pass":N,"fail":N,"warn":N,"skip":N,"failures":[{"check":"","expected":"","actual":"","evidence":""}],"warnings":[...],"notes":""}`.
  Every checklist item is exactly one of pass/fail/warn/skip. Use warn for
  behaviour that works but looks off; never silently skip - record skips with a
  reason in notes. Do not modify any repo files. Do not kill or restart any
  process. Do not call any `/_mock/behaviour` endpoint unless your area file
  explicitly says to."*

| Agent | Area file | Model |
|---|---|---|
| core-routing | `.claude/skills/olla-validate/areas/core-routing.md` | haiku |
| openai-api | `.claude/skills/olla-validate/areas/openai-api.md` | haiku |
| anthropic (passthrough section) | `.claude/skills/olla-validate/areas/anthropic.md` | sonnet |
| observability | `.claude/skills/olla-validate/areas/observability.md` | haiku |
| limits-failures (client section - 41142 only) | `.claude/skills/olla-validate/areas/limits-failures.md` | haiku |

Wave 1 agents are read-only against the mocks: **no fault injection**, so they
can run concurrently without poisoning each other's assertions.

## Phase 5 - Wave 2: resilience agent (solo)

After wave 1 completes, `POST /_mock/reset` to all seven mocks (ports 19431-19437), confirm all
endpoints healthy again, then spawn **one** agent (`model: sonnet`) for
`.claude/skills/olla-validate/areas/resilience.md` (mode-appropriate
checklist). This agent **is** allowed to inject faults via `/_mock/behaviour`
and (nightly) to ask you to kill/restart a mock - for process kill/restart
steps the agent reports back what it needs and you perform the kill/restart
yourself, or simpler: give the agent the PID table and let it use taskkill/kill
itself, then verify afterwards that all seven mocks are running again (restart
any that are not, from the same command lines as Phase 3).

After wave 2: reset all mock behaviours, re-confirm all endpoints return to
healthy within 60s (this is itself a recovery assertion - record it; health
probes tick globally every 30s regardless of per-endpoint check_interval).

## Phase 6 - Nightly-only extended passes (sequential)

Skip this phase entirely in quick mode.

### 6a. Upstream-failure section of limits-failures
Spawn the limits-failures agent again (`model: sonnet` - it mutates mock-a
behaviour), pointing it at the **upstream section** of its area file (runs
solo). Reset mocks after.

### 6b. Translation-forced Anthropic pass
```bash
sed 's/passthrough_enabled: true/passthrough_enabled: false/' \
  test/validate/config.validate.yaml > "$LOGDIR/config.translation.yaml"
```
Restart olla-main with that config, wait for readiness (Phase 3 gates 2/4/5),
zero mock stats (`POST /_mock/reset` on all), then spawn the anthropic agent
(`model: sonnet`) against the **translation-forced section** of its area file. Restart olla-main
on the original config afterwards and re-confirm readiness.

### 6c. Sherpa engine pass
```bash
sed 's/engine: "olla"/engine: "sherpa"/' \
  test/validate/config.validate.yaml > "$LOGDIR/config.sherpa.yaml"
```
Restart olla-main on it, wait for readiness, then run **quick** checklists of
core-routing, openai-api and anthropic (passthrough) - three parallel agents
(haiku, haiku, sonnet respectively, as in wave 1).
Sherpa is maintenance-mode: quick depth is deliberate. Restore the original
config and readiness afterwards.

### 6d. Model-kinds pass (tei-routing)
Boots the nightly-only fleet from the topology table, so `--quick` and the
phases above never see it. Check ports 19438-19445 and 41143 are free first.

```bash
build/validate/ollamock$EXE --addr 127.0.0.1:19438 --name mock-tei-embed  --models "BAAI/bge-small-en-v1.5"      --tei-kind embedding  > "$LOGDIR/mock-tei-embed.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19439 --name mock-tei-rerank --models "BAAI/bge-reranker-base"      --tei-kind reranker   > "$LOGDIR/mock-tei-rerank.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19440 --name mock-tei-class  --models "SamLowe/roberta-go-emotions" --tei-kind classifier > "$LOGDIR/mock-tei-class.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19441 --name mock-ollama-h   --models "llama3.2:3b,nomic-embed-text:latest,qwen3-vl:8b" --loaded "llama3.2:3b" --tags-capabilities > "$LOGDIR/mock-ollama-h.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19442 --name mock-ollama-i   --models "qwen3:8b,mxbai-embed-large"  --loaded "qwen3:8b" > "$LOGDIR/mock-ollama-i.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19443 --name mock-vllm-j     --models "intfloat/e5-base-v2,vllm-chat-model" > "$LOGDIR/mock-vllm-j.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19444 --name mock-sglang-k   --models "sglang-chat-model"            > "$LOGDIR/mock-sglang-k.log" 2>&1
build/validate/ollamock$EXE --addr 127.0.0.1:19445 --name mock-lmdeploy-l --models "lmdeploy-chat-model"          > "$LOGDIR/mock-lmdeploy-l.log" 2>&1
build/validate/olla$EXE --config test/validate/config.validate.kinds.yaml > "$LOGDIR/olla-kinds.log" 2>&1
```

Readiness: each mock answers `/health`; `curl -sf http://127.0.0.1:41143/internal/health`;
poll `/internal/status/endpoints` until all 8 are healthy and `/olla/models` lists
`BAAI/bge-reranker-base` and `mxbai-embed-large`. Then spawn one agent
(`model: haiku`) for `.claude/skills/olla-validate/areas/tei-routing.md`, nightly
depth. It is read-only and shares nothing with the main fleet. Its section B needs
unmerged work (#3, #4, #5, #6, #8) and probes for it, so it records skips, not failures,
on a tree without them. Kill these nine PIDs in Phase 7 with the rest.

### 6e. Soak + chaos
Run for `$SOAK_MINUTES` minutes against olla-main (olla engine, original
config):

- Background load: a loop issuing ~5 req/s mixed traffic (non-stream chat,
  streaming chat, anthropic messages, /olla/models) - a small bash loop is
  fine; log status-code counts.
- Every 60s sample `/internal/process` (goroutines, heap) to
  `$LOGDIR/soak-samples.jsonl`.
- Chaos: every ~3 minutes pick one mock, inject `fail_health` or `mode=error`,
  hold for 60s, then reset. (Health probes tick globally every 30s, so
  shorter faults may never be observed by the checker.) Never fault more than
  one mock at a time, and never mock-a and mock-b simultaneously (they are
  the only openai-compatible-typed pair).

Pass criteria: zero client-visible 5xx for requests issued while at least one
compatible backend was healthy and un-faulted (allow a small transition window
of ≤5s after each injection); goroutine count at end within 25% of the
pre-soak baseline after a 60s settle; heap not monotonically growing across
the final third of samples. Anything outside → FAIL with the samples quoted.

### 6f. Bench snapshot
```bash
make bench-balancer 2>&1 | tee "$LOGDIR/bench-balancer.log"
```
Record results in the report (informational - WARN if any benchmark fails to
run, never FAIL on numbers).

## Phase 7 - Teardown (always)

Kill every started PID (olla instances first, then mocks). On Windows Git
Bash, `kill <pid>` works for processes you started; fall back to
`taskkill //F //PID <pid>` if needed. Verify ports are released. Remove
`build/validate/` binaries only if the user's tree was clean of them before.

## Phase 8 - Report & verdict

Aggregate every agent JSON plus the phase-level checks you performed yourself
(gates, boot, recovery-after-reset, soak) into `$REPORT`:

```markdown
# Olla Validation - <mode> - <RUN_TS>
- Commit: <GIT_SHA>  Branch: <branch>  Engine passes: olla[, sherpa]
- Verdict: PASS | FAIL
- Totals: P/F/W/S

| Area | Pass | Fail | Warn | Skip |
|---|---|---|---|---|
...

## Failures
<one block per failure: check, expected, actual, evidence>

## Warnings / Notes / Soak samples summary / Bench snapshot
```

Append one line to `$RESULTS/last-runs.md`
(create with a header if missing):

```
<UTC date time> | olla-validate (<mode>) | PASS|FAIL | <P>P/<F>F/<W>W | <GIT_SHA> | report:<REPORT path>
```

**Verdict rule:** any FAIL anywhere (including the Phase 1 gate, boot, soak or
a dead agent) → overall FAIL. Warnings never fail the gate but must all appear
in the report. Finish by telling the user the verdict, the totals, the three
most important findings in plain sentences, and the report path.
