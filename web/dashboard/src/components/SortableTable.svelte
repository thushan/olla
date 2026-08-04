<script module lang="ts">
  // Exported so callers (panels) can share the column / sort-state shape
  // without re-declaring it. Lives in module scope: instance scope cannot
  // hold `export` modifiers, and these types don't depend on `Row`.
  export type SortDirection = 'asc' | 'desc';
  export interface SortState {
    key: string;
    dir: SortDirection;
  }
  export interface Column {
    key: string;
    label: string;
    sortable?: boolean;
    num?: boolean;
    align?: 'right';
    sticky?: boolean;
    title?: string;
  }
</script>

<script lang="ts" generics="Row extends Record<string, unknown>">
  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';

  // Generic column-driven table. Children are rendered via the `row` snippet,
  // receiving the sorted row. Column sort state is owned here per-table
  // instance; the parent only describes the columns and provides the data.
  //
  // `columns`: [{ key, label, sortable=false, num=false, align, sticky=false, title }]
  //   `title` is an optional caveat rendered as the header's native tooltip -
  //   e.g. qualifying a metric's real meaning when the label alone doesn't
  //   have room to. `num` is sort semantics only (numeric compare). `align` is the
  //   presentation concern ('right' today) and is the ONLY thing that drives
  //   right-alignment, on both the header and the cell - see `cellClass`
  //   below. Keeping these separate means a future composite column (a bar,
  //   chips) declares `align: 'right'` and gets a real alignment mechanism
  //   (margin-left: auto on its flex child, via components.css) instead of
  //   text-align, which only ever moved inline text and silently did nothing
  //   for block/flex content (C10).
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

  interface Props {
    columns?: Column[];
    rows?: Row[];
    initialSort?: SortState | null;
    rowId?: (row: Row, i: number) => string;
    rowDomId?: ((row: Row, i: number) => string) | null;
    rowSnippet?: Snippet<[{ row: Row; cellClass: (key: string) => string }]> | null;
    groupSnippet?:
      | Snippet<
          [
            {
              rows: Row[];
              sort: SortState | null;
              sortRows: (list: Row[]) => Row[];
              cellClass: (key: string) => string;
            }
          ]
        >
      | null;
    showScrollHint?: boolean;
  }

  let {
    columns = [],
    rows = [],
    initialSort = null,
    rowId = (_r: Row, i: number): string => `r-${i}`,
    rowDomId = null,
    rowSnippet = null,
    groupSnippet = null,
    showScrollHint = true,
  }: Props = $props();

  // Initial sort seeds local state once; later prop changes are deliberately
  // ignored: the user owns sort state after mount.
  let sort: SortState | null = $state(untrack(() => initialSort));

  function toggle(key: string): void {
    if (!sort || sort.key !== key) {
      sort = { key, dir: 'desc' };
      return;
    }
    sort = { key, dir: sort.dir === 'asc' ? 'desc' : 'asc' };
  }

  // Column-key-driven comparator. Runtime behaviour is unchanged: string
  // branch lowercases both; otherwise null/undefined coalesces to 0; then a
  // plain `<`/`>` compare. The narrowing just lets `unknown` from `Row[key]`
  // type-check at the comparison point without changing what runs.
  function compare(a: Row, b: Row): number {
    if (!sort) return 0;
    const key = sort.key;
    let va: unknown = a[key];
    let vb: unknown = b[key];
    if (typeof va === 'string') {
      va = va.toLowerCase();
      vb = String(vb ?? '').toLowerCase();
    } else if (vb === null || vb === undefined) {
      vb = 0;
    } else if (va === null || va === undefined) {
      va = 0;
    }
    // Both operands are now strings or numbers; cast to satisfy TS without
    // altering the relational compare semantics.
    const cmp = (va as string | number) < (vb as string | number)
      ? -1
      : (va as string | number) > (vb as string | number)
        ? 1
        : 0;
    return sort.dir === 'asc' ? cmp : -cmp;
  }

  const sorted: Row[] = $derived(sort ? [...rows].sort(compare) : rows);

  // Defence in depth against each_key_duplicate: Svelte throws (and, with no
  // error boundary, blanks the whole table body) the moment two rows share a
  // key. Callers must key on a truly unique field, but if they don't - or if
  // the data genuinely carries duplicates (two endpoints with the same name) -
  // we suffix collisions with an ordinal so the table keeps rendering. The
  // raw rowId is preserved for the non-colliding majority.
  const uniqueKeyed = $derived(dedupeKeys(sorted, rowId));

  function dedupeKeys(
    list: Row[],
    keyFor: (row: Row, i: number) => string
  ): { row: Row; key: string }[] {
    const totals = new Map<string, number>();
    for (const r of list) {
      const k = keyFor(r, 0);
      totals.set(k, (totals.get(k) ?? 0) + 1);
    }
    const issued = new Map<string, number>();
    return list.map((row, i) => {
      const k = keyFor(row, i);
      if ((totals.get(k) ?? 0) === 1) return { row, key: k };
      const n = issued.get(k) ?? 0;
      issued.set(k, n + 1);
      return { row, key: `${k}#${n}` };
    });
  }

  // Exposed to groupSnippet so a grouped view (Models panel) can sort each
  // family's own row list against the same comparator/sort state the header
  // click updates. Without this the group snippet has no way to see `sort`,
  // so clicking a header flipped the indicator/aria-sort but never touched
  // row order.
  function sortRows(list: Row[]): Row[] {
    return sort ? [...list].sort(compare) : list;
  }

  function ariaSort(c: Column): 'none' | 'ascending' | 'descending' {
    if (!sort || sort.key !== c.key) return 'none';
    return sort.dir === 'asc' ? 'ascending' : 'descending';
  }
  function indicator(c: Column): string {
    if (!sort || sort.key !== c.key) return '↕';
    return sort.dir === 'asc' ? '▲' : '▼';
  }

  // The single source of truth for a cell's classes, driven entirely by the
  // column definition. Handed to rowSnippet/groupSnippet so callers stop
  // hand-authoring "num align-right" literals on each <td> - the class a row
  // renders can no longer drift from what its column declares.
  function cellClass(key: string): string {
    const c = columns.find((col) => col.key === key);
    if (!c) return '';
    return [c.num ? 'num' : '', c.align === 'right' ? 'align-right' : ''].filter(Boolean).join(' ');
  }

  function headerClass(c: Column): string {
    return [c.align === 'right' ? 'align-right' : '', c.sticky ? 'col-sticky' : '']
      .filter(Boolean)
      .join(' ');
  }
</script>

<div class="table-scroll">
  {#if showScrollHint}<div class="scroll-hint">→ scroll for more columns</div>{/if}
  <table>
    <thead>
      <tr>
        {#each columns as c}
          <th
            class={headerClass(c)}
            aria-sort={c.sortable ? ariaSort(c) : undefined}
            title={c.title}
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
        {@render groupSnippet({ rows: sorted, sort, sortRows, cellClass })}
      {:else}
        {#each uniqueKeyed as kr, i (kr.key)}
          <tr id={rowDomId ? rowDomId(kr.row, i) : undefined} tabindex={rowDomId ? -1 : undefined}>
            {@render rowSnippet?.({ row: kr.row, cellClass })}
          </tr>
        {/each}
      {/if}
    </tbody>
  </table>
</div>
