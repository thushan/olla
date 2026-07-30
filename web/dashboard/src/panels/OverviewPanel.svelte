<script>
  import { overview } from '../lib/stores/overview.svelte.js';
  import { endpoints } from '../lib/stores/endpoints.svelte.js';
  import StatTile from '../components/StatTile.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import StatusTag from '../components/StatusTag.svelte';
  import PctBar from '../components/PctBar.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import { fmtBytes, fmtInt, fmtUptime, fmtMs } from '../lib/format.js';
  import { getNow as liveNow } from '../lib/clock.svelte.js';

  // Cross-panel navigation is delegated to App.svelte so this panel never
  // imports the navigation store (spec §7.2.1).
  let { onJumpToEndpoints = () => {} } = $props();

  const sys = $derived(overview.data?.system);
  const proxy = $derived(overview.data?.proxy);
  const loading = $derived(overview.status === 'loading');

  // endpoints_up arrives as a pre-formatted "x/y" string; split for the tile.
  const upTotal = $derived(sys?.endpoints_up ?? '/');
  const up = $derived(Number(upTotal.split('/')[0]) || 0);
  const total = $derived(Number(upTotal.split('/')[1]) || 0);

  const successPctNum = $derived(parseFloat(sys?.success_rate) || 0);
  const trafficBytes = $derived(parseTrafficBytes(sys?.total_traffic));

  const endpointList = $derived(endpoints.data?.endpoints ?? []);

  // Glance table columns, wired through the shared SortableTable so the
  // interaction and aria-sort semantics match the Endpoints panel exactly.
  const glanceColumns = [
    { key: 'name', label: 'Endpoint', sortable: true, sticky: true },
    { key: 'status_rank', label: 'Status', sortable: true },
    { key: 'success_rate_num', label: 'Success', sortable: true, num: true },
    { key: 'avg_latency_ms', label: 'Avg latency', sortable: true, num: true },
  ];

  // Default sort is unhealthy-first: an ops glance view should surface the
  // backends that need attention before the ones already fine.
  function statusRank(s) {
    if (s === 'offline' || s === 'critical') return 0;
    if (s === 'degraded' || s === 'unhealthy') return 1;
    if (s === 'healthy') return 2;
    return 3;
  }

  // avg_latency_ms is the API's numeric average PROXY latency field - never
  // derive it from response_time (last HEALTH-CHECK probe latency, a
  // different metric, and a formatted string like "1.5s" that parseInt
  // would silently mangle to 1).
  const glanceRows = $derived(
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

  function statusGlyph(s) {
    return s === 'healthy' ? '●' : s === 'degraded' || s === 'unhealthy' ? '◐' : '○';
  }
  function statusCls(s) {
    return s === 'healthy' ? 'green' : s === 'degraded' || s === 'unhealthy' ? 'amber' : 'red';
  }

  // total_traffic arrives formatted as e.g. "172.02 GB". Re-parse to bytes so
  // fmtBytes (our single formatting authority) renders it consistently.
  function parseTrafficBytes(s) {
    if (!s) return null;
    const m = String(s).match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)$/);
    if (!m) return null;
    const units = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4, PB: 1024 ** 5 };
    return parseFloat(m[1]) * units[m[2]];
  }

  function jumpToEndpoints(name) {
    onJumpToEndpoints();
    queueMicrotask(() => {
      document.getElementById(`ep-${cssId(name)}`)?.scrollIntoView({ block: 'center' });
    });
  }
  function cssId(name) {
    return String(name).replace(/[^a-z0-9]+/gi, '-');
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
        {#each Array(8) as _, i}<div class="skeleton tile-skel"></div>{/each}
      </div>
    </div>
    <div class="section">
      <div class="section-head"><h3>Herd at a glance</h3></div>
      {#each Array(4) as _, i}<div class="skeleton row-skel"></div>{/each}
    </div>
  {:else if sys}
    <div class="section">
      <div class="tile-grid">
        <StatTile
          label="System status"
          valueHtml={true}
          value={`<span class="glyph g-${statusCls(sys.status)}" aria-hidden="true">${statusGlyph(sys.status)}</span> ${sys.status}`}
          subHtml={true}
          sub={`<strong>${proxy?.engine ?? 'olla'}</strong> engine · <strong>${proxy?.balancer ?? 'priority'}</strong> balancing`}
        />
        <StatTile
          label="Endpoints up"
          valueHtml={true}
          value={`${up}<span class="unit">/ ${total}</span>`}
          sub="{total - up} unavailable"
        />
        <StatTile
          label="Success rate"
          value={sys.success_rate}
          sub="{fmtInt(sys.total_failures)} failures logged"
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
        initialSort={{ key: 'status_rank', dir: 'asc' }}
        rowId={(row) => `glance-${cssId(row.name)}`}
      >
        {#snippet rowSnippet({ row: e })}
          <td class="col-sticky">
            <button
              class="glance-link"
              type="button"
              onclick={() => jumpToEndpoints(e.name)}
              title="Open {e.name} in the Endpoints panel"
            >
              <span class="glyph g-{statusCls(e.status)}" aria-hidden="true">{statusGlyph(e.status)}</span>
              <span class="txt">{e.name}</span>
            </button>
          </td>
          <td><StatusTag status={e.status} /></td>
          <td class="num"><PctBar pct={e.success_rate_num} hasData={e.request_count > 0} /></td>
          <td class="num">
            {#if typeof e.avg_latency_ms === 'number' && e.request_count > 0}{fmtMs(e.avg_latency_ms)}{:else}<span class="dash">—</span>{/if}
          </td>
        {/snippet}
      </SortableTable>
    </div>
  {/if}
</div>
