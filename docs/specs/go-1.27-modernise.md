# Go 1.27 modernisation (refactor spec)

Branch: `chore/go-1.27-modernise`, cut from main at 9238e1f4. Measurements below were taken at that commit.

## 1. Purpose and scope

Olla builds with Go 1.27 (`go 1.27.0`, `toolchain go1.27.4`, golangci-lint v2.15.0 from the repo-local tool directory). Most of the code was written for Go 1.21 to 1.24 and predates the idioms 1.22 to 1.27 made standard. This spec adopts them in single-category PRs, mostly by tool, and then enables the `modernize` linter so the backlog cannot regrow.

**This spec authorises no behaviour change.** A diff that changes output, ordering, the JSON wire format or an exported `pkg/` signature is out of scope for the PR that contains it. This is not a `/debt` item, so there is no register entry to pay down.

### Measurement that changes

| Measure | Command (run from the repo root) | Before | After |
|---|---|---|---|
| modernize findings, whole repo | `./.bin/golangci-lint run --enable-only modernize ./...` | 1,416 (339 non-test) | 0, apart from the analyzers on the deliberate disable list (FR-24) |
| `go fix` pending hunks | `go fix -diff ./...` | 662 hunks, 177 files (24 analyzers) | only the analyzers on the disable list remain |
| usetesting findings in tests, exclusion lifted | `./.bin/golangci-lint run --enable-only usetesting ./...` with the `_test.go` exclusion for `usetesting` removed | 722 | 0 |
| SA1029 | `./.bin/golangci-lint run --enable-only staticcheck ./...` with `-SA1029` removed | suppressed | 0 findings, suppression gone |

Per analyzer (lint total / non-test): `any` 1,164/254; `newexpr` 98/2; `waitgroupgo` 35/2; `errorsastype` 32/28; `slicescontains` 20/11; `mapsloop` 18/9; `minmax` 11/11; `embedlit` 11/0; `stringsseq` 6/5; `stringscutprefix` 6/4; `atomictypes` 5/5; `stringscut` 4/2; `reflecttypefor` 2/2; `stringsbuilder`, `stditerators`, `slicessort`, `omitzero` 1 each. `go fix` hunks: `any` 432; `newexpr` 51; `slicescontains` 37; `mapsloop` 28; `errorsastype` 26; `atomictypes` 18; `minmax` 8; `embedlit` 7; `stringsseq` 6; `stringscutprefix` 4; `stringsbuilder` 4; `stringscut` 3; `testingcontext` 3; `forvar` 2; `reflecttypefor` 2; `slicessort` 2; `omitzero` 1; `stditerators` 1.

### Out of scope (deferred, not requirements)

- Swapping json-iterator or `encoding/json` for `encoding/json/v2`.
- An `omitempty` to `omitzero` sweep (a wire-format change).
- `b.Loop` and `testing/synctest` sweeps. synctest is only for a specific test the user nominates later.
- Range-over-func iterators, `unique`, `weak` and ServeMux pattern adoption as a sweep.
- `Server.MaxHeaderValueCount` adoption (a separate security decision).
- Any Sherpa feature work.
- Changes to `web/` or user docs, beyond a release-notes line where Decision D4 calls for one.
- Follow-ups from the last release (query-string API keys in logs, makefile tool-gover with spaces in paths and others): tracked elsewhere.

## 2. Decisions

- **D1 (settled): PR split and order.** PR 1, 2, 3a to 3d, 4, 5, 6 as in section 8. One category per PR so a reviewer can accept it by skimming.
- **D2 (default, user may overrule): benchmark baseline required before PR 2 and PR 4**, scoped to the touched hot packages (stats, proxy core, balancer). Before and after go in the PR body. `benchstat` is used only if already installed; never installed.
- **D3 (default, user may overrule): these analyzers stay off:** `slicesdelete` and `appendclipped` (not nil-preserving), `bloop` (perturbs benchmarks), and the `omitempty` to `omitzero` conversion (`omitzero`, a JSON wire-format change).
- **D4 (default, user may overrule): Go 1.27 runtime behaviours are verification items, not code changes.** `encoding/json` now runs on v2 internals (rejects invalid UTF-8 and duplicate keys; error text may differ) and `Response.Body.Close` drains unread bytes. They are checked in the `/olla-validate --quick` runs, with a release-notes line only if the behaviour is observable to users.
- **D5 (settled, no work):** CLAUDE.md already states Go 1.27; nothing uses `httputil.ReverseProxy` or `Director`; container-aware GOMAXPROCS and the Green Tea GC need no action.
- **D6 (settled):** implementation by Sonnet agents; the user merges via GitHub; reviews are Fable (or Opus) plus Codex, never Sonnet.

## 3. Functional requirements

Each requirement is a behaviour-preserving change. "Zero findings" means the analyzer's finding count from the commands above is 0 after the package.

- **FR-1 (PR 1).** Every `interface{}` is rewritten to `any` using `go fix -any ./...`. The change is a pure rename: no other token changes. `any` findings go to 0 (from 1,164 lint findings; 1,181 `interface{}` occurrences).
- **FR-2 (PR 2).** The analyzers `slicescontains`, `mapsloop`, `minmax`, `stringscut`, `stringscutprefix`, `stringsseq`, `errorsastype`, `reflecttypefor`, `stditerators`, `slicessort`, `embedlit`, `forvar` and `testingcontext` are applied with `go fix -<name> ./...`, one analyzer per commit, each commit reproducible by re-running that command on its parent. Each analyzer's finding count goes to 0.
- **FR-3 (PR 2).** `atomictypes`, `newexpr`, `omitzero` and `stringsbuilder` are not applied in PR 2. `stringsbuilder` is not in the brief's PR split; it stays out of every PR and goes on the disable list (see Assumption A3).
- **FR-4 (PR 3a).** `sync.WaitGroup.Go` replaces `wg.Add(1)` plus `go func(){ defer wg.Done() ... }()` at all 35 `waitgroupgo` sites (2 non-test), applied with `./.bin/golangci-lint run --enable-only modernize --fix` restricted to `waitgroupgo` (`go fix` has no such analyzer). Findings go to 0.
- **FR-5 (PR 3b).** The `usetesting` exclusion for `_test.go` files is lifted in the lint config, and the 722 sites convert to `t.Context()` or `b.Context()` (benchmarks need `b.Context()`), with `os.Setenv` in tests becoming `t.Setenv` and the remaining `tc := tc` copies removed. Findings go to 0.
- **FR-6 (PR 3b).** A converted context used inside a `t.Cleanup` callback is not converted, because `t.Context()` is cancelled before cleanups run. Such sites keep an uncancelled context and carry a one-line comment saying why.
- **FR-7 (PR 3c).** The pointer helpers are replaced by `new(expr)` at their call sites and the helpers are deleted: the non-test helper in config (`ptrInt`) and the test helpers (`ptrInt`, `strPtr`, `ptrString`, `ptrTime` in the discovery and unifier tests). No `//go:fix inline` directive is left behind. `newexpr` findings go to 0.
- **FR-8 (PR 3d).** `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` and `sort.Sort` calls (27 non-test sites, 17 in handlers) become `slices.Sort`, `slices.SortFunc` or `slices.SortStableFunc` with `cmp.Compare`. `sort.SliceStable` maps to `slices.SortStableFunc`. Where a comparator is not a total order, ties keep their current relative order: use the stable variant if any test or consumer can observe tie order.
- **FR-9 (PR 3d).** The balancer priority sort (per-request path) is converted only if the before and after balancer benchmark shows no regression beyond noise; otherwise it is left on `sort.Slice` and recorded in the PR body.
- **FR-10 (PR 4).** Counters accessed through `atomic.AddInt64`, `LoadInt64`, `CompareAndSwapInt64` and siblings move to `atomic.Int64` or `atomic.Uint64` in the stats collectors (collector, model collector, endpoint data), the proxy core counters, the discovery HTTP client metrics and the round-robin balancer counter. All call sites of a field move together in one commit per package.
- **FR-11 (PR 4).** Snapshot and result structs, and anything serialised or returned to callers, remain plain integer types; only the live counter fields change type. No struct holding an atomic type is copied, compared or returned by value (`go vet` copylocks clean).
- **FR-12 (PR 5).** `modernize` is enabled in the lint config with every analyzer on by default and an explicit `disable` list holding only the analyzers deliberately skipped (D3 plus `stringsbuilder`), each with a reason comment. PR 5 lands only after PRs 1 to 4.
- **FR-13 (PR 6).** The context-storage role gets a typed key (type `contextKey string`, the type the sticky-session keys already use), separate untyped string constants serve the log-attribute and InspectionMeta roles, and accessor helpers wrap every read and write. Reading by raw string must not remain anywhere. The SA1029 exclusion in the lint config is removed.
- **FR-14 (PR 6).** Every context-key reader, including the `util.StripRoutePrefix` route-prefix lookup and each handler, middleware, logger and router reader (19 non-test `context.WithValue` call sites), has a test proving the value written is the value read.
- **FR-15 (all).** Public API in `pkg/` (container, eventbus, pool, format, nerdstats, profiler) is unchanged; all `pkg/` findings are test-only. Sherpa receives mechanical rewrites only.
- **FR-16 (all).** No dependency is added and nothing is installed with `go install`. Tools come from the repo-local tool directory via the makefile; the shared GOPATH bin is never written.

## 4. Non-functional requirements

- **NFR-1.** Hot paths (the proxy engines, balancer, stats, and proxy core retry and common) show ns/op, B/op and allocs/op no worse than run-to-run noise in the before and after benchmarks of PR 2, 3d and 4. "Noise" is the spread between two baseline runs taken on the same machine in the same session; the PR body states both baseline runs and the after run.
- **NFR-2.** Struct layout changes pass `make align` (betteralign) with no suppression added.
- **NFR-3.** PRs touching concurrency (3a, 4) pass `make test-race`.
- **NFR-4.** Heavy commands run in the capped slice, one heavy suite at a time across all agents.
- **NFR-5.** Commits are humanised one-liners, one coherent change each, targeted package tests per commit, `make ready` once on each PR's final commit.

## 5. Existing capability and reuse

Reuse map, verified against the repo: this spec adds no new component. It uses existing tools and extends existing configuration.

| Need | Existing component | Extension and effect on existing callers |
|---|---|---|
| Mechanical rewrites | `go fix` (24 analyzers on go1.27.4); `go fix -diff` for read-only checks; `go tool fix help` lists analyzers | Used as is. No callers affected. |
| `waitgroupgo` | golangci-lint `modernize` linter (v2.15.0, repo-local, `--fix`) | `go fix` lacks this analyzer, so the linter is the only tool. Used with `--enable-only modernize` for the fix, so other linters are not touched. |
| Lint guard | Lint config (`linters.enable`, `settings`, `exclusions`); `intrange`, `copyloopvar`, `usetesting` already enabled | PR 5 adds `modernize` to `linters.enable`. CI runs the same linter version through the action, so enabling it applies there with no workflow edit. PR 3b removes `usetesting` from the `_test.go` exclusion rule only (other linters in that rule stay excluded). PR 6 removes `-SA1029` from the staticcheck checks. |
| Benchmarks | makefile targets `bench`, `bench-balancer`, `bench-repo`; package benchmarks in stats, proxy core, balancer | Used for baselines. `bench` runs everything, so baselines use package-scoped `go test -bench=. -benchmem` on the touched hot packages instead. No makefile change. |
| Context keys | The sticky-session keys already use a typed `contextKey`; the core constants package holds the other keys as untyped strings | PR 6 extends the typed pattern to the remaining keys. Existing readers that rely on the shared untyped constants are the near miss: typing the constants in place would break their slog, InspectionMeta and `util.StripRoutePrefix` uses. So the spec splits roles (typed storage key plus separate string constants) instead of retyping. |
| Pointer helpers | `ptrInt` in config and test helpers | Removed, not extended. `new(expr)` is a language feature. |
| End-to-end proof | `/olla-validate --quick` (ollamock fleet plus two Olla instances) | Used unchanged for PR 2, 4 and 6. |
| Test suites that cover the seams | `make ready` (short tests, race, fmt, vet, lint, align); package suites for stats, proxy core, balancer, discovery, registry, handlers, middleware, router, logger, config, unifier | Unchanged and must pass unchanged. Cases move only where a deleted helper required it; none are dropped. |

## 6. Data and interfaces

No schema, API, config-key or wire-format change. The only interfaces touched are Go-internal:

- PR 4: unexported counter fields change to atomic types. Exported snapshot structs keep their field types and JSON tags.
- PR 6: the constants package gains a typed key and accessor helpers; the existing untyped string constants remain for the log and metadata roles. The shape of the decision:

```go
type contextKey string // storage role only; never used as a slog or metadata key
```

  Accessor helpers take and return `context.Context` and the value type, so call sites stop naming keys.
- The `pkg/` exported surface is unchanged (FR-15).

## 7. Consequences

| What it is | What in this spec changes it | Handling |
|---|---|---|
| Lint config: `modernize` enable plus disable list | FR-12 | PR 5 |
| Lint config: `usetesting` in the `_test.go` exclusion rule | FR-5 | PR 3b |
| Lint config: `-SA1029` and its comment explaining the shared constants | FR-13 | PR 6 |
| Lint config: the comment on the context constants being plain strings on purpose is made untrue | FR-13 | PR 6 (same edit as the removal) |
| CLAUDE.md Development Guidelines: optionally one line, "modernize linter enforced" | FR-12 | PR 5 adds the one line. It is the only lines-of-guidance change; the Go version text is already correct. |
| docs/content/about/release-notes.md | D4 | Deferred: a line is added only if the `/olla-validate --quick` run finds user-observable JSON or body-drain behaviour. Pure refactors need no entry. The section name depends on the next version (Open question Q1). |
| docs/content/development/{contributing,patterns,benchmarking}.md contain `interface{}` in sample code | FR-1 | Deferred: user docs are out of scope for this spec, and the samples remain valid Go. Candidate for a docs follow-up. |
| docs/content/development/{setup,contributing}.md Go version text | none | Already correct (Go 1.27); no row action. |
| makefile bench targets | D2 | None: package-scoped `go test -bench` is used for baselines. |
| CI workflow lint step | FR-12 | None: the action uses the same golangci-lint version, so enabling `modernize` applies automatically. |
| `.claude/skills/olla-validate` | none | None: the harness is used unchanged. |
| Anything calling the removed pointer helpers or retyped counters | FR-7, FR-10 | Handled inside PR 3c and PR 4 (compile errors force it; no exported use outside the packages, `pkg/` has no hits). |

Searches run: grep for `modernize`, `usetesting` and `SA1029` across the lint config, makefile and `.github/workflows`; grep for Go version text across CLAUDE.md and `docs/content/development`; `interface{}` across `docs/content`; listing of `.claude/skills`. No hooks or `.claude` configuration reference the touched identifiers. No flow is made redundant, so no `remove` package is needed.

## 8. Acceptance criteria and work packages

Every package ends with `make ready` on its final commit (NFR-5). "Reproducible" means: check out the PR's parent, run the stated command, and `git diff` against the PR's commit is empty (apart from a whitespace-only `make fmt` delta in the same commit, noted in the PR body, since Go 1.27 gofmt alignment shifts).

Run-order: baseline, PR 1, PR 2, then PR 3a to 3d and PR 4 in any order, then PR 5, then PR 6. PR 4 may go before PR 3 if the stats benchmarks are wanted early.

### WP-0. Benchmark baseline (no PR; precondition of PR 2 and PR 4, per D2)

Mode: not applicable (measurement only). Specialist: test-architect.
- Run `make ready` on the base once as a green-baseline check (unrecorded as a PR).
- Run package-scoped `go test -bench=. -benchmem` on stats, proxy core, the balancer and the proxy engines, twice, to expose noise. Save outputs in the run's evidence directory; the PR 2, 3d and 4 bodies cite them.
- Criterion: both baseline runs exist for every touched hot package, seam: CLI invocation. `benchstat` is used only if already installed.

### WP-1. PR 1: `any` only

Mode: `refactor`. Specialist: Go (go-principal-architect). Depends on: nothing.
- Apply `go fix -any ./...`. Sherpa and tests included.
- AC-1: `./.bin/golangci-lint run --enable-only modernize ./...` reports 0 `any` findings (seam: CLI).
- AC-2: re-running `go fix -any ./...` on the base reproduces the diff exactly, and a follow-up `go fix -diff -any ./...` is empty.
- AC-3: the diff contains only `interface{}` to `any` changes plus gofmt alignment (check: every changed non-whitespace line differs only by that token).
- AC-4: `go build ./...` passes and existing suites pass unchanged. No benchmarks needed.

### WP-2. PR 2: remaining mechanical `go fix` analyzers

Mode: `refactor`. Specialist: Go. Depends on: WP-0, WP-1.
- One commit per analyzer, in the FR-2 list, test and non-test files together.
- AC-1: for each analyzer, `go fix -diff -<name> ./...` is empty after its commit and its lint count is 0; the commit reproduces from its parent.
- AC-2: review checklist recorded in the PR body per analyzer: `errorsastype` (the target variable is not used after the `if`); `slicescontains` (search operands have no side effects, since evaluation count changes from per element to once); `mapsloop` (nil-ness preserved: `maps.Clone` of a nil map is nil, `maps.Copy` into an existing map is exact; any case that would turn nil into empty is left as it was and listed); `minmax` (no floats, already skipped by the tool).
- AC-3: before and after benchmarks for stats and proxy core (retry, common) in the PR body, within NFR-1.
- AC-4: `make test-race` is not required here; `/olla-validate --quick` passes (request-path code, seam: agent-driven end-to-end), and the run also records the D4 JSON-v2 and body-drain observations.
- AC-5: existing package suites pass unchanged.

### WP-3a. PR 3a: `sync.WaitGroup.Go`

Mode: `refactor`. Specialist: Go. Depends on: WP-1 (any order against 3b to 3d).
- Apply the linter fix limited to `waitgroupgo`, then review each site by diff.
- AC-1: `./.bin/golangci-lint run --enable-only modernize ./...` reports 0 `waitgroupgo` findings.
- AC-2: no `wg.Add` remains paired with a rewritten `go` statement, and no closure captures a loop variable incorrectly (diff review).
- AC-3: `make test-race` passes (seam: existing race suite, including the eventbus tests).
- AC-4: the diff is reproducible by re-running the linter fix on the base.

### WP-3b. PR 3b: test-only sweep

Mode: `refactor`. Specialist: test-architect. Depends on: WP-1. Droppable without blocking others.
- Lift `usetesting` from the test exclusion; convert per package (anthropic, discovery, balancer, registry, inspector, unifier, security, handlers and the rest).
- AC-1: lint with the exclusion lifted reports 0 `usetesting` findings (seam: CLI).
- AC-2: `go test -race` passes for every converted package; each `t.Cleanup` that used the context is reviewed and kept uncancelled with a comment (FR-6).
- AC-3: the number of test cases and their assertions is unchanged: `go test -list '.*' ./...` output before and after is identical.
- AC-4: no goroutine-leak or hang regression: the full short test suite wall time does not grow materially (recorded, not gated).

### WP-3c. PR 3c: `newexpr`

Mode: `refactor`. Specialist: Go. Depends on: WP-1.
- AC-1: 0 `newexpr` findings; the helpers named in FR-7 no longer exist (`grep` for each name returns nothing in the repo).
- AC-2: no `//go:fix inline` directive remains.
- AC-3: config tests and discovery and unifier suites pass unchanged; `pkg/` exported API is untouched.

### WP-3d. PR 3d: `sort.*` to `slices`

Mode: `refactor`. Specialist: Go. Depends on: WP-0, WP-1.
- AC-1: no `sort.Slice`, `sort.SliceStable`, `sort.Strings`, `sort.Ints` or `sort.Sort` call remains in non-test code, except sites recorded in the PR body as deliberately left (priority balancer sort, per FR-9).
- AC-2: every converted comparator is shown in the PR body to be a total order or to use the stable variant; handlers and stats outputs keep their ordering (existing handler tests pass unchanged; a case is added only where tie order was unpinned and observable).
- AC-3: `make bench-balancer` and the stats percentile benchmarks before and after are within NFR-1.

### WP-4. PR 4: `atomictypes`

Mode: `refactor`. Specialist: Go. Depends on: WP-0, WP-1 (WP-2 recommended first, since it edits adjacent lines in the stats collector).
- AC-1: 0 `atomictypes` findings and no `atomic.AddInt64`, `LoadInt64`, `CompareAndSwapInt64` or `AddUint64` call on a struct field remains in the five named areas; `go fix -diff -atomictypes ./...` is empty.
- AC-2: `go vet ./...` (copylocks) and `make align` pass; exported snapshot structs and their JSON tags are unchanged (`go test` on the stats and discovery handlers plus a diff check of exported types).
- AC-3: `make test-race` passes; `/olla-validate --quick` passes.
- AC-4: before and after benchmarks for the stats collectors, proxy core and engines, and the round-robin balancer are within NFR-1.

### WP-5. PR 5: enable `modernize`

Mode: `extend` (config only). Specialist: Go. Depends on: WP-1, WP-2, WP-3a to WP-3d, WP-4.
- AC-1: `./.bin/golangci-lint run ./...` is clean with `modernize` enabled (seam: CLI, the same command CI runs).
- AC-2: the disable list contains exactly `slicesdelete`, `appendclipped`, `bloop`, `omitzero` and `stringsbuilder`, each with a reason comment (D3, A3). Nothing else is disabled.
- AC-3: a throwaway file with `interface{}` fails lint locally, proving the guard fires (not committed; result in the evidence summary).
- AC-4: CLAUDE.md gains the single line on the enforced linter (Consequences row).

### WP-6. PR 6: SA1029 typed context keys

Mode: `refactor` (no behaviour change), preceded by a characterisation commit inside the same PR. Specialist: Go with test-architect for the tests. Depends on: WP-5 (so the linter guards new code), never combined with a mechanical PR.
- Characterisation first: before any code moves, a commit adds tests at every context-key reader that pin current behaviour (value written is value read) and lands green; the run is recorded in the PR body.
- AC-1: the SA1029 exclusion is removed and the staticcheck run reports 0 SA1029 findings.
- AC-2: no `context.WithValue` or `ctx.Value` site uses a raw string key (`grep` returns none); all go through the accessors.
- AC-3: the tests in FR-14 pass before and after the key split without edits to their assertions; each reader of the 19 non-test sites is covered, and route-table-wide cases loop the registered route table rather than hitting one route (provider routes, `/olla/proxy/`, translator routes).
- AC-4: slog output keys, InspectionMeta keys and `util.StripRoutePrefix` behaviour are byte-identical (existing suites plus the new tests).
- AC-5: `/olla-validate --quick` passes.

## 9. Risks implementers must be briefed on

copylocks with atomic types; struct layout and betteralign; sort tie order; `slicescontains` evaluating side-effecting operands once; `mapsloop` nil-ness; `errorsastype` where the variable is used after the `if`; `t.Context()` cancelled before `t.Cleanup` runs; gofmt 1.27 alignment shifts (accept whitespace-only diffs in files touched, in the same commit); running `go fix` without `-diff` writes files, so apply on a clean branch and review each result with `git diff`.

## 10. Assumptions

- **A1.** The research numbers are taken as the measurement; the spec restates the commands so the implementer re-measures at the base of each PR (counts may drift if main moves).
- **A2.** WP-0 is a measurement step, not a PR, because the brief reduces the old prep PR to the benchmark baseline. Rejected: a PR with no diff.
- **A3.** `stringsbuilder` and `omitzero` have 1 finding each and are not in the brief's PR split. `omitzero` is off by D3. `stringsbuilder` is also left off (disable list) because its change is not a category in the split; the alternative, adding it to PR 2, was rejected to keep to the brief.
- **A4.** The brief's single `go fix` finding for `slicessort` and the `sort.*` sweep (PR 3d) overlap on one site; the `go fix` commit in PR 2 takes it and PR 3d skips it.
- **A5.** PR 6 is typed `refactor` with an internal characterisation commit, since the repo already covers few readers directly and the guidance for coverage below what a refactor needs is characterisation first. The alternative, a separate preceding test PR, was rejected as the tests and the change review best together.
- **A6.** PR 4 follows PR 2 where both touch the stats collector, to avoid conflicts; the brief allows any order, this is a preference only.
- **A7.** The CLAUDE.md "modernize linter enforced" line is added in PR 5 (the brief calls it optional).
- **A8.** Docs samples using `interface{}` are not rewritten (user docs out of scope).
- **A9.** The brief states CLAUDE.md already states Go 1.27 and the repo confirms it, so no CLAUDE.md version edit is required.

## 11. Open questions

- **Q1.** What is the next version after v0.2.0? Only needed if D4 produces a release-notes line (section heading in the release notes page). Not blocking; pure refactors need no entry.
- **Q2.** Do the user want to overrule D2 to D4 (baseline, analyzer-off list, runtime verification items)? Spec proceeds on the defaults.

## 12. Implementation handoff

This spec is implemented by running `/orchestrate docs/specs/go-1.27-modernise.md` in a fresh session. That session is the orchestrator, not an implementer: it decomposes this spec into the work packages above, dispatches each to the matching specialist subagent, reviews their reports against the acceptance criteria, and iterates fixes back to the same agent. It never implements, explores or debugs in its own context beyond trivial glue; anything that needs reading or writing code is delegated so the orchestrator's context stays lean and its judgement stays fresh for review. Bug-shaped packages go through bug-investigator before a specialist touches code.

Anything found wrong along the way is a bug report for `/bug-hunt`, not a fix inside this spec.
