# Olla dashboard

Read-only operator dashboard for [Olla](https://thushan.github.io/olla/), the LLM proxy and
load balancer. Built with TypeScript, Svelte 5 (runes), Vite and Tailwind CSS. The
production build output (`dist/`) is embedded into the Go binary via `go:embed`. The embed
source is gitignored and regenerated at build time by `make build-web` (run automatically by
the build/release targets); this package owns only the SPA source.

## What it shows

Three panels, each backed by an independent poll against Olla's `/internal/status*` JSON
endpoints:

| Panel     | Source                              | Interval |
| --------- | ----------------------------------- | -------- |
| Overview  | `/internal/status`                  | 5 s      |
| Endpoints | `/internal/status/endpoints`        | 5 s      |
| Models    | `/internal/status/models` (detailed) | 15 s     |

Overview also renders a live requests-per-second sparkline (`SparkStrip`), built from a
rolling history of poll samples, with a hover readout and visible outage markers when a
poll fails. Endpoints and Models support click-through: clicking a model's host pill jumps
to and highlights that endpoint's row, and the current panel plus any jumped-to endpoint is
reflected in the URL hash so a refresh or shared link restores the same view. Models also
shows any configured aliases under each model name.

The panels never mutate state. There is no auth in this layer; access control is network-
layer only (allowed CIDRs plus an allowed-hosts check, with IP-literal Hosts always
accepted since DNS rebinding needs a resolved hostname).

## Quickstart

Requires Bun 1.1 or newer.

```bash
cd web/dashboard
bun install
bun run dev          # vite dev server on http://localhost:5173/internal/ui/
bun run build        # outputs dist/ (consumed by go:embed)
bun run test         # unit tests: format byte-match, WCAG AA contrast, store/router logic
bun run check        # svelte-check, type-checks the whole SPA
bun run preview      # serve the built dist/ locally
```

`make build-web` regenerates `internal/app/handlers/dashboard/dist/` (the gitignored embed
source) from this tree. The build and release targets run it automatically before compiling,
so a shipped binary always carries the current SPA. Run it manually only when iterating on
the Go side while you also want a fresh dashboard embedded.

## Architecture

Component tree, polling model, formatting and theming for the implementation. Every source
file is TypeScript (`.ts`, or `.svelte.ts` for rune-based modules); there is no plain
JavaScript left in `src/`.

```
src/
  App.svelte                       mounts the layout, starts/stops the scheduler, hosts the router
  main.ts                          entry, imports global styles, mounts App
  app.css                          design tokens (ported from the v2-terminal-dense mockup)
  components.css                   shared component styles (ported from the mockup)
  vite-env.d.ts                    Vite ambient types
  layout/
    DashboardLayout.svelte         shell: header, status strip, nav region, main, footer
    Header.svelte                  brand, clock, ThemeToggle
    StatusStrip.svelte             compact system summary (always visible)
    NavTabs.svelte                 SOLE consumer of navigation.svelte.ts
  panels/
    OverviewPanel.svelte           stat tiles, fleet-at-a-glance, SparkStrip
    EndpointsPanel.svelte          sortable table of every backend, deep-link target
    ModelsPanel.svelte             sortable table grouped by family, aliases, endpoint pills
  components/
    StatTile, StatusTag, RangeBar, PctBar, SortableTable, SparkStrip, ThemeToggle, StatusBanner
  lib/
    types.ts                       shared response/view-model types
    dom-id.ts                      stable per-row DOM id derivation
    router.ts                      URL hash router: panel + optional endpoint deep-link
    jump-to-endpoint.ts            shared "scroll to and flash a row" helper for click-through
    poll-scheduler.ts              the ONLY module that owns setInterval/setTimeout
    format.ts                      fmtBytes/fmtMs/fmtPct/fmtInt, mirrors pkg/format.Bytes
    clock.svelte.ts                ticking clock for the header
    stores/
      poll-store.svelte.ts         shared factory: data/status/lastUpdated/error getters
      overview.svelte.ts
      endpoints.svelte.ts
      models.svelte.ts
      history.svelte.ts            ring buffer of overview samples feeding SparkStrip
      theme.svelte.ts              auto/light/dark, persisted
      navigation.svelte.ts         current panel (consumed only by NavTabs)
```

Tests sit alongside the modules they cover (`*.test.ts`), not in a separate tree.

### Poll states

Every store exposes four visibly-distinct states:

| State   | When                                                  | Rendering                                                 |
| ------- | ----------------------------------------------------- | --------------------------------------------------------- |
| loading | first mount, no data yet                              | pulsing skeleton tiles/rows                               |
| ok      | most recent poll succeeded (200 or 304)               | normal render                                             |
| error   | most recent poll failed, but last success < 3*interval | red inline banner, prior data greyed but still visible  |
| stale   | last success older than 3*interval (two failures)     | amber banner with stronger copy, data greyed, dots stale  |

The server side backs this with `ETag`/`If-None-Match` conditional requests and gzip
compression on the status JSON endpoints, so a healthy poll cycle is a cheap `304` most of
the time rather than a full payload re-fetch.
