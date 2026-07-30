<script>
  import { endpoints } from '../lib/stores/endpoints.svelte.js';
  import SortableTable from '../components/SortableTable.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import StatusTag from '../components/StatusTag.svelte';
  import RangeBar from '../components/RangeBar.svelte';
  import PctBar from '../components/PctBar.svelte';
  import { fmtAgo, fmtUntil } from '../lib/format.js';
  import { getNow as liveNow } from '../lib/clock.svelte.js';

  const data = $derived(endpoints.data?.endpoints ?? []);
  const loading = $derived(endpoints.status === 'loading');
  // App.svelte only mounts this panel when it is the active one, so the panel
  // is always rendered active once it appears.

  const globalLatencyMax = $derived(data.reduce((m, e) => Math.max(m, e.max_latency_ms ?? 0), 0));

  // success_rate arrives pre-formatted ("98.5%") so we expose a numeric
  // variant for sort/scaling. avg_latency_ms is the API's own numeric field
  // (average PROXY request latency) - it must NOT be derived from
  // response_time, which is the last HEALTH-CHECK probe's latency, an
  // unrelated metric. It may be absent (older backend) or null (no traffic
  // yet), in which case RangeBar shows the no-data placeholder rather than a
  // misleading 0.
  const rows = $derived(
    data.map((e) => ({
      ...e,
      success_rate_num: parseFloat(e.success_rate) || 0,
      avg_latency_ms: typeof e.avg_latency_ms === 'number' ? e.avg_latency_ms : null,
    }))
  );

  // Re-evaluated each second so the relative "Xs ago" labels stay live
  // without per-row setInterval. Reading liveNow() inside $derived wires
  // the dependency on the shared clock.
  const now = $derived(liveNow());

  const columns = [
    { key: 'name', label: 'Endpoint', sortable: true, sticky: true },
    { key: 'type', label: 'Type', sortable: true },
    { key: 'priority', label: 'Priority', sortable: true, num: true },
    { key: 'success_rate_num', label: 'Success', sortable: true, num: true },
    { key: 'avg_latency_ms', label: 'Latency', sortable: true, num: true },
    { key: 'model_count', label: 'Models', sortable: true, num: true },
    { key: 'request_count', label: 'Requests', sortable: true, num: true },
    { key: 'active_connections', label: 'Conn', sortable: true, num: true },
    { key: 'url', label: 'URL', sortable: false },
    { key: 'health_check_at', label: 'Last chk', sortable: false },
    { key: 'next_check_at', label: 'Next chk', sortable: false },
    { key: 'issues', label: 'Issues', sortable: false },
  ];

  function rowId(row) {
    return `ep-${cssId(row.name)}`;
  }
  function cssId(name) {
    return String(name).replace(/[^a-z0-9]+/gi, '-');
  }

  function issueList(issues) {
    if (!issues) return [];
    return String(issues)
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  }
</script>

<div
  id="panel-endpoints"
  class="panel is-active"
  role="tabpanel"
  aria-labelledby="tab-endpoints"
  tabindex="0"
  data-state={endpoints.status === 'error' || endpoints.status === 'stale' ? endpoints.status : null}
>
  <h2>Endpoints</h2>
  <p class="panel-intro">
    Every configured backend, its health-check state and routing weight.
  </p>

  <StatusBanner store={endpoints} />

  {#if loading}
    <div class="table-scroll"><div class="scroll-hint">loading…</div>
      {#each Array(6) as _, i}<div class="skeleton row-skel" style="margin:6px 10px"></div>{/each}
    </div>
  {:else}
    <SortableTable
      {columns}
      {rows}
      initialSort={{ key: 'priority', dir: 'desc' }}
      {rowId}
    >
      {#snippet rowSnippet({ row: e })}
        <td class="col-sticky">
          <div class="endpoint-cell">
            <span class="ep-name">
              <StatusTag status={e.status} />
              <span class="name-text" title={e.name}>{e.name}</span>
            </span>
            <span class="badge-type">{e.type}</span>
          </div>
        </td>
        <td>{e.type}</td>
        <td class="num">{e.priority}</td>
        <td class="num">
          <PctBar pct={e.success_rate_num} hasData={e.request_count > 0} />
        </td>
        <td class="num">
          <RangeBar
            min={e.min_latency_ms ?? 0}
            avg={e.avg_latency_ms}
            max={e.max_latency_ms ?? 0}
            globalMax={globalLatencyMax}
            label="latency"
          />
        </td>
        <td class="num">{#if e.model_count === 0}<span class="dash">0</span>{:else}{e.model_count}{/if}</td>
        <td class="num">{e.request_count?.toLocaleString('en-AU') ?? 0}</td>
        <td class="num">{#if e.active_connections > 0}{e.active_connections}{:else}<span class="dash">0</span>{/if}</td>
        <td class="url-cell" title={e.url}>{e.url || '—'}</td>
        <td>{fmtAgo(e.health_check_at, now) || 'never'}</td>
        <td>{fmtUntil(e.next_check_at, now) || '—'}</td>
        <td>
          {#if issueList(e.issues).length}
            <ul class="issues-list">
              {#each issueList(e.issues) as i}<li>{i}</li>{/each}
            </ul>
          {:else}
            <ul class="issues-list none"><li>none</li></ul>
          {/if}
        </td>
      {/snippet}
    </SortableTable>
  {/if}
</div>
