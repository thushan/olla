<script>
  import { navigationStore, navigateToSection } from '$lib/stores/navigation.svelte.js';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import ThemeToggle from './ThemeToggle.svelte';
  
  let { children } = $props();
  
  const currentSection = $derived(navigationStore.currentSection);
  const sections = $derived(navigationStore.sections);
  const systemHealth = $derived(dashboardStore.systemHealth);
  const wsConnected = $derived(dashboardStore.wsConnected);
  
  // Get health color
  function getHealthColor(health) {
    switch(health?.toLowerCase()) {
      case 'operational':
      case 'healthy':
        return 'text-green-500';
      case 'degraded':
        return 'text-yellow-500';
      case 'critical':
      case 'unhealthy':
        return 'text-red-500';
      default:
        return 'text-gray-500';
    }
  }
</script>

<div class="flex h-screen bg-gray-50 dark:bg-gray-900">
  <!-- Left Navigation -->
  <nav class="w-64 bg-white dark:bg-gray-800 shadow-lg flex flex-col">
    <!-- Header -->
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-purple-500 to-purple-600 flex items-center justify-center shadow-lg">
          <span class="text-white font-bold text-lg">O</span>
        </div>
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">Olla Dashboard</h1>
          <div class="flex items-center gap-2 mt-1">
            <div class="w-2 h-2 rounded-full {getHealthColor(systemHealth)} animate-pulse"></div>
            <span class="text-xs {getHealthColor(systemHealth)} capitalize">{systemHealth || 'Unknown'}</span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- Navigation Items -->
    <div class="flex-1 overflow-y-auto py-4">
      {#each sections as section}
        <button
          class="w-full px-4 py-3 flex items-center gap-3 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors {
            currentSection === section.id ? 'bg-purple-50 dark:bg-purple-900/20 border-r-4 border-purple-500' : ''
          }"
          onclick={() => navigateToSection(section.id)}
        >
          <span class="text-2xl">{section.icon}</span>
          <div class="text-left">
            <div class="font-medium text-gray-900 dark:text-white">{section.name}</div>
            <div class="text-xs text-gray-600 dark:text-gray-400">{section.description}</div>
          </div>
        </button>
      {/each}
    </div>
    
    <!-- Footer -->
    <div class="p-4 border-t border-gray-200 dark:border-gray-700 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
          <div class="w-2 h-2 rounded-full {wsConnected ? 'bg-green-500' : 'bg-red-500'}"></div>
          <span>{wsConnected ? 'Connected' : 'Disconnected'}</span>
        </div>
        <ThemeToggle />
      </div>
      <div class="text-xs text-gray-600 dark:text-gray-400 text-center">
        <div>{new Date().toLocaleTimeString()}</div>
        <div class="opacity-50 mt-1">Dashboard v1.3.0</div>
      </div>
    </div>
  </nav>
  
  <!-- Main Content Area -->
  <main class="flex-1 overflow-y-auto">
    <div class="p-6">
      {@render children?.()}
    </div>
  </main>
</div>

<style>
  /* Custom scrollbar for navigation */
  nav::-webkit-scrollbar {
    width: 4px;
  }
  
  nav::-webkit-scrollbar-track {
    background: transparent;
  }
  
  nav::-webkit-scrollbar-thumb {
    background: rgba(156, 163, 175, 0.3);
    border-radius: 2px;
  }
  
  nav::-webkit-scrollbar-thumb:hover {
    background: rgba(156, 163, 175, 0.5);
  }
</style>