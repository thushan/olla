# Spec: Lean Read-Only Admin Dashboard (PR 1)

Branch: `feature/simple-dashboard` (created off `main` at `f4bde8d`).

## 0. Read this first: how this spec must be executed

This spec is written for an orchestrating agent using the `/orchestrate` workflow. You will decompose it into work packages, dispatch each to the specialist named for it, review the specialist's output against this spec's acceptance criteria, and dispatch fixes back to the *same* specialist when work deviates. Do not implement work packages yourself in the main orchestrator context — dispatch them.

### THE GOVERNING PRINCIPLE — KISSAI

**Keep It Simple, Stupid, AI.** Take the leanest path to a working, read-only dashboard. No over-engineering, no cleverness, no speculative generality, no refactoring of adjacent code because it looks improvable.

### Why this matters here specifically

A previous attempt at this exact feature (`feature/dashboard-impl`) reached **151 commits** and had to be abandoned as unreviewable. It started as a dashboard and accreted four successive rewrites of the circuit breaker, request-accounting changes across both proxy engines, and streaming-path changes. Each individual step was defensible — verification found a real adjacent bug, it got fixed in place, that fix needed its own verification, repeat — but the accumulated scope was unreviewable. The dashboard work itself, inspected in isolation, is sound and largely reusable. The circuit-breaker/proxy/streaming work is not part of this branch.

You can see the evidence yourself: diff `main` against `feature/dashboard-impl` and note it touches `internal/adapter/health/circuit_breaker.go` (+479 lines), five new circuit-breaker test files, `internal/adapter/proxy/olla/service.go`, `internal/adapter/proxy/sherpa/service_retry.go`, `internal/app/services/proxy.go`, and adds a whole `circuit_breaker:` config block that does not exist anywhere on `main` today. None of that is dashboard work. It must not come back on this branch.

### The scope rule — the single most important line in this spec

> **A bug or limitation discovered during this work is FIXED only if this branch introduced it. Anything pre-existing is LOGGED, never fixed.**

Maintain a running findings list at `docs/spec/simple-dashboard-findings.md`. Create it at the start of work. Every specialist that notices a pre-existing bug, limitation, or improvement opportunity outside this spec's scope appends an entry (what, where, why it's out of scope, suggested severity) instead of fixing it. This file is the seed input for PR 2. In your final report, you must explicitly state that nothing outside this spec's scope was fixed, and point at the findings file.

Two tripwires, both hard stops requiring you to reconsider before proceeding, not soft suggestions:

1. **Any change touching a subsystem outside "read-only admin dashboard" is evidence it belongs in the findings list, not the diff.** If a work package's diff touches `internal/adapter/proxy/`, `internal/adapter/health/circuit_breaker.go`, Sherpa, or streaming code, stop and re-scope — log the trigger, don't fix it.
2. **A second attempt at the same fix within this branch is a hard stop.** If a specialist's fix comes back needing a second correction for the same underlying issue, escalate to the user rather than iterating a third time. Iteration loops on a single fix are exactly the failure mode that produced 151 commits.

## 1. Purpose and scope

Olla (`D:\projects\thushan-olla`) is a Go 1.24 proxy and load balancer for local, self-hosted LLM inference backends (Ollama, LM Studio, vLLM, llama.cpp, SGLang, Lemonade, OpenAI-compatible). Hexagonal architecture: `internal/core` (domain), `internal/adapter` (infrastructure), `internal/app` (application/HTTP). Two proxy engines: Sherpa (maintenance-mode — **no changes permitted, ever**) and Olla (active development, but **out of scope for this branch** — zero edits under `internal/adapter/proxy/`).

Operators currently see fleet state only via JSON endpoints (`/internal/status`, `/internal/status/endpoints`, `/internal/status/models`) or logs. This PR adds a minimal, read-only, browser-based dashboard so an operator can glance at fleet health without curling JSON.

**In scope for PR 1:**
- A Svelte 5 + Tailwind SPA, built to static assets, embedded into the Go binary via `go:embed`, served at `/internal/ui/` on the existing proxy HTTP listener (same origin — no CORS is ever needed).
- No authentication. Access restricted to loopback by default via a per-request network-layer check (CIDR + Host), widenable by config.
- Three panels: Overview, Endpoints, Models.
- Polling for freshness, not realtime (no WebSocket, no SSE).
- Small additive JSON fields on existing `/internal/status*` handlers so the frontend has enough data to render without new endpoints.

**Explicitly out of scope for PR 1** (see §4 for detail on each and why):
- ETag / conditional GET.
- The circuit breaker column / "why degraded" text.
- Per-model traffic columns in the Models panel.
- Any change to proxy engines, request accounting, streaming, or the circuit breaker. Zero edits under `internal/adapter/proxy/`. Zero edits to Sherpa.
- Actions, mutation, config editing, log streaming, authentication.

## 2. Constraints (restate in every work package's brief)

- Go 1.24. Do not bump the toolchain. Held-back `golang.org/x/*` pins are documented in the root `CLAUDE.md` — do not run `go get -u ./...` unqualified.
- No new Go or npm dependencies.
- No panics in production code (project convention — guard closes with CAS or `sync.Once`, never a bare panic path).
- No changes to Sherpa (`internal/adapter/proxy/sherpa/`). No changes anywhere under `internal/adapter/proxy/`.
- Do not weaken the existing security chain or middleware ordering. `feature/dashboard-impl` at one point exempted all `/internal/*` paths from it — that must not recur. The dashboard's access gate is additive and applies only to `/internal/ui/` by default (see §5).
- `make ready` must pass, and must never require Node, before any work package is reported complete.
- Commit small and often: one coherent change per commit, humanised one-line messages, no `Co-Authored-By` lines.
- Do not merge to `main`. Do not push. Stop at "branch is ready and verified" and report.

## 3. Functional requirements

- **FR-1**: The dashboard is served at `GET /internal/ui/` and all sub-paths, on the same HTTP listener Olla already serves the proxy and `/internal/*` API from. No new listener, no new port.
- **FR-2**: The dashboard has three panels: Overview, Endpoints, Models, reachable via client-side navigation (no full page reload between them).
- **FR-3**: Overview shows fleet-level status: endpoints up/total, aggregate success rate, aggregate average latency, total traffic, uptime, version, active connections, security violation count.
- **FR-4**: Endpoints panel lists every configured endpoint with: name, type, status, priority, success rate, average/min/max proxy latency, request count, model count, last health check (relative and absolute), next health check (relative and absolute), last model sync (relative and absolute), sanitised URL, and per-endpoint model list.
- **FR-5**: Models panel lists discovered models with: name, family, parameter size, quantisation, size on disk, which endpoints host each model, last seen (relative and absolute). No per-model traffic figures (§4.3).
- **FR-6**: The frontend polls each panel's backing endpoint on an interval with jittered backoff when the tab is not visible (reuse the existing scheduler from `feature/dashboard-impl`, §5.1). It does not use ETag/conditional GET (§4.1).
- **FR-7**: Both light and dark themes are supported, following system preference by default, with a manual toggle.
- **FR-8**: Endpoint and model tables are sortable by column, client-side.
- **FR-9**: When the dashboard is disabled (`dashboard.enabled: false`), the `/internal/ui/` route is not registered at all — a request gets the default mux 404, not a 403, so the mount point is not discoverable by an external scanner.
- **FR-10**: When a binary is built without the frontend having been built first (no Node toolchain available, or `make build-web` skipped), the Go binary still builds and runs; the dashboard route serves an explanatory 503 rather than failing to compile or serving garbage.
- **FR-11**: The access policy accepts any request whose `Host` header parses as an IP literal (loopback or otherwise), regardless of the configured `allowed_hosts` list, because DNS rebinding protection via hostname allowlisting is meaningless against a client that already resolved to a raw IP. Non-IP hostnames must appear in `allowed_hosts`.
- **FR-12**: `DefaultConfig()` includes `localhost` in `dashboard.access_policy.allowed_hosts` by default, so a no-config-file install (`go install`, curl installer) is not 403'd on first run against Olla's own shipped default of binding `0.0.0.0`.
- **FR-13**: Existing `/internal/status`, `/internal/status/endpoints`, `/internal/status/models` responses gain additive fields only (§4.4). No existing field is renamed, retyped, or removed. This is a hard requirement — a previous attempt broke it.
- **FR-14**: Endpoint URLs surfaced anywhere in the dashboard's JSON are sanitised: userinfo stripped, `RawQuery` and `Fragment` stripped wholesale (do not attempt to allowlist "safe" query parameters — strip everything). Endpoint URLs containing userinfo are rejected at config validation time with an error directing the operator to the existing `auth` config block.
- **FR-15**: Output that depends on iteration order (endpoint lists, model lists, map-derived slices) is sorted deterministically before being written to JSON, so responses are stable across polls and diffable in tests.

## 4. Detail on scope boundaries

### 4.1 Out of scope: ETag / conditional GET

The frontend only sends `If-None-Match` after it has already seen an `ETag` from a prior response. Never implementing ETag means the frontend simply always gets a full `200` body — it degrades to plain polling with zero breakage, not a partial feature. Skipping this avoids an entire class of cache-validity bugs (stale ETag after config reload, hash collisions, etc.) for a freshness feature nobody asked for. `feature/dashboard-impl` built a full ETag layer (`status_etag.go`, 142 lines, plus a 533-line test file) — do not copy or reimplement it.

### 4.2 Out of scope: circuit breaker column

On `main` today, the circuit breaker (in `internal/adapter/health/`) trips only on health-probe failures, not on live proxy-traffic failures. A dashboard column showing breaker state would therefore under-report real traffic failures and mislead the operator into thinking a backend is fine when it is actively failing proxy requests. Omit the circuit-breaker column from the Endpoints panel entirely, and omit any "why degraded" text derived from breaker state in the status strip. This gap (breaker not wired to proxy-path HTTP 5xx) is a known pre-existing limitation — log it in the findings file if not already tracked, do not fix it here. It is PR 2 scope, contingent on separate proxy-path changes this branch must not make.

### 4.3 Out of scope: per-model traffic in the Models panel

`ports.StatsCollector.RecordModelRequest` exists (`internal/core/ports/stats.go`) and `/internal/stats/models` (`handler_status_models... ` — actually served by a separate model-stats handler) returns a shape with requests/success-rate/p95/p99 columns. But nothing on `main` currently calls `RecordModelRequest` from the proxy request path in either engine, so those figures are always zero. Displaying always-zero traffic columns is worse than not displaying them — it reads as "this model has never been used" when the truth is "we never counted it."

**PR 1's Models panel shows discovery data only**: name, family, parameter size, quantisation, size, and which endpoints host each model — all sourced from `GetEndpointModelMap` / `domain.ModelInfo.Details`, which is live and correct on `main` today. Remove any per-model traffic columns and the companion `/internal/stats/models` fetch from the copied frontend (§5.3). Wiring `RecordModelRequest` into the proxy path is a proxy-engine change and is explicitly forbidden on this branch (§2) — it is PR 2 scope.

### 4.4 Additive handler fields — what already exists vs what's new

Verified against `main` at `f4bde8d`:

- `ports.EndpointStats` (`internal/core/ports/stats.go`) **already has** `AverageLatency` (`avg_latency_ms`), `MinLatency` (`min_latency_ms`), `MaxLatency` (`max_latency_ms`) as plain `int64` fields with no zero-value guard. The struct is fine as-is; the gap is that neither HTTP handler that reports per-endpoint data (`EndpointSummary` in `handler_status_endpoints.go`, `EndpointResponse` in `handler_status.go`) currently surfaces min/max latency at all, and `handler_status.go`'s `AvgLatency` is a pre-formatted string (`"12ms"`), not a machine-readable number.
- `EndpointSummary.ResponseTime` (`handler_status_endpoints.go`) is `format.Latency(endpoint.LastLatency.Milliseconds())` — the **last health-check probe's** latency, formatted as a string (e.g. `"14ms"`). This is a different metric from proxy request latency and must never be parsed back into a number or presented as if it were the average proxy latency.
- `EndpointSummary` has no `url` field today. Add one, sanitised per FR-14.
- `EndpointSummary`/`EndpointResponse` carry only relative time strings today (`format.TimeAgo`, `format.TimeUntil`) for health-check and model-sync timestamps. Add absolute RFC3339 timestamps alongside them (see below), additive.
- Per-endpoint model detail (`GetEndpointModelMap`, `domain.EndpointModels`, `domain.ModelInfo.Details`) already exists and is already partially surfaced (`ModelCount`, `LastModelSync` in `EndpointSummary`). Extend to include the model list itself where the Endpoints panel needs it.

Concretely, add to the endpoints response (`/internal/status/endpoints`, i.e. `EndpointSummary`, and equivalently to `/internal/status`'s `EndpointResponse` where it makes sense):

| Field | Type | Notes |
|---|---|---|
| `min_latency_ms` | `int64` | From `ports.EndpointStats.MinLatency`. Only meaningful when `hasStats && TotalRequests > 0`; omit or zero-value per existing convention used for `RequestCount`/`SuccessRate` in that handler — match whichever the wire-shape test pins as current behaviour for zero-traffic endpoints. |
| `max_latency_ms` | `int64` | From `ports.EndpointStats.MaxLatency`. Same zero-traffic handling as above. |
| `avg_latency_ms` | `*int64` with `omitempty` | Average **proxy request** latency from `ports.EndpointStats.AverageLatency`. Pointer + `omitempty` so an endpoint with no traffic **omits** the field rather than emitting a misleading `0`. Do not confuse with `response_time` (health-probe latency, a string, unrelated metric — see above). |
| `url` | `string` | Sanitised per FR-14 (userinfo stripped, query/fragment stripped). |
| `next_check_at` | `*time.Time` with `omitempty`, RFC3339 | Absolute form of the existing `next_check` relative string. Additive — `next_check` stays exactly as-is. |
| `health_check_at` | `*time.Time` with `omitempty`, RFC3339 | Absolute form of the existing `health_check`/`last_check` relative string. Additive. |
| `last_model_sync_at` | `*time.Time` with `omitempty`, RFC3339 | Absolute form of the existing `last_model_sync` relative string. Additive. |
| `last_seen_at` | `*time.Time` with `omitempty`, RFC3339 | Per-model last-seen, absolute form, alongside the existing relative `last_seen` string on model entries (`ModelSummary` in `handler_status_models.go`). Additive. |

**Hard requirement (FR-13)**: nothing existing may be renamed, retyped, or removed from `StatusResponse`, `EndpointStatusResponse`/`EndpointSummary`, or `ModelStatusResponse`/`ModelSummary`. A previous attempt broke this. The wire-shape pinning test (work package 2, §6) is non-negotiable and must be written and passing before any additive field lands.

Also apply FR-15 (deterministic ordering) to every slice/map iteration in these three handlers that isn't already sorted (`handler_status.go`'s `buildUnifiedEndpoints` already sorts; `handler_status_models.go`'s family-grouping map iteration and endpoint-name map iteration do not — sort them).

## 5. Reuse plan: `feature/dashboard-impl`

An earlier branch, `feature/dashboard-impl`, contains a verified, final-form implementation of most of this work, buried inside the 151-commit history described in §0. **Do not attempt to cherry-pick commits from it — the history is the problem, not the endpoint state.** Instead, copy the *current tip state* of specific paths using `git checkout feature/dashboard-impl -- <path>`, then verify each copied file compiles and passes in its new context on this branch. Do not assume it still works unmodified — this branch's `main` base has diverged and moved forward since `feature/dashboard-impl` was cut.

**Whole-directory/whole-file copies (safe — these paths contain no circuit-breaker/proxy scope creep):**

- `web/dashboard/` — the entire Svelte 5 frontend tree. Self-contained, zero runtime dependencies, its own vitest suite and `svelte-check` config. Includes a jittered poll scheduler with `visibilityState` backoff (`src/lib/poll-scheduler.js`), a shared clock (`src/lib/clock.svelte.js`), a theme store with system/light/dark (`src/lib/stores/theme.svelte.js`), and a `SortableTable` component (`src/components/SortableTable.svelte`).
- `internal/app/handlers/dashboard/` — `embed.go` (the `go:embed` handler: `fs.Sub`, SPA fallback restricted to extensionless paths, real 404s for asset-shaped misses, a MIME override map, cache headers, the "not built" 503 fallback) and `access.go` (the network-layer access policy: RemoteAddr-only IP check, Host allowlist with IP-literal auto-accept per FR-11, self-diagnosing 403 body). Copy their test files too (`embed_test.go`, `access_test.go`).
- Makefile frontend targets: `install-web`, `build-web`, `test-web`, `lint-web`, `clean-web`, `check-fonts-web`, `ci-web`, and the Node-toolchain check appended to `install-deps`/`check-tools`. The project's Makefile is lowercase (`makefile`), not `Makefile` — check both are not present, and edit the one that exists.
- `docs/content/configuration/dashboard.md` and the frontend build-prerequisite note added to `docs/content/development/setup.md`.

**Surgical extraction only — do NOT whole-file copy these** (verified: each of these files on `feature/dashboard-impl` also carries unrelated circuit-breaker or ETag changes that must not land):

- `internal/config/types.go` — copy only the `DashboardConfig` and `AccessPolicyConfig` type blocks and their methods (`Validate`, `ParsedCIDRs`). Do **not** copy any `CircuitBreakerConfig` additions in the same file.
- `internal/config/config.go` — copy only the `Dashboard: DashboardConfig{...}` block inside `DefaultConfig()` and the `c.Dashboard.Validate()` call inside `Config.Validate()`. Do **not** copy the `CircuitBreaker: CircuitBreakerConfig{...}` block or the `c.Proxy.CircuitBreaker.Validate()` call that sit next to them on that branch.
- `config/config.yaml` — copy only the `dashboard:` block. Do **not** copy the `circuit_breaker:` block that sits above it in the `proxy:` section on that branch.
- `internal/app/handlers/server_routes.go` — copy only: the dashboard import, the `dashboard.RegisterRoutes(...)` call mounted **last** (so it cannot shadow any provider or internal route registered earlier), and `assertNoRouteCollidesWithDashboard` run before that mount. Do **not** copy the `gateInternal`/`gateIfWanted` wrapping of every existing `/internal/*` route registration *unless* you are implementing `GateInternalAPI` support (optional — see §5.2); if you skip `GateInternalAPI`, still copy the `DashboardConfig.Enabled` gate and the collision check, just without wrapping the other routes.
- `handler_status.go`, `handler_status_endpoints.go`, `handler_status_models.go` — do **not** copy these at all. `feature/dashboard-impl`'s versions bundle the additive fields together with the full ETag layer and the per-model traffic columns this spec excludes. Reimplement the additive fields fresh per §4.4 (work package 2).
- `.github/workflows/ci.yml`, `.goreleaser.yml`, `.gitignore` — copy only the Node setup step / `make ci-web` step / `make build-web` before-hook / dashboard-dist gitignore entries shown in the `feature/dashboard-impl` diff. These files may have moved on since that branch was cut — re-apply as a patch, don't blindly overwrite.

**Known-good properties of the copied work — re-verify empirically, do not assume they still hold after the copy:**

- `install-web` must be a prerequisite of `test-web`, `lint-web`, and `build-web`, with no inline `npm ci` inside `build-web` itself. Verify by running `rm -rf web/dashboard/node_modules && make ci-web` from a clean state and confirming it succeeds.
- Built assets are **not** committed. `internal/app/handlers/dashboard/dist/` is gitignored except for a `.gitkeep` sentinel. The Go binary must build and run with **no Node present at all** (`make ready`, `make test`, `make build` via plain `go build` must all succeed without Node); the dashboard route serves the explanatory 503 (FR-10) in that state.
- `make ready` must never require Node. `ci-web` is a separate target, not folded into `ready`.
- The `.woff2` magic-byte check (`check-fonts-web`) exists because a font file that was actually a saved HTML error page once shipped in this history. Keep it and verify it actually catches a corrupted font (swap in a text file with a `.woff2` extension temporarily and confirm the target fails, then restore).
- The access policy reads the client IP from `r.RemoteAddr` only. `X-Forwarded-For` / `X-Real-IP` must never be consulted for this decision, under any config. Verify this is actually true in the copied `access.go`, not merely commented as true.
- Host header check accepts any IP literal (FR-11) plus configured hostnames. `DefaultConfig()` must include `localhost` in `AllowedHosts` (FR-12) — verify a no-config-file `olla` process serving on `0.0.0.0` is reachable at `http://localhost:PORT/internal/ui/` without a 403.

### 5.1 Frontend architecture notes (for the specialist, informational)

The copied frontend already implements: a shared jittered poll scheduler with visibility-based backoff, a shared reactive clock (for relative-time display without re-fetching), a theme store (system/light/dark with manual override, persisted), and a generic `SortableTable` component used by both the Endpoints and Models panels. Reuse these as-is; do not rewrite them.

### 5.2 `GateInternalAPI` is optional for PR 1

`feature/dashboard-impl`'s `DashboardConfig.GateInternalAPI` extends the same access policy to the rest of `/internal/*` and `/version`. This is a genuinely separable feature from "serve a dashboard at `/internal/ui/`." Include it if the copy-and-verify comes cheaply (the code exists and is small), but do not treat it as blocking — if it introduces any risk of the security-chain regression called out in §2 (exempting `/internal/*` from existing middleware), default it to `false` and log a note in the findings file rather than debug it under this branch's time budget.

## 6. Non-functional requirements

- **NFR-1**: Dashboard JSON endpoints (the existing `/internal/status*` handlers, now with additive fields) must not measurably regress response time for existing consumers — no new expensive computation on the hot path (e.g. no per-request DNS, no per-request URL re-parsing beyond what sanitisation strictly requires).
- **NFR-2**: The access-policy check runs before any dashboard asset is read from the embedded FS or written to the response, so a rejected request never touches the embed layer.
- **NFR-3**: No dashboard code path may panic. Config parsing/validation errors surface at startup (`Config.Validate()`), not as runtime panics on first request.
- **NFR-4**: The SPA must render correctly at 375px, 768px, and 1440px viewport widths, in both light and dark themes, with no horizontal overflow.
- **NFR-5**: Status indication (healthy/degraded/offline) must remain legible in greyscale (colour is not the only signal — pair with text/shape).

## 7. Suggested work package decomposition

Dispatch each to the named specialist. Dependencies are listed; packages with no dependency on each other can run in parallel.

**WP-1: Copy and wire frontend + embed handler + access policy + config**
Specialist: `svelte-expert-developer` for the frontend half, `go-principal-architect` for the Go half — split into WP-1a (frontend copy, `svelte-expert-developer`) and WP-1b (embed handler, access policy, config, route mounting, `go-principal-architect`). WP-1b depends on nothing from WP-1a except knowing the route (`/internal/ui/`) and that the frontend build output lands in `internal/app/handlers/dashboard/dist/`.
Acceptance: `web/dashboard/` copied and building (`make build-web` succeeds); `internal/app/handlers/dashboard/{embed,access}.go` and tests copied and passing; `DashboardConfig`/`AccessPolicyConfig` surgically extracted into `types.go`/`config.go`/`config.yaml` per §5 (no circuit-breaker leakage — diff the PR against `feature/dashboard-impl`'s file to confirm nothing extra came along); route mounted last in `server_routes.go` with the collision check; `make ready` green with no Node present; dashboard reachable at `/internal/ui/` from loopback, 403 from a non-loopback source (test via a spoofed `RemoteAddr` or an actual non-loopback bind), 404 when `dashboard.enabled: false`.

**WP-2: Additive handler fields + wire-shape pinning test**
Specialist: `go-principal-architect`, with `test-architect` writing the pinning test (or `go-principal-architect` writing it under `test-architect` review — orchestrator's call based on the pairing that worked in earlier packages).
Depends on: nothing (independent of WP-1, touches different files).
Acceptance: fields listed in §4.4 added to `EndpointSummary`/`EndpointResponse`/`ModelSummary` per FR-13/FR-14/FR-15; a test exists that captures `main`'s current wire shape for `/internal/status`, `/internal/status/endpoints`, and `/internal/status/models` (field names and JSON types) and asserts every pre-existing field still round-trips under the same key and type after this work package's changes; URL sanitisation verified with a test case containing userinfo and a query string; config validation rejects an endpoint URL containing userinfo with a clear error naming the `auth` config block; `make ready` green.

**WP-3: Frontend trim — remove circuit-breaker column, Models traffic columns, companion fetch**
Specialist: `svelte-expert-developer`.
Depends on: WP-1a (needs the copied frontend in place first).
Acceptance: no circuit-breaker column or "why degraded" text anywhere in the Endpoints panel or status strip; Models panel shows only discovery columns (name, family, params, quant, size, endpoints, last seen); the `/internal/stats/models` fetch and its store are removed, not just hidden in the UI; existing vitest suite updated to match and passing; `svelte-check` clean.

**WP-4: Build/CI/Makefile wiring + clean-checkout verification**
Specialist: `go-principal-architect` (Makefile and CI are Go-project tooling here, not frontend).
Depends on: WP-1a and WP-1b (needs both the frontend and the embed handler to exist to wire the full pipeline).
Acceptance: Makefile targets copied/patched per §5 and confirmed against the project's actual Makefile conventions (box-rule banners, section order, `.PHONY`, `install-tools`/`check-tools` entries for Node — see root `CLAUDE.md` Makefile conventions); `.github/workflows/ci.yml` and `.goreleaser.yml` patched (not overwritten) to add the Node setup step and `make ci-web`/`make build-web` calls; `rm -rf web/dashboard/node_modules && make ci-web` succeeds from a genuinely clean state; `make ready` succeeds with `web/dashboard/node_modules` absent and no Node binary on `PATH` (simulate by adjusting `PATH` for that one invocation); a plain `go build ./...` with no prior `make build-web` succeeds and the resulting binary serves the 503 not-built response at `/internal/ui/`.

**WP-5: Documentation**
Specialist: `docs-writer`.
Depends on: WP-1 through WP-4 substantially complete (docs describe the shipped behaviour, not the plan).
Acceptance: `docs/content/configuration/dashboard.md` copied/adapted from `feature/dashboard-impl` and updated to match this PR's actual scope (no circuit-breaker column, no per-model traffic — the docs must not promise features this PR doesn't ship); `docs/content/development/setup.md` gains the frontend build prerequisite note; every YAML snippet in the docs is validated against the actual shipped config (project convention: Olla silently ignores unknown config keys, so a typo in a doc snippet won't be caught by Olla itself — the docs-writer must run each snippet through `Config.Validate()` or equivalent, not eyeball it).

**WP-6: Orchestrator-run seeded Playwright verification**
Run by: the orchestrator itself, using `web-verifier`, after WP-1 through WP-5 are all merged into the branch's working state. Not delegated wholesale — the orchestrator drives the fleet setup and traffic generation, then hands the running instance to `web-verifier` for the browser assertions.
Depends on: WP-1, WP-2, WP-3, WP-4.
Procedure and acceptance: see §8.

**WP-7: Code review**
Specialist: `code-reviewer`.
Depends on: WP-1 through WP-6 all complete and green.
Acceptance: full-diff review against this spec's acceptance criteria; any finding that is in-scope gets dispatched back to the originating specialist for a fix (subject to the two-strikes tripwire in §0); any finding that is out-of-scope gets appended to `docs/spec/simple-dashboard-findings.md` instead of fixed. `code-reviewer` output is the last gate before the orchestrator declares the branch ready.

## 8. Verification the orchestrator must run itself (not optional)

1. `make ready` green. `make ci-web` green from a genuinely clean checkout with no `node_modules` present.
2. **Seeded UI verification with Playwright, via `web-verifier`.** The repo has mock backends at `test/cmd/ollamock` and `test/cmd/mockbackend`, and an `olla-validate` skill that boots an ollamock fleet — use or adapt that boot process. Boot a realistic multi-endpoint fleet (several backends, several models, at least one endpoint deliberately unhealthy or offline), point Olla at it, drive real traffic through the proxy so counters actually move, then hand off to `web-verifier`. Required assertions:
   - All three panels render real (non-placeholder, non-zero-everywhere) data.
   - Counters visibly change between two successive polls.
   - Clicking a column header reorders the table rows.
   - Both light and dark themes render without visual breakage.
   - No console errors during normal navigation and polling.
   - No horizontal overflow at 375px, 768px, and 1440px viewport widths.
   - Status indicators are distinguishable in a greyscale rendering (NFR-5).
3. **Verify against a long-running instance, not a freshly started one.** Leave Olla running and the dashboard polling for several minutes before asserting anything — a previous verification pass missed a real bug because a freshly started instance never reached steady state (e.g. averages computed from a single sample, health-check cycles that hadn't completed a full round yet).
4. **Assert rendered values, not just text content, where a bug could hide in prop wiring.** Specifically check computed `style` attributes (e.g. width/height percentages) on any bar or meter component, not only its adjacent text label. A prop-name mismatch has previously silently zeroed every latency bar's visual width while its text label still read the correct number — a text-only assertion would have passed.
5. `code-reviewer` pass on the full diff (WP-7) before the orchestrator declares the branch complete.

Report format for this section: pass/fail per assertion above, with screenshots or console/network evidence from `web-verifier` attached to the final report.

## 9. Acceptance criteria summary (traceability)

| Requirement | Verified by |
|---|---|
| FR-1 through FR-3, FR-7, FR-8 | WP-1 acceptance + WP-6 Playwright pass |
| FR-4, FR-5 | WP-1 + WP-3 acceptance + WP-6 Playwright pass |
| FR-6 | WP-1a (scheduler reused unmodified) + WP-6 "counters change between polls" |
| FR-9, FR-10 | WP-1b / WP-4 acceptance |
| FR-11, FR-12 | WP-1b acceptance (loopback + localhost checks) |
| FR-13, FR-14, FR-15 | WP-2 acceptance (wire-shape pinning test, sanitisation test) |
| NFR-1 | WP-2 review (no new hot-path cost) |
| NFR-2, NFR-3 | WP-1b acceptance + code-reviewer (WP-7) |
| NFR-4, NFR-5 | WP-6 Playwright assertions |

## 10. Assumptions

- **A1**: "Endpoints response" in the user's brief refers to `/internal/status/endpoints` (`EndpointSummary`) as the primary target for the new fields, with equivalent fields added to `/internal/status`'s `EndpointResponse` where they don't already exist in some form (e.g. `AvgLatency` there is already a formatted string; add the raw `avg_latency_ms` alongside it rather than replacing it). Rejected alternative: adding fields only to `/internal/status` and leaving `/internal/status/endpoints` as-is — rejected because the dashboard's Endpoints panel is the primary consumer and that handler currently has the least detail.
- **A2**: `GateInternalAPI` (§5.2) is treated as optional/best-effort rather than a hard requirement of this PR, since the user's brief describes it as part of the reused config blocks but the core ask (three panels, loopback-gated, read-only) does not depend on it. Rejected alternative: making it mandatory — rejected because it's the one piece of the copied `server_routes.go` diff that touches every existing route registration, which is exactly the kind of broad surface change §0's tripwires warn about; better to default it off and log rather than force it through under time pressure.
- **A3**: The min/max/avg latency zero-traffic display convention (whether zero-traffic endpoints show `0`, omit the field, or show a sentinel) for `min_latency_ms`/`max_latency_ms` follows whatever the existing handler already does for adjacent zero-traffic fields (`RequestCount`, `SuccessRate` show `"N/A"` for the latter) rather than inventing a new convention — the wire-shape test (WP-2) should pin whichever choice is made so it doesn't drift silently later. `avg_latency_ms` is explicitly pointer+omitempty per the user's brief, which is unambiguous; min/max are not, hence this assumption. Rejected alternative: also making min/max pointer+omitempty for consistency — plausible, but the brief only specified this for `avg_latency_ms`, so treating it as the more conservative additive-field default (plain zero value, matching the existing struct's current non-pointer fields) avoids guessing beyond what was asked.
- **A4**: `web-verifier`'s Playwright session is run by the orchestrator against a fleet booted via the `olla-validate` skill's ollamock harness (or a close adaptation of it) rather than a bespoke fleet setup, since that harness already exists and is CI-safe. Rejected alternative: standing up real Ollama/vLLM instances for verification — rejected as unnecessary weight for a UI-rendering check, and inconsistent with the project's existing mock-backend-first testing convention.

## 11. Open questions

None genuinely blocking. If the orchestrator hits a case not covered above, default to the more conservative reading (narrower scope, additive-only, log rather than fix) per §0, and note the decision in the findings file rather than pausing for stakeholder input.
