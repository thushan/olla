<script>
  import { overview } from '../lib/stores/overview.svelte.js';
  import { endpoints } from '../lib/stores/endpoints.svelte.js';
  import { fmtUptime } from '../lib/format.js';
  import { getNow as liveNow } from '../lib/clock.svelte.js';

  // The strip renders from whatever the overview store last saw. Before the
  // first poll lands, render dashes so the operator can see "pending" rather
  // than a structurally empty header.
  const sys = $derived(overview.data?.system);
  const has = $derived(!!sys);

  // "degraded" on its own tells the operator nothing actionable. Derive a
  // short reason from real endpoint data (offline count) rather than
  // fabricate one; when healthy or when we have no endpoint data yet, say
  // nothing so the strip stays quiet. Breaker state is deliberately not
  // consulted: on main it trips only on health-probe failures, not live
  // proxy traffic, so it would under-report real failures (spec §4.2).
  const endpointList = $derived(endpoints.data?.endpoints ?? []);
  const degradedReason = $derived(reasonFor(sys?.status, endpointList));

  function reasonFor(status, list) {
    if (!status || status === 'healthy' || !list.length) return null;
    const offline = list.filter((e) => e.status === 'offline' || e.status === 'critical').length;
    const parts = [];
    if (offline) parts.push(`${offline} offline`);
    return parts.length ? parts.join(', ') : null;
  }

  // uptime is derived from start_time each tick so the figure stays live
  // without the backend baking a relative string into the payload.
  const now = $derived(liveNow());
  const uptime = $derived(sys?.start_time ? fmtUptime(sys.start_time, now) : null);

  // Tiny lookups inlined to avoid pulling StatusTag (pill-styled, too noisy
  // for the compact strip).
  function glyph(s) {
    return s === 'healthy' ? '●' : s === 'degraded' || s === 'unhealthy' ? '◐' : '○';
  }
  function cls(s) {
    return s === 'healthy' ? 'green' : s === 'degraded' || s === 'unhealthy' ? 'amber' : 'red';
  }
</script>

<dl class="status-strip" aria-label="System status summary">
  <div class="status-cell system-status">
    <dt>Status</dt>
    <dd>
      {#if has}
        <span class="glyph g-{cls(sys.status)}" aria-hidden="true">{glyph(sys.status)}</span>{sys.status}
      {:else}<span class="dash">—</span>{/if}
    </dd>
    {#if degradedReason}
      <span class="status-reason" title={degradedReason}>{degradedReason}</span>
    {/if}
  </div>
  <div class="status-cell">
    <dt>Endpoints</dt>
    <dd>{has ? sys.endpoints_up : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Success</dt>
    <dd>{has ? sys.success_rate : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Avg latency</dt>
    <dd>{has ? sys.avg_latency : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Uptime</dt>
    <dd>{has ? (uptime ?? '—') : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Version</dt>
    <dd>{has ? sys.version : '—'}</dd>
  </div>
</dl>
