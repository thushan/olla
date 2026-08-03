<script lang="ts">
  import { tick } from 'svelte';
  import { overview } from '../lib/stores/overview.svelte';
  import { endpoints } from '../lib/stores/endpoints.svelte';
  import StatTile from '../components/StatTile.svelte';
  import SparkStrip from '../components/SparkStrip.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import StatusTag from '../components/StatusTag.svelte';
  import PctBar from '../components/PctBar.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import type { Column, SortState } from '../components/SortableTable.svelte';
  import { fmtBytes, fmtInt, fmtUptime, fmtMs } from '../lib/format';
  import { stableId } from '../lib/dom-id';
  import { getNow as liveNow } from '../lib/clock.svelte';
  import type { EndpointSummary } from '../lib/types';

  interface Props {
    onJumpToEndpoints?: () => void;
  }

  // Cross-panel navigation is delegated to App.svelte so this panel never
  // imports the navigation store (spec §7.2.1).
  let { onJumpToEndpoints = () => {} }: Props = $props();

  const sys = $derived(overview.data?.system);
  const proxy = $derived(overview.data?.proxy);
  // security sits at the StatusResponse root, not on system; surface it here so
  // the Security violations tile can show status/blocked/rate-vs-size detail
  // without a second fetch.
  const sec = $derived(overview.data?.security);
  const loading = $derived(overview.status === 'loading');

  // endpoints_up arrives as a pre-formatted "x/y" string; split for the tile.
  const upTotal = $derived(sys?.endpoints_up ?? '/');
  const up = $derived(Number(upTotal.split('/')[0]) || 0);
  const total = $derived(Number(upTotal.split('/')[1]) || 0);

  const successPctNum = $derived(parseFloat(sys?.success_rate ?? '') || 0);
  const trafficBytes = $derived(parseTrafficBytes(sys?.total_traffic));

  const endpointList = $derived(endpoints.data?.endpoints ?? []);

  // Aggregate fields already present on EndpointSummary but not previously
  // surfaced. Each tiles out into its own derived so a partial payload never
  // throws and the no-data states stay explicit.
  const totalDiscoveredModels = $derived(
    endpointList.reduce((n, e) => n + (e.model_count ?? 0), 0)
  );

  // Fleet latency range = min of per-endpoint min, max of per-endpoint max.
  // An idle fleet reports zeros across the board; we must NOT render that as
  // "0ms-0ms" implying a real measurement. Gate on the fleet max being > 0,
  // mirroring the glance table's request_count guard.
  const fleetMinLatency = $derived(
    endpointList.reduce((m, e) => Math.min(m, e.min_latency_ms ?? 0), Infinity)
  );
  const fleetMaxLatency = $derived(
    endpointList.reduce((m, e) => Math.max(m, e.max_latency_ms ?? 0), 0)
  );
  const hasFleetLatency = $derived(endpointList.length > 0 && fleetMaxLatency > 0);

  // Backend-type histogram, sorted by count desc then name for deterministic
  // output. Empty fleet renders as the no-data dash inside the tile.
  const backendBreakdown = $derived(
    Object.entries(
      endpointList.reduce<Record<string, number>>((acc, e) => {
        const t = e.type || 'unknown';
        acc[t] = (acc[t] ?? 0) + 1;
        return acc;
      }, {})
    )
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .map(([type, n]) => `${n} ${type}`)
      .join(' · ')
  );

  // Glance view-model row: the endpoint contract plus the sort/scale fields
  // the glance table presents (status_rank drives the default unhealthy-first
  // ordering; success_rate_num mirrors EndpointsPanel; avg_latency_ms is
  // normalised to number|null so the cell can pick its no-data placeholder).
  // Type alias (not interface) so it satisfies SortableTable's
  // `Row extends Record<string, unknown>` via an implicit index signature.
  type GlanceRow = Omit<EndpointSummary, 'avg_latency_ms'> & {
    status_rank: number;
    success_rate_num: number;
    avg_latency_ms: number | null;
  };

  // Glance table columns, wired through the shared SortableTable so the
  // interaction and aria-sort semantics match the Endpoints panel exactly.
  const glanceColumns: Column[] = [
    { key: 'name', label: 'Endpoint', sortable: true, sticky: true },
    { key: 'status_rank', label: 'Status', sortable: true },
    {
      key: 'success_rate_num',
      label: 'Resp. rate',
      sortable: true,
      num: true,
      align: 'right',
      title: 'Counts any streamed response, regardless of HTTP status',
    },
    { key: 'avg_latency_ms', label: 'Avg latency', sortable: true, num: true, align: 'right' },
  ];

  const glanceInitialSort: SortState = { key: 'status_rank', dir: 'asc' };

  // Default sort is unhealthy-first: an ops glance view should surface the
  // backends that need attention before the ones already fine.
  function statusRank(s: string): number {
    if (s === 'offline' || s === 'critical') return 0;
    if (s === 'degraded' || s === 'unhealthy') return 1;
    if (s === 'healthy') return 2;
    return 3;
  }

  // avg_latency_ms is the API's numeric average PROXY latency field - never
  // derive it from response_time (last HEALTH-CHECK probe latency, a
  // different metric, and a formatted string like "1.5s" that parseInt
  // would silently mangle to 1).
  const glanceRows: GlanceRow[] = $derived(
    endpointList.map((e) => ({
      ...e,
      status_rank: statusRank(e.status),
      success_rate_num: parseFloat(e.success_rate) || 0,
      avg_latency_ms: typeof e.avg_latency_ms === 'number' ? e.avg_latency_ms : null,
    }))
  );

  // uptime is now derived client-side from start_time so the ETag stays
  // stable; the live clock re-renders this each second.
  const now = $derived(liveNow());
  const uptimeText = $derived(sys?.start_time ? fmtUptime(sys.start_time, now) : '—');

  function statusGlyph(s: string): string {
    return s === 'healthy' ? '●' : s === 'degraded' || s === 'unhealthy' ? '◐' : '○';
  }
  function statusCls(s: string): string {
    return s === 'healthy' ? 'green' : s === 'degraded' || s === 'unhealthy' ? 'amber' : 'red';
  }

  // total_traffic arrives formatted as e.g. "172.02 GB". Re-parse to bytes so
  // fmtBytes (our single formatting authority) renders it consistently.
  function parseTrafficBytes(s: string | undefined): number | null {
    if (!s) return null;
    const m = String(s).match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)$/);
    if (!m) return null;
    const units: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4, PB: 1024 ** 5 };
    return parseFloat(m[1]) * units[m[2]];
  }

  // onJumpToEndpoints() swaps the active panel (App.svelte unmounts this one
  // entirely), so `await tick()` before touching the DOM - without it the
  // lookup below always missed, because EndpointsPanel's row for this endpoint
  // hadn't been rendered yet: the panel swap and this call raced, the jump
  // never scrolled anywhere, and the clicked button (now removed from the
  // DOM) dropped keyboard focus to <body> with no replacement.
  //
  // The DOM id is computed from the endpoint's unique identity (id, falling
  // back to url/name for backends predating the field) via the same
  // collision-resistant hash EndpointsPanel uses, so the lookup resolves to
  // the matching row even when two endpoints share a display url once query
  // strings are stripped.
  // Accepts the structural identity shape rather than the full EndpointSummary:
  // GlanceRow's avg_latency_ms (number | null) is deliberately incompatible with
  // EndpointSummary's (number | undefined), so typing the param against the
  // contract would reject the row. The jump only needs identity fields.
  async function jumpToEndpoints(e: { id?: string; url?: string; name: string }): Promise<void> {
    onJumpToEndpoints();
    await tick();
    const el = document.getElementById(`ep-${stableId(e.id ?? e.url ?? e.name)}`);
    if (!el) return;
    el.scrollIntoView({ block: 'center' });
    el.focus();
  }
</script>

<div
  id="panel-overview"
  class="panel is-active"
  role="tabpanel"
  aria-labelledby="tab-overview"
  tabindex="0"
  data-state={overview.status === 'error' || overview.status === 'stale' ? overview.status : null}
>
  <h2>Overview</h2>
  <p class="panel-intro">
    System-wide health for the herd, polled from Olla's internal status endpoint. All figures are
    read-only snapshots: no controls here mutate configuration or traffic.
  </p>

  <StatusBanner store={overview} />

  {#if loading}
    <div class="section">
      <div class="tile-grid">
        {#each Array(9) as _, i}<div class="skeleton tile-skel"></div>{/each}
      </div>
    </div>
    <div class="section">
      <div class="section-head"><h3>Herd at a glance</h3></div>
      {#each Array(4) as _, i}<div class="skeleton row-skel"></div>{/each}
    </div>
  {:else if sys}
    <div class="section">
      <SparkStrip />
    </div>
    <div class="section">
      <div class="tile-grid">
        <StatTile label="System status">
          {#snippet children()}
            <span class="glyph g-{statusCls(sys.status)}" aria-hidden="true">{statusGlyph(sys.status)}</span>
            {sys.status}
          {/snippet}
          {#snippet subSnippet()}
            <strong>{proxy?.engine ?? 'olla'}</strong> engine · <strong>{proxy?.balancer ?? 'priority'}</strong> balancing{#if proxy?.profile} · <strong>{proxy.profile}</strong> profile{/if}
          {/snippet}
        </StatTile>
        <StatTile
          label="Endpoints up"
          sub="{total - up} unavailable"
        >
          {#snippet children()}
            {up}<span class="unit">/ {total}</span>
          {/snippet}
        </StatTile>
        <StatTile
          label="Response rate"
          value={sys.success_rate}
          sub="counts any streamed response, regardless of HTTP status — {fmtInt(sys.total_failures)} failures logged"
        />
        <StatTile
          label="Avg latency"
          value={sys.avg_latency}
          sub="across all backends"
        />
        <StatTile
          label="Total traffic"
          value={trafficBytes !== null ? fmtBytes(trafficBytes) : sys.total_traffic}
          sub="since boot · {uptimeText}"
        />
        <StatTile
          label="Active connections"
          value={fmtInt(sys.active_connections)}
          sub="in flight right now"
        />
        <StatTile
          label="Total requests"
          value={fmtInt(sys.total_requests)}
          sub="{sys.version} · {sys.commit}"
        />
        <StatTile
          label="Total failures"
          value={fmtInt(sys.total_failures)}
          sub="{(100 - successPctNum).toFixed(1)}% of traffic"
        />
        <StatTile label="Security violations" value={fmtInt(sys.security_violations)}>
          {#snippet subSnippet()}
            {#if sec}
              {sec.status ?? 'normal'} status{#if sec.blocked_ips > 0} · {fmtInt(sec.blocked_ips)} blocked IPs{/if}
              · {fmtInt(sec.violations?.rate_limits ?? 0)} rate / {fmtInt(sec.violations?.size_limits ?? 0)} size
            {:else}
              rejected by security layer
            {/if}
          {/snippet}
        </StatTile>
        <StatTile label="Discovered models" value={fmtInt(totalDiscoveredModels)}
          sub="across {fmtInt(endpointList.length)} endpoints"
        />
        <StatTile label="Latency range">
          {#snippet children()}
            {#if hasFleetLatency}{fmtMs(fleetMinLatency)}–{fmtMs(fleetMaxLatency)}{:else}<span class="dash">—</span>{/if}
          {/snippet}
          {#snippet subSnippet()}fleet min–max{/snippet}
        </StatTile>
        <StatTile label="Backend types">
          {#snippet children()}
            {backendBreakdown || '—'}
          {/snippet}
          {#snippet subSnippet()}{fmtInt(endpointList.length)} endpoints{/snippet}
        </StatTile>
      </div>
    </div>

    <div class="section">
      <div class="section-head">
        <h3>Herd at a glance</h3>
        <span class="section-note">click a name to jump to its endpoint detail · sorted unhealthy first by default</span>
      </div>
      <SortableTable
        columns={glanceColumns}
        rows={glanceRows}
        initialSort={glanceInitialSort}
        rowId={(row) => row.id ?? row.url ?? row.name}
        rowDomId={(row) => `glance-${stableId(row.id ?? row.url ?? row.name)}`}
      >
        {#snippet rowSnippet({ row: e, cellClass })}
          <td class="col-sticky">
            <button
              class="glance-link"
              type="button"
              onclick={() => jumpToEndpoints(e)}
              title="Open {e.name} in the Endpoints panel"
            >
              <span class="glyph g-{statusCls(e.status)}" aria-hidden="true">{statusGlyph(e.status)}</span>
              <span class="txt">{e.name}</span>
            </button>
          </td>
          <td><StatusTag status={e.status} /></td>
          <td class={cellClass('success_rate_num')}><PctBar pct={e.success_rate_num} hasData={e.request_count > 0} status={e.status} /></td>
          <td class={cellClass('avg_latency_ms')}>
            {#if typeof e.avg_latency_ms === 'number' && e.request_count > 0}{fmtMs(e.avg_latency_ms)}{:else}<span class="dash">—</span>{/if}
          </td>
        {/snippet}
      </SortableTable>
    </div>
  {/if}
</div>
