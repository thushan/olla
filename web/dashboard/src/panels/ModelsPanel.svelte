<script>
  import { models } from '../lib/stores/models.svelte.js';
  import SortableTable from '../components/SortableTable.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import { fmtAgo } from '../lib/format.js';
  import { getNow as liveNow } from '../lib/clock.svelte.js';

  const loading = $derived(models.status === 'loading');
  // Mounted only when active (App.svelte does the routing).

  // Detailed+group form: model_groups is populated. Fall back to recent_models
  // if the backend didn't honour the query params.
  const groups = $derived(models.data?.model_groups ?? []);
  const flatRecent = $derived(models.data?.recent_models ?? []);

  // Per-model request/latency stats from /internal/stats/models, keyed by
  // model name so the panel can merge without restructuring the grouped form.
  const stats = $derived(models.data?.stats ?? {});
  const statsSummary = $derived(models.data?.stats_summary ?? null);

  // Re-evaluated each tick so last_seen_at stays "Xs ago" live.
  const now = $derived(liveNow());

  // Sort each family's models by name as the API already does; SortableTable
  // will re-sort within group based on the column the user clicks.
  const columns = [
    { key: 'name', label: 'Model', sortable: true, sticky: true },
    { key: 'params', label: 'Params', sortable: true },
    { key: 'quant', label: 'Quant', sortable: true },
    { key: 'size_bytes', label: 'Size', sortable: true, num: true },
    { key: 'endpoints_count', label: 'Endpoints', sortable: true, num: true },
    { key: 'total_requests', label: 'Requests', sortable: true, num: true },
    { key: 'success_rate_num', label: 'Success', sortable: true, num: true },
    { key: 'p95_latency_ms', label: 'p95', sortable: true, num: true },
    { key: 'p99_latency_ms', label: 'p99', sortable: true, num: true },
    { key: 'last_seen_at', label: 'Last seen', sortable: false },
  ];

  // Derive size_bytes for sort/aria: prefer summed per_endpoint bytes, fall
  // back to parsing the merged size string. Renders as the API's size string.
  function sizeBytesOf(m) {
    if (m.per_endpoint?.length) {
      const sum = m.per_endpoint.reduce((s, p) => s + (p.size_bytes || 0), 0);
      if (sum > 0) return sum;
    }
    if (!m.size) return 0;
    const u = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4, PB: 1024 ** 5 };
    const match = String(m.size).match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)$/);
    return match ? parseFloat(match[1]) * u[match[2]] : 0;
  }

  // success_rate/p95_latency/p99_latency arrive pre-formatted ("98.5%",
  // "1.5s") for display. Sorting on the formatted strings compares
  // lexicographically ("100ms" < "9ms"), so every numeric column needs a
  // parsed numeric twin to sort against; the formatted string still renders
  // verbatim.
  function parseLatencyMs(s) {
    if (!s) return 0;
    const m = String(s).trim().match(/^([\d.]+)\s*(ms|s)$/);
    if (!m) return 0;
    const value = parseFloat(m[1]);
    return m[2] === 's' ? value * 1000 : value;
  }

  function normalise(m) {
    // Merge per-model stats from /internal/stats/models onto the row so the
    // request/success/latency columns have a single source of truth.
    const s = stats[m.name] ?? {};
    const merged = { ...m, ...s };
    return {
      ...merged,
      size_bytes: sizeBytesOf(m),
      endpoints_count: m.endpoints?.length ?? 0,
      success_rate_num: parseFloat(merged.success_rate) || 0,
      p95_latency_ms: parseLatencyMs(merged.p95_latency),
      p99_latency_ms: parseLatencyMs(merged.p99_latency),
    };
  }

  // Group order: alphabetical, with "unknown" pushed to the end (matches backend).
  function groupOrder(groups) {
    return [...groups].sort((a, b) => {
      if (a.family === 'unknown') return 1;
      if (b.family === 'unknown') return -1;
      return a.family < b.family ? -1 : a.family > b.family ? 1 : 0;
    });
  }

  function rowId(m) {
    return `model-${cssId(m.name)}`;
  }
  function cssId(name) {
    return String(name).replace(/[^a-z0-9]+/gi, '-');
  }

  function fmtRequests(n) {
    if (!Number.isFinite(Number(n)) || Number(n) <= 0) return '—';
    return Number(n).toLocaleString('en-AU');
  }

  const orderedGroups = $derived(groupOrder(groups));
</script>

<div
  id="panel-models"
  class="panel is-active"
  role="tabpanel"
  aria-labelledby="tab-models"
  tabindex="0"
  data-state={models.status === 'error' || models.status === 'stale' ? models.status : null}
>
  <h2>Models</h2>
  <p class="panel-intro">
    Models discovered across the herd, grouped by family. Per-endpoint detail is folded into the
    Endpoints column tooltip; sorting applies within each family group.
  </p>

  <StatusBanner store={models} />

  {#if loading}
    <div class="table-scroll"><div class="scroll-hint">loading…</div>
      {#each Array(6) as _, i}<div class="skeleton row-skel" style="margin:6px 10px"></div>{/each}
    </div>
  {:else if orderedGroups.length}
    <SortableTable
      {columns}
      rows={[]}
      initialSort={null}
      showScrollHint={true}
    >
      {#snippet groupSnippet({ sortRows })}
        <!-- The grouped layout is rendered by iterating families here; each
             group's own row list is run through sortRows() so a header click
             (which flips the table's aria-sort) actually reorders rows,
             instead of only flipping the indicator. -->
        {#each orderedGroups as group (group.family)}
          {@const normalised = group.models.map(normalise)}
          {@const sorted = sortRows(normalised)}
          <tr class="family-row">
            <th colspan={columns.length} scope="rowgroup">
              {group.family}
              <span class="family-count">{group.model_count} model{group.model_count === 1 ? '' : 's'} · {group.endpoints.length} endpoint{group.endpoints.length === 1 ? '' : 's'}</span>
            </th>
          </tr>
          {#each sorted as m (m.name)}
            <tr id={rowId(m)}>
              <td class="col-sticky"><strong>{m.name}</strong></td>
              <td>{m.params || '—'}</td>
              <td>{m.quant || '—'}</td>
              <td class="num">{m.size || '—'}</td>
              <td class="num">
                {#if m.endpoints?.length}
                  <div class="endpoint-pills">
                    {#each m.endpoints as ep}
                      <span class="pill" title={m.per_endpoint?.find((p) => p.endpoint === ep)?.parameter_size ?? ''}>{ep}</span>
                    {/each}
                  </div>
                {:else}
                  <span class="dash">0</span>
                {/if}
              </td>
              <td class="num">{m.total_requests ? fmtRequests(m.total_requests) : '—'}</td>
              <td>{m.success_rate || '—'}</td>
              <td>{m.p95_latency || '—'}</td>
              <td>{m.p99_latency || '—'}</td>
              <td>{fmtAgo(m.last_seen_at, now) || 'never'}</td>
            </tr>
          {/each}
        {/each}
      {/snippet}
    </SortableTable>
  {:else if flatRecent.length}
    <SortableTable {columns} rows={flatRecent.map(normalise)} initialSort={{ key: 'name', dir: 'asc' }} rowId={rowId}>
      {#snippet rowSnippet({ row: m })}
        <td class="col-sticky"><strong>{m.name}</strong></td>
        <td>{m.params || '—'}</td>
        <td>{m.quant || '—'}</td>
        <td class="num">{m.size || '—'}</td>
        <td class="num">
          {#if m.endpoints?.length}
            <div class="endpoint-pills">
              {#each m.endpoints as ep}<span class="pill">{ep}</span>{/each}
            </div>
          {:else}<span class="dash">0</span>{/if}
        </td>
        <td class="num">{m.total_requests ? fmtRequests(m.total_requests) : '—'}</td>
        <td>{m.success_rate || '—'}</td>
        <td>{m.p95_latency || '—'}</td>
        <td>{m.p99_latency || '—'}</td>
        <td>{fmtAgo(m.last_seen_at, now) || 'never'}</td>
      {/snippet}
    </SortableTable>
  {:else}
    <p class="panel-intro">No models discovered yet. Once backends respond to discovery, models will appear here.</p>
  {/if}
</div>
