<script>
  import './app.css';
  import DashboardLayout from './layout/DashboardLayout.svelte';
  import OverviewPanel from './panels/OverviewPanel.svelte';
  import EndpointsPanel from './panels/EndpointsPanel.svelte';
  import ModelsPanel from './panels/ModelsPanel.svelte';
  import { pollScheduler } from './lib/poll-scheduler.js';
  import { startClock } from './lib/clock.svelte.js';
  import { theme } from './lib/stores/theme.svelte.js';
  import { navigation } from './lib/stores/navigation.svelte.js';

  // Importing the stores registers them with the scheduler (module side-effect).
  import './lib/stores/overview.svelte.js';
  import './lib/stores/endpoints.svelte.js';
  import './lib/stores/models.svelte.js';

  // App is the router: it reads the same navigation store NavTabs does,
  // rather than keeping its own separate `current`. Two sources of truth for
  // the active section is exactly how a programmatic jump (below) used to
  // render the right panel while the tab bar kept announcing the old one.

  // Lifecycle: start the shared scheduler on mount, stop on teardown. The
  // scheduler owns every setInterval/setTimeout in the SPA (spec §7.3).
  // The clock registers first so its job is in the map before start().
  $effect(() => {
    startClock();
    pollScheduler.start();
    return () => {
      pollScheduler.stop();
      theme.destroy();
    };
  });

  function jumpToEndpoints() {
    navigation.set('endpoints');
  }
</script>

<DashboardLayout>
  {#if navigation.current === 'overview'}
    <OverviewPanel onJumpToEndpoints={jumpToEndpoints} />
  {:else if navigation.current === 'endpoints'}
    <EndpointsPanel />
  {:else if navigation.current === 'models'}
    <ModelsPanel />
  {/if}
</DashboardLayout>
