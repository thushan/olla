# Simple dashboard: pre-merge remediation

Branch: `feature/simple-dashboard`. This spec covers the fixes required before PR 1
can be pushed. It is a companion to `docs/spec/simple-dashboard-findings.md`, which
holds the verified detail for every item referenced here. Read that file first: it
has file:line evidence, reproduction steps and fix shapes for all of them.

## Context

PR 1 is a lean, read-only admin dashboard: Svelte 5 + Tailwind, embedded via
`go:embed`, served at `/internal/ui/` on the existing proxy listener, no auth,
loopback-gated by a per-request check. Three panels: Overview, Endpoints, Models.
Polled, not realtime.

A predecessor attempt at this feature reached 151 commits and was abandoned as
unreviewable. It began as a dashboard and accreted four successive rewrites of the
circuit breaker, request-accounting changes across both proxy engines, and
streaming-path changes. Every individual step was defensible: verification found a
real adjacent bug, it was fixed in place, that fix needed its own verification,
repeat. This branch is the disciplined redo and currently sits at 39 commits. Two
independent architect reviews and four verification passes have produced the
findings file. Keep the discipline.

## Governing rules

**KISSAI.** The leanest change that fixes the item. No refactoring adjacent code
because it looks improvable, no speculative generality, no cleverness.

**Scope is mechanical, not a judgement call.** Run this at every work-package
review and again before declaring the branch ready. It must return empty:

```
git diff --name-only main...HEAD | grep -vE '^(web/dashboard/|internal/app/handlers/dashboard/|internal/app/handlers/handler_status.*\.go$|internal/app/handlers/handler_routes_dashboard_test\.go$|internal/app/handlers/server_routes\.go$|internal/app/middleware/logging.*\.go$|internal/config/types\.go$|internal/config/config\.go$|internal/config/.*_test\.go$|internal/adapter/discovery/repository.*\.go$|config/config\.yaml$|test/config_docs/|makefile$|\.github/workflows/(ci|release)\.yml$|\.goreleaser\.yml$|\.gitignore$|docs/)'
```

If a fix genuinely needs a path outside that list, STOP and escalate to the user.
Do not widen the allowlist yourself: an agent that edits its own gate has no gate.

**Implement B1-B3 and C1-C10 from the findings file. Nothing else.**
D1-D9 in that file are deliberately deferred to PR 2 and MUST NOT be touched, even
though they are described in enough detail to look actionable. Any NEW problem
discovered during this work is appended to the findings file, not fixed.

**Every regression test must fail before the fix and pass after.** Confirm it
explicitly and report the pre-fix failure output. This branch has already produced
two tests that passed against the broken code, one of which documented the bug in a
comment rather than asserting it. A test shaped to avoid its own defect is worse
than no test.

**Two strikes.** A second attempt at the same fix is an escalation to the user, not
a third try. This overrides the orchestrate skill's own iteration cap.

**Concurrency.** Assign each work package a disjoint set of files and state the
ownership in the dispatch. No agent runs `git stash` - it is repo-global and will
disturb peers holding uncommitted work.

**Do not merge. Do not push.** Stop at "branch is ready and verified" and report.

## Work packages

Ordering: WP1 first (it is the blocker and touches middleware nothing else needs).
WP2-WP5 may run in parallel given the file ownership below. WP6 and WP7 are last
and are the orchestrator's own work.

### WP1 - Access log regression (BLOCKER)
Agent: `go-principal-architect`. Owns `internal/app/middleware/logging.go` and its tests.

Findings item B1. Split the quiet-classification into a console variant (path-only,
preserving main's deliberate pre-existing proxy quieting) and an access-log variant
that is status-aware. Proxy 2xx/3xx quiet; proxy 4xx/5xx at Info.

Acceptance: table-driven tests asserting access-log level for (proxy, 200) Debug,
(proxy, 500) Info, (proxy, 400) Info, (`/internal/health`, 200) Debug,
(`/internal/health`, 404) Info, and a non-proxy non-internal path always Info.
`mockStyledLogger` already exists in that test file. Console behaviour unchanged
from main - diff it to prove that. `make ready` green.

### WP2 - Security hardening
Agent: `go-principal-architect`. Owns `internal/app/handlers/handler_status_endpoints.go`,
`internal/app/handlers/dashboard/embed.go`, `internal/app/handlers/dashboard/access.go`,
`internal/config/config.go`, `internal/config/types.go`.

Findings items C1, C2, C3.
- C1: `sanitiseDisplayURL` must fail closed. Return a sentinel on parse error, never
  the raw input. One caller; nothing depends on the current behaviour.
- C2: add a startup `slog.Warn` when `GateInternalAPI` is set true, matching the
  `notBuiltWarn` pattern. Keep the field. Do not delete it, do not hard-fail.
- C3: security headers must be set before any response is written, covering the 405,
  both `http.NotFound` branches, the not-built 503 and the access-policy 403. Add
  `Referrer-Policy: no-referrer`.

Acceptance: a test proving a credentialed unparseable URL never appears in JSON;
tests asserting headers on an error path and on the 403; `make ready` green.

### WP3 - Row identity and jump targets
Agent: `svelte-expert-developer`. Owns `web/dashboard/src/panels/EndpointsPanel.svelte`,
`web/dashboard/src/panels/OverviewPanel.svelte`, `web/dashboard/src/panels/ModelsPanel.svelte`,
`web/dashboard/src/components/SortableTable.svelte` and their tests.

Findings items B2, B3.
- B2: key rows on `row.url`, which is structurally unique (the backend keys its
  endpoint map by URL, so duplicates collapse before the frontend sees them). Also
  make `SortableTable` incapable of emitting a duplicate key regardless of what a
  caller passes.
- B3: derive DOM ids from the exact value or an index disambiguator, across all
  three panels, so `getElementById` cannot resolve the wrong row.

Acceptance: tests covering two endpoints both named `ollama`, two with empty names,
and the `node.a`/`node-a` slug pair - all render every row. The existing
`App.jump-focus.test.js` case that deliberately avoids the failing direction is
flipped to assert jumping from `node-a` lands on `node-a`. All must fail pre-fix.

### WP4 - UI honesty
Agent: `svelte-expert-developer`. Owns `web/dashboard/src/layout/StatusStrip.svelte`,
`web/dashboard/src/components/PctBar.svelte`, `web/dashboard/src/components/StatTile.svelte`.

Findings items C4, C6, C7, plus the D2 tile relabel.
- C4: remove `{@html}`, `valueHtml` and `subHtml` from `StatTile`; express the same
  markup as snippets. Verified contained to three call sites with no layout ripple.
- C6: StatusStrip must reflect staleness. It currently reads `overview.data` without
  consulting `overview.status`, so it shows confident numbers during an outage. Reuse
  the `data-state` pattern the panels already use.
- C7: gate the success bar on endpoint status so an offline endpoint stops rendering
  a full green bar beside its red pill.
- **From D2, and easy to miss because it sits inside a deferred item:** relabel or
  footnote the success-rate tile so it does not present an unqualified percentage.
  The underlying metric counts any streamed response as success regardless of HTTP
  status, so an all-500 fleet currently reads 100%. Fixing the metric is PR 2; making
  the label honest is PR 1.

Acceptance: tests for the stale state and the offline bar; no `{@html}` remains in
the component tree; `npm test -- --run` and `npx svelte-check` clean.

### WP5 - Presentation and docs
Agent: `svelte-expert-developer` for the frontend items, `docs-writer` for C9.
Owns `web/dashboard/src/components.css`, the column definitions in the three panels,
and `.goreleaser.yml`.

Findings items C10, C8, C9, C5.
- C10: fix the column alignment mismatch. `td.num { text-align: right }` only moves
  inline content, so bar and chip cells ignore it while their headers obey. Separate
  `num` (sort behaviour) from alignment (presentation) so the mismatch cannot recur
  as columns are added. Also constrain the over-wide Endpoints column on Models.
- C8: drop the duplicate type badge.
- C9: correct the `.goreleaser.yml` doc comment - the documented Docker command
  cannot run `make build-web`. The real release path is already fixed.
- C5: add a name/URL tie-breaker to both sort comparators
  (`handler_status_endpoints.go`, `handler_status.go`) so equal-priority endpoints
  stop reordering between polls. This one is Go - assign it to
  `go-principal-architect` and note it touches files WP2 also owns, so serialise
  those two or fold C5 into WP2.

Acceptance: alignment verified visually in the WP6 browser pass, not just by reading
CSS; `make ready` and the frontend suite green.

### WP6 - Verification (orchestrator runs this personally)

Do not delegate the decision to accept. Boot the seeded fleet, drive traffic, verify
in a browser, and report evidence.

- Build, then run Olla against mock backends. The repo has `test/cmd/ollamock` and
  `test/cmd/mockbackend`, and an `olla-validate` skill that boots an ollamock fleet -
  read it for the established pattern rather than inventing one.
- Seed at least 4 backends with overlapping models, one offline, **plus two endpoints
  named `ollama` and `ollama`, and two named `node.a` and `node-a`** - those fixtures
  are what B2 and B3 are about.
- Drive real traffic so counters move, and leave it running several minutes so it
  reaches steady state. A fresh instance hides bugs a settled one shows.
- Then use `web-verifier` for the browser assertions.

Required assertions: every panel renders with the collision fixtures present and no
`each_key_duplicate` in the console; jumping from `node-a` focuses `node-a`; column
headers align with their cell content in all three panels (check computed styles, not
just appearance); the status strip shows a stale indicator when the backend is killed;
an offline endpoint does not show a green success bar; security headers present on a
404 and on a 403; no console errors; no horizontal overflow at 375/768/1440.

### WP7 - Untrack the specs, then final review
Agent: orchestrator, then `code-reviewer`.

**The spec files must not ship.** `docs/spec/` is in `.gitignore` and these files were
force-added. Before declaring ready:

```
git rm --cached docs/spec/simple-dashboard.md docs/spec/simple-dashboard-findings.md docs/spec/simple-dashboard-remediation.md
```

Commit that removal. The files stay on disk for reference; they leave the branch diff.
Then confirm `git diff --name-only main...HEAD | grep '^docs/spec/'` returns empty.

Finally, a `code-reviewer` pass over the full diff against this spec's acceptance
criteria. Any in-scope finding goes back to the originating specialist (subject to two
strikes); anything out of scope is appended to the findings file.

## Out of scope, explicitly

- D1-D9 in the findings file.
- Any change under `internal/adapter/proxy/` or `internal/adapter/health/`. Zero files.
  Verify with `git diff --name-only main...HEAD -- internal/adapter/proxy/ internal/adapter/health/`
  returning empty.
- Any change to Sherpa.
- `internal/app/handlers/application.go` and `internal/app/services/http.go` must stay
  byte-identical to main. Verify the same way.
- The pre-existing Makefile portability gap. Git Bash is a supported prerequisite.
- Pinning `betteralign`. Logged, deliberately deferred.

## Definition of done

`make ready` green. `make ci-web` green from a clean checkout with no `node_modules`.
Frontend suite and `svelte-check` clean. Scope gate empty. Proxy, health,
`application.go` and `services/http.go` all untouched. No `docs/spec/` files in the
diff. WP6 evidence captured. Findings file updated with anything new. Branch not
merged, not pushed.
