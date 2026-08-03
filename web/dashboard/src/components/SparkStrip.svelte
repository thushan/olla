<script module lang="ts">
  // Pure helper exported so the pointer-to-index mapping is unit-testable
  // without a DOM. Maps a 0..1 fraction (pointer x relative to chart width)
  // to the index of the nearest sample in an n-element series.
  export function nearestSampleIndex(fraction: number, n: number): number {
    if (n <= 0) return -1;
    if (n === 1) return 0;
    const clamped = Math.max(0, Math.min(1, fraction));
    return Math.round(clamped * (n - 1));
  }
</script>

<script lang="ts">
  // Hand-rolled inline-SVG sparkline with a req/s y-axis scale and a hover
  // reading of the sample under the pointer. No chart library (hard rule).
  // The strip samples once per overview poll tick by keying an $effect off
  // overview.lastUpdated, which advances on BOTH the 200 and 304 paths (see
  // poll-store.svelte.ts onSuccess). On a 304 the data object is unchanged,
  // so the computed delta is naturally zero: the x-axis stays real time.

  import { untrack } from 'svelte';
  import { overview } from '../lib/stores/overview.svelte';
  import { history, type Sample } from '../lib/stores/history.svelte';
  import { fmtInt, fmtDuration } from '../lib/format';

  interface Props {
    // Injectable for tests; production reads the singleton store.
    samples?: Sample[];
  }
  let { samples }: Props = $props();

  const view = $derived<Sample[]>(samples ?? history.samples);

  // Sampling hook. untrack prevents the push inside history.append from
  // adding history.samples as a dependency of this effect (write loop).
  $effect(() => {
    const lu = overview.lastUpdated;
    if (!lu) return;
    const data = overview.data;
    if (data) untrack(() => history.append(data));
  });

  // viewBox dimensions: arbitrary because preserveAspectRatio="none" stretches
  // the drawing to the container. The 100x30 box gives the path math a clean
  // range; rendered height is fixed by .spark-svg so there is no layout shift.
  const W = 100;
  const H = 30;

  const reqSeries = $derived(view.map((s) => s.reqPerSec));
  const connSeries = $derived(view.map((s) => s.activeConnections));
  // Floor at 1 so a flat-zero series still scales (avoids div-by-zero).
  const maxReq = $derived(Math.max(1, ...reqSeries));
  const maxConn = $derived(Math.max(1, ...connSeries));

  // Ceiling label for the req/s axis: one decimal under 10, integer above,
  // matching the precision of the live readout.
  const maxReqLabel = $derived(maxReq < 10 ? maxReq.toFixed(1) : Math.round(maxReq).toString());

  function areaPath(reqs: number[]): string {
    if (reqs.length < 2) return '';
    const n = reqs.length;
    const step = W / (n - 1);
    let d = '';
    for (let i = 0; i < n; i++) {
      const x = i * step;
      const y = H - (reqs[i] / maxReq) * H;
      d += i === 0 ? `M ${x} ${y}` : ` L ${x} ${y}`;
    }
    return `${d} L ${W} ${H} L 0 ${H} Z`;
  }

  function linePath(series: number[], max: number): string {
    if (series.length < 2) return '';
    const n = series.length;
    const step = W / (n - 1);
    let d = '';
    for (let i = 0; i < n; i++) {
      const x = i * step;
      const y = H - (series[i] / max) * H;
      d += i === 0 ? `M ${x} ${y}` : ` L ${x} ${y}`;
    }
    return d;
  }

  const reqArea = $derived(areaPath(reqSeries));
  const connLine = $derived(linePath(connSeries, maxConn));

  const latest = $derived(view.length > 0 ? view[view.length - 1] : null);
  // Need >=2 points to draw any line, so the warm-up hint covers the first
  // tick's lone sample even though its readout is available.
  const warming = $derived(view.length < 2);

  const ariaLabel = $derived(
    latest
      ? `${latest.reqPerSec.toFixed(1)} requests per second, ${fmtInt(latest.activeConnections)} active connections`
      : 'gathering telemetry',
  );

  // --- hover reading (progressive enhancement for pointer users) ---
  // The static readout + aria-label remain the non-pointer baseline; the
  // hover adds a historical inspection layer without removing or depending
  // on it. No keyboard coupling: the guide + tooltip are visual-only.
  let chartEl: HTMLElement | null = $state(null);
  let hoverIndex = $state<number | null>(null);

  const hoverSample = $derived(hoverIndex !== null ? view[hoverIndex] ?? null : null);
  // Guide line sits at the exact sample x; the tooltip is clamped so its
  // translateX(-50%) centre can't push it past either edge of the chart.
  const guidePct = $derived(
    hoverIndex !== null && view.length >= 2 ? (hoverIndex / (view.length - 1)) * 100 : 0,
  );
  const tooltipPct = $derived(Math.max(15, Math.min(85, guidePct)));

  function onPointerMove(e: PointerEvent): void {
    if (!chartEl || view.length < 2) return;
    // preserveAspectRatio="none" stretches viewBox coords, so map via the
    // container's real pixel rect, not the SVG's internal coordinate space.
    const rect = chartEl.getBoundingClientRect();
    if (rect.width <= 0) return;
    const fraction = (e.clientX - rect.left) / rect.width;
    hoverIndex = nearestSampleIndex(fraction, view.length);
  }

  function onPointerLeave(): void {
    hoverIndex = null;
  }
</script>

<div class="spark-strip">
  <div
    class="spark-chart"
    bind:this={chartEl}
    onpointermove={onPointerMove}
    onpointerleave={onPointerLeave}
    role="img"
    aria-label={ariaLabel}
  >
    <svg class="spark-svg" viewBox="0 0 {W} {H}" preserveAspectRatio="none" aria-hidden="true">
      {#if !warming}
        <line class="spark-grid" x1="0" y1="0" x2={W} y2="0" />
        <line class="spark-grid" x1="0" y1={H} x2={W} y2={H} />
        <path class="spark-area" d={reqArea} />
        <path class="spark-line" d={connLine} />
      {/if}
    </svg>
    {#if !warming}
      <span class="spark-axis spark-axis-top">{maxReqLabel}</span>
      <span class="spark-axis spark-axis-bottom">0</span>
    {/if}
    {#if hoverSample && view.length >= 2}
      <div class="spark-guide" style="left: {guidePct}%"></div>
      <div class="spark-tooltip" style="left: {tooltipPct}%">
        {hoverSample.reqPerSec.toFixed(1)} req/s · {fmtInt(hoverSample.activeConnections)} conns · {fmtDuration(Date.now() - hoverSample.t)} ago
      </div>
    {/if}
    {#if warming}
      <span class="spark-hint">gathering data</span>
    {/if}
  </div>
  {#if latest}
    <span class="spark-readout">
      {latest.reqPerSec.toFixed(1)} req/s · {fmtInt(latest.activeConnections)} conns
    </span>
  {/if}
</div>

<style>
  /* Tile chrome: same surface, border and padding as StatTile so the strip
     reads as part of the grid rather than an injected widget. */
  .spark-strip {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    padding: var(--space-3) var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .spark-chart {
    position: relative;
    width: 100%;
    /* Fixed from first render so accumulating samples never shift layout.
       Roomy enough that the series and axis labels don't read as squashed
       now the strip spans the full content width above the tabs. */
    height: 88px;
  }
  .spark-svg {
    width: 100%;
    height: 100%;
    display: block;
    /* Let gridline strokes at the viewBox boundary render fully instead of
       being half-clipped at the top/bottom edge. */
    overflow: visible;
  }
  /* Area fill: accent-soft (the light tint of the primary accent) so it reads
     as the same family as the tile glyphs and status pills without competing
     with the value text. */
  .spark-area {
    fill: var(--accent-soft);
    stroke: none;
  }
  /* Secondary line: the neutral token is the dashboard's muted foreground, the
     same grey used for non-committal status pills, so connections stay legible
     over the accent-soft area without demanding equal weight. */
  .spark-line {
    fill: none;
    stroke: var(--neutral);
    stroke-width: 1.5;
    vector-effect: non-scaling-stroke;
  }
  /* Faint horizontal gridlines at the labelled values (ceiling + zero
     baseline). --border is the same token the tile grid uses for its rules,
     so the lines read in both themes without competing with the series. */
  .spark-grid {
    stroke: var(--border);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }
  .spark-axis {
    position: absolute;
    left: 2px;
    font-size: 0.6rem;
    line-height: 1;
    color: var(--text-faint);
    pointer-events: none;
    /* Semi-opaque surface so the label stays legible over the area fill. */
    padding: 0 3px;
    background: color-mix(in srgb, var(--bg-elevated) 80%, transparent);
    border-radius: 1px;
  }
  .spark-axis-top {
    top: 1px;
  }
  .spark-axis-bottom {
    bottom: 1px;
  }
  .spark-guide {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--text-faint);
    pointer-events: none;
  }
  .spark-tooltip {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 2px 6px;
    font-size: 0.66rem;
    color: var(--text);
    white-space: nowrap;
    pointer-events: none;
    z-index: 1;
  }
  .spark-readout {
    font-size: 0.72rem;
    color: var(--text-dim);
    white-space: nowrap;
  }
  .spark-hint {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    font-size: 0.72rem;
    color: var(--text-faint);
  }

  @media (prefers-reduced-motion: reduce) {
    /* The chart is static; no animated transitions to throttle. Included so
       any future transition on the guide/tooltip respects the preference. */
    .spark-guide,
    .spark-tooltip {
      transition: none;
    }
  }
</style>
