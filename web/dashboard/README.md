# Olla dashboard

Read-only operator dashboard for [Olla](https://thushan.github.io/olla/), the LLM proxy and
load balancer. Built with Svelte 5 (runes), Vite and Tailwind CSS. The production build
output (`dist/`) is embedded into the Go binary via `go:embed`. The embed source is
gitignored and regenerated at build time by `make build-web` (run automatically by the
build/release targets); this package owns only the SPA source.

## What it shows

Three panels, each backed by an independent poll against Olla's `/internal/status*` JSON
endpoints:

| Panel     | Source                              | Interval |
| --------- | ----------------------------------- | -------- |
| Overview  | `/internal/status`                  | 5 s      |
| Endpoints | `/internal/status/endpoints`        | 5 s      |
| Models    | `/internal/status/models` (detailed) | 15 s     |

The panels never mutate state. There is no auth in this layer; access control is network-
layer only (see `docs/spec/simple-dashboard.md` §3, FR-11/FR-12).

## Quickstart

Requires Bun 1.1 or newer.

```bash
cd web/dashboard
bun install
bun run dev          # vite dev server on http://localhost:5173/internal/ui/
bun run build        # outputs dist/ (consumed by go:embed)
bun run test         # format byte-match + WCAG AA contrast tests
bun run preview      # serve the built dist/ locally
```

`make build-web` regenerates `internal/app/handlers/dashboard/dist/` (the gitignored embed
source) from this tree. The build and release targets run it automatically before compiling,
so a shipped binary always carries the current SPA. Run it manually only when iterating on
the Go side while you also want a fresh dashboard embedded.

## Architecture

Component tree, polling model, formatting and theming are specified verbatim in
`docs/spec/simple-dashboard.md` §5 (reuse plan) and §5.1 (frontend architecture notes).
The implementation lives here:

```
src/
  App.svelte                       mounts the layout, starts/stops the scheduler
  main.js                          entry, imports global styles
  app.css                          design tokens (ported from the v2-terminal-dense mockup)
  components.css                   shared component styles (ported from the mockup)
  layout/
    DashboardLayout.svelte         shell: header, status strip, nav region, main, footer
    Header.svelte                  brand, clock, ThemeToggle
    StatusStrip.svelte             compact system summary (always visible)
    NavTabs.svelte                 SOLE consumer of navigation.svelte.js
  panels/
    OverviewPanel.svelte           stat tiles + fleet-at-a-glance
    EndpointsPanel.svelte          sortable table of every backend
    ModelsPanel.svelte             sortable table grouped by family
  components/
    StatTile, StatusTag, RangeBar, PctBar, SortableTable, ThemeToggle, StatusBanner
  lib/
    poll-scheduler.js              the ONLY module that owns setInterval/setTimeout
    format.js                      fmtBytes/fmtMs/fmtPct/fmtInt, mirrors pkg/format.Bytes
    format.test.js                 byte-for-byte table-driven test
    contrast.test.js               WCAG AA contrast checks for both themes
    stores/
      poll-store.svelte.js         shared factory: data/status/lastUpdated/error getters
      overview.svelte.js
      endpoints.svelte.js
      models.svelte.js
      theme.svelte.js              auto/light/dark, persisted
      navigation.svelte.js         current panel (consumed only by NavTabs)
```

### Poll states

Every store exposes four visibly-distinct states, per spec §7.3:

| State   | When                                                  | Rendering                                                 |
| ------- | ----------------------------------------------------- | --------------------------------------------------------- |
| loading | first mount, no data yet                              | pulsing skeleton tiles/rows                               |
| ok      | most recent poll succeeded (200 or 304)               | normal render                                             |
| error   | most recent poll failed, but last success < 3*interval | red inline banner, prior data greyed but still visible  |
| stale   | last success older than 3*interval (two failures)     | amber banner with stronger copy, data greyed, dots stale  |

See [`docs/verification.md`](./docs/verification.md) for the manual pass that exercises each.
