# Go 1.27 modernisation (refactor spec)

Branch: `chore/go-1.27-modernise`, cut from main at 9238e1f4. Measurements below were taken at that commit.

## 1. Purpose and scope

Olla builds with Go 1.27 (`go 1.27.0`, `toolchain go1.27.4`, golangci-lint v2.15.0 from the repo-local tool directory). Most of the code was written for Go 1.21 to 1.24 and predates the idioms 1.22 to 1.27 made standard. This spec adopts them in single-category PRs, mostly by tool, and then enables the `modernize` linter so the backlog cannot regrow.

**This spec authorises no behaviour change.** A diff that changes output, ordering, the JSON wire format or an exported `pkg/` signature is out of scope for the PR that contains it. This is not a `/debt` item, so there is no register entry to pay down. Test edits are allowed only where FR-17 says.

### Measurement that changes

`golangci-lint run` caps output at 50 issues per linter and 3 identical messages, and dedupes by line, so a default run hides most sites. **Every lint count command in this spec carries the three uncapped flags, printed literally** (a shell variable holding them expands as one argument in zsh and the linter exits 3 before loading packages): `--max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false`. Counts are taken by analyzer (from the issue text). A package-load error is a failure, not a zero.

Each number below is labelled with the command that produced it. Implementers re-measure at each PR's base (A1).

| Measure | Command (repo root) | Before | After |
|---|---|---|---|
| modernize findings, whole repo | `./.bin/golangci-lint run --enable-only modernize --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false ./...` | 1,464 (358 non-test) | 0 apart from the disable-list analyzers (FR-12) and recorded exceptions (FR-19) |
| `go fix` pending hunks, all analyzers including `waitgroupgo` | `go fix -diff ./...` | 662 hunks, 177 files | only the disable-list analyzers and recorded exceptions remain |
| `waitgroupgo` alone | `go fix -diff -waitgroupgo ./...` | 50 hunks, 26 files | 0 |
| usetesting in tests | `./.bin/golangci-lint run --enable-only usetesting --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false ./...` with the `_test.go` exclusion for `usetesting` removed and `context-background: true`, `context-todo: true` set | 722: 713 context sites (666 `t.Context`, 47 `b.Context`) plus 9 `os.Setenv` | 0 |
| SA1029 | `./.bin/golangci-lint run --enable-only staticcheck --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false ./...` with `-SA1029` removed | suppressed | 0 findings, suppression gone |

Per analyzer from the modernize command (total / non-test): `any` 1,212/273; `newexpr` 98/2; `waitgroupgo` 35/2; `errorsastype` 32/28; `slicescontains` 20/11; `mapsloop` 18/9; `minmax` 11/11; `embedlit` 11/0; `stringsseq` 6/5; `stringscutprefix` 6/4; `atomictypes` 5/5; `stringscut` 4/2; `reflecttypefor` 2/2; `stringsbuilder`, `stditerators`, `slicessort`, `omitzero` 1 each. Per analyzer `go fix` hunks (`go fix -diff -<name> ./...`, 26 analyzers on go1.27.4): `any` 432; `newexpr` 51; `slicescontains` 37; `mapsloop` 28; `errorsastype` 26; `atomictypes` 18; `minmax` 8; `embedlit` 7; `stringsseq` 6; `stringscutprefix` 4; `stringsbuilder` 4; `stringscut` 3; `testingcontext` 3; `forvar` 2; `reflecttypefor` 2; `slicessort` 2; `omitzero` 1; `stditerators` 1; `waitgroupgo` 50.

### Out of scope (deferred, not requirements)

- Swapping json-iterator or `encoding/json` for `encoding/json/v2`.
- An `omitempty` to `omitzero` sweep (a wire-format change).
- `b.Loop` and `testing/synctest` sweeps. synctest is only for a specific test the user nominates later.
- Range-over-func iterators, `unique`, `weak` and ServeMux pattern adoption as a sweep.
- `Server.MaxHeaderValueCount` adoption (a separate security decision).
- Any Sherpa feature work.
- Changes to `web/` or user docs, including the stale snippets listed in section 7.
- **Atomic types beyond the linter's findings** (D7): proxy core `ProxyStats` (the live struct; tests read its exported fields directly after `IncrementRequests`, so copying on read changes observable state and retyping in place is unrequired churn), the round-robin counter, and the discovery metrics (which keep their existing atomics). Reason for all three: not a modernize finding; the existing atomic-function code is correct.
- **Go 1.27 runtime behaviour checks** (JSON decode options, `Response.Body` drain on close). They belong to the toolchain change, which already shipped in v0.2.0 and which no PR here alters, so a validation run cannot attribute anything to this series. A separate toolchain item if wanted.
- Deleting or merging dead and duplicate context keys (Q3).
- Follow-ups from the last release (query-string API keys in logs, makefile tool-gover with spaces in paths and others): tracked elsewhere.

## 2. Decisions

- **D1 (settled): PR split and order.** PR 1, 2, 3a to 3d, 4, 5, 6 as in section 8. One category per PR so a reviewer can accept it by skimming.
- **D2 (default, user may overrule): benchmark before and after for each PR that touches a hot path** (PR 2, 3d, 4), scoped to the touched packages. Before and after go in the PR body. See NFR-1 for the method.
- **D3 (default, user may overrule): analyzers that stay off.** `omitzero` (a JSON wire-format change) and `stringsbuilder` (not a category in the split) go on the `settings.modernize.disable` list. `slicesdelete`, `appendclipped` and `bloop` are not valid `disable` names in golangci-lint v2.15.0 (`config verify` rejects them; `run` silently ignores them; CI sets `verify: false`). They are not analyzers in that version's modernize set, and `slicesclip` is the current name for the clipped-append check. They are recorded in a comment, not in the list. `slicesclip` goes on the list only if the nil-preservation concern applies at a site found during PR 2 or 3d.
- **D5 (settled, no work):** CLAUDE.md already states Go 1.27; nothing uses `httputil.ReverseProxy` or `Director`; container-aware GOMAXPROCS and the Green Tea GC need no action.
- **D6 (settled):** implementation by Sonnet agents; the user merges via GitHub; reviews are Fable (or Opus) plus Codex, never Sonnet.
- **D7 (scope decision): atomic types only where the linter flags them.** PR 4 covers the stats collector and the model collector's per-endpoint data, including the fields in those same structs that `go fix -atomictypes` leaves for hand work (`minLatency`, `lastUsed`, `lastCleanup`, `consecutiveErrors`). Proxy core, round-robin and discovery metrics are deferred (out of scope above).

(D4, the Go 1.27 runtime behaviour verification item, is removed and deferred.)

## 3. Functional requirements

"Zero findings" means the analyzer's count from the literal-flag commands above is 0 after the package.

- **FR-1 (PR 1).** Every `interface{}` type is rewritten to `any` using `go fix -any ./...`. Defined over Go types, not text: `interface{}` inside comments or strings may remain. The change is a pure rename. `any` findings go to 0.
- **FR-2 (PR 2).** The analyzers `slicescontains`, `mapsloop`, `minmax`, `stringscut`, `stringscutprefix`, `stringsseq`, `errorsastype`, `reflecttypefor`, `stditerators`, `slicessort`, `embedlit`, `forvar` and `testingcontext` are applied with `go fix -<name> ./...`, one analyzer per commit, each commit reproducible by re-running that command on its parent (plus the formatter delta, FR-18, and the reviewed edits FR-17 and FR-19 allow). Each analyzer's finding count goes to 0 except recorded exceptions.
- **FR-3 (PR 2).** `atomictypes`, `newexpr`, `omitzero`, `stringsbuilder` and `waitgroupgo` are not applied in PR 2. `stringsbuilder` stays out of every PR (A3).
- **FR-4 (PR 3a).** `sync.WaitGroup.Go` replaces `wg.Add(1)` plus `go func(){ defer wg.Done() ... }()` at the `waitgroupgo` sites (35 lint findings, 2 non-test), applied with `go fix -waitgroupgo ./...`, like FR-2. golangci-lint `--fix` is never used for this: it cannot be limited to one analyzer from the command line and would also apply `omitzero` to the deferred JSON tag on the profile config's `AuthHint`. Findings go to 0.
- **FR-5 (PR 3b).** In the lint config, the `usetesting` exclusion for `_test.go` files is lifted and `usetesting` settings `context-background: true` and `context-todo: true` are set (both default to false in v2.15.0; lifting the exclusion alone yields only the 9 `os.Setenv` findings). The 713 context sites convert to `t.Context()` or `b.Context()` (benchmarks need `b.Context()`), `os.Setenv` in tests becomes `t.Setenv`, and the remaining `tc := tc` copies are removed. Findings go to 0.
- **FR-6 (PR 3b).** A converted context used inside a `t.Cleanup` callback is not left as `t.Context()`, because it is cancelled before cleanups run. Where teardown uses the test context (for example the unified memory registry removal test calling `Stop`), use `context.WithoutCancel(t.Context())`, with a one-line comment saying why.
- **FR-7 (PR 3c).** The pointer helpers are replaced by `new(expr)` at their call sites and the helpers are deleted. There are nine package-level declarations under six spellings: `ptrInt` (config, non-test, and test copies), `strPtr`, `ptrString`, `ptrTime`, `int64Ptr` (converter tests and unifier integration tests) and `ptrInt64` (unifier metadata tests). There are also two local closures, `ptrInt64` and `ptrString`, in the default unifier's model-merging test; `go fix -newexpr` skips closures, so their calls are replaced by hand and the closures deleted. Where `*int64` (or another non-default type) is required, an untyped literal takes an explicit type: `new(int64(8192))`, not `new(8192)`. `go fix -newexpr` adds `//go:fix inline` directives to the wrappers; the wrappers are deleted after inlining (the `unused` linter flags them) and no directive is left behind. `newexpr` findings go to 0.
- **FR-8 (PR 3d).** `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` and `sort.Sort` calls (27 non-test sites, 17 in handlers) become `slices.Sort`, `slices.SortFunc` or `slices.SortStableFunc`. Rules:
  - `sort.SliceStable` maps to `slices.SortStableFunc`.
  - Where the sorted input comes from map iteration and the key can tie (the stats models and translators handlers, the model collector eviction), tie order is already undefined: carry the comparator over unchanged, with no stable variant added.
  - Use a stable variant only where the input order is deterministic and a test pins tie order.
  - `time.Time` values compare with `time.Time.Compare`; integer (`int64`) timestamps with `cmp.Compare`. Each site keeps its existing direction and secondary keys.
- **FR-9 (PR 3d).** The priority balancer sort (per-request path) stays `sort.Slice` unless both hold: (a) the NFR-1 benchmark threshold passes; (b) on a labelled fixture where the current unstable sort demonstrably reorders equal-priority endpoints (tied priorities, more than 12 elements, mixed tiers), the new code produces the identical order. `BenchmarkLargeEndpointSet` uses distinct priorities and cannot test ties. If either fails, the site keeps `sort.Slice` with an explanatory comment; it is not a modernize finding, and is a recorded exception (FR-19).
- **FR-10 (PR 4).** In the stats collector and the model collector's per-endpoint data, counters accessed through `atomic.Add`, `Load`, `Store`, `CompareAndSwap` and `Swap` move to `atomic.Int32`, `atomic.Int64` or `atomic.Uint64`: the five fields the linter flags plus `minLatency`, `lastUsed`, `lastCleanup` and `consecutiveErrors` (int32 in the model collector's per-endpoint data, so `atomic.Int32`). All call sites of a field move together in one commit. `go fix -atomictypes` converts only part of this (18 hunks, 3 files); the rest is hand work.
- **FR-11 (PR 4).** Snapshot and result structs, and anything serialised or returned to callers, remain plain integer types; only live counter fields change type. No struct holding an atomic type is copied, compared or returned by value (`go vet` copylocks clean).
- **FR-12 (PR 5).** `modernize` is enabled in the lint config with every analyzer on by default and an explicit `settings.modernize.disable` list of exactly `omitzero` and `stringsbuilder` (plus `slicesclip` only per D3), each with a reason comment. A further comment records that `slicesdelete`, `appendclipped` and `bloop` are "not analyzers in golangci-lint v2.15.0's modernize set; `slicesclip` is the current name for the clipped-append check". PR 5 lands only after PRs 1, 2, 3a, 3c, 3d and 4.
- **FR-13 (PR 6).** Every context key is classified by role (section 6 lists them). A key used for context storage that is an untyped string becomes the typed `contextKey` the sticky keys already use. Where a constant also serves another role (slog key, InspectionMeta key, path-prefix default), that role keeps a separate untyped string constant. The SA1029 exclusion in the lint config is removed.
- **FR-14 (PR 6).** Accessors extend the existing ones (the middleware package's own `contextKey` and `GetLogger`, `GetRequestID`); they are not duplicated. The accessor for `core.CanonicalModel` lives in proxy core, beside the type, because placing it in the constants package would create an import cycle. The accessor for the existing typed sticky key lives in the constants package and uses `*domain.StickyOutcome` (`balancer.StickyOutcome` is an alias of the domain type). There is no cycle here, since domain imports nothing internal and constants imports nothing internal, so constants importing domain is acyclic; verify with `go list -deps`. This is a placement choice, not a cycle workaround, and domain must not import constants (its package doc forbids it). No site reads or writes a context value by raw string, including the proxy handler (which today bypasses `GetRequestID`), the logger package's raw detailed-cookie key and about 29 test-side writes (one with the literal `"stream"` in the streaming test).
- **FR-15 (all).** Public API in `pkg/` (container, eventbus, pool, format, nerdstats, profiler) is unchanged; all `pkg/` findings are test-only. Sherpa receives mechanical rewrites only.
- **FR-16 (all).** No dependency is added. Tools come from the repo-local tool directory: a fresh checkout installs the pinned tools into that directory through the makefile's ensure targets, which is permitted. Any install outside that directory, or into the shared GOPATH bin, is forbidden.
- **FR-17 (all).** Test assertions and expectations are unchanged. Test setup and field access change only where required: where a retyped field or key forces it (the collector test rewritten by `atomictypes`, the test-side context-key writers in PR 6), and these enumerated setup migrations that change no expectation: `os.Setenv` to `t.Setenv`, the eventbus `context.WithCancel(t.Context())` with its deferred cancel kept, and the `context.WithoutCancel` cleanup (FR-6). Each such edit is listed in the PR body.
- **FR-18 (all).** A commit may carry a formatter delta. `go fix -slicescontains` adds a second `import "slices"` declaration in two domain files; goimports (the configured formatter) merges them and `go fmt` does not. The delta comes from `./.bin/golangci-lint fmt`, in the same commit as the change.
- **FR-19 (all).** A deliberately kept site (the priority sort, FR-9; the eventbus test, WP-2; the mapsloop residual, WP-2) must clear both tools, and every recorded exception names its expected residual in each tool in the PR body. `//nolint:modernize // <reason>` clears the linter only; it cannot clear `go fix -diff`, so a residual `go fix` hunk is accepted explicitly by naming it.

## 4. Non-functional requirements

- **NFR-1.** Benchmarked PRs (2, 3d, 4) each take their own baseline at their parent commit, in the same session as their after-run (no shared baseline).
  - **Method.** Each side runs `-count=6 -benchmem -run '^$'`, compared with `benchstat` (present on the box; if it is absent the PR cannot claim the gate and says so; there is no medians fallback). Allocations are compared as well as ns/op. The fixture is identical across the compared revisions. Each PR body names the benchmark function that exercises each changed site, and shows 6 results per named row per side as evidence the row ran.
  - **Missing rows.** Where a required row does not exist at the parent (a populated percentile-read benchmark, a large handler sort, XFF parsing with multiple addresses, retry classification that reaches the changed classifier), a benchmark-only commit adds it first, and the identical benchmark file is applied to both revisions.
  - **Threshold.** Derived from the PR's own two baseline sets: a regression is a benchstat-significant slowdown (p < 0.05) larger than the spread between the two baseline sets, or 5%, whichever is larger.
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
| Benchmarks | makefile targets `bench`, `bench-balancer`, `bench-repo`; package benchmarks in stats, proxy core, balancer, handlers | Targets are not enough alone: `bench-balancer` filters out `BenchmarkLargeEndpointSet`, the stats percentile benchmark times `Add` while the sort is in `GetPercentiles` under the lock, and proxy core has no retry-classification benchmark. Missing rows are added per NFR-1. No makefile change. |
| Context keys | The sticky-session keys, `ContextModelKey`, `EndpointPathMap`, `CanonicalModel` and `InputTokens` are already typed; the middleware package owns `contextKey` with `GetLogger` and `GetRequestID` | PR 6 extends the typed pattern to the untyped keys and extends the existing accessors. Retyping the shared constants in place is the near miss: they double as slog keys, InspectionMeta keys and the proxy `Configuration`'s default path prefix. |
| Pointer helpers | nine declarations and two closures (FR-7) | Removed. `new(expr)` is a language feature. |
| Atomic counters | stats collector and model collector per-endpoint data | Extended to atomic types (FR-10). Proxy core, round-robin and discovery metrics are untouched (D7). |
| End-to-end proof | `/olla-validate --quick` | Used unchanged for PR 2, 4 and 6. |
| Test suites that cover the seams | `make ready`; package suites for stats, proxy core, balancer, registry, handlers, middleware, router, logger, config, unifier | Assertions pass unchanged; setup and field access change only per FR-17. |

## 6. Data and interfaces

No schema, API, config-key or wire-format change. Go-internal interfaces touched:

- **PR 4 counters.** Live counter fields in the stats collector and the model collector's per-endpoint data change to atomic types. Snapshot structs and exported fields keep plain types. Nothing in proxy core, the round-robin balancer or the discovery client changes.
- **PR 6 context keys.** The shape of the decision:

```go
type contextKey string // storage role only; never used as a slog or metadata key
```

Key inventory (roles to be confirmed by the implementer against the code; the characterisation commit pins them):

| Key | Today | Roles to preserve |
|---|---|---|
| `route_prefix` | untyped | context storage; the proxy `Configuration`'s `GetProxyPrefix` default path prefix (URL building's `StripPrefix`), which must stay the literal `"route_prefix"`; `util.StripRoutePrefix` lookup |
| `request_id` | untyped | context storage; slog key. A second, already-typed `request_id` key exists in the middleware package. |
| `request_time` | untyped | written, never read (Q3) |
| `original_path` | untyped | never used (Q3) |
| `stream` | untyped | read in streaming with no production writer; test writers use the literal `"stream"` |
| `provider_type` | untyped | written, never read (Q3) |
| `model_alias_map` | untyped | two roles. The proxy handler writes it to the string-keyed InspectionMeta, and the translation handler reads it from InspectionMeta and installs it in the context for the selected backend. The string `"model_alias_map"` stays for the InspectionMeta role; a typed key serves the context role only. |
| `"detailed"` (`DefaultDetailedCookie`, logger package) | raw | read in the logger; written in the logging middleware and both styled loggers |
| middleware `request_id` and `logger` keys | already typed (middleware `contextKey`) | no key change; accessors `GetRequestID`, `GetLogger` are extended, not duplicated |
| `ContextModelKey`, three sticky keys, `EndpointPathMap`, `CanonicalModel`, `InputTokens` | already typed | no key change; add accessors where readers use the raw key |

Accessors take and return `context.Context` and the value type, so call sites stop naming keys. Placement follows FR-14.

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
| docs/content/development/benchmarking.md: presents `atomic.AddInt64` as the "Good" pattern, plus sample output citing it | none (the sample's code stays valid; D7 keeps atomic functions elsewhere) | Deferred with the docs row above, same reason. |
| Proxy core `ProxyStats`, round-robin counter, discovery metrics: atomic-function code left in place | D7 | Deferred: not a modernize finding; the existing atomic-function code is correct. |
| docs/content/development/{setup,contributing}.md Go version text | none | Already correct; no row action. |
| makefile bench targets | NFR-1 | None: package-scoped `go test -bench` is used. |
| CI workflow lint step | FR-12 | None: same linter version; `verify: false` there is why invalid disable names pass CI silently, so WP-5 checks `config verify` locally. |
| `.claude/skills/olla-validate` | none | None: used unchanged. |
| Anything calling the removed pointer helpers, retyped counters or retyped keys | FR-7, FR-10, FR-13 | Handled inside PR 3c, PR 4 and PR 6 (compile errors force it; `pkg/` has no hits). |

Searches run: `modernize`, `usetesting`, `SA1029` across the lint config, makefile and `.github/workflows`; Go version text across CLAUDE.md and `docs/content/development`; `interface{}`, `context.WithValue`, `sort.Slice` and `atomic.AddInt64` across `docs/content`; listing of `.claude/skills`. No hooks or `.claude` configuration reference the touched identifiers. No flow is made redundant, so there is no `remove` package.

## 8. Acceptance criteria and work packages

Every package ends with `make ready` on its final commit and then an empty `git status --porcelain` (NFR-5). "Reproducible" means: check out the PR's parent, run the stated command, and the result matches the PR's commit, allowing the formatter delta (FR-18) and reviewed edits listed in the commit message (FR-17, FR-19).

Order: PR 1, PR 2, then PR 3a, 3c, 3d, 3b and PR 4 in any order, then PR 5, then PR 6. PR 4 follows PR 2 where both touch the stats collector (A6).

### WP-1. PR 1: `any` only

Mode: `refactor`. Specialist: Go (go-principal-architect). Depends on: nothing.
- AC-1: the modernize command (literal flags) reports 0 `any` findings (seam: CLI; package-load errors fail the check).
- AC-2: re-running `go fix -any ./...` on the base reproduces the diff, and `go fix -diff -any ./...` is empty afterwards.
- AC-3: every changed non-whitespace line differs only by `interface{}` becoming `any`.
- AC-4: `go build ./...` passes and existing suites pass with assertions unchanged. No benchmarks.

### WP-2. PR 2: remaining mechanical `go fix` analyzers

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- One commit per analyzer in FR-2, tests and non-test files together.
- AC-1: for each analyzer, `go fix -diff -<name> ./...` is empty after its commit (or shows only an explicitly accepted residual, FR-19) and its lint count is 0 except recorded exceptions; the commit reproduces from its parent.
- AC-2: PR-body checklist per analyzer: `errorsastype` (the target variable is not used after the `if`); `slicescontains` (search operands have no side effects, since evaluation count changes from per element to once); `minmax` (no floats, skipped by the tool).
- AC-3: `mapsloop` audit covers nil-ness (`maps.Clone` of a nil map is nil; `maps.Copy` into an existing map is exact; any case turning nil into empty is left as it was and listed) and destination expressions with side effects such as `w.Header()` (call counts must not change). For the benchmark upstream loop that must keep one `w.Header()` call per header, the reviewed rewrite is `dst := w.Header()` then `dst[k] = v` inside the loop, which preserves one call per header and zero calls for an empty map. The implementer checks whether that clears `go fix`; if not, the exact residual hunk is accepted explicitly (FR-19).
- AC-4: `testingcontext` defer-order audit. The analyzer moves cancellation after bus shutdown in the eventbus test, leaving the watcher alive through teardown. Where order matters, keep `context.WithCancel(t.Context())` with its deferred cancel, as a reviewed edit (FR-17); the expected residual in `go fix -diff` and the linter is named (FR-19).
- AC-5: benchmarks per NFR-1, each changed site named, with benchmark-only commits first for missing rows: a populated percentile-read benchmark exercising `GetPercentiles`, retry classification in proxy core that reaches the changed classifier, XFF parsing with multiple addresses in proxy core common, and the stats collector minmax site.
- AC-6: `/olla-validate --quick` passes (request-path code; seam: agent-driven end-to-end). Existing package suites pass with assertions unchanged.

### WP-3a. PR 3a: `sync.WaitGroup.Go`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: `go fix -diff -waitgroupgo ./...` is empty afterwards and the lint count for `waitgroupgo` is 0.
- AC-2: re-running `go fix -waitgroupgo ./...` on the parent reproduces the diff; the diff contains no `omitzero` change (golangci-lint `--fix` is not used).
- AC-3: no `wg.Add` remains paired with a rewritten `go` statement; no closure captures a loop variable incorrectly (diff review).
- AC-4: `make test-race` passes (existing race suite, including the eventbus tests).

### WP-3b. PR 3b: test-only sweep (droppable)

Mode: `refactor`. Specialist: test-architect. Depends on: WP-1. Droppable: no other package depends on it.
- The package carries its own lint config change (FR-5). The measurement is the `usetesting` row in section 1.
- AC-1: that command reports 0 findings (seam: CLI).
- AC-2: `go test -race` passes for every converted package; each `t.Cleanup` that used the context follows FR-6.
- AC-3: the package-qualified run, skip, pass and fail events from focused non-short runs (`go test -json -count=1 -run <affected tests>`) of the affected packages are identical before and after, plus a diff review of assertions and table entries. (`-short` hides subtests skipped before registration, for example the proxy integration performance test, and `go test -list` cannot see a removed subtest or assertion.)
- AC-4: no individual test's duration grows beyond its `-timeout`, and each package still passes under `make test-race`.

### WP-3c. PR 3c: `newexpr`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: 0 `newexpr` findings; `grep -rnE --include='*.go' '\b(ptrInt|strPtr|ptrString|ptrTime|int64Ptr|ptrInt64)\(' .` returns nothing (scoped to Go files, because this spec names the helpers; this also proves the two unifier-test closures are gone).
- AC-2: no `//go:fix inline` directive remains in Go files; `unused` reports no orphaned wrapper.
- AC-3: typed literals are explicit where a non-default pointer type is needed (the build proves it). Config, discovery, converter and unifier suites pass; `pkg/` exported API is untouched.

### WP-3d. PR 3d: `sort.*` to `slices`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: no `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` or `sort.Sort` call remains in non-test code, except recorded exceptions (the priority balancer sort, FR-9, with the FR-19 clearance).
- AC-2: each converted site's PR-body line states which FR-8 rule applied. Handler and stats outputs keep their order (existing handler tests pass with assertions unchanged).
- AC-3: the priority sort is converted only if FR-9 (a) and (b) both hold; the labelled tied-priority fixture is a new test in the balancer suite (the suite already exists). Otherwise the site carries the comment and is listed as an exception.
- AC-4: benchmarks per NFR-1: balancer including `BenchmarkLargeEndpointSet` (which `make bench-balancer` filters out, so the package benchmark is run directly), the percentile read, and a large handler sort covering a case where `SortFunc` passes large summary structs by value (benchmark-only commit first where missing).

### WP-4. PR 4: `atomictypes` (stats collectors only, D7)

Mode: `refactor`. Specialist: Go. Depends on: WP-1 (WP-2 first, A6).
- Briefing, hand-work: `go fix -atomictypes` covers only part of this; `minLatency`, `lastUsed`, `lastCleanup` and `consecutiveErrors` are by hand. The stats collector's minimum-latency sentinel is -1 (0 is a valid latency), set in the `LoadOrCompute` literal and the collector constructor; sentinel fields are initialised before the object is published, never `.Store(-1)` afterwards. Proxy core, round-robin and discovery are not touched.
- AC-1: 0 `atomictypes` findings and no `atomic.Add*`, `Load*`, `Store*`, `CompareAndSwap*` or `Swap*` call on a struct field remains in the stats collector or the model collector's per-endpoint data, for Int32, Int64 and Uint64.
- AC-2: `go vet ./...` (copylocks) and `make align-check` pass; exported snapshot structs and their JSON tags are unchanged.
- AC-3: the -1 minimum-latency sentinel and first-sample behaviour are pinned by an existing or characterising stats test before the change (a new test only if the stats suite does not already assert them).
- AC-4: the collector test rewritten by `atomictypes` is listed per FR-17 and its assertions are unchanged.
- AC-5: `make test-race` and `/olla-validate --quick` pass.
- AC-6: benchmarks per NFR-1 for the stats collectors and the percentile read.

### WP-5. PR 5: enable `modernize`

Mode: `extend` (config only). Specialist: Go. Depends on: WP-1, WP-2, WP-3a, WP-3c, WP-3d, WP-4 (not WP-3b).
- Preflight (read-only, before opening the PR): run the PR 5 config with the literal uncapped flags and confirm every remaining hit is an FR-19 recorded exception.
- AC-1: `./.bin/golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false ./...` is clean (seam: CLI).
- AC-2: `./.bin/golangci-lint config verify` exits 0.
- AC-3: the disable list is exactly `omitzero` and `stringsbuilder` (plus `slicesclip` only per D3), each with a reason comment, and the comment worded per FR-12 is present.
- AC-4: a throwaway file with `interface{}` fails lint locally (not committed; result in the evidence summary).
- AC-5: CLAUDE.md gains the single line.
- If WP-3b is dropped, `usetesting` stays excluded for tests and nothing else in PR 5 changes.

### WP-6. PR 6: typed context keys

Mode: `refactor`, with a characterisation commit first inside the PR. Specialist: Go with test-architect for the tests. Depends on: WP-5 (so the linter guards new code). Never combined with a mechanical PR.
- Characterisation commit first, landing green, run recorded in the PR body. Producers and consumers are recorded separately and the presence and absence cases are pinned through observable behaviour, including: the `stream` key read in streaming with no production writer; `route_prefix` supplied by proxy registration, overridden by the provider handler with the raw alias, and absent for translator registration; the path-prefix default `"route_prefix"` through `GetProxyPrefix` and `StripPrefix`; and `model_alias_map`, where the forwarded body carries the backend-specific model name after alias rewrite.
- AC-1: the SA1029 exclusion is removed and the staticcheck run reports 0 SA1029 findings. staticcheck is excluded for `_test.go`, so AC-1 cannot see test writers; AC-2 covers them.
- AC-2: no untyped context key remains, including in `_test.go`. The verifier runs `git grep -nE 'context\.WithValue\([^,]+, *("|constants\.Context[A-Za-z]+Key\b|DefaultDetailedCookie)' -- '*.go'`; every remaining hit must be a typed key, checked against the section 6 table. Readers (`Context.Value`) are verified by receiver and key type in review, not by an unconditional `.Value(` grep, which also matches counter `.Value()` reads.
- AC-3: the tests pass before and after the key split without edits to their assertions; test-side writers move to the accessors per FR-17 and are listed. Route-table-wide cases loop the registered route table (provider routes, `/olla/proxy/`, translator routes) rather than one route. Path-stripping tests pass variable string keys; the one that deliberately supplies an unrelated key the proxy must ignore gives that key its own defined type, not the real route-prefix accessor, so it still exercises "ignored key".
- AC-4: slog output keys, InspectionMeta keys and `util.StripRoutePrefix` behaviour are byte-identical (existing suites plus the new tests). Which request ID is logged is unchanged (the duplicate request-ID keys are not merged here, Q3).
- AC-5: `/olla-validate --quick` passes.

## 9. Risks implementers must be briefed on

copylocks with atomic types; struct layout and betteralign (`make align-check`); sort tie order and the priority sort over 12 endpoints; `slicescontains` evaluating side-effecting operands once; `mapsloop` nil-ness and side-effecting destinations; `errorsastype` where the variable is used after the `if`; `testingcontext` moving cancellation after teardown; `t.Context()` cancelled before `t.Cleanup` runs; the stats collector's -1 sentinel (initialise before publication, never `.Store` after); untyped literals needing an explicit type under `new(expr)`; two unifier-test closures that `go fix -newexpr` skips; gofmt and goimports alignment shifts (accept in files touched, same commit); lint commands must carry the three flags literally in zsh; running `go fix` without `-diff` writes files, so apply on a clean branch and review each result with `git diff`; `make ready` mutates the tree.

## 10. Assumptions

- **A1.** The research numbers are the measurement, re-measured with the literal-flag commands at each PR's base; counts may drift if main moves.
- **A2.** There is no shared baseline package. Each benchmarked PR takes its own baseline (NFR-1). Rejected: one baseline up front, which drifts with the box.
- **A3.** `stringsbuilder` and `omitzero` have one finding each and are not in the brief's PR split. `omitzero` is off by D3. `stringsbuilder` is also off because it is not a category in the split; adding it to PR 2 was rejected.
- **A4.** The one `slicessort` finding that `go fix` handles in PR 2 is skipped in PR 3d.
- **A5.** PR 6 is typed `refactor` with a characterisation commit inside the PR, since the readers are not directly covered. The alternative, a separate test PR, was rejected.
- **A6.** PR 4 follows PR 2 where both touch the stats collector, to avoid conflicts. A preference only.
- **A7.** The CLAUDE.md "modernize linter enforced" line is added in PR 5 (the brief calls it optional).
- **A8.** Stale docs snippets are deferred (section 7), not fixed in the series. A cheaper per-PR docs line is acceptable if the implementer finds one, but is not required.
- **A9.** CLAUDE.md already states Go 1.27 (confirmed), so no version edit is needed.
- **A10.** `benchstat` is on PATH on the box. If a PR's session lacks it, the PR cannot claim the benchmark gate and says so; nothing is installed.

## 11. Open questions

- **Q2.** Do you want to overrule D2, D3 or D7 (benchmark method, the analyzer-off list, the narrowed atomic scope)? The spec proceeds on the defaults.
- **Q3.** `request_time` and `provider_type` are written and never read, `original_path` is never used, and there are two request-ID keys (the constants and the middleware package). Should PR 6 delete or merge them? Recommendation: type them as-is in PR 6, and delete or merge in a follow-up, since merging the request-ID keys could change which ID is logged.

## 12. Implementation handoff

This spec is implemented by running `/orchestrate docs/specs/go-1.27-modernise.md` in a fresh session. That session is the orchestrator, not an implementer: it decomposes this spec into the work packages above, dispatches each to the matching specialist subagent, reviews their reports against the acceptance criteria, and iterates fixes back to the same agent. It never implements, explores or debugs in its own context beyond trivial glue; anything that needs reading or writing code is delegated so the orchestrator's context stays lean and its judgement stays fresh for review. Bug-shaped packages go through bug-investigator before a specialist touches code.

Anything found wrong along the way is a bug report for `/bug-hunt`, not a fix inside this spec.
