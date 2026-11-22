# Go 1.27 modernisation (refactor spec)

Branch: `chore/go-1.27-modernise`, cut from main at 9238e1f4. Measurements below were taken at that commit.

## 1. Purpose and scope

Olla builds with Go 1.27 (`go 1.27.0`, `toolchain go1.27.4`, golangci-lint v2.15.0 from the repo-local tool directory). Most of the code was written for Go 1.21 to 1.24 and predates the idioms 1.22 to 1.27 made standard. This spec adopts them in single-category PRs, mostly by tool, and then enables the `modernize` linter so the backlog cannot regrow.

**This spec authorises no behaviour change.** A diff that changes output, ordering, the JSON wire format or an exported `pkg/` signature is out of scope for the PR that contains it. This is not a `/debt` item, so there is no register entry to pay down. Test edits are allowed only where a retyped field or key forces them (see FR-17).

### Measurement that changes

`golangci-lint run` caps output at 50 issues per linter and 3 identical messages, and dedupes by line, so a default run hides most sites. **Every count command in this spec uses the uncapped flags**, named `LINTFLAGS` below:

`LINTFLAGS="--max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false"`

Counts are taken by analyzer (from the issue text), and a package-load error counts as a failure, not as zero.

| Measure | Command (repo root) | Before | After |
|---|---|---|---|
| modernize findings, whole repo | `./.bin/golangci-lint run --enable-only modernize $LINTFLAGS ./...` | 1,416 (339 non-test) | 0 apart from the analyzers on the disable list (FR-12) |
| `go fix` pending hunks | `go fix -diff ./...` | 662 hunks, 177 files (before `waitgroupgo`, which adds 50 hunks in 26 files) | only the disable-list analyzers remain |
| usetesting, context and setenv, in tests | `./.bin/golangci-lint run --enable-only usetesting $LINTFLAGS ./...` with the `_test.go` exclusion for `usetesting` removed and `context-background: true`, `context-todo: true` set | 722 context sites plus 9 `os.Setenv` sites | 0 |
| SA1029 | `./.bin/golangci-lint run --enable-only staticcheck $LINTFLAGS ./...` with `-SA1029` removed | suppressed | 0 findings, suppression gone |

Per analyzer (lint total / non-test): `any` 1,164/254; `newexpr` 98/2; `waitgroupgo` 35/2; `errorsastype` 32/28; `slicescontains` 20/11; `mapsloop` 18/9; `minmax` 11/11; `embedlit` 11/0; `stringsseq` 6/5; `stringscutprefix` 6/4; `atomictypes` 5/5; `stringscut` 4/2; `reflecttypefor` 2/2; `stringsbuilder`, `stditerators`, `slicessort`, `omitzero` 1 each. `go fix` hunks (26 analyzers on go1.27.4): `any` 432; `newexpr` 51; `slicescontains` 37; `mapsloop` 28; `errorsastype` 26; `atomictypes` 18; `minmax` 8; `embedlit` 7; `stringsseq` 6; `stringscutprefix` 4; `stringsbuilder` 4; `stringscut` 3; `testingcontext` 3; `forvar` 2; `reflecttypefor` 2; `slicessort` 2; `omitzero` 1; `stditerators` 1; `waitgroupgo` 50 hunks over 26 files. The research counts were measured with the default caps in some runs; implementers re-measure with `LINTFLAGS` at each PR's base (A1).

### Out of scope (deferred, not requirements)

- Swapping json-iterator or `encoding/json` for `encoding/json/v2`.
- An `omitempty` to `omitzero` sweep (a wire-format change).
- `b.Loop` and `testing/synctest` sweeps. synctest is only for a specific test the user nominates later.
- Range-over-func iterators, `unique`, `weak` and ServeMux pattern adoption as a sweep.
- `Server.MaxHeaderValueCount` adoption (a separate security decision).
- Any Sherpa feature work.
- Changes to `web/` or user docs, including the stale snippets listed in section 7.
- **Go 1.27 runtime behaviour checks** (JSON decode options, `Response.Body` drain on close). They belong to the toolchain change, which already shipped in v0.2.0 and which no PR here alters, so a validation run cannot attribute anything to this series. A separate toolchain item if wanted.
- Deleting or merging dead and duplicate context keys (Q3).
- Follow-ups from the last release (query-string API keys in logs, makefile tool-gover with spaces in paths and others): tracked elsewhere.

## 2. Decisions

- **D1 (settled): PR split and order.** PR 1, 2, 3a to 3d, 4, 5, 6 as in section 8. One category per PR so a reviewer can accept it by skimming.
- **D2 (default, user may overrule): benchmark before and after for each PR that touches a hot path** (PR 2, 3d, 4), scoped to the touched packages. Before and after go in the PR body. See NFR-1 for the method. `benchstat` is used only if already present; never installed.
- **D3 (default, user may overrule): analyzers that stay off.** `omitzero` (a JSON wire-format change) and `stringsbuilder` (not a category in the split) go on the `settings.modernize.disable` list. `slicesdelete`, `appendclipped` and `bloop` are not valid `disable` names in golangci-lint v2.15.0 (`config verify` rejects them; `run` silently ignores them; CI sets `verify: false`), and they are already off upstream. They are recorded in a comment, not in the list. `slicesclip` is added to the list only if the nil-preservation concern applies at a site found during PR 2 or 3d.
- **D5 (settled, no work):** CLAUDE.md already states Go 1.27; nothing uses `httputil.ReverseProxy` or `Director`; container-aware GOMAXPROCS and the Green Tea GC need no action.
- **D6 (settled):** implementation by Sonnet agents; the user merges via GitHub; reviews are Fable (or Opus) plus Codex, never Sonnet.

(D4, the Go 1.27 runtime behaviour verification item, is removed and deferred; see out of scope.)

## 3. Functional requirements

"Zero findings" means the analyzer's count from the `LINTFLAGS` commands is 0 after the package.

- **FR-1 (PR 1).** Every `interface{}` type is rewritten to `any` using `go fix -any ./...`. Defined over Go types, not text: `interface{}` inside comments or strings may remain. The change is a pure rename. `any` findings go to 0.
- **FR-2 (PR 2).** The analyzers `slicescontains`, `mapsloop`, `minmax`, `stringscut`, `stringscutprefix`, `stringsseq`, `errorsastype`, `reflecttypefor`, `stditerators`, `slicessort`, `embedlit`, `forvar` and `testingcontext` are applied with `go fix -<name> ./...`, one analyzer per commit, each commit reproducible by re-running that command on its parent (plus the formatter delta, FR-18). Each analyzer's finding count goes to 0.
- **FR-3 (PR 2).** `atomictypes`, `newexpr`, `omitzero`, `stringsbuilder` and `waitgroupgo` are not applied in PR 2. `stringsbuilder` stays out of every PR (A3).
- **FR-4 (PR 3a).** `sync.WaitGroup.Go` replaces `wg.Add(1)` plus `go func(){ defer wg.Done() ... }()` at the `waitgroupgo` sites (35 lint findings, 2 non-test), applied with `go fix -waitgroupgo ./...`, like FR-2. golangci-lint `--fix` is never used for this: it cannot be limited to one analyzer from the command line and would also apply `omitzero` to the deferred JSON tag on the profile config's `AuthHint`. Findings go to 0.
- **FR-5 (PR 3b).** In the lint config, the `usetesting` exclusion for `_test.go` files is lifted and `usetesting` settings `context-background: true` and `context-todo: true` are set (both default to false in v2.15.0; lifting the exclusion alone yields only the 9 `os.Setenv` findings). The 722 context sites convert to `t.Context()` or `b.Context()` (benchmarks need `b.Context()`), `os.Setenv` in tests becomes `t.Setenv`, and the remaining `tc := tc` copies are removed. Findings go to 0.
- **FR-6 (PR 3b).** A converted context used inside a `t.Cleanup` callback is not left as `t.Context()`, because it is cancelled before cleanups run. Where teardown uses the test context (for example the unified memory registry removal test calling `Stop`), use `context.WithoutCancel(t.Context())`, with a one-line comment saying why.
- **FR-7 (PR 3c).** The pointer helpers are replaced by `new(expr)` at their call sites and the helpers are deleted. There are seven: `ptrInt` (config, non-test), and the test helpers `ptrInt`, `strPtr`, `ptrString`, `ptrTime`, `int64Ptr` (converter tests and unifier integration tests) and `ptrInt64` (unifier metadata tests). Where `*int64` (or another non-default type) is required, an untyped literal takes an explicit type: `new(int64(4096))`, not `new(4096)`. `go fix -newexpr` adds `//go:fix inline` directives to the wrappers; the wrappers are deleted after inlining (the `unused` linter flags them) and no directive is left behind. `newexpr` findings go to 0.
- **FR-8 (PR 3d).** `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` and `sort.Sort` calls (27 non-test sites, 17 in handlers) become `slices.Sort`, `slices.SortFunc` or `slices.SortStableFunc`. Rules:
  - `sort.SliceStable` maps to `slices.SortStableFunc`.
  - Where the sorted input comes from map iteration and the key can tie (the stats models and translators handlers, the model collector eviction), tie order is already undefined: carry the comparator over unchanged, with no stable variant added.
  - Use a stable variant only where the input order is deterministic and a test pins tie order.
  - Comparators over timestamps use `time.Time.Compare`, not `cmp.Compare`.
- **FR-9 (PR 3d).** The priority balancer sort (per-request path) keeps `sort.Slice` semantics: with more than 12 mixed-tier endpoints `sort.Slice` can reorder ties, and the weighted draw then picks a different backend. It is converted only if a fixed-input comparison test over more than 12 mixed-tier endpoints shows identical order before and after. Otherwise it stays `sort.Slice` with an explanatory comment and is a recorded exception (FR-19).
- **FR-10 (PR 4).** Counters accessed through `atomic.Add`, `Load`, `Store`, `CompareAndSwap` and `Swap` (int32, int64 and uint64 variants) move to `atomic.Int32`, `atomic.Int64` or `atomic.Uint64` in the stats collectors (collector, model collector, endpoint data, including `minLatency`, `lastUsed` and `lastCleanup`), the proxy core counters (including `consecutiveErrors`, which is int32 and becomes `atomic.Int32`) and the round-robin balancer counter. All call sites of a field move together in one commit per package. `go fix -atomictypes` converts only part of the stats collectors (18 hunks, 3 files); the rest is hand work.
- **FR-11 (PR 4).** Snapshot and result structs, and anything serialised or returned to callers, remain plain integer types; only live counter fields change type. No struct holding an atomic type is copied, compared or returned by value (`go vet` copylocks clean).
- **FR-12 (PR 5).** `modernize` is enabled in the lint config with every analyzer on by default and an explicit `settings.modernize.disable` list of exactly `omitzero` and `stringsbuilder` (plus `slicesclip` only per D3), each with a reason comment. A further comment records that `slicesdelete`, `appendclipped` and `bloop` are already off upstream. PR 5 lands only after PRs 1, 2, 3a, 3c, 3d and 4.
- **FR-13 (PR 6).** Every context key is classified by role (section 6 lists them). Keys used for context storage that are untyped strings become the typed `contextKey` the sticky keys already use. Where a constant also serves another role (slog key, InspectionMeta key, path-prefix default), that role keeps a separate untyped string constant. The SA1029 exclusion in the lint config is removed.
- **FR-14 (PR 6).** Accessors extend the existing ones (the middleware package's own `contextKey` and `GetLogger`, `GetRequestID`); they are not duplicated. An accessor taking or returning a type that lives outside the constants package (`core.CanonicalModel`, the sticky outcome) sits beside that type, because placing it in the constants package would create an import cycle. No site reads or writes a context value by raw string, including the proxy handler (which today bypasses `GetRequestID`), the logger package's raw detailed-cookie key and about 29 test-side writes (one with the literal `"stream"` in the streaming test).
- **FR-15 (all).** Public API in `pkg/` (container, eventbus, pool, format, nerdstats, profiler) is unchanged; all `pkg/` findings are test-only. Sherpa receives mechanical rewrites only. Proxy core's `ProxyStats` has exported counter fields; FR-10 must not change their exported types (see section 6).
- **FR-16 (all).** No dependency is added. Tools come from the repo-local tool directory: a fresh checkout installs the pinned tools into that directory through the makefile's ensure targets, which is permitted. Any install outside that directory, or into the shared GOPATH bin, is forbidden.
- **FR-17 (all).** Test assertions are unchanged. Test setup and field access change only where a retyped field or key requires it (the round-robin overflow test that assigns the counter directly, the collector test rewritten by `atomictypes`, the test-side context-key writers in PR 6), and each such edit is listed in the PR body.
- **FR-18 (all).** A commit may carry a formatter delta. `go fix -slicescontains` adds a second `import "slices"` declaration in two domain files; goimports (the configured formatter) merges them and `go fmt` does not. The delta comes from `./.bin/golangci-lint fmt`, in the same commit as the change.
- **FR-19 (all).** A deliberately kept site (the priority sort, FR-9; an eventbus test per WP-2) must clear both tools: it is not a modernize finding, or it carries `//nolint:modernize // <reason>`.

## 4. Non-functional requirements

- **NFR-1.** Benchmarked PRs (2, 3d, 4) each take their own baseline at their parent commit, in the same session as their after-run (no shared baseline). Each side uses `-count=6` with `-run '^$'`, compared with `benchstat` if present (never installed), otherwise by medians. A regression is a statistically significant slowdown of more than 5% on a named row (5% sits above typical shared-box noise). The benchmark fixture is identical across the compared revisions. Each PR body names the benchmark function that exercises each changed site.
- **NFR-2.** Struct layout changes pass `make align-check` (not `make align`, which applies fixes) with no suppression added.
- **NFR-3.** PRs touching concurrency (3a, 4) pass `make test-race`.
- **NFR-4.** Heavy commands run in the capped slice, one heavy suite at a time across all agents.
- **NFR-5.** Commits are humanised one-liners, one coherent change each, targeted package tests per commit, `make ready` once on each PR's final commit. `make ready` mutates (`lint --fix`, `align -apply`), so after it, `git status --porcelain` must be empty.

## 5. Existing capability and reuse

This spec adds no new component. It uses existing tools and extends existing configuration.

| Need | Existing component | Extension and effect on existing callers |
|---|---|---|
| Mechanical rewrites, including WaitGroup.Go | `go fix` (26 analyzers on go1.27.4, `waitgroupgo` included); `go fix -diff` for read-only checks; `go tool fix help` lists them | Used as is. No callers affected. |
| Lint guard | Lint config (`linters.enable`, `settings`, `exclusions`); `intrange`, `copyloopvar`, `usetesting` already enabled | PR 5 adds `modernize` and its disable list. CI runs the same linter version through the action, so it applies there with no workflow edit. PR 3b removes `usetesting` from the `_test.go` exclusion rule only and sets its two context settings (other linters in that rule stay excluded). PR 6 removes `-SA1029`. |
| Benchmarks | makefile targets `bench`, `bench-balancer`, `bench-repo`; package benchmarks in stats, proxy core, balancer, handlers | Targets are not enough on their own: `bench-balancer` filters out `BenchmarkLargeEndpointSet`, the stats percentile benchmark times `Add` while the sort is in `GetPercentiles` under the lock, and proxy core has no retry-classification benchmark. See WP-2, 3d and 4 for the benchmarks added or named. No makefile change. |
| Context keys | The sticky-session keys, `ContextModelKey`, `EndpointPathMap`, `CanonicalModel` and `InputTokens` are already typed; the middleware package owns `contextKey` with `GetLogger` and `GetRequestID` | PR 6 extends the typed pattern to the seven untyped keys and extends the existing accessors. Retyping the shared constants in place is the near miss: they double as slog keys, InspectionMeta keys and the proxy `Configuration`'s default path prefix. |
| Pointer helpers | seven helpers (FR-7) | Removed. `new(expr)` is a language feature. |
| Discovery metrics | HTTP discovery client's `DiscoveryMetrics`, a live counter struct whose copy `GetMetrics()` returns, written only under the client's mutex | Atomics are redundant there. Fields become plain int64 under the existing mutex (FR-10 scope, section 6). Atomic fields would break FR-11 and copylocks, because the struct is returned by value. |
| End-to-end proof | `/olla-validate --quick` | Used unchanged for PR 2, 4 and 6. |
| Test suites that cover the seams | `make ready`; package suites for stats, proxy core, balancer, discovery, registry, handlers, middleware, router, logger, config, unifier | Assertions pass unchanged; setup and field access change only per FR-17. |

## 6. Data and interfaces

No schema, API, config-key or wire-format change. Go-internal interfaces touched:

- **PR 4 counters.** Live counter fields change to atomic types: stats collectors, proxy core counters, round-robin counter. Discovery metrics move to plain int64 fields under the existing mutex (no atomics, no type change visible to callers). Proxy core's `ProxyStats` counter fields are exported: they stay plain integer types, and the live atomic counters are separate unexported fields copied into `ProxyStats` on read.
- **PR 6 context keys.** The shape of the decision:

```go
type contextKey string // storage role only; never used as a slog or metadata key
```

Key inventory (roles to be confirmed by the implementer against the code; the characterisation commit pins them):

| Key | Today | Roles to preserve |
|---|---|---|
| `route_prefix` | untyped | context storage; the proxy `Configuration`'s `GetProxyPrefix` default path prefix (URL building's `StripPrefix`), which must stay the literal `"route_prefix"`; `util.StripRoutePrefix` lookup |
| `request_id` | untyped | context storage; slog key. A second request-ID key exists in the middleware package. |
| `request_time` | untyped | written, never read (Q3) |
| `original_path` | untyped | never used (Q3) |
| `stream` | untyped | read in streaming with no production writer; test writers use the literal `"stream"` |
| `provider_type` | untyped | written, never read (Q3) |
| `model_alias_map` | untyped | context storage |
| `ContextModelKey`, three sticky keys, `EndpointPathMap`, `CanonicalModel`, `InputTokens` | already typed | no key change; add accessors where readers use the raw key |

Accessors take and return `context.Context` and the value type, so call sites stop naming keys. Accessors for `core.CanonicalModel` and the sticky outcome sit beside those types.

- The `pkg/` exported surface is unchanged (FR-15).

## 7. Consequences

| What it is | What in this spec changes it | Handling |
|---|---|---|
| Lint config: `modernize` enable plus disable list and comments | FR-12 | PR 5 |
| Lint config: `usetesting` in the `_test.go` exclusion rule, and its `context-background`/`context-todo` settings | FR-5 | PR 3b |
| Lint config: `-SA1029` and its comment saying the constants are plain strings on purpose | FR-13 | PR 6 (same edit) |
| CLAUDE.md Development Guidelines | FR-12 | PR 5 adds one line, "modernize linter enforced". The Go version text is already correct. |
| docs/content/about/release-notes.md | none (D4 removed) | Pure refactors need no entry; no row action. |
| docs/content/development/{contributing,patterns,benchmarking}.md `interface{}` samples | FR-1 | Deferred: user docs are out of scope; samples remain valid Go. |
| docs/content/development/architecture.md: `context.WithValue(r.Context(), "request-id", ...)` (a lint error after PR 6) and the `sort.Slice` priority sort | FR-13, FR-9 | Deferred with the docs row above (user docs out of scope; recommended as one docs follow-up). |
| docs/content/development/benchmarking.md: presents `atomic.AddInt64` as the "Good" pattern, plus sample output citing it | FR-10 | Deferred with the docs row above, same reason. |
| docs/content/development/{setup,contributing}.md Go version text | none | Already correct; no row action. |
| makefile bench targets | NFR-1 | None: package-scoped `go test -bench` is used. |
| CI workflow lint step | FR-12 | None: same linter version; `verify: false` there is why invalid disable names would pass CI silently, so WP-5 checks `config verify` locally. |
| `.claude/skills/olla-validate` | none | None: used unchanged. |
| Anything calling the removed pointer helpers, retyped counters or retyped keys | FR-7, FR-10, FR-13 | Handled inside PR 3c, PR 4 and PR 6 (compile errors force it; `pkg/` has no hits). |

Searches run: `modernize`, `usetesting`, `SA1029` across the lint config, makefile and `.github/workflows`; Go version text across CLAUDE.md and `docs/content/development`; `interface{}`, `context.WithValue`, `sort.Slice` and `atomic.AddInt64` across `docs/content`; listing of `.claude/skills`. No hooks or `.claude` configuration reference the touched identifiers. No flow is made redundant, so there is no `remove` package.

## 8. Acceptance criteria and work packages

Every package ends with `make ready` on its final commit and then an empty `git status --porcelain` (NFR-5). "Reproducible" means: check out the PR's parent, run the stated command, and the result matches the PR's commit, allowing the formatter delta from FR-18.

Order: PR 1, PR 2, then PR 3a, 3c, 3d, 3b and PR 4 in any order, then PR 5, then PR 6. PR 4 follows PR 2 where both touch the stats collector (A6).

### WP-1. PR 1: `any` only

Mode: `refactor`. Specialist: Go (go-principal-architect). Depends on: nothing.
- AC-1: `./.bin/golangci-lint run --enable-only modernize $LINTFLAGS ./...` reports 0 `any` findings (seam: CLI; package-load errors fail the check).
- AC-2: re-running `go fix -any ./...` on the base reproduces the diff, and `go fix -diff -any ./...` is empty afterwards.
- AC-3: every changed non-whitespace line differs only by `interface{}` becoming `any`.
- AC-4: `go build ./...` passes and existing suites pass with assertions unchanged. No benchmarks.

### WP-2. PR 2: remaining mechanical `go fix` analyzers

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- One commit per analyzer in FR-2, tests and non-test files together.
- AC-1: for each analyzer, `go fix -diff -<name> ./...` is empty after its commit and its lint count is 0; the commit reproduces from its parent (a reviewed edit listed in the commit message is allowed where AC-3 or AC-4 require it).
- AC-2: PR-body checklist per analyzer: `errorsastype` (the target variable is not used after the `if`); `slicescontains` (search operands have no side effects, since evaluation count changes from per element to once); `minmax` (no floats, skipped by the tool).
- AC-3: `mapsloop` audit covers nil-ness (`maps.Clone` of a nil map is nil; `maps.Copy` into an existing map is exact; any case turning nil into empty is left as it was and listed) and destination expressions with side effects such as `w.Header()` (call counts must not change, including inside benchmarks).
- AC-4: `testingcontext` defer-order audit. The analyzer moves cancellation after bus shutdown in the eventbus test, leaving the watcher alive through teardown. Where order matters, keep `context.WithCancel(t.Context())` with its deferred cancel, as a reviewed edit (also a FR-19 exception, `//nolint:modernize` with a reason, if the linter would flag it).
- AC-5: benchmarks per NFR-1, each changed site named: a populated percentile-read benchmark exercising `GetPercentiles` (new, since the existing percentile benchmark times `Add`; this is a new test file only if no stats benchmark file covers it), a focused retry-classification benchmark in proxy core for the `errorsastype` sites (or a recorded statement that it is covered indirectly and by which benchmark), the XFF parse in proxy core common, and the stats collector minmax site.
- AC-6: `/olla-validate --quick` passes (request-path code; seam: agent-driven end-to-end). Existing package suites pass with assertions unchanged.

### WP-3a. PR 3a: `sync.WaitGroup.Go`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: `go fix -diff -waitgroupgo ./...` is empty afterwards and the lint count for `waitgroupgo` is 0 (`LINTFLAGS`).
- AC-2: re-running `go fix -waitgroupgo ./...` on the parent reproduces the diff; the diff contains no `omitzero` change (golangci-lint `--fix` is not used).
- AC-3: no `wg.Add` remains paired with a rewritten `go` statement; no closure captures a loop variable incorrectly (diff review).
- AC-4: `make test-race` passes (existing race suite, including the eventbus tests).

### WP-3b. PR 3b: test-only sweep (droppable)

Mode: `refactor`. Specialist: test-architect. Depends on: WP-1. Droppable: no other package depends on it.
- The package carries its own lint config change (FR-5). The measurement is the `usetesting` row in section 1.
- AC-1: that command reports 0 findings (seam: CLI).
- AC-2: `go test -race` passes for every converted package; each `t.Cleanup` that used the context follows FR-6.
- AC-3: the normalised, package-qualified sets of test and subtest pass/fail results from `go test -json -short -count=1` on focused packages are identical before and after, plus a diff review of assertions and table entries. (`go test -list` cannot see a removed subtest or assertion.)
- AC-4: total package wall time is within 20% of before (recorded per package).

### WP-3c. PR 3c: `newexpr`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: 0 `newexpr` findings; `grep -rnE --include='*.go' '\b(ptrInt|strPtr|ptrString|ptrTime|int64Ptr|ptrInt64)\(' .` returns nothing (scoped to Go files, because this spec names the helpers).
- AC-2: no `//go:fix inline` directive remains in Go files; `unused` reports no orphaned wrapper.
- AC-3: typed literals are explicit where a non-default pointer type is needed (the build proves it). Config, discovery, converter and unifier suites pass; `pkg/` exported API is untouched.

### WP-3d. PR 3d: `sort.*` to `slices`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: no `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` or `sort.Sort` call remains in non-test code, except recorded exceptions (the priority balancer sort, FR-9, with the FR-19 clearance).
- AC-2: each converted site's PR-body line states which FR-8 rule applied. Handler and stats outputs keep their order (existing handler tests pass with assertions unchanged).
- AC-3: if the priority sort is converted, a fixed-input comparison test over more than 12 mixed-tier endpoints shows identical order before and after (new test in the balancer suite; the suite already exists). If not converted, the site carries the comment and is listed as an exception.
- AC-4: benchmarks per NFR-1: balancer including `BenchmarkLargeEndpointSet` (which `make bench-balancer` filters out, so the package benchmark is run directly), the percentile read, and a handler sort benchmark covering a case where `SortFunc` passes large summary structs by value.

### WP-4. PR 4: `atomictypes`

Mode: `refactor`. Specialist: Go. Depends on: WP-1 (WP-2 first, A6).
- Briefing, hand-work: `go fix -atomictypes` covers only part of the stats collectors; `minLatency`, `lastUsed`, `lastCleanup`, proxy core and round-robin are by hand. `minLatency` starts at -1 (0 is a valid latency), set inside the `LoadOrCompute` literal and the collector constructor; sentinel fields are initialised before the object is published, never `.Store(-1)` afterwards.
- AC-1: 0 `atomictypes` findings and no `atomic.Add*`, `Load*`, `Store*`, `CompareAndSwap*` or `Swap*` call on a struct field remains in the named areas, for Int32, Int64 and Uint64.
- AC-2: `go vet ./...` (copylocks) and `make align-check` pass; exported snapshot structs and their JSON tags are unchanged (`ProxyStats` exported fields keep their types); the discovery client has no atomic calls and its metrics stay under the existing mutex.
- AC-3: `minLatency` initial value is pinned by an existing or characterising stats test before the change (a new test only if the stats suite does not already assert the -1 sentinel and first-sample behaviour).
- AC-4: the round-robin overflow test is migrated per FR-17 and still asserts wraparound.
- AC-5: `make test-race` and `/olla-validate --quick` pass.
- AC-6: benchmarks per NFR-1: stats collectors, proxy core and engines, round-robin balancer.

### WP-5. PR 5: enable `modernize`

Mode: `extend` (config only). Specialist: Go. Depends on: WP-1, WP-2, WP-3a, WP-3c, WP-3d, WP-4 (not WP-3b).
- Preflight (read-only, before opening the PR): run the PR 5 config with `LINTFLAGS` and confirm every remaining hit is an FR-19 recorded exception.
- AC-1: `./.bin/golangci-lint run $LINTFLAGS ./...` is clean (seam: CLI).
- AC-2: `./.bin/golangci-lint config verify` exits 0.
- AC-3: the disable list is exactly `omitzero` and `stringsbuilder` (plus `slicesclip` only per D3), each with a reason comment, and the comment naming the three upstream-off analyzers is present.
- AC-4: a throwaway file with `interface{}` fails lint locally (not committed; result in the evidence summary).
- AC-5: CLAUDE.md gains the single line.
- If WP-3b is dropped, `usetesting` stays excluded for tests and nothing else in PR 5 changes.

### WP-6. PR 6: typed context keys

Mode: `refactor`, with a characterisation commit first inside the PR. Specialist: Go with test-architect for the tests. Depends on: WP-5 (so the linter guards new code). Never combined with a mechanical PR.
- Characterisation commit first, landing green, run recorded in the PR body. Producers and consumers are recorded separately and the presence and absence cases are pinned through observable behaviour, including: the `stream` key read in streaming with no production writer; `route_prefix` supplied by proxy registration, overridden by the provider handler with the raw alias, and absent for translator registration; path-prefix default `"route_prefix"` through `GetProxyPrefix` and `StripPrefix`.
- AC-1: the SA1029 exclusion is removed and the staticcheck run reports 0 SA1029 findings. staticcheck is excluded for `_test.go`, so AC-1 cannot see test writers; AC-2 covers them.
- AC-2: no raw-string context key remains, including in `_test.go`. The verifier runs an exact grep (agreed in the PR body) over every `context.WithValue(` and `.Value(` site in Go files, tests included, and each hit uses a typed key or an accessor.
- AC-3: the tests pass before and after the key split without edits to their assertions; test-side writers move to the accessors per FR-17 and are listed. Route-table-wide cases loop the registered route table (provider routes, `/olla/proxy/`, translator routes) rather than one route.
- AC-4: slog output keys, InspectionMeta keys and `util.StripRoutePrefix` behaviour are byte-identical (existing suites plus the new tests). Which request ID is logged is unchanged (the duplicate request-ID keys are not merged here, Q3).
- AC-5: `/olla-validate --quick` passes.

## 9. Risks implementers must be briefed on

copylocks with atomic types; struct layout and betteralign (`make align-check`); sort tie order and the priority sort over 12 endpoints; `slicescontains` evaluating side-effecting operands once; `mapsloop` nil-ness and side-effecting destinations; `errorsastype` where the variable is used after the `if`; `testingcontext` moving cancellation after teardown; `t.Context()` cancelled before `t.Cleanup` runs; atomic sentinel fields (initialise before publication, never `.Store` after); untyped literals needing an explicit type under `new(expr)`; gofmt and goimports alignment shifts (accept in files touched, same commit); running `go fix` without `-diff` writes files, so apply on a clean branch and review each result with `git diff`; `make ready` mutates the tree.

## 10. Assumptions

- **A1.** The research numbers are the measurement, but some were taken under default lint caps. The spec restates the commands with `LINTFLAGS`, and implementers re-measure at each PR's base.
- **A2.** There is no shared baseline package. Each benchmarked PR takes its own baseline (NFR-1). Rejected: one baseline up front, which drifts with the box.
- **A3.** `stringsbuilder` and `omitzero` have one finding each and are not in the brief's PR split. `omitzero` is off by D3. `stringsbuilder` is also off because it is not a category in the split; adding it to PR 2 was rejected.
- **A4.** The one `slicessort` finding that `go fix` handles in PR 2 is skipped in PR 3d.
- **A5.** PR 6 is typed `refactor` with a characterisation commit inside the PR, since the readers are not directly covered. The alternative, a separate test PR, was rejected.
- **A6.** PR 4 follows PR 2 where both touch the stats collector, to avoid conflicts. A preference only.
- **A7.** The CLAUDE.md "modernize linter enforced" line is added in PR 5 (the brief calls it optional).
- **A8.** Stale docs snippets are deferred (section 7), not fixed in the series. A cheaper per-PR docs line is acceptable if the implementer finds one, but is not required.
- **A9.** CLAUDE.md already states Go 1.27 (confirmed), so no version edit is needed.
- **A10.** Discovery metrics switch to plain int64 under the existing mutex rather than leaving FR-10; the writes already happen under that mutex, so this is a simplification with no behaviour change. Rejected: atomic fields (breaks FR-11 and copylocks).
- **A11.** The 5% threshold in NFR-1 is a judgement and the user may tighten it.

## 11. Open questions

- **Q2.** Do you want to overrule D2 or D3 (benchmark method and the analyzer-off list)? The spec proceeds on the defaults.
- **Q3.** `request_time` and `provider_type` are written and never read, `original_path` is never used, and there are two request-ID keys (the constants and the middleware package). Should PR 6 delete or merge them? Recommendation: type them as-is in PR 6, and delete or merge in a follow-up, since merging the request-ID keys could change which ID is logged.

(Q1, the next version number for a release-notes line, is removed with D4: no release-notes entry arises from this series.)

## 12. Implementation handoff

This spec is implemented by running `/orchestrate docs/specs/go-1.27-modernise.md` in a fresh session. That session is the orchestrator, not an implementer: it decomposes this spec into the work packages above, dispatches each to the matching specialist subagent, reviews their reports against the acceptance criteria, and iterates fixes back to the same agent. It never implements, explores or debugs in its own context beyond trivial glue; anything that needs reading or writing code is delegated so the orchestrator's context stays lean and its judgement stays fresh for review. Bug-shaped packages go through bug-investigator before a specialist touches code.

Anything found wrong along the way is a bug report for `/bug-hunt`, not a fix inside this spec.
