<script>
  // `avg` is deliberately nullable: it's the average PROXY request latency,
  // a separate metric from `min`/`max` (also proxy latency) that may be
  // absent on a backend that hasn't shipped it yet, or null on an endpoint
  // with no traffic yet. Never derive it from a health-check figure -
  // health-check latency (response_time) measures a different thing
  // entirely (see EndpointsPanel/OverviewPanel).
  import { fmtMs, scalePct } from '../lib/format';

  let { min = 0, avg = null, max = 0, globalMax = 0, label = 'latency' } = $props();

  const noData = $derived(!max);
  const hasAvg = $derived(typeof avg === 'number' && Number.isFinite(avg));
  const fillPct = $derived(noData || !hasAvg ? 0 : scalePct(avg, globalMax));
  const tickPct = $derived(noData ? 0 : scalePct(max, globalMax));
  const avgText = $derived(hasAvg ? fmtMs(avg) : '—');

  const ariaLabel = $derived(
    noData ? `${label}: no data` : `${label}: average ${avgText}, range ${fmtMs(min)} to ${fmtMs(max)}`
  );
</script>

{#if noData}
  <span class="dash">— no data</span>
{:else}
  <div class="range-wrap">
    <div class="range-bar" role="img" aria-label={ariaLabel}>
      <div class="fill" style="width:{fillPct.toFixed(1)}%"></div>
      <div class="tick" style="left:{tickPct.toFixed(1)}%"></div>
    </div>
    <span class="range-labels">{avgText} <span class="peak">/ {fmtMs(max)} pk</span></span>
  </div>
{/if}
