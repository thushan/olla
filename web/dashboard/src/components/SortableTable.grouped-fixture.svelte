<script>
  // Test-only fixture: exercises SortableTable's grouped (groupSnippet) path
  // with real Svelte snippet syntax, standing in for ModelsPanel's grouped
  // view without dragging in the whole panel + its stores. See
  // SortableTable.test.js.
  import SortableTable from './SortableTable.svelte';

  let { columns, groups } = $props();
</script>

<SortableTable {columns} rows={[]} initialSort={null} showScrollHint={false}>
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
