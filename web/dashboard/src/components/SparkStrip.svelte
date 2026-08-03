<script lang="ts">
  // Hand-rolled inline-SVG sparkline. No chart library (hard rule). The strip
  // samples once per overview poll tick by keying an $effect off
  // overview.lastUpdated, which advances on BOTH the 200 and 304 paths (see
  // poll-store.svelte.ts onSuccess). On a 304 the data object is unchanged,
  // so the computed delta is naturally zero: the x-axis stays real time while
  // a quiet herd shows a flat baseline rather than a frozen clock.

  import { untrack } from 'svelte';
  import { overview } from '../lib/stores/overview.svelte';
  import { history, type Sample } from '../lib/stores/history.svelte';
  import { fmtInt } from '../lib/format';

  interface Props {
    // Injectable for tests; production reads the singleton store.
    samples?: Sample[];
  }
  let { samples }: Props = $props();

  const view = $derived<Sample[]>(samples ?? history.samples);

  // Sampling hook. Reads lastUpdated (retriggers every tick, 200 or 304) then
  // appends a derived sample. untrack is essential: history.append pushes to
  // the $state samples array, and without untrack the proxy read inside push
  // would add samples as a dependency of THIS effect, creating a write loop
  // (append -> samples changes -> effect reruns -> append -> ...).
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
  // Floor at 1 so a flat-zero series still scales (avoids div-by-zero) and a
  // single-value max resolves to a full-height peak rather than NaN.
  const maxReq = $derived(Math.max(1, ...reqSeries));
  const maxConn = $derived(Math.max(1, ...connSeries));

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
    // Close down to the bottom-right then bottom-left for a filled area.
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
</script>

<div class="spark-strip">
  <div class="spark-chart" role="img" aria-label={ariaLabel}>
    <svg
      class="spark-svg"
      viewBox="0 0 {W} {H}"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      {#if !warming}
        <path class="spark-area" d={reqArea} />
        <path class="spark-line" d={connLine} />
      {/if}
    </svg>
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
    /* Fixed from first render so accumulating samples never shift layout. */
    height: 48px;
  }
  .spark-svg {
    width: 100%;
    height: 100%;
    display: block;
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
    /* Keep the stroke visually constant despite preserveAspectRatio="none"
       stretching the path geometry to fill the container width. */
    vector-effect: non-scaling-stroke;
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
</style>
