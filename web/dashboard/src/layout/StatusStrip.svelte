<script lang="ts">
  import { overview } from '../lib/stores/overview.svelte';
  import { fmtUptime } from '../lib/format';
  import { getNow as liveNow } from '../lib/clock.svelte';
  import type { EndpointResponse } from '../lib/types';

  // The strip renders from whatever the overview store last saw. Before the
  // first poll lands, render dashes so the operator can see "pending" rather
  // than a structurally empty header.
  const sys = $derived(overview.data?.system);

  // Without consulting overview.status the strip would display confident
  // numbers during an outage while the panels below report unreachable. The
  // data-state attribute mirrors the panels' pattern so CSS and assistive tech
  // can tell a live strip from a stale one, and the visible marker gives the
  // operator a non-numeric signal that the figures are frozen.
  const stateStatus = $derived(
    overview.status === 'stale' || overview.status === 'error' ? overview.status : null
  );
  const staleWord = $derived(
    overview.status === 'stale' ? 'stale' : overview.status === 'error' ? 'unreachable' : null
  );

  // No-traffic signal mirroring OverviewPanel: the WP-4 has_traffic flag is
  // always present, with total_requests===0 as a fallback for older backends.
  // Used to drop the Resp. rate cell to a no-data dash so the strip matches
  // the OverviewPanel tile rather than showing a literal "N/A".
  const hasTraffic = $derived(sys?.has_traffic === true || (sys?.total_requests ?? 0) > 0);

  // "degraded" on its own tells the operator nothing actionable. Derive a
  // short reason from real endpoint data (offline count) rather than
  // fabricate one; when healthy or when we have no endpoint data yet, say
  // nothing so the strip stays quiet. Breaker state is deliberately not
  // consulted: it trips only on health-probe failures, not live proxy
  // traffic, so it would under-report real failures.
  //
  // Source matters: the endpoints store is pausable (stopped when the
  // EndpointsPanel is unmounted) so its count can freeze mid-outage. The
  // overview payload's embedded endpoints array is always fresh because the
  // overview store runs for the lifetime of the dashboard, and the backend
  // populates it from the same registry on each /internal/status tick.
  const overviewEndpoints = $derived(overview.data?.endpoints ?? []);
  const degradedReason = $derived(reasonFor(sys?.status, overviewEndpoints));

  function reasonFor(status: string | undefined, list: EndpointResponse[]): string | null {
    if (!status || status === 'healthy' || !list.length) return null;
    const offline = list.filter((e) => e.status === 'offline' || e.status === 'critical').length;
    return offline > 0 ? `${offline} offline` : null;
  }

  // uptime is derived from start_time each tick so the figure stays live
  // without the backend baking a relative string into the payload.
  const now = $derived(liveNow());
  const uptime = $derived(sys?.start_time ? fmtUptime(sys.start_time, now) : null);

  // Tiny lookups inlined to avoid pulling StatusTag (pill-styled, too noisy
  // for the compact strip).
  function glyph(s: string | undefined): string {
    return s === 'healthy' ? '●' : s === 'degraded' || s === 'unhealthy' ? '◐' : '○';
  }
  function cls(s: string | undefined): string {
    return s === 'healthy' ? 'green' : s === 'degraded' || s === 'unhealthy' ? 'amber' : 'red';
  }
</script>

<dl class="status-strip" aria-label="System status summary" data-state={stateStatus}>
  <div class="status-cell system-status">
    <dt>Status</dt>
    <dd>
      {#if sys}
        <span class="glyph g-{cls(sys.status)}" aria-hidden="true">{glyph(sys.status)}</span>{sys.status}
      {:else}<span class="dash">—</span>{/if}
    </dd>
    {#if staleWord}
      <span class="status-reason stale" data-stale title="Last poll {stateStatus}; figures below may be frozen">{staleWord}</span>
    {:else if degradedReason}
      <span class="status-reason" title={degradedReason}>{degradedReason}</span>
    {/if}
  </div>
  <div class="status-cell">
    <dt>Endpoints</dt>
    <dd>{sys ? sys.endpoints_up : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Resp. rate</dt>
    <dd title="Counts any streamed response, regardless of HTTP status">{#if sys}{#if hasTraffic}{sys.success_rate}{:else}<span class="dash">—</span>{/if}{:else}—{/if}</dd>
  </div>
  <div class="status-cell">
    <dt>Avg latency</dt>
    <dd>{sys ? sys.avg_latency : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Uptime</dt>
    <dd>{sys ? (uptime ?? '—') : '—'}</dd>
  </div>
  <div class="status-cell">
    <dt>Version</dt>
    <dd>{sys ? sys.version : '—'}</dd>
  </div>
</dl>
