# Simple Dashboard — Findings (PR 2 input)

> Seed file created per spec §0. Pre-existing bugs, limitations, and
> improvement opportunities discovered during the dashboard work are **logged
> here, never fixed on this branch**. This is the input to PR 2. It is expected
> to be non-empty when the branch is declared ready.

Scope rule (spec §0): a bug or limitation is **fixed only if this branch
introduced it**. Anything pre-existing is appended below instead.

Findings are added by specialists as they encounter them, and by the
orchestrator during review/verification. Format: what, where, why out of scope
for PR 1, suggested severity.

---

## Findings

_(populated as work packages proceed)_

### WP-2 — Additive handler fields + FR-14 validation

- **`endpointsStatusHandler` dereferences `a.statsCollector` without a nil guard**
  (`internal/app/handlers/handler_status_endpoints.go`). Pre-existing: the
  handler already called `a.statsCollector.GetEndpointStats()` unconditionally
  before this branch. WP-2 added `a.statsCollector.GetConnectionStats()` in the
  same pattern (required to populate the new `active_connections` field), so the
  latent nil-panic surface is unchanged but is now exercised by one more call.
  A nil `statsCollector` is not a configuration the application constructs in
  production (the DI container always wires one), so this is theoretical. Fixing
  it would mean editing this handler defensively, which is out of scope
  (additive-only). Severity: low.

- **`buildStatusResponse` dereferences `a.Config` without a nil guard**
  (`internal/app/handlers/handler_status.go:157`, `a.Config.Proxy`). Pre-existing
  on main. Out of scope for the same reason. Severity: low.

- **`parseTimeAgoOptimised` is a coarse heuristic** — **RESOLVED, not a PR 2
  item.** (`internal/app/handlers/handler_status_models.go`). This entry
  originally recorded a cosmetic precision-loss concern. A verified review
  found the actual defect was worse than cosmetic: `parseTimeAgoOptimised`'s
  substring checks (`"second"`, `"minute"`, `"hour"`, `"day"`) never matched
  `format.TimeAgo`'s real compact output (`"10m ago"`, `"2h ago"`), so every
  model fell into the same fallback bucket and `recent_models` was
  unconditionally alphabetical, never actually recency-ordered, on any real
  fleet. This branch introduced the bug (the handler and its call to
  `format.TimeAgo` are new on this branch), so it was fixed here rather than
  logged: sorting and multi-endpoint "which timestamp wins" comparisons now
  use the real `time.Time` value (`ModelSummary.LastSeenAt`) instead of
  round-tripping through the formatted string, and `parseTimeAgoOptimised` was
  deleted (its only caller). See `newerModelTimestamp`/`modelLastSeenTime` in
  the same file.

### WP-3 — Frontend trim and contract completion

- **`ModelsPanel` flat-path rows have no DOM id** (`web/dashboard/src/panels/ModelsPanel.svelte`,
  `web/dashboard/src/components/SortableTable.svelte`). Pre-existing in the
  copied frontend: the grouped path's snippet renders `<tr id={rowId(m)}>` on
  each row, but the flat `recent_models` path goes through `SortableTable`'s
  default `{#each sorted ...}` wrapper which renders a bare `<tr>` with no id
  (the `rowId` prop is used only as Svelte's keyed-each key, not a DOM id).
  WP-3's new `ModelsPanel.test.js` locates the flat row by name text rather
  than id to sidestep this. Fixing it means either planting the id in the flat
  `rowSnippet` or teaching `SortableTable` to emit `id` on rows, both broader
  than the trim brief. Severity: low (no operator-facing impact; only affects
  test selectors and the `#model-<name>` anchor, which OverviewPanel's "jump to
  endpoint" pattern does not target for models).

- **`StatusTag`'s breaker branch is now unreachable**
  (`web/dashboard/src/components/StatusTag.svelte`). The `kind="breaker"` code
  path (its `CB` status map) remained after WP-3 removed the EndpointsPanel
  Breaker column, which was its sole caller. It is dead but harmless. Left in
  place rather than trimmed because removing it is component surgery beyond
  the column-removal brief, and PR 2 may revisit how breaker state is
  presented once the breaker is wired to proxy-path failures (§4.2).
  Severity: low (dead branch, no runtime effect).

### WP-1b — Go wiring (embed handler, access policy, config, route mount, quiet-poll logging)

- **`DashboardConfig.GateInternalAPI` is inert on this branch**
  (`internal/config/types.go`, `config/config.yaml`). The field ships (default
  `false`) so PR 2 needs no config migration, but no wrapping logic is
  implemented: setting it `true` has no effect. The intended behaviour (extend
  the same `AccessPolicy` to the rest of `/internal/*` and `/version`) is PR 2
  scope per simple-dashboard.md §5.2. Prior art for the wiring lives in
  `feature/dashboard-impl`'s `server_routes.go` (`gateInternal`/`gateIfWanted`).
  Severity: low (documented gap, no operator-facing impact since the field
  defaults off and is explicitly inert).

- **`logDashboardPolicy` startup summary line is absent.** Under the
  no-signature-change route-mounting design (§5), `registerRoutes()` does not
  return an error, so `internal/app/services/http.go` stays byte-for-byte
  unchanged and the `logDashboardPolicy()` helper that lived there on
  `feature/dashboard-impl` does not exist on this branch. 403s remain
  self-diagnosing via `access.go`'s `reject()` body, so the dashboard is
  operable without it. PR 2 can reintroduce the line (it logs the effective
  CIDR/host allowlist at startup) without revisiting the route-mounting design.
  Severity: low (cosmetic observability gap).

### FR-4 / §4.4 — Per-endpoint model list cut

- **No per-endpoint model list in the Endpoints panel** (spec FR-4, §4.4 audit).
  The Endpoints panel shows a model *count* per endpoint but not the model
  names themselves. Cut because there is no prior art for it anywhere in the
  copied frontend or in `handler_status_endpoints.go` — `EndpointSummary` only
  ever carried `model_count`, never a name list — and the Models panel's own
  `endpoints` column on each model already answers the inverse question
  (which endpoints host a given model), so the two panels together cover the
  same information without duplicating a name list in both directions.
  Building it would mean a new additive field (an array of model names per
  endpoint) plus frontend rendering, both beyond the additive-fields brief.
  Severity: low (nice-to-have, not a gap in current capability).

### §4.4 — `per_endpoint` tooltip data cut

- **`per_endpoint` field (per-endpoint parameter size, used only for a pill
  tooltip on the Models panel) not added** (spec §4.4 field-audit table).
  `feature/dashboard-impl`'s frontend read a `per_endpoint` key off
  `ModelSummary` to show a tooltip when the same model reports different
  parameter sizes across endpoints, but there is no prior art for this field
  on `main` — `ModelSummary` has never carried it, and it is not part of this
  branch's additive-field list (§4.4). Cut the tooltip lookup rather than add
  a new field for a single UI affordance nobody has asked for on this branch.
  Same shape of finding as the FR-4 per-endpoint model list above: both are
  two-sided design work (new backend field + new frontend rendering) that
  belongs in PR 2 if wanted. Severity: low (cosmetic, tooltip-only).

### §4.2 — Circuit breaker not wired to proxy-path failures

- **The circuit breaker only trips on health-probe failures, never on live
  proxy-traffic failures** (`internal/adapter/health/circuit_breaker.go`).
  This is the underlying gap behind the decision to omit the
  `circuit_breaker` column from the Endpoints panel entirely (§4.2): today, a
  backend that returns 5xx to every proxied request but still answers health
  probes cleanly keeps its breaker closed and keeps receiving traffic, so a
  breaker-state column would misleadingly show "closed"/healthy while the
  backend is actively failing every real request. This is distinct from the
  StatusTag finding above (which documents dead *frontend* code, the
  now-unreachable `kind="breaker"` branch) — this entry is the backend
  behaviour that made adding that column pointless in the first place. Fixing
  it means wiring HTTP 5xx observation from both proxy engines into the
  breaker, which is explicitly forbidden on this branch (§2: zero edits under
  `internal/adapter/proxy/` and `internal/adapter/health/`). PR 2 scope,
  contingent on separate proxy-path changes. Severity: medium (operator-facing
  correctness gap once a breaker-state column is eventually added, but no
  regression on this branch since PR 1 never surfaces breaker state at all).

### WP-6 — Seeded Playwright verification

- **`StatusTag` rendering for `busy`/`warming`/`config_error`/`rate_limited`
  is unverified** (`web/dashboard/src/components/StatusTag.svelte`, statuses
  from `internal/core/domain/endpoint.go`). These domain statuses have no
  explicit entry in `StatusTag`'s STATUS map (per spec §4.4.1) and fall through
  to a neutral glyph with the raw status text as the label. The seeded ollamock
  fleet can only emit `healthy` and `offline`, so these intermediate states were
  never exercised in the WP-6 browser pass (spec §8: do not fabricate one).
  Untested, not broken. PR 2 should drive an endpoint into one of these states
  (or unit-test `StatusTag` directly) before claiming coverage. Severity: low
  (unlikely render path; the fallback is non-crashing).


### Build-pipeline review — pre-existing Windows parse-time dependencies (acknowledged, not planned)

- **`makefile:2` and `makefile:5` shell out to `awk`/`sed`/`date` at parse
  time.** `RUNTIME := $(shell go version | awk '{print $$3}' | sed
  's/go//')` and `DATE := $(shell date +%Y-%m-%dT%H:%M:%S%z)` are `:=`
  immediate assignments, so they execute on every `make` invocation
  regardless of target — including `make help` and `make ready` — not just
  build targets. This predates this branch (present on `main` before
  `feature/simple-dashboard`) and is unrelated to the two build-pipeline
  defects this branch's build-pipeline fix addressed (the `build-web` copy
  step and the missing `setup-node` step in `release.yml`). Also pre-existing
  in the same vein: `mkdir -p`, `rm -rf` and `uname -m` used in the
  cross-compile `validate-*`/`docker-build-local` targets. The maintainer has
  confirmed Git Bash is an accepted, supported prerequisite for `make` on
  Windows (they run `make ready` under it routinely), so none of this blocks
  native Windows development today and no work is planned against it. Recorded
  here only so a future contributor investigating Windows Makefile behaviour
  finds the answer already known rather than re-deriving it. Severity: none
  (acknowledged working-as-intended under the Git-Bash-required policy).

### Release pipeline — documented goreleaser Docker verification path lacks Node (and Go, and make)

- **The `docker run ... goreleaser/goreleaser:latest release --snapshot
  --clean` commands documented in `.goreleaser.yml:1-5`** run inside the
  official `goreleaser/goreleaser` image, which ships only the goreleaser
  binary and git on a minimal base — no Go toolchain, no `make`, and no
  Node/npm. `before.hooks` in the same file calls `make build-web`, so
  running that documented command as-is fails inside the container on three
  missing tools at once, not just Node. This is a pre-existing local-testing
  convenience comment, not the GitHub Actions release path (which runs
  goreleaser directly on the `ubuntu-latest` runner via
  `goreleaser-action`, now fixed to install Node explicitly - see
  `.github/workflows/release.yml`). Fixing the documented container path
  would mean building and maintaining a custom goreleaser image with Go,
  Node and make pinned, which is materially more than this branch's two
  build-pipeline defects. Logged for PR 2 or a dedicated release-tooling
  pass. Severity: low (local convenience command only; does not affect
  actual tagged releases).

### Post-review backend hardening pass — verified findings not fixed on this branch

- **Endpoint ordering is unstable for ties.** The comparators in
  `internal/app/handlers/handler_status_endpoints.go:76-81` and
  `internal/app/handlers/handler_status.go:300-305` sort by priority and
  health class but have no final name/URL tie-breaker, and their input comes
  from map iteration. Two endpoints with equal priority in the same health
  class (e.g. both healthy, same priority) can reorder between polls purely
  from map-iteration randomisation, even though nothing about the fleet
  changed. Verified byte-for-byte identical on `main`, so this predates this
  branch and is not something the dashboard work introduced. The spec's
  §4.4's FR-15 claim that these sorts satisfy "deterministic ordering" holds
  only in the coarse priority/health-class sense — it does not mean "stable
  across polls" for ties within a class. Severity: low (cosmetic table
  jitter in the dashboard between polls; no data-correctness impact).

- **Scoped IPv6 addresses cannot match configured CIDRs.**
  `internal/app/handlers/dashboard/access.go:107-112,133-134` passes the raw
  client IP (from `r.RemoteAddr`) straight to `net.ParseIP`, which returns
  `nil` for a zone-scoped literal like `fe80::1%eth0`. `net.ParseIP` rejects
  the `%zone` suffix outright. This fails closed: a legitimate link-local
  IPv6 client gets a self-diagnosing 403 (per `access.go`'s `reject()`), not
  a bypass of the access policy — so it is a usability gap, not a security
  hole. Fix would be stripping everything from `%` onward before calling
  `ParseIP` in both `ipInAnyCIDR` and wherever the client IP is first
  extracted. Not fixed here because it is a behavioural change to the access
  policy beyond this pass's four scoped defects. Severity: low (only affects
  operators reaching the dashboard over a link-local IPv6 address with a
  zone identifier, an uncommon deployment shape).

- **`{@html}` sinks in `StatTile.svelte` are inert today but are a latent XSS
  path.** All three `{@html}` call sites in
  `web/dashboard/src/components/StatTile.svelte` (via its `valueHtml`/
  `subHtml` props) are currently fed only closed, server-controlled enum
  values or operator-configured strings — never raw endpoint names, model
  names, or any other value that could originate from an untrusted or
  externally-influenced source. So there is no exploitable path today. A
  future contributor who pipes an endpoint name or model name through
  `valueHtml`/`subHtml` (e.g. to add inline formatting to a table cell)
  would introduce a real stored-XSS surface, since neither prop is escaped.
  Not fixed here because it requires no code change today — the risk is in
  how the component could be used, not how it is used — and locking down
  the prop contract (e.g. splitting a plain-text prop from a
  deliberately-unescaped one) is a `web/dashboard/` change outside this
  pass's backend-only scope. Severity: low today, escalating to medium the
  moment any call site starts passing endpoint/model-derived strings through
  it — flagged so a future PR checks this before doing so.

### OverviewPanel rowId fix follow-up — DOM ids still collide, only the each-key was fixed

- **`ep-<slug>` / `glance-<slug>` DOM ids are still lossy and can collide,
  making "jump to endpoint" resolve to the wrong row.**
  (`web/dashboard/src/panels/EndpointsPanel.svelte`'s `rowDomId`,
  `web/dashboard/src/panels/OverviewPanel.svelte`'s `rowDomId` and
  `jumpToEndpoints`, `web/dashboard/src/panels/ModelsPanel.svelte`'s
  `domId`). This branch fixed OverviewPanel's glance table using the same
  lossy `cssId()` slug as Svelte's each-block *key* (rowId), matching the
  fix already applied to `EndpointsPanel`/`ModelsPanel` in `eb14006` — that
  was the assigned defect and it is now fixed everywhere (verified by
  grepping every `rowId=`/`rowId(` call site in `web/dashboard/src/`; none
  remain lossy). But all three components still derive their DOM `id`
  attribute from that same slug, and DOM ids are not required to be
  unique the way each-keys are, so `document.getElementById` silently
  resolves to whichever row happened to render first when two names
  collide once slugged. Confirmed via test: with endpoints named
  `node.a` (priority 100) and `node-a` (priority 90), both render `id="ep-
  node-a"`; clicking "jump to endpoint" from the `node-a` glance row
  scrolls to and focuses `node.a`'s row instead, with no error and no
  visible indication anything went wrong. `App.jump-focus.test.js`'s new
  collision case exercises the jump from `node.a` specifically (which
  self-resolves correctly, since it renders first) to avoid encoding the
  wrong-target behaviour as a passing spec; the `node-a` case is left
  unasserted here as the documented gap. Fixing this properly needs a
  disambiguation strategy for the DOM id (e.g. an index suffix, or a
  DOM-safe hash of the exact name instead of the current char-strip slug)
  across all three components, which is broader than this branch's single
  assigned defect (the each-key fix). Severity: low-medium (only affects
  operators with two endpoint or model names that collide once
  punctuation is stripped, and the failure mode is a wrong jump target,
  not a crash or data-correctness issue elsewhere in the panel).

## Table column alignment: header and cell disagree on composite columns

Reported by the maintainer with screenshots of all three panels. One root
cause produces every symptom.

`web/dashboard/src/components.css:368-371` right-aligns columns flagged
`num: true`:

    thead th.num,
    td.num { text-align: right; }

`text-align` only positions INLINE content. The cells for Success, Latency
and Endpoints do not contain text, they contain a block/flex child (the
bar component, or a wrapped set of endpoint chips). Those children ignore
`text-align` entirely, so the header right-aligns and the cell content
stays hard left, leaving a wide dead gap between the two.

Affected columns, all flagged `num: true` while rendering non-numeric
content:

- `success_rate_num` (Endpoints, `EndpointsPanel.svelte:42`, cell at :117)
  and the equivalent Success column in Overview's "Herd at a glance"
- `avg_latency_ms` (Endpoints, :43, cell at :120) - the min/avg/max range bar
- `endpoints_count` (Models, `ModelsPanel.svelte:28`) - wrapped chips

Genuinely numeric columns are correct and should not be touched: Priority,
Models, Requests, Conn (`EndpointsPanel.svelte:116,129,130,131`) and Size
(`ModelsPanel.svelte:27`) all hold plain text, so `text-align: right`
works as intended and header and value line up.

Two fix directions, pick one and apply it consistently:

1. Stop flagging bar/chip columns as `num` and left-align their headers to
   match their content. Simplest and arguably most correct - a progress bar
   is not a number, and a right-aligned header over a left-anchored bar
   will always read as broken. Keeps `num` meaning "holds a numeral".
2. Keep `num` for sort purposes but give the cells a real alignment
   mechanism, e.g. `justify-content: flex-end` on the flex container inside
   `.num` cells, so the composite content actually moves right.

Note the two concerns are currently conflated: `num` is doing double duty
as "sort numerically" and "align right". Separating them (e.g. `num` for
sort, an explicit `align` field for presentation) would prevent this class
of mismatch recurring as columns are added.

Also reported alongside, same area, needs a decision rather than a
mechanical fix: Models' Endpoints column is over-wide, so even once
alignment is corrected the chips sit far from the header - it needs a width
constraint or a different treatment for models hosted on many endpoints.

Severity: cosmetic, no data-correctness impact, but it affects every row of
every table and reads as an unfinished UI. Cheap to fix once the alignment
model is decided.

# CONSOLIDATED PRE-MERGE LIST (spec input)

Two independent architect reviews plus two verification passes. Every item below was verified in code; scope calls note whether this branch caused it.

## Blocks the push

**B1. Proxy access logs suppressed at default level. REGRESSION, ours.**
`internal/app/middleware/logging.go:107` - `isQuietPollOutcome` opens with `if IsProxyRequest(path) { return true }`, unconditional on status. `AccessLoggingMiddleware` (~:312) now picks its level from that helper, so every proxy access record is Debug. On main the access log was unconditionally `slog.LevelInfo` (`git show main:...logging.go:239-240`). Main separately quiets proxy CONSOLE lines (~:130-138) - deliberate, pre-existing, must stay. This branch conflated the two. Note the helper's own doc comment (:98-105) promises 404/5xx are never swallowed; the proxy branch returns before any status check, so the code contradicts its comment.
Fix: split into a console variant (path-only) and an access-log variant (status-aware: proxy 2xx/3xx quiet, 4xx/5xx at Info).
Tests: NONE exist asserting access-log level for a proxy path at any status. `TestAccessLoggingMiddleware` (logging_test.go:74-109) only checks a 200 response body. Need table-driven: (proxy,200)->Debug, (proxy,500)->Info, (proxy,400)->Info, (/internal/health,200)->Debug, (/internal/health,404)->Info. `mockStyledLogger` already exists.

**B2. Exact-duplicate and empty endpoint names blank both tables.**
`EndpointsPanel.svelte:59-61` and `OverviewPanel.svelte:192` key on `row.name`. Two endpoints both named `ollama`, or two with empty names, throw `each_key_duplicate` as an uncaught exception inside a reactive effect - zero rows render, whole table blanks. Verified this throws in PRODUCTION builds too, not just dev (`svelte/src/internal/client/dom/blocks/each.js:351-357` still calls it in the non-DEV branch).
Reachable from valid config: neither `Config.Validate()` (`internal/config/config.go:191-237`) nor `validateEndpointConfig` (`internal/adapter/discovery/repository.go:299-343`) checks endpoint name emptiness or uniqueness.
Existing tests only cover slug-colliding but DISTINCT names (`node.a`/`node-a`); none use `ollama`/`ollama` or empty/empty.
Fix: key on `row.url`. Verified structurally unique - `repository.go:223` does `newEndpoints[urlString] = newEndpoint`, so duplicate URLs collapse before the frontend sees them, and `EndpointSummary.URL` is already in every row. Two lines. Recommend ALSO making `SortableTable` incapable of emitting a duplicate key (index disambiguation) so no future caller can reintroduce the crash.

**B3. Slug-colliding names jump to the WRONG endpoint.**
`OverviewPanel.svelte:93` resolves `ep-${cssId(name)}` via `getElementById`; `EndpointsPanel.svelte:62-67`, `OverviewPanel.svelte:193` and `ModelsPanel.svelte` all derive DOM ids from the same lossy slug. With `node.a` and `node-a` present, both rows get `id="ep-node-a"` and the lookup always returns the first in DOM order.
The existing test deliberately avoids the failing direction - its own comment (`App.jump-focus.test.js:126-134`) says jumping from `node-a` "would land on node.a's row instead, silently" and declines to assert it. Honestly logged, but the wrong-target behaviour has zero coverage.
Fix: same pattern as the each-key fix - derive the DOM id from the exact value (or an index disambiguator) across all three components, and flip the avoided assertion to cover the failing direction.

## Cheap, do now

- **C1.** `sanitiseDisplayURL` (`handler_status_endpoints.go:189-202`) ends `if err != nil { return raw }`, returning credentials verbatim when `url.Parse` fails (e.g. a space in the password). Config-load validation only catches URLs that parse. Return a sentinel. Single caller, nothing depends on the fail-open behaviour.
- **C2.** `GateInternalAPI` is inert. Correction to earlier framing: it IS documented as inert in `types.go:143-149`, `config.go:169` and two tests - the gap is operator-facing only, since nothing signals at startup. Add a `slog.Warn` when set true (same pattern as `notBuiltWarn`), keep the field so PR 2 needs no migration. Do not delete, do not hard-fail.
- **C3.** Security headers only on the success path. `setSecurityHeaders` (`dashboard/embed.go:254-259`) is called from the 200 path (:187) and `serveIndex` (:209) only - not the 405 (:125), the two `http.NotFound` branches (:142, :157), or `notBuiltHandler`'s 503 (:114). `access.go`'s `reject` (:152-158) sets only `X-Content-Type-Options`. No `Referrer-Policy` anywhere. Set headers once before any response is written, and add `Referrer-Policy: no-referrer`.
- **C4.** Remove `{@html}` / `valueHtml` / `subHtml` from `StatTile.svelte:7-19`. No live XSS (all three callers pass an enum, config values or an integer) but it is an opt-in footgun on a shared component. Verified contained: three call sites in `OverviewPanel`, no ripple into tile layout CSS. Express as snippets instead.
- **C5.** Sort comparators lack a tie-breaker (`handler_status_endpoints.go:76-81`, `handler_status.go:300-305`, both byte-identical to main). Equal-priority same-health endpoints reorder between polls; the dashboard makes the jitter visible. Append a name/URL comparison.
- **C6.** StatusStrip has no staleness indicator - it reads `overview.data` without consulting `overview.status`, so during an outage it shows confident numbers while the panel below says unreachable. The `data-state` pattern already exists (`EndpointsPanel.svelte:84`) to copy.
- **C7.** Offline endpoints render a full green success bar. `PctBar` derives from `success_rate_num`/`hasData` with no reference to `e.status`, so lifetime counters sit beside a red offline pill. Gate the bar on status.
- **C8.** Duplicate type badge - `EndpointsPanel.svelte:112` renders a `badge-type` span inside the name cell and :115 renders a Type column, both showing `e.type`. Drop one.
- **C9.** `.goreleaser.yml:1-6` documents a `docker run goreleaser/goreleaser` command that cannot work: the image has no Node, no make, and the `before.hooks` now run `make build-web`. Doc comment only - the real tagged-release path IS correctly fixed (`release.yml:33-36` pins `setup-node` before goreleaser).
- **C10.** Table column alignment (see the detailed section above). `td.num { text-align: right }` only moves inline content, so bar and chip cells ignore it while their headers obey. Affects Success and Latency on Endpoints and Overview, and Endpoints on Models. Separate `num` (sort) from alignment (presentation) so it cannot recur.

## Defer to PR 2

- **D1.** Zero-traffic fleets report status `critical`. `handler_status.go:199` leaves success rate at 0 when `TotalRequests == 0` and `:206` classifies below 90% as critical, so a fresh healthy install opens red. Verified byte-for-byte identical to main - pre-existing, and the fix touches shared classification code that `/internal/status` consumers may alert on. Strong PR 2 opener; too much blast radius to rush pre-push.
- **D2.** Wire upstream 5xx into stats and the breaker. One change unlocks an honest success rate, the `circuit_breaker` column and issue #144 together. Until then the success-rate tile reads 100% against an all-500 fleet (`proxy/core/base.go:83` counts any streamed response as success). PR 1 should relabel the tile so it does not overclaim.
- **D3.** Zone-scoped IPv6 (`fe80::1%eth0`) fails CIDR matching - `access.go:133-144` passes it to `net.ParseIP`, which rejects zones. Fail-CLOSED (legitimate client denied), narrow, not a bypass.
- **D4.** Overview tile IA: nine tiles of which five duplicate the status strip directly above, and at desktop width THREE empty grid cells (not one) - `repeat(4, 1fr)` with 9 tiles. Content decision, not a defect.
- **D5.** Endpoints table is 12 columns with a horizontal scrollbar at 1280px. `NEXT CHK` reads "now" on every row; `fmtUntil` (`lib/format.js:122-128`) returns "now" for any past timestamp, so the frontend is consistent with the claim, but whether `next_check_at` genuinely lags the health checker was NOT traced end to end - needs a backend look before deciding which layer to fix.
- **D6.** Per-endpoint last error and per-endpoint model list. Rated by review as worth more than half of what is currently displayed: during an incident, "connection refused" vs "503 upstream" vs "probe timeout" is the entire question.
- **D7.** Mobile: Endpoints shows 2 of 12 columns at 390px. Overview degrades well; Endpoints needs a card layout.
- **D8.** Sticky sessions are a shipped feature with no dashboard representation at all.
- **D9.** Rate limiting on `/internal/*` (none today, and JSON ETags were deliberately cut), real `gate_internal_api` wiring, and a startup warning on non-private CIDRs.

## Process notes

- `make ready` does not run `ci-web`, so a frontend regression passes the local pre-commit gate and only `make ci` catches it. Defensible (keeps `ready` Node-free) but CI must be the enforcing gate.
- `docs/spec/` is in `.gitignore`; files here need `git add -f`. A fresh agent may silently fail to commit into this directory.
- `betteralign` runs at v0.11.0 against a v0.8.2 pin and rewrites struct field order repo-wide during `make ready`. Pre-existing; pin later.
- Concurrent agents need a no-`git stash` rule as well as file ownership - stash is repo-global, and one agent stashed while peers had uncommitted work.
- Allowlist edits by an implementing agent should be a human-review flag: an agent can make its own diff pass by widening the gate.
