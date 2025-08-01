<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const unifiedModels = $derived(dashboardStore.unifiedModels || []);
  const modelStats = $derived(dashboardStore.modelStats || {});
  const endpoints = $derived(dashboardStore.endpoints || []);
  const models = $derived(dashboardStore.models || []);
  const totalModelCount = $derived(unifiedModels.length);
  
  // Debug logging
  $effect(() => {
    console.log('[ModelList] UnifiedModels:', unifiedModels);
    console.log('[ModelList] Models:', models);
    console.log('[ModelList] ModelStats:', modelStats);
  });
  
  // Selected model for details
  let selectedModel = $state(null);
  
  // Sort models by request count
  const sortedModels = $derived.by(() => {
    if (!unifiedModels || !Array.isArray(unifiedModels)) return [];
    return [...unifiedModels].sort((a, b) => {
      const aStats = modelStats?.models?.[a.id || a.name] || {};
      const bStats = modelStats?.models?.[b.id || b.name] || {};
      const aRequests = aStats.requests || 0;
      const bRequests = bStats.requests || 0;
      return bRequests - aRequests;
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
  
  // Get endpoint for model
  function getModelEndpoint(model) {
    if (!model.endpoint) return null;
    return endpoints.find(ep => ep.name === model.endpoint);
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Available Models</h3>
      <span class="text-sm text-gray-600 dark:text-gray-400">{totalModelCount} model{totalModelCount !== 1 ? 's' : ''} available</span>
    </div>
  </div>
  
  <div class="flex">
    <!-- Model List -->
    <div class="w-full lg:w-1/2 border-r border-gray-200 dark:border-gray-700">
      <div class="divide-y divide-gray-200 dark:divide-gray-700 max-h-[600px] overflow-y-auto">
        {#each sortedModels as model}
          {@const stats = getModelStats(model)}
          {@const endpoint = getModelEndpoint(model)}
          <button
            class="w-full text-left p-4 hover:bg-gray-50 dark:hover:bg-gray-700/50 {selectedModel?.id === model.id ? 'bg-gray-50 dark:bg-gray-700/50' : ''}"
            on:click={() => selectedModel = model}
          >
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <div class="font-medium text-gray-900 dark:text-white">
                  {model.id || model.name}
                </div>
                {#if model.olla?.family}
                  <div class="text-sm text-gray-600 dark:text-gray-400">
                    {model.olla.family}
                  </div>
                {/if}
                <div class="flex items-center gap-4 mt-1">
                  {#if endpoint}
                    <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {endpoint.status === 'online' ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-300' : 'bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-300'}">
                      {endpoint.name}
                    </span>
                  {/if}
                  {#if stats.requests > 0}
                    <span class="text-xs text-gray-500 dark:text-gray-400">
                      {formatNumber(stats.requests)} requests
                    </span>
                  {/if}
                </div>
              </div>
              <div class="text-right">
                {#if stats.avg_latency}
                  <div class="text-sm font-medium text-gray-900 dark:text-white">
                    {stats.avg_latency}ms
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">
                    avg latency
                  </div>
                {/if}
              </div>
            </div>
          </button>
        {/each}
        
        {#if sortedModels.length === 0}
          <div class="text-center py-12 text-gray-500 dark:text-gray-400">
            <p>No models available</p>
          </div>
        {/if}
      </div>
    </div>
    
    <!-- Model Details -->
    <div class="w-full lg:w-1/2 p-6">
      {#if selectedModel}
        {@const stats = getModelStats(selectedModel)}
        {@const endpoint = getModelEndpoint(selectedModel)}
        
        <div class="space-y-6">
          <!-- Model Name -->
          <div>
            <h4 class="text-xl font-semibold text-gray-900 dark:text-white">
              {selectedModel.id || selectedModel.name}
            </h4>
            {#if selectedModel.olla?.family}
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {selectedModel.olla.family} family
              </p>
            {/if}
          </div>
          
          <!-- Model Info -->
          <div class="grid grid-cols-2 gap-4">
            {#if selectedModel.olla?.parameter_size}
              <div>
                <div class="text-sm text-gray-600 dark:text-gray-400">Parameters</div>
                <div class="text-lg font-medium text-gray-900 dark:text-white">
                  {selectedModel.olla.parameter_size}
                </div>
              </div>
            {/if}
            {#if selectedModel.context_length}
              <div>
                <div class="text-sm text-gray-600 dark:text-gray-400">Context Length</div>
                <div class="text-lg font-medium text-gray-900 dark:text-white">
                  {selectedModel.context_length.toLocaleString()}
                </div>
              </div>
            {/if}
          </div>
          
          <!-- Performance Stats -->
          {#if stats.requests > 0}
            <div>
              <h5 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Performance</h5>
              <div class="grid grid-cols-2 gap-4">
                <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                  <div class="text-sm text-gray-600 dark:text-gray-400">Total Requests</div>
                  <div class="text-xl font-semibold text-gray-900 dark:text-white">
                    {formatNumber(stats.requests)}
                  </div>
                </div>
                <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                  <div class="text-sm text-gray-600 dark:text-gray-400">Avg Latency</div>
                  <div class="text-xl font-semibold text-gray-900 dark:text-white">
                    {stats.avg_latency || 0}ms
                  </div>
                </div>
                <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                  <div class="text-sm text-gray-600 dark:text-gray-400">Success Rate</div>
                  <div class="text-xl font-semibold text-gray-900 dark:text-white">
                    {stats.success_rate || '100'}%
                  </div>
                </div>
                <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                  <div class="text-sm text-gray-600 dark:text-gray-400">Min/Max Latency</div>
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">
                    {stats.min_latency || 0}ms / {stats.max_latency || 0}ms
                  </div>
                </div>
              </div>
            </div>
          {/if}
          
          <!-- Endpoint Info -->
          {#if endpoint}
            <div>
              <h5 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Endpoint</h5>
              <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                <div class="flex items-center justify-between">
                  <div>
                    <div class="font-medium text-gray-900 dark:text-white">{endpoint.name}</div>
                    <div class="text-sm text-gray-600 dark:text-gray-400">{endpoint.url}</div>
                  </div>
                  <span class="px-2 py-1 text-xs font-medium rounded-full {endpoint.status === 'online' ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-300' : 'bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-300'}">
                    {endpoint.status}
                  </span>
                </div>
              </div>
            </div>
          {/if}
          
          <!-- Capabilities -->
          {#if selectedModel.olla?.capabilities && Array.isArray(selectedModel.olla.capabilities)}
            <div>
              <h5 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Capabilities</h5>
              <div class="flex flex-wrap gap-2">
                {#each selectedModel.olla.capabilities as capability}
                  <span class="px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-300 rounded">
                    {capability.replace(/-/g, ' ').replace(/\b\w/g, l => l.toUpperCase())}
                  </span>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {:else}
        <div class="flex items-center justify-center h-full text-gray-500 dark:text-gray-400">
          <p>Select a model to view details</p>
        </div>
      {/if}
    </div>
  </div>
</div>