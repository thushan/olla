<script lang="ts">
  // Test-only fixture: exercises SortableTable's grouped (groupSnippet) path
  // with real Svelte snippet syntax, standing in for ModelsPanel's grouped
  // view without dragging in the whole panel + its stores. See
  // SortableTable.test.ts.
  import SortableTable from './SortableTable.svelte';
  import type { Column } from './SortableTable.svelte';

  interface FixtureRow extends Record<string, unknown> {
    id: string;
    name: string;
    n: number;
  }
  interface FixtureGroup {
    id: string;
    rows: FixtureRow[];
  }

  interface Props {
    columns: Column[];
    groups: FixtureGroup[];
  }
  let { columns, groups }: Props = $props();
</script>

<SortableTable {columns} rows={[] as FixtureRow[]} initialSort={null} showScrollHint={false}>
  {#snippet groupSnippet({ sortRows })}
    {#each groups as group (group.id)}
      <tr class="family-row" data-testid="group-header" data-group={group.id}>
        <th colspan={columns.length}>{group.id}</th>
      </tr>
      {#each sortRows(group.rows) as row (row.id)}
        <tr data-testid="row" data-group={group.id} data-id={row.id}>
          <td>{row.name}</td>
          <td class="num">{row.n}</td>
        </tr>
      {/each}
    {/each}
  {/snippet}
</SortableTable>
