<script lang="ts">
  // Sticky sessions, translators and process cards for the Overview. Each card
  // is a row of plain StatTiles driven by its own 15s poll store. A card is
  // hidden (not broken) when its feature is off, its payload is empty, or the
  // endpoint has never answered (403/404/5xx on first load). Once a card has
  // data, a later failure greys it and adds a muted note instead of blanking.
  import StatTile from '../components/StatTile.svelte';
  import { sticky, translators, processStats } from '../lib/stores/runtime.svelte';
  import { fmtInt } from '../lib/format';
  import {
    stickyVisible,
    stickyHitRateText,
    stickyFill,
    translatorList,
    fallbackTotal,
    fallbackBreakdown,
    translatorHasTraffic,
    processUsable,
  } from '../lib/runtime';

  // Same lifecycle as the endpoints store on this panel: poll only while the
  // cards are mounted. The scheduler backs off on hidden tabs.
  $effect(() => {
    sticky.start();
    translators.start();
    processStats.start();
    return () => {
      sticky.stop();
      translators.stop();
      processStats.stop();
    };
  });

  const stickyData = $derived(sticky.data);
  const showSticky = $derived(stickyVisible(stickyData));
  const fill = $derived(stickyData && showSticky ? stickyFill(stickyData) : null);

  const translatorRows = $derived(translatorList(translators.data));
  const proc = $derived(processUsable(processStats.data) ? processStats.data : null);

  const degraded = (s: string) => (s === 'error' || s === 'stale' ? s : null);
  const note = (s: string) =>
    s === 'stale' ? 'unreachable for a while, showing the last good snapshot' : 'refresh failed, retrying';
</script>

{#if showSticky && stickyData}
  <div class="section" data-testid="card-sticky">
    <div class="section-head">
      <h3>Sticky sessions</h3>
      <span class="section-note">
        idle TTL {fmtInt(stickyData.idle_ttl_seconds ?? 0)}s{#if degraded(sticky.status)} · {note(sticky.status)}{/if}
      </span>
    </div>
    <div class="tile-grid" data-state={degraded(sticky.status)}>
      <StatTile label="Active sessions" value={fmtInt(stickyData.active_sessions)}
        sub={fill === null ? 'no session cap' : fill > 0 && fill < 0.1 ? '<0.1% of capacity' : `${fill.toFixed(fill < 10 && fill > 0 ? 1 : 0)}% of capacity`} />
      <StatTile label="Hit rate" value={stickyHitRateText(stickyData)}
        sub="{fmtInt(stickyData.hits)} hits / {fmtInt(stickyData.misses)} misses" />
      <StatTile label="Evictions" value={fmtInt(stickyData.evictions)}
        sub="{fmtInt(stickyData.insertions)} sessions pinned" />
      <StatTile label="Max sessions" value={fmtInt(stickyData.max_sessions)} sub="LRU cap" />
    </div>
  </div>
{/if}

{#each translatorRows as t (t.translator_name)}
  <div class="section" data-testid="card-translator">
    <div class="section-head">
      <h3>Translator: {t.translator_name}</h3>
      <span class="section-note">
        {fmtInt(t.total_requests)} requests{#if degraded(translators.status)} · {note(translators.status)}{/if}
      </span>
    </div>
    <div class="tile-grid" data-state={degraded(translators.status)}>
      <StatTile label="Passthrough">
        {#snippet children()}
          {#if translatorHasTraffic(t)}{t.passthrough_rate}{:else}<span class="dash">—</span>{/if}
        {/snippet}
        {#snippet subSnippet()}{fmtInt(t.passthrough_requests)} passthrough / {fmtInt(t.translation_requests)} translated{/snippet}
      </StatTile>
      <StatTile label="Streaming" value={fmtInt(t.streaming_requests)}
        sub="{fmtInt(t.non_streaming_requests)} non-streaming" />
      <StatTile label="Fallbacks" value={fmtInt(fallbackTotal(t))} sub={fallbackBreakdown(t)} />
      <StatTile label="Avg latency">
        {#snippet children()}
          {#if translatorHasTraffic(t)}{t.average_latency}{:else}<span class="dash">—</span>{/if}
        {/snippet}
        {#snippet subSnippet()}{translatorHasTraffic(t) ? `${t.success_rate} success` : 'no traffic yet'}{/snippet}
      </StatTile>
    </div>
  </div>
{/each}

{#if proc}
  <div class="section" data-testid="card-process">
    <div class="section-head">
      <h3>Process</h3>
      <span class="section-note">
        {#if proc.runtime.uptime}up {proc.runtime.uptime} · {/if}{proc.runtime.go_version || 'Go'} · {fmtInt(proc.runtime.num_cpu)} CPUs{#if degraded(processStats.status)} · {note(processStats.status)}{/if}
      </span>
    </div>
    <div class="tile-grid" data-state={degraded(processStats.status)}>
      <StatTile label="Heap" value={proc.memory.heap_alloc || '—'} sub="{proc.memory.heap_sys || '—'} reserved" />
      <StatTile label="Memory pressure" value={proc.memory.memory_pressure.toLowerCase() || '—'} sub="heap in use {proc.memory.heap_inuse || '—'}" />
      <StatTile label="Goroutines" value={fmtInt(proc.goroutines.count)} sub={proc.goroutines.health_status.toLowerCase()} />
      <StatTile label="Avg GC pause">
        {#snippet children()}
          {#if proc.garbage_collection.avg_gc_pause}{proc.garbage_collection.avg_gc_pause}{:else}<span class="dash">—</span>{/if}
        {/snippet}
        {#snippet subSnippet()}{fmtInt(proc.garbage_collection.num_gc_cycles)} cycles{/snippet}
      </StatTile>
    </div>
  </div>
{/if}
