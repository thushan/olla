<script>
  import { fmtPct, pctBucket } from '../lib/format.js';

  // `status` gates the bar's COLOUR on endpoint health: a down backend
  // (offline, unhealthy, critical) with lifetime counters would otherwise
  // paint a confident green bar beside its red status pill. It must not gate
  // the FIGURE away too - an endpoint that served 10,000 requests before
  // dying carries real history, and rendering it identically to an endpoint
  // that never took a request throws that history away exactly when an
  // operator needs it. Optional and backward-compatible - callers that omit
  // status get the original behaviour.
  let { pct = 0, hasData = false, status } = $props();

  const DOWN = new Set(['offline', 'unhealthy', 'critical']);
  const down = $derived(DOWN.has(status));
  // We have a real figure but the endpoint is currently down: show the
  // number (and its real fill width) but force the neutral bucket so it
  // never reads as current, live health.
  const historical = $derived(hasData && down);
  const bucket = $derived(historical ? 'neutral' : pctBucket(pct, hasData));
  const label = $derived(
    !hasData
      ? 'no data'
      : historical
        ? `${pct.toFixed(1)}% response rate (historical, endpoint offline)`
        : `${pct.toFixed(1)}% response rate`
  );
</script>

<span class="pct-cell">
  <span class="pct-bar" role="img" aria-label={label}>
    <span class="fill" style="width:{hasData ? pct : 0}%;background:var(--{bucket})"></span>
  </span>
  {#if !hasData}
    <span class="dash">no data</span>
  {:else if historical}
    <span class="historical" title="Historical - endpoint currently offline">{fmtPct(pct)}</span>
  {:else}
    {fmtPct(pct)}
  {/if}
</span>
