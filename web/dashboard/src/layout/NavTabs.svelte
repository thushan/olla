<script>
  // Tab presentation over the shared navigation store (spec §7.2.1: panels
  // never import this directly, only App.svelte's router and this component
  // do) - the store is what would swap to a sidebar presenter later without
  // any panel change.
  import { navigation } from '../lib/stores/navigation.svelte.js';

  const LABELS = { overview: 'Overview', endpoints: 'Endpoints', models: 'Models' };

  function activate(name) {
    navigation.set(name);
    // Move focus to the newly-active tab once the DOM has updated.
    queueMicrotask(() => {
      document.getElementById(`tab-${name}`)?.focus();
    });
  }

  function onKeydown(e) {
    const idx = navigation.sections.indexOf(navigation.current);
    let next = null;
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
