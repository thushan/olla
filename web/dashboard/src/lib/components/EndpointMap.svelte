<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  const endpoints = $derived(dashboardStore.endpoints || []);
  let selectedEndpoint = $state(null);
  
  // Group endpoints by type
  const endpointGroups = $derived.by(() => {
    const groups = {
      ollama: [],
      'lm-studio': [],
      openai: [],
      other: []
    };
    
    endpoints.forEach(ep => {
      const type = ep.backend_type || ep.type || 'other';
      const groupKey = groups[type] ? type : 'other';
      groups[groupKey].push(ep);
    });
    
    return groups;
  });
  
  // Get status color
  function getStatusColor(status) {
    switch(status) {
      case 'online': return 'from-green-400 to-green-600';
      case 'offline': return 'from-red-400 to-red-600';
      case 'degraded': return 'from-yellow-400 to-yellow-600';
      default: return 'from-gray-400 to-gray-600';
    }
  }
  
  // Get status glow
  function getStatusGlow(status) {
    switch(status) {
      case 'online': return 'shadow-green-500/50';
      case 'offline': return 'shadow-red-500/50';
      case 'degraded': return 'shadow-yellow-500/50';
      default: return 'shadow-gray-500/50';
    }
  }
  
  // Format latency
  function formatLatency(ms) {
    if (!ms) return '--';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 h-full">
  <div class="mb-4">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Endpoint Status</h2>
  </div>
  
  <!-- Endpoint Groups -->
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
    {#each Object.entries(endpointGroups) as [type, typeEndpoints]}
      {#if typeEndpoints.length > 0}
        <div class="bg-gray-50 dark:bg-gray-900/50 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
          <div class="flex items-center gap-2 mb-3">
            <span class="text-lg">
              {type === 'ollama' ? '🦙' : type === 'lm-studio' ? '🎨' : type === 'openai' ? '🤖' : '🔌'}
            </span>
            <h3 class="font-medium text-gray-900 dark:text-white capitalize">{type}</h3>
          </div>
          <div class="space-y-2">
            {#each typeEndpoints as endpoint}
              <button
                class="w-full text-left p-2 rounded hover:bg-gray-100 dark:hover:bg-gray-800 {selectedEndpoint?.name === endpoint.name ? 'bg-gray-100 dark:bg-gray-800' : ''}"
                on:click={() => selectedEndpoint = selectedEndpoint?.name === endpoint.name ? null : endpoint}
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <div class="w-2 h-2 rounded-full {endpoint.status === 'online' ? 'bg-green-500' : 'bg-red-500'}"></div>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{endpoint.name}</span>
                  </div>
                  {#if endpoint.priority >= 100}
                    <span class="text-xs text-purple-600 dark:text-purple-400">⚡</span>
                  {/if}
                </div>
                {#if endpoint.last_latency_ms}
                  <div class="text-xs text-gray-600 dark:text-gray-400 ml-4">
                    {formatLatency(endpoint.last_latency_ms)}
                  </div>
                {/if}
              </button>
            {/each}
          </div>
        </div>
      {/if}
    {/each}
  </div>
  
  <!-- Selected Endpoint Details -->
  {#if selectedEndpoint}
    <div class="mt-4 p-4 bg-gray-50 dark:bg-gray-900/50 rounded-xl">
      <div class="flex items-center justify-between mb-2">
        <h3 class="font-semibold text-gray-900 dark:text-white">{selectedEndpoint.name}</h3>
        <span class="px-2 py-1 text-xs font-medium rounded-full {selectedEndpoint.status === 'online' ? 'bg-green-100 text-green-700 dark:bg-green-900/20 dark:text-green-400' : 'bg-red-100 text-red-700 dark:bg-red-900/20 dark:text-red-400'}">
          {selectedEndpoint.status}
        </span>
      </div>
      <div class="grid grid-cols-2 gap-4 text-sm">
        <div>
          <span class="text-gray-600 dark:text-gray-400">Type:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.backend_type}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Priority:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.priority}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Models:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.models?.length || 0}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Health Checks:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.health_checks || 0}</span>
        </div>
      </div>
    </div>
  {/if}
</div>

