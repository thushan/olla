<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { globalCache, endpointCacheKey } from '$lib/utils/dataCache.js';
  
  const endpoints = $derived(dashboardStore.endpoints || []);
  let selectedEndpoint = $state(null);
  
  // Cache for endpoint data to prevent UI jumping
  let endpointCache = $state({});
  
  // Update cache with stable values
  $effect(() => {
    endpoints.forEach(ep => {
      if (!endpointCache[ep.name]) {
        endpointCache[ep.name] = {
          name: ep.name,
          backend_type: ep.backend_type || ep.type,
          status: ep.status,
          priority: ep.priority,
          last_latency_ms: ep.last_latency_ms,
          model_count: ep.models?.count || ep.model_count || 0,
          health_checks: ep.health_checks || 0
        };
      } else {
        // Update only if values are meaningful
        if (ep.status) endpointCache[ep.name].status = ep.status;
        if (ep.priority !== undefined) endpointCache[ep.name].priority = ep.priority;
        if (ep.last_latency_ms) endpointCache[ep.name].last_latency_ms = ep.last_latency_ms;
        
        // Use stable cache for counts to prevent jumping
        const modelCount = ep.models?.count || ep.model_count || 0;
        endpointCache[ep.name].model_count = globalCache.update(
          endpointCacheKey(ep, 'model_count'),
          modelCount
        );
        
        const healthChecks = ep.health_checks || 0;
        endpointCache[ep.name].health_checks = globalCache.update(
          endpointCacheKey(ep, 'health_checks'),
          healthChecks
        );
      }
    });
  });
  
  // Group endpoints by type with stable ordering
  const endpointGroups = $derived.by(() => {
    // Always show all groups in consistent order
    const groups = {
      ollama: { label: 'Ollama Endpoints', icon: '🦙', endpoints: [] },
      'lm-studio': { label: 'LM Studio Endpoints', icon: '🎨', endpoints: [] },
      openai: { label: 'OpenAI Compatible', icon: '🤖', endpoints: [] },
      vllm: { label: 'vLLM Endpoints', icon: '⚡', endpoints: [] },
      other: { label: 'Other Endpoints', icon: '🔌', endpoints: [] }
    };
    
    // Use cached endpoints to prevent jumping
    Object.values(endpointCache).forEach(ep => {
      const type = ep.backend_type || 'other';
      const groupKey = groups[type] ? type : 'other';
      groups[groupKey].endpoints.push(ep);
    });
    
    // Sort endpoints within each group by priority
    Object.values(groups).forEach(group => {
      group.endpoints.sort((a, b) => (b.priority || 0) - (a.priority || 0));
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
  
  <!-- Endpoint Groups - Always visible for stability -->
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-4">
    {#each Object.entries(endpointGroups) as [type, group]}
      <div class="bg-gray-50 dark:bg-gray-900/50 rounded-lg p-4 border border-gray-200 dark:border-gray-700 min-h-[200px]">
        <div class="flex items-center gap-2 mb-3">
          <span class="text-lg">{group.icon}</span>
          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">{group.label}</h3>
            <p class="text-xs text-gray-500 dark:text-gray-400">{group.endpoints.length} endpoint{group.endpoints.length !== 1 ? 's' : ''}</p>
          </div>
        </div>
        <div class="space-y-2 max-h-[300px] overflow-y-auto">
          {#if group.endpoints.length === 0}
            <div class="text-center py-8 text-gray-400 dark:text-gray-500">
              <p class="text-sm">No endpoints</p>
            </div>
          {:else}
            {#each group.endpoints as endpoint}
              <button
                class="w-full text-left p-2 rounded hover:bg-gray-100 dark:hover:bg-gray-800 {selectedEndpoint?.name === endpoint.name ? 'bg-gray-100 dark:bg-gray-800' : ''}"
                on:click={() => selectedEndpoint = selectedEndpoint?.name === endpoint.name ? null : endpoint}
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <div class="w-2 h-2 rounded-full {endpoint.status === 'online' || endpoint.status === 'healthy' ? 'bg-green-500' : 'bg-red-500'}"></div>
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
          {/if}
        </div>
      </div>
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

