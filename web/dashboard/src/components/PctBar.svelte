<script>
  import { fmtPct, pctBucket } from '../lib/format.js';

  // `status` gates the bar on endpoint health: a down backend (offline,
  // unhealthy, critical) with lifetime counters would otherwise paint a
  // confident green bar beside its red status pill. Optional and
  // backward-compatible - callers that omit it get the original behaviour.
  let { pct = 0, hasData = false, status } = $props();

  const DOWN = new Set(['offline', 'unhealthy', 'critical']);
  const down = $derived(DOWN.has(status));
  const showData = $derived(hasData && !down);
  const bucket = $derived(pctBucket(pct, showData));
</script>

<span class="pct-cell">
  <span class="pct-bar" role="img" aria-label={showData ? `${pct.toFixed(1)}% success` : 'no data'}>
    <span class="fill" style="width:{showData ? pct : 0}%;background:var(--{bucket})"></span>
  </span>
  {#if showData}{fmtPct(pct)}{:else}<span class="dash">no data</span>{/if}
</span>
