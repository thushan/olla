<script lang="ts">
  import type { StatusFilter } from '../lib/herd';

  // Search box plus all/unhealthy chips, shared by the Overview herd table and
  // the Endpoints panel. State is owned by the parent so it can filter rows.
  interface Props {
    query?: string;
    status?: StatusFilter;
    label?: string;
  }
  let { query = $bindable(''), status = $bindable('all'), label = 'Filter endpoints' }: Props = $props();
</script>

<div class="herd-filter">
  <input
    class="filter-input"
    type="search"
    placeholder="Filter by name or type"
    aria-label={label}
    bind:value={query}
  />
  <div class="chip-group" role="group" aria-label="Status filter">
    <button type="button" class="chip" class:on={status === 'all'} aria-pressed={status === 'all'} onclick={() => (status = 'all')}>All</button>
    <button type="button" class="chip" class:on={status === 'unhealthy'} aria-pressed={status === 'unhealthy'} onclick={() => (status = 'unhealthy')}>Unhealthy</button>
  </div>
</div>
