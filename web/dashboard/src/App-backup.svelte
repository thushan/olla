<script>
  import { onMount, onDestroy } from 'svelte';
  import { setThemeStore } from '$lib/stores/theme.svelte.js';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { websocketService } from '$lib/services/websocket.js';
  import { navigationStore } from '$lib/stores/navigation.svelte.js';
  
  // Layout
  import Layout from '$lib/components/Layout.svelte';
  
  // Pages
  import Overview from '$lib/pages/Overview.svelte';
  import Endpoints from '$lib/pages/Endpoints.svelte';
  import Models from '$lib/pages/Models.svelte';
  import Performance from '$lib/pages/Performance.svelte';
  import Security from '$lib/pages/Security.svelte';
  import System from '$lib/pages/System.svelte';
  
  import './app.css';
  
  // Initialize theme store
  const theme = setThemeStore();
  
  // Current section
  const currentSection = $derived(navigationStore.currentSection);
  
  // Initialize dashboard and WebSocket on mount
  onMount(() => {
    dashboardStore.init();
    websocketService.connect();
  });
  
  onDestroy(() => {
    dashboardStore.destroy();
    websocketService.disconnect();
  });
</script>

<Layout>
  {#if currentSection === 'overview'}
    <Overview />
  {:else if currentSection === 'endpoints'}
    <Endpoints />
  {:else if currentSection === 'models'}
    <Models />
  {:else if currentSection === 'performance'}
    <Performance />
  {:else if currentSection === 'security'}
    <Security />
  {:else if currentSection === 'system'}
    <System />
  {/if}
</Layout>

