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
  // Hand-rolled inline-SVG sparkline with a req/s y-axis scale, a pointer
  // hover reading, and outage markers (red vertical lines at failed-poll
  // ticks). No chart library (hard rule).
  //
  // Sampling is driven by the overview store's onTick callback wired in
  // overview.svelte.ts, which feeds the history ring buffer on BOTH success
  // and failure. This component is purely a view of history.samples; it does
  // not sample or poll itself.

  import { history, type Sample } from '../lib/stores/history.svelte';
  import { fmtInt, fmtDuration } from '../lib/format';

  interface Props {
    // Injectable for tests; production reads the singleton store.
    samples?: Sample[];
  }
  let { samples }: Props = $props();

  const view = $derived<Sample[]>(samples ?? history.samples);

  // viewBox dimensions: arbitrary because preserveAspectRatio="none" stretches
  // the drawing to the container. The 100x30 box gives the path math a clean
  // range; rendered height is fixed by .spark-svg so there is no layout shift.
  const W = 100;
  const H = 30;

  const step = $derived(view.length >= 2 ? W / (view.length - 1) : 0);
  const maxReq = $derived(Math.max(1, ...view.map((s) => s.reqPerSec)));
  const maxConn = $derived(Math.max(1, ...view.map((s) => s.activeConnections)));

  // Ceiling label for the req/s axis: one decimal under 10, integer above.
  const maxReqLabel = $derived(maxReq < 10 ? maxReq.toFixed(1) : Math.round(maxReq).toString());

  // Area wash: one closed sub-path per run of consecutive non-error samples.
  // Error samples produce a visual gap (the run is closed at the boundary)
  // rather than bridging the outage with fabricated data.
  function buildArea(list: Sample[], maxR: number): string {
    const n = list.length;
    if (n < 2) return '';
    const s = W / (n - 1);
    let d = '';
    let runStartX = 0;
    let lastX = 0;
    let inRun = false;
    for (let i = 0; i < n; i++) {
      const sample = list[i];
      if (sample.error) {
        if (inRun) {
          d += ` L ${lastX} ${H} L ${runStartX} ${H} Z`;
          inRun = false;
        }
        continue;
      }
      const x = i * s;
      const y = H - (sample.reqPerSec / maxR) * H;
      if (!inRun) {
        d += ` M ${x} ${y}`;
        runStartX = x;
        inRun = true;
      } else {
        d += ` L ${x} ${y}`;
      }
      lastX = x;
    }
    if (inRun) {
      d += ` L ${lastX} ${H} L ${runStartX} ${H} Z`;
    }
    return d.trim();
  }

  // Stroke path (edge line or connections): open sub-paths per run of
  // non-error samples. A lone M from a single-point run draws nothing.
  function buildLine(list: Sample[], pick: (s: Sample) => number, max: number): string {
    const n = list.length;
    if (n < 2) return '';
    const s = W / (n - 1);
    let d = '';
    let inRun = false;
    for (let i = 0; i < n; i++) {
      const sample = list[i];
      if (sample.error) {
        inRun = false;
        continue;
      }
      const x = i * s;
      const y = H - (pick(sample) / max) * H;
      if (!inRun) {
        d += ` M ${x} ${y}`;
        inRun = true;
      } else {
        d += ` L ${x} ${y}`;
      }
    }
    return d.trim();
  }

  const reqAreaD = $derived(buildArea(view, maxReq));
  const reqEdgeD = $derived(buildLine(view, (s) => s.reqPerSec, maxReq));
  const connLineD = $derived(buildLine(view, (s) => s.activeConnections, maxConn));
  const errorIndices = $derived(
    view.length >= 2
      ? view.map((s, i) => (s.error ? i : -1)).filter((i) => i >= 0)
      : [],
  );

  const latest = $derived(view.length > 0 ? view[view.length - 1] : null);
  const warming = $derived(view.length < 2);

  const ariaLabel = $derived(
    latest
      ? latest.error
        ? 'connection lost'
        : `${latest.reqPerSec.toFixed(1)} requests per second, ${fmtInt(latest.activeConnections)} active connections`
      : 'gathering telemetry',
  );

  // --- hover reading (progressive enhancement for pointer users) ---
  let chartEl: HTMLElement | null = $state(null);
  let hoverIndex = $state<number | null>(null);

  const hoverSample = $derived(hoverIndex !== null ? view[hoverIndex] ?? null : null);
  const guidePct = $derived(
    hoverIndex !== null && view.length >= 2 ? (hoverIndex / (view.length - 1)) * 100 : 0,
  );
  const tooltipPct = $derived(Math.max(15, Math.min(85, guidePct)));

  function onPointerMove(e: PointerEvent): void {
    if (!chartEl || view.length < 2) return;
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
        <path class="spark-area" d={reqAreaD} />
        <path class="spark-edge" d={reqEdgeD} />
        <path class="spark-line" d={connLineD} />
        {#each errorIndices as i}
          <line class="spark-error" x1={i * step} y1="0" x2={i * step} y2={H} />
        {/each}
      {/if}
    </svg>
    {#if !warming}
      <span class="spark-axis spark-axis-top">{maxReqLabel}</span>
      <span class="spark-axis spark-axis-bottom">0</span>
    {/if}
    {#if hoverSample && view.length >= 2}
      <div class="spark-guide" class:is-error={hoverSample.error} style="left: {guidePct}%"></div>
      <div class="spark-tooltip" style="left: {tooltipPct}%">
        {#if hoverSample.error}
          connection lost · {fmtDuration(Date.now() - hoverSample.t)} ago
        {:else}
          {hoverSample.reqPerSec.toFixed(1)} req/s · {fmtInt(hoverSample.activeConnections)} conns · {fmtDuration(Date.now() - hoverSample.t)} ago
        {/if}
      </div>
    {/if}
    {#if warming}
      <span class="spark-hint">gathering data</span>
    {/if}
  </div>
  {#if latest}
    <span class="spark-readout">
      {#if latest.error}
        connection lost
      {:else}
        {latest.reqPerSec.toFixed(1)} req/s · {fmtInt(latest.activeConnections)} conns
      {/if}
    </span>
  {/if}
</div>

<style>
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
    height: 88px;
  }
  .spark-svg {
    width: 100%;
    height: 100%;
    display: block;
    overflow: visible;
  }
  .spark-area {
    fill: var(--accent);
    fill-opacity: 0.2;
    stroke: none;
  }
  .spark-edge {
    fill: none;
    stroke: var(--accent);
    stroke-width: 1.5;
    vector-effect: non-scaling-stroke;
  }
  .spark-line {
    fill: none;
    stroke: var(--neutral);
    stroke-width: 1.5;
    vector-effect: non-scaling-stroke;
  }
  .spark-grid {
    stroke: var(--border);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }
  .spark-error {
    stroke: var(--red);
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
  .spark-guide.is-error {
    background: var(--red);
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
    .spark-guide,
    .spark-tooltip {
      transition: none;
    }
  }
</style>
