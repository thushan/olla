<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const unifiedModels = $derived(dashboardStore.unifiedModels || []);
  const modelStats = $derived(dashboardStore.modelStats || {});
  const endpoints = $derived(dashboardStore.endpoints || []);
  const totalModelCount = $derived(unifiedModels.length);
  
  // Debug logging
  $effect(() => {
    console.log('[ModelList] UnifiedModels:', unifiedModels);
    console.log('[ModelList] ModelStats:', modelStats);
  });
  
  // Sort models: loaded models first, then by requests, then alphabetically
  const sortedModels = $derived.by(() => {
    if (!unifiedModels || !Array.isArray(unifiedModels)) return [];
    return [...unifiedModels].sort((a, b) => {
      // First sort by whether model is loaded (available) anywhere
      const aLoaded = a.olla?.availability?.some(avail => avail.state === 'available' || avail.state === 'loaded') ? 1 : 0;
      const bLoaded = b.olla?.availability?.some(avail => avail.state === 'available' || avail.state === 'loaded') ? 1 : 0;
      if (aLoaded !== bLoaded) return bLoaded - aLoaded;
      
      // Then by request count
      const aStats = modelStats?.models?.[a.id || a.name] || {};
      const bStats = modelStats?.models?.[b.id || b.name] || {};
      const aRequests = aStats.requests || 0;
      const bRequests = bStats.requests || 0;
      if (aRequests !== bRequests) return bRequests - aRequests;
      
      // Finally alphabetically
      return (a.id || a.name).localeCompare(b.id || b.name);
    });
  });
  
  // Get model stats
  function getModelStats(model) {
    const modelId = model.id || model.name;
    return modelStats?.models?.[modelId] || {};
  }
  
  // Format number
  function formatNumber(num) {
    if (!num) return '0';
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
  }
  
  // Get endpoint icon
  function getEndpointIcon(endpointType) {
    const icons = {
      'ollama': '🦙',
      'lm-studio': '🎨', 
      'openai': '🤖',
      'vllm': '⚡',
      'other': '🔌'
    };
    return icons[endpointType] || icons.other;
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-4 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Available Models</h3>
      <span class="text-sm text-gray-600 dark:text-gray-400">{totalModelCount} model{totalModelCount !== 1 ? 's' : ''} available</span>
    </div>
  </div>
  
  <div class="divide-y divide-gray-200 dark:divide-gray-700 max-h-[400px] overflow-y-auto">
    {#each sortedModels as model}
      {@const stats = getModelStats(model)}
      {@const isLoaded = model.olla?.availability?.some(avail => avail.state === 'available' || avail.state === 'loaded')}
      <div class="p-3 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
        <div class="flex items-center justify-between gap-3">
          <!-- Model Info -->
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <div class="font-medium text-gray-900 dark:text-white truncate">
                {model.id || model.name}
              </div>
              {#if isLoaded}
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-300">
                  Ready
                </span>
              {:else}
                <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400">
                  Not Loaded
                </span>
              {/if}
            </div>
            
            <div class="flex items-center gap-3 mt-1">
              <!-- Family -->
              {#if model.olla?.family}
                <span class="text-xs text-gray-500 dark:text-gray-400">{model.olla.family}</span>
              {/if}
              
              <!-- Endpoint Availability -->
              <div class="flex items-center gap-1">
                {#if model.olla?.availability && model.olla.availability.length > 0}
                  {#each model.olla.availability as avail}
                    {@const endpoint = endpoints.find(ep => ep.name === avail.endpoint)}
                    {@const endpointType = endpoint?.type || endpoint?.backend_type || 'unknown'}
                    {@const isAvailable = avail.state === 'available' || avail.state === 'loaded'}
                    <span 
                      class="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-xs {isAvailable ? 'bg-green-100 text-green-700 dark:bg-green-900/20 dark:text-green-300' : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'}"
                      title="{avail.endpoint} - {isAvailable ? 'Ready' : 'Not loaded'}"
                    >
                      <span class="text-xs">{getEndpointIcon(endpointType)}</span>
                      <span class="font-mono text-xs">{avail.endpoint}</span>
                    </span>
                  {/each}
                {:else}
                  <span class="text-xs text-gray-400 dark:text-gray-500">No endpoints</span>
                {/if}
              </div>
            </div>
          </div>
          
          <!-- Stats -->
          <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
            {#if stats.requests > 0}
              <div class="text-right">
                <div class="font-medium text-gray-900 dark:text-white">{formatNumber(stats.requests)}</div>
                <div class="text-xs">requests</div>
              </div>
            {/if}
            {#if stats.avg_latency}
              <div class="text-right">
                <div class="font-medium text-gray-900 dark:text-white">{stats.avg_latency}ms</div>
                <div class="text-xs">latency</div>
              </div>
            {/if}
            {#if stats.success_rate && stats.requests > 0}
              <div class="text-right">
                <div class="font-medium text-gray-900 dark:text-white">{stats.success_rate}%</div>
                <div class="text-xs">success</div>
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/each}
    
    {#if sortedModels.length === 0}
      <div class="text-center py-8 text-gray-500 dark:text-gray-400">
        <p>No models available</p>
      </div>
    {/if}
  </div>
</div>