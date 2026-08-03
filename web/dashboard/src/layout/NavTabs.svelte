<script lang="ts">
  // Tab presentation over the shared navigation store (spec §7.2.1: panels
  // never import this directly, only App.svelte's router and this component
  // do) - the store is what would swap to a sidebar presenter later without
  // any panel change. navigatePanel also pushes a history entry so browser
  // back/forward traverses panel switches.
  import { navigation, type Section } from '../lib/stores/navigation.svelte';
  import { navigatePanel } from '../lib/router';

  const LABELS: Record<Section, string> = { overview: 'Overview', endpoints: 'Endpoints', models: 'Models' };

  function activate(name: Section): void {
    navigatePanel(name);
    // Move focus to the newly-active tab once the DOM has updated.
    queueMicrotask(() => {
      document.getElementById(`tab-${name}`)?.focus();
    });
  }

  function onKeydown(e: KeyboardEvent): void {
    const idx = navigation.sections.indexOf(navigation.current);
    let next: number | null = null;
    if (e.key === 'ArrowRight') next = (idx + 1) % navigation.sections.length;
    else if (e.key === 'ArrowLeft') next = (idx - 1 + navigation.sections.length) % navigation.sections.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = navigation.sections.length - 1;
    if (next !== null) {
      e.preventDefault();
      activate(navigation.sections[next]);
    }
  }
</script>

<div class="tabs" role="tablist" tabindex="-1" aria-label="Dashboard sections" onkeydown={onKeydown}>
  {#each navigation.sections as name}
    <button
      id="tab-{name}"
      class="tab"
      role="tab"
      aria-selected={navigation.current === name ? 'true' : 'false'}
      aria-controls="panel-{name}"
      tabindex={navigation.current === name ? 0 : -1}
      onclick={() => activate(name)}
    >
      {LABELS[name] ?? name}
    </button>
  {/each}
</div>
