<script>
  import { untrack } from 'svelte';

  // Generic column-driven table. Children are rendered via the `row` snippet,
  // receiving the sorted row. Column sort state is owned here per-table
  // instance; the parent only describes the columns and provides the data.
  //
  // `columns`: [{ key, label, sortable=false, num=false, sticky=false }]
  // `rows`: array of objects keyed by column.key
  // `initialSort`: { key, dir: 'asc'|'desc' } or null
  // `rowId`: (row, i) => string, used as the tr KEY (defaults to index). Must
  //   be the row's exact unique identity - never a lossy transform (e.g. a
  //   CSS-safe slug), or two rows that collapse to the same slug trigger
  //   Svelte's each_key_duplicate and blank the entire table body.
  // `rowDomId`: optional (row, i) => string, used as the tr's `id` ATTRIBUTE
  //   in the generic rowSnippet path (e.g. so another panel can scrollIntoView
  //   + focus a specific row). Deliberately separate from `rowId`: the DOM id
  //   can use a CSS-safe slug since a jump target only needs to exist, not be
  //   collision-proof, but that same slug must never be used as the each key.
  // `rowSnippet`: snippet receiving { row } for each body row's <td> markup
  // `groupSnippet`: optional snippet rendered between family groups (Models
  //   panel), receiving { rows, sort, sortRows } - `sortRows(list)` applies
  //   the table's current comparator/sort state to a group's own row array,
  //   so the header click that flips `aria-sort` also reorders each group.

  let {
    columns = [],
    rows = [],
    initialSort = null,
    rowId = (_r, i) => `r-${i}`,
    rowDomId = null,
    rowSnippet = null,
    groupSnippet = null,
    showScrollHint = true,
  } = $props();

  // Initial sort seeds local state once; later prop changes are deliberately
  // ignored: the user owns sort state after mount.
  let sort = $state(untrack(() => initialSort));

  function toggle(key) {
    if (!sort || sort.key !== key) {
      sort = { key, dir: 'desc' };
      return;
    }
    sort = { key, dir: sort.dir === 'asc' ? 'desc' : 'asc' };
  }

  function compare(a, b) {
    if (!sort) return 0;
    let va = a[sort.key];
    let vb = b[sort.key];
    if (typeof va === 'string') {
      va = va.toLowerCase();
      vb = String(vb ?? '').toLowerCase();
    } else if (vb === null || vb === undefined) {
      vb = 0;
    } else if (va === null || va === undefined) {
      va = 0;
    }
    const cmp = va < vb ? -1 : va > vb ? 1 : 0;
    return sort.dir === 'asc' ? cmp : -cmp;
  }

  const sorted = $derived(sort ? [...rows].sort(compare) : rows);

  // Exposed to groupSnippet so a grouped view (Models panel) can sort each
  // family's own row list against the same comparator/sort state the header
  // click updates. Without this the group snippet has no way to see `sort`,
  // so clicking a header flipped the indicator/aria-sort but never touched
  // row order.
  function sortRows(list) {
    return sort ? [...list].sort(compare) : list;
  }

  function ariaSort(c) {
    if (!sort || sort.key !== c.key) return 'none';
    return sort.dir === 'asc' ? 'ascending' : 'descending';
  }
  function indicator(c) {
    if (!sort || sort.key !== c.key) return '↕';
    return sort.dir === 'asc' ? '▲' : '▼';
  }
</script>

<div class="table-scroll">
  {#if showScrollHint}<div class="scroll-hint">→ scroll for more columns</div>{/if}
  <table>
    <thead>
      <tr>
        {#each columns as c}
          <th
            class="{c.num ? 'num ' : ''}{c.sticky ? 'col-sticky' : ''}"
            aria-sort={c.sortable ? ariaSort(c) : undefined}
          >
            {#if c.sortable}
              <button class="sort-btn" type="button" onclick={() => toggle(c.key)}>
                {c.label}<span class="sort-ind" aria-hidden="true">{indicator(c)}</span>
              </button>
            {:else}
              <span class="sort-btn" style="cursor:default">{c.label}</span>
            {/if}
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#if groupSnippet}
        {@render groupSnippet({ rows: sorted, sort, sortRows })}
      {:else}
        {#each sorted as row, i (rowId(row, i))}
          <tr id={rowDomId ? rowDomId(row, i) : undefined} tabindex={rowDomId ? -1 : undefined}>
            {@render rowSnippet({ row })}
          </tr>
        {/each}
      {/if}
    </tbody>
  </table>
</div>
