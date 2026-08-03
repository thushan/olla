<script lang="ts">
  import { endpoints } from '../lib/stores/endpoints.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import type { Column, SortState } from '../components/SortableTable.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import StatusTag from '../components/StatusTag.svelte';
  import RangeBar from '../components/RangeBar.svelte';
  import PctBar from '../components/PctBar.svelte';
  import { fmtAgo, fmtUntil } from '../lib/format';
  import { stableId } from '../lib/dom-id';
  import { getNow as liveNow } from '../lib/clock.svelte';
  import type { EndpointSummary } from '../lib/types';

  // View-model row: the contract row plus the numeric variants the table sorts
  // and the bars scale on. success_rate arrives pre-formatted ("98.5%") so a
  // numeric companion drives sort/PctBar; avg_latency_ms is normalised to
  // number|null so RangeBar can pick its no-data placeholder. Declared as a
  // type alias (not an interface) so it picks up an implicit index signature
  // and satisfies SortableTable's `Row extends Record<string, unknown>`.
  type EndpointRow = Omit<EndpointSummary, 'avg_latency_ms'> & {
    success_rate_num: number;
    avg_latency_ms: number | null;
  };

  const data = $derived(endpoints.data?.endpoints ?? []);
  const loading = $derived(endpoints.status === 'loading');
  // App.svelte only mounts this panel when it is the active one, so the panel
  // is always rendered active once it appears.

  // WP-B3: this panel is the lifecycle owner of the endpoints store. Mount
  // activates the job (start() fires an immediate tick, so a tab switch
  // refreshes rather than showing stale data); unmount stops it so the
  // endpoints payload stops firing while Overview or Models is open. The
  // scheduler handles aborting the in-flight tick on stop.
  $effect(() => {
    endpoints.start();
    return () => endpoints.stop();
  });

  const globalLatencyMax = $derived(data.reduce((m, e) => Math.max(m, e.max_latency_ms ?? 0), 0));

  // success_rate arrives pre-formatted ("98.5%") so we expose a numeric
  // variant for sort/scaling. avg_latency_ms is the API's own numeric field
  // (average PROXY request latency) - it must NOT be derived from
  // response_time, which is the last HEALTH-CHECK probe's latency, an
  // unrelated metric. It may be absent (older backend) or null (no traffic
  // yet), in which case RangeBar shows the no-data placeholder rather than a
  // misleading 0.
  const rows: EndpointRow[] = $derived(
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

  const columns: Column[] = [
    { key: 'name', label: 'Endpoint', sortable: true, sticky: true },
    { key: 'type', label: 'Type', sortable: true },
    { key: 'priority', label: 'Priority', sortable: true, num: true, align: 'right' },
    {
      key: 'success_rate_num',
      label: 'Resp. rate',
      sortable: true,
      num: true,
      align: 'right',
      title: 'Counts any streamed response, regardless of HTTP status',
    },
    { key: 'avg_latency_ms', label: 'Latency', sortable: true, num: true, align: 'right' },
    { key: 'model_count', label: 'Models', sortable: true, num: true, align: 'right' },
    { key: 'request_count', label: 'Requests', sortable: true, num: true, align: 'right' },
    { key: 'active_connections', label: 'Conn', sortable: true, num: true, align: 'right' },
    { key: 'url', label: 'URL', sortable: false },
    { key: 'health_check_at', label: 'Last chk', sortable: false },
    { key: 'next_check_at', label: 'Next chk', sortable: false },
    { key: 'issues', label: 'Issues', sortable: false },
  ];

  const initialSort: SortState = { key: 'priority', dir: 'desc' };

  // Svelte's keyed-each identity must be structurally unique. `row.id` is a
  // stable identifier the backend derives from the RAW (pre-sanitisation)
  // endpoint URL (see stableEndpointID in handler_status_endpoints.go) - it
  // survives sanitiseDisplayURL stripping query/fragment from `url`, so two
  // endpoints differing only by query string (both arriving with the same
  // displayed url) still get distinct rows. Falls back to url/name for
  // backends that predate the `id` field. Keying on name alone blanks the
  // table when two endpoints share a name (or both have empty names) -
  // each_key_duplicate, with no error boundary to contain it. SortableTable
  // additionally disambiguates any residual collision as a last line of
  // defence.
  function rowId(row: EndpointRow): string {
    return row.id ?? row.url ?? row.name;
  }
  // DOM id derived from the same identity via a collision-resistant hash -
  // NOT the lossy cssId slug, which made "node.a" and "node-a" both resolve
  // to ep-node-a so getElementById returned the wrong row. See lib/dom-id.
  function rowDomId(row: EndpointRow): string {
    return `ep-${stableId(row.id ?? row.url ?? row.name)}`;
  }

  function issueList(issues?: string): string[] {
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
      {initialSort}
      {rowId}
      {rowDomId}
    >
      {#snippet rowSnippet({ row: e, cellClass })}
        <td class="col-sticky">
          <div class="endpoint-cell">
            <span class="ep-name">
              <StatusTag status={e.status} />
              <span class="name-text" title={e.name}>{e.name}</span>
            </span>
          </div>
        </td>
        <td>{e.type}</td>
        <td class={cellClass('priority')}>{e.priority}</td>
        <td class={cellClass('success_rate_num')}>
          <PctBar pct={e.success_rate_num} hasData={e.request_count > 0} status={e.status} />
        </td>
        <td class={cellClass('avg_latency_ms')}>
          <RangeBar
            min={e.min_latency_ms ?? 0}
            avg={e.avg_latency_ms}
            max={e.max_latency_ms ?? 0}
            globalMax={globalLatencyMax}
            label="latency"
          />
        </td>
        <td class={cellClass('model_count')}>{#if e.model_count === 0}<span class="dash">0</span>{:else}{e.model_count}{/if}</td>
        <td class={cellClass('request_count')}>{e.request_count?.toLocaleString('en-AU') ?? 0}</td>
        <td class={cellClass('active_connections')}>{#if e.active_connections > 0}{e.active_connections}{:else}<span class="dash">0</span>{/if}</td>
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
