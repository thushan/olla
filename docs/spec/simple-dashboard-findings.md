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

- **`parseTimeAgoOptimised` is a coarse heuristic**
  (`internal/app/handlers/handler_status_models.go`). Pre-existing: it buckets
  relative-time strings (e.g. every "N seconds" maps to `now-30s`, every
  "N minutes" with single-digit N uses only the first character). WP-2 added a
  name-based tiebreaker to `getRecentModels` so observable ordering is now
  deterministic (FR-15), but the underlying precision loss in the comparator
  remains. Replacing it with a real `time.Time` comparison would require
  threading the absolute timestamp through `ModelSummary` as the sort key, which
  is a refactor beyond the additive brief. Severity: low (cosmetic, only
  affects recent-models ordering within a coarse time bucket).

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

