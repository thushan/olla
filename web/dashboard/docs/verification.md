# Dashboard verification pass

This document captures the manual verification done for the four poll-store states
(the copied `poll-store.svelte.js` factory, WP-1a) and the other acceptance criteria
from WP-1a/WP-3 (see `docs/spec/simple-dashboard.md` §7) that aren't covered by
automated tests.

## Setup

The verification was run against `bun run build` + `bun run preview`, with Playwright's
`page.route()` intercepting the three `/internal/status*` endpoints so each state could be
reproduced deterministically without standing up a real Olla backend. Mocks used the
shapes documented in `internal/app/handlers/handler_status*.go` (post WP-1b / WP-2).

## The four poll states

### `loading` — initial mount, no data yet

Triggered by intercepting every request and never resolving it (mock `fetch` returns a
permanently-pending promise). The first paint after mount shows the pulsing skeleton
tiles in the Overview panel; the status strip renders dashes for every field.

Visually: 8 grey placeholder rectangles where the stat tiles will appear, plus 4 thin
skeleton rows where the fleet-at-a-glance will appear. No error banner.

### `ok` — most recent poll succeeded

Triggered by mocking all three endpoints with valid JSON. Stat tiles populate, fleet
glance list appears with status glyphs and success bars, status strip shows live values.
Endpoints panel renders the sortable table; Models panel renders grouped by family.

Visually: full data, green/amber/red status glyphs, accent-coloured accent borders on
the active tab.

### `error` — last poll failed, last success within 3*interval

Triggered by intercepting `/internal/status` to succeed on the first poll then return
`503` on subsequent polls. Within the first 15 seconds after the first success, the
banner is the error variant (red background, "Couldn't reach Olla's overview status API
— retrying"). The prior data stays rendered, greyed via the `[data-state="error"]` CSS
rule. A "retry now" button is in the banner.

Visually: red banner, dimmed but populated tiles, error message names which store is
affected.

### `stale` — last success older than 3*interval

Triggered by leaving the failure mock in place for ~17 seconds (overview interval is 5s;
3*5 = 15s threshold). Once the threshold is crossed the banner escalates to the stale
variant (amber background, longer copy: "Olla's overview status has been unreachable for
a while — showing the last good snapshot, retrying"). Data remains rendered but greyed.

Visually: amber banner, dimmed data, "last ok" timestamp in the banner's meta line.

## Other acceptance criteria verified manually

### Left-nav future-proofing

`grep -rn "navigation.svelte" web/dashboard/src` shows `navigation.svelte.js` imported
only from `NavTabs.svelte` (and `App.svelte` imports the module path for side-effect
registration of stores that don't touch navigation). `App.svelte` does read
`navigation.current` to choose which panel to render, but it does not import the
navigation store; it accesses it indirectly through the panel components. The active
tab state is owned by NavTabs alone.

If that grep ever gains another hit, the structural guarantee is broken.

### Single poll scheduler

`grep -rn "setInterval\|setTimeout" web/dashboard/src` returns matches only in
`lib/poll-scheduler.js` (and one in `layout/Header.svelte` for the 1s clock tick, which
is not a poll). Three poll stores, one scheduler, as specified.

### `If-None-Match` / `304` handling

`lib/stores/poll-store.svelte.js` sends `If-None-Match` whenever it has a prior ETag
and treats `304 Not Modified` as a successful poll that bumps `lastUpdated` without
touching `data`. This machinery is deliberately dormant on this branch (spec §4.1):
the JSON status endpoints never send an `ETag`, so the `if (etag)` gate never fires
and every poll behaves as a plain `200`. Verified by code review; runtime
verification of the conditional-GET path itself is out of scope for PR 1 and waits
on PR 2's eventual ETag re-enable.

### `prefers-reduced-motion`

Verified via `page.emulateMedia({ reducedMotion: 'reduce' })` and probing computed
styles: `animationDuration` and `transitionDuration` both collapse to `1e-06s` on
every element when the media feature is active, per the `@media (prefers-reduced-
motion: reduce)` rule in `src/app.css`.

### Keyboard tab navigation

Verified by focusing the tablist and pressing Arrow Right, End, Home in sequence:

- Initial state: Overview tab has `aria-selected="true"` and `tabindex=0`; Endpoints
  and Models have `aria-selected="false"` and `tabindex=-1` (roving tabindex).
- Arrow Right on Overview → focus moves to Endpoints, Endpoints becomes the active
  panel, tabindexes swap (Endpoints now 0, Overview now -1).
- End → focus moves to Models, Models becomes active.
- Home → focus moves back to Overview.

### WCAG AA contrast

`src/lib/contrast.test.js` computes contrast ratios for every text/background pair in
both themes and asserts each meets the WCAG AA threshold (4.5:1 for body text, 3:1 for
the `text-faint` colour reserved for non-essential labels). Run with `bun run test`.

### Sortable columns

`SortableTable.svelte` toggles `aria-sort` between `ascending` / `descending` on click
and re-sorts the body accordingly. Verified by clicking the Priority header in the
Endpoints panel and observing the aria-sort state flip from `descending` (initial) to
`ascending`.

### Build

`bun run build` produces a clean output in `dist/` (~70 KB JS gzipped to ~26 KB, ~19 KB
CSS gzipped to ~5 KB, plus three self-hosted woff2 fonts).
