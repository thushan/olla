<script lang="ts">
  import { models } from '../lib/stores/models.svelte';
  import SortableTable from '../components/SortableTable.svelte';
  import type { Column, SortState } from '../components/SortableTable.svelte';
  import StatusBanner from '../components/StatusBanner.svelte';
  import { fmtAgo } from '../lib/format';
  import { stableId } from '../lib/dom-id';
  import { jumpToEndpoint } from '../lib/jump-to-endpoint';
  import { getNow as liveNow } from '../lib/clock.svelte';
  import type { ModelSummary } from '../lib/types';

  // View-model row: the contract row plus derived numeric fields the table
  // sorts on (size_bytes from the size string, endpoints_count from the
  // endpoints list length). Omit+intersect so the row type flows through a
  // mapped type, which picks up the implicit index signature required by
  // SortableTable's `Row extends Record<string, unknown>` constraint; a
  // plain `ModelSummary & {...}` intersection does not.
  type ModelRow = Omit<ModelSummary, never> & {
    size_bytes: number;
    endpoints_count: number;
  };

  const loading = $derived(models.status === 'loading');
  // Mounted only when active (App.svelte does the routing).

  // This panel owns the models store lifecycle. The models payload is
  // the heaviest (detailed+group), so it must only poll while Models is open.
  // start() fires an immediate tick on mount; stop() deactivates and aborts
  // any in-flight request on unmount.
  $effect(() => {
    models.start();
    return () => models.stop();
  });

  // Detailed+group form: model_groups is populated. Fall back to recent_models
  // if the backend didn't honour the query params.
  const groups = $derived(models.data?.model_groups ?? []);
  const flatRecent = $derived(models.data?.recent_models ?? []);

  // Re-evaluated each tick so last_seen_at stays "Xs ago" live.
  const now = $derived(liveNow());

  // Discovery-only columns. Per-model traffic figures (requests/success/
  // p95/p99) are intentionally absent: nothing on the proxy path populates
  // them, so they would always read zero and mislead the operator. Wiring
  // that up on the proxy path is deferred to a later change.
  const columns: Column[] = [
    { key: 'name', label: 'Model', sortable: true, sticky: true },
    { key: 'params', label: 'Params', sortable: true },
    { key: 'quant', label: 'Quant', sortable: true },
    { key: 'size_bytes', label: 'Size', sortable: true, num: true, align: 'right' },
    { key: 'endpoints_count', label: 'Endpoints', sortable: true, num: true, align: 'right' },
    { key: 'last_seen_at', label: 'Last seen', sortable: false },
  ];

  // Derive a numeric size for sort/aria from the model's own size string.
  // Previously this preferred a summed per_endpoint bytes total, but
  // per_endpoint has no prior art in the backend response and was cut.
  function sizeBytesOf(m: ModelSummary): number {
    if (!m.size) return 0;
    const u: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4, PB: 1024 ** 5 };
    const match = String(m.size).match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)$/);
    return match ? parseFloat(match[1]) * u[match[2]] : 0;
  }

  function normalise(m: ModelSummary): ModelRow {
    return {
      ...m,
      size_bytes: sizeBytesOf(m),
      endpoints_count: m.endpoints?.length ?? 0,
    };
  }

  // Pill click-through delegates to the shared jumpToEndpoint helper
  // (lib/jump-to-endpoint). Endpoint display names are legitimately
  // non-unique (EndpointsPanel.dup-names.test), so the target must be keyed
  // on the per-model endpoint_ids array, not a name->id lookup. The backend
  // sorts endpoint_ids positionally against endpoints, so endpoints[i] is
  // hosted on endpoint_ids[i]. The helper owns the retry/scroll/focus/flash
  // concerns; see its header comment for the first-navigation fetch race.

  // Group order: alphabetical, with "unknown" pushed to the end (matches backend).
  function groupOrder(g: typeof groups): typeof groups {
    return [...g].sort((a, b) => {
      if (a.family === 'unknown') return 1;
      if (b.family === 'unknown') return -1;
      return a.family < b.family ? -1 : a.family > b.family ? 1 : 0;
    });
  }

  // rowKey is the each-key for the flat path; domId is the DOM id attribute
  // the grouped view sets manually below. Both use the collision-resistant stableId hash
  // (lib/dom-id) rather than the old punctuation-stripping slug, so two
  // model names that collide once slugged ("qwen3:8b" vs a hypothetical
  // "qwen3-8b") cannot resolve to the same DOM id. SortableTable additionally
  // disambiguates any each-key collision as a last line of defence.
  function rowKey(m: ModelRow): string {
    return m.name;
  }
  function domId(m: ModelRow): string {
    return `model-${stableId(m.name)}`;
  }

  const orderedGroups = $derived(groupOrder(groups));

  // Empty rows literal typed against ModelRow so the generic table infers
  // Row = ModelRow. Without this, `rows={[]}` collapses inference to `never`
  // and the groupSnippet's sortRows(list: never[]) cannot accept the
  // normalised model list.
  const emptyRows: ModelRow[] = [];
  const flatInitialSort: SortState = { key: 'name', dir: 'asc' };
</script>

<div
  id="panel-models"
  class="panel is-active"
  role="tabpanel"
  aria-labelledby="tab-models"
  tabindex="0"
  data-state={models.status === 'error' || models.status === 'stale' ? models.status : null}
>
  <StatusBanner store={models} />

  {#if loading}
    <div class="table-scroll"><div class="scroll-hint">loading…</div>
      {#each Array(6) as _, i}<div class="skeleton row-skel" style="margin:6px 10px"></div>{/each}
    </div>
  {:else if orderedGroups.length}
    <SortableTable
      {columns}
      rows={emptyRows}
      initialSort={null}
      showScrollHint={true}
    >
      {#snippet groupSnippet({ sortRows, cellClass })}
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
            <tr id={domId(m)}>
              <td class="col-sticky">
                <strong>{m.name}</strong>
                {#if m.aliases?.length}
                  <div class="model-aliases">{m.aliases.join(', ')}</div>
                {/if}
              </td>
              <td>{m.params || '—'}</td>
              <td>{m.quant || '—'}</td>
              <td class={cellClass('size_bytes')}>{m.size || '—'}</td>
              <td class={cellClass('endpoints_count')}>
                {#if m.endpoints?.length}
                  <div class="endpoint-pills">
                    {#each m.endpoints as ep, i}
                      {@const id = m.endpoint_ids?.[i]}
                      {#if id}
                        <button
                          type="button"
                          class="pill pill-link"
                          data-endpoint-id={id}
                          title="Open {ep} in the Endpoints panel"
                          onclick={() => jumpToEndpoint(id)}
                        >{ep}</button>
                      {:else}
                        <span class="pill">{ep}</span>
                      {/if}
                    {/each}
                  </div>
                {:else}
                  <span class="dash">0</span>
                {/if}
              </td>
              <td>{fmtAgo(m.last_seen_at, now) || 'never'}</td>
            </tr>
          {/each}
        {/each}
      {/snippet}
    </SortableTable>
  {:else if flatRecent.length}
    <SortableTable {columns} rows={flatRecent.map(normalise)} initialSort={flatInitialSort} rowId={rowKey}>
      {#snippet rowSnippet({ row: m, cellClass })}
        <td class="col-sticky">
          <strong>{m.name}</strong>
          {#if m.aliases?.length}
            <div class="model-aliases">{m.aliases.join(', ')}</div>
          {/if}
        </td>
        <td>{m.params || '—'}</td>
        <td>{m.quant || '—'}</td>
        <td class={cellClass('size_bytes')}>{m.size || '—'}</td>
        <td class={cellClass('endpoints_count')}>
          {#if m.endpoints?.length}
            <div class="endpoint-pills">
              {#each m.endpoints as ep, i}
                {@const id = m.endpoint_ids?.[i]}
                {#if id}
                  <button
                    type="button"
                    class="pill pill-link"
                    data-endpoint-id={id}
                    title="Open {ep} in the Endpoints panel"
                    onclick={() => jumpToEndpoint(id)}
                  >{ep}</button>
                {:else}
                  <span class="pill">{ep}</span>
                {/if}
              {/each}
            </div>
          {:else}<span class="dash">0</span>{/if}
        </td>
        <td>{fmtAgo(m.last_seen_at, now) || 'never'}</td>
      {/snippet}
    </SortableTable>
  {:else if models.hasData}
    <!-- Gated on hasData (data !== null), not just "groups/flatRecent are
         empty": a failed fetch also leaves both empty, and without this
         guard the empty-state copy rendered alongside StatusBanner's error,
         telling the operator "no models" when the real story was "the
         request failed". -->
    <p class="panel-intro">No models discovered yet. Once backends respond to discovery, models will appear here.</p>
  {/if}
</div>

<style>
  /* Clickable pill: visually identical to the plain span pill at rest, with a
     quiet accent hover and the shared focus ring so keyboard users see where
     focus landed. Background/font reset because <button> ships its own. */
  .pill-link {
    cursor: pointer;
    background: transparent;
    font: inherit;
  }
  .pill-link:hover {
    color: var(--accent);
    border-color: var(--accent);
  }
  .pill-link:focus-visible {
    outline: var(--focus-ring);
    outline-offset: 1px;
  }
  /* Quiet alias subtext under the model name in the sticky cell. */
  .model-aliases {
    font-size: 0.65rem;
    color: var(--text-faint);
    font-weight: normal;
  }
</style>
