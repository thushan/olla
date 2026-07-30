<script>
  import { fmtPct, pctBucket } from '../lib/format.js';

  let { pct = 0, hasData = false } = $props();
  const bucket = $derived(pctBucket(pct, hasData));
</script>

<span class="pct-cell">
  <span class="pct-bar" role="img" aria-label={hasData ? `${pct.toFixed(1)}% success` : 'no data'}>
    <span class="fill" style="width:{hasData ? pct : 0}%;background:var(--{bucket})"></span>
  </span>
  {#if hasData}{fmtPct(pct)}{:else}<span class="dash">no data</span>{/if}
</span>
