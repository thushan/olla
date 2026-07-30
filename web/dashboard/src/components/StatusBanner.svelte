<script>
  // Renders the inline banner shown when a panel's source is in error or
  // stale. Keeps the prior data rendered (greyed) rather than blanking.
  let { store } = $props();

  const show = $derived(store.status === 'error' || store.status === 'stale');
  const isStale = $derived(store.status === 'stale');

  const message = $derived(
    isStale
      ? `Olla's ${store.name} status has been unreachable for a while, showing the last good snapshot and retrying.`
      : `Couldn't reach Olla's ${store.name} status API, retrying.`
  );

  function fmtTime(d) {
    if (!d) return 'never';
    return d.toLocaleTimeString('en-AU', { hour12: false });
  }
</script>

{#if show}
  <div
    class="banner {isStale ? 'stale' : 'error'}"
    role="status"
    aria-live="polite"
  >
    <span class="glyph g-{isStale ? 'amber' : 'red'}" aria-hidden="true">{isStale ? '◐' : '○'}</span>
    <span class="banner-message">{message}</span>
    <span class="banner-meta">last ok: {fmtTime(store.lastUpdated)}</span>
    <button class="theme-toggle" type="button" onclick={() => store.refresh()}>retry now</button>
  </div>
{/if}
