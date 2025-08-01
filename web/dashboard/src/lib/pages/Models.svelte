<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import ModelList from '$lib/components/ModelList.svelte';
  
  const modelStats = $derived(dashboardStore.modelStats);
  const unifiedModels = $derived(dashboardStore.unifiedModels || []);
  const endpoints = $derived(dashboardStore.endpoints || []);
  
  // Extract model data
  const models = $derived(modelStats?.models || {});
  const modelFamilies = $derived((() => {
    const families = new Set();
    unifiedModels.forEach(model => {
      // Check both olla.family and family fields
      const family = model.olla?.family || model.family;
      if (family) families.add(family);
    });
    return Array.from(families);
  })());
  
  // Debug logging
  $effect(() => {
    console.log('[Models] UnifiedModels:', unifiedModels);
    console.log('[Models] ModelStats:', modelStats);
    console.log('[Models] Endpoints:', endpoints);
  });
  
  // Create a map of models to their endpoints
  const modelEndpointMap = $derived((() => {
    const map = new Map();
    
    // Process unified models which contain endpoint information
    unifiedModels.forEach(model => {
      const modelId = model.id || model.name;
      if (!map.has(modelId)) {
        map.set(modelId, {
          model: model,
          endpoints: [],
          totalCapability: 0
        });
      }
      
      // Add endpoint information
      if (model.endpoint) {
        const endpoint = endpoints.find(ep => ep.name === model.endpoint);
        if (endpoint) {
          map.get(modelId).endpoints.push({
            name: endpoint.name,
            status: endpoint.status,
            priority: endpoint.priority,
            type: endpoint.backend_type || endpoint.type
          });
        }
      }
    });
    
    return map;
  })());
  
  // Calculate totals
  const totalModels = $derived(Object.keys(models).length);
  const totalRequests = $derived(Object.values(models).reduce((sum, m) => sum + (m.requests || 0), 0));
  const avgLatency = $derived((() => {
    const latencies = Object.values(models).map(m => m.avg_latency || 0).filter(l => l > 0);
    if (latencies.length === 0) return 0;
    return Math.round(latencies.reduce((sum, l) => sum + l, 0) / latencies.length);
  })());
  
  // Get top models by requests
  const topModels = $derived((() => {
    return Object.entries(models)
      .map(([name, stats]) => ({ name, ...stats }))
      .sort((a, b) => (b.requests || 0) - (a.requests || 0))
      .slice(0, 5);
  })());
  
  // Format percentile
  function formatPercentile(value) {
    if (!value) return 'N/A';
    return `${value}ms`;
  }
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div>
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">Models</h2>
    <p class="text-gray-600 dark:text-gray-400 mt-1">Model inventory and performance across all endpoints</p>
  </div>
  
  <!-- Summary Cards -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Total Models</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{totalModels}</p>
        </div>
        <div class="w-12 h-12 bg-purple-100 dark:bg-purple-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">🤖</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Model Families</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{modelFamilies.length}</p>
        </div>
        <div class="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">🏷️</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Total Requests</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{totalRequests.toLocaleString()}</p>
        </div>
        <div class="w-12 h-12 bg-green-100 dark:bg-green-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">📊</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Avg Latency</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{avgLatency}ms</p>
        </div>
        <div class="w-12 h-12 bg-yellow-100 dark:bg-yellow-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">⚡</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Model List -->
  <div>
    <ModelList />
  </div>
  
  <!-- Two Column Layout -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- Top Models -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
      <div class="p-6 border-b border-gray-200 dark:border-gray-700">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Top Models by Usage</h3>
      </div>
      <div class="p-6">
        {#if topModels.length > 0}
          <div class="space-y-4">
            {#each topModels as model, i}
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <span class="text-2xl font-bold text-gray-400 dark:text-gray-600 w-8">{i + 1}</span>
                  <div>
                    <div class="font-medium text-gray-900 dark:text-white">{model.name}</div>
                    <div class="text-sm text-gray-600 dark:text-gray-400">{model.requests || 0} requests</div>
                  </div>
                </div>
                <div class="text-right">
                  <div class="text-sm font-medium text-gray-900 dark:text-white">{model.avg_latency || 0}ms</div>
                  <div class="text-xs text-gray-600 dark:text-gray-400">avg latency</div>
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <div class="text-center py-8">
            <span class="text-3xl mb-2 block">📊</span>
            <p class="text-gray-500 dark:text-gray-400">No model usage data yet</p>
          </div>
        {/if}
      </div>
    </div>
    
    <!-- Model Performance Stats -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
      <div class="p-6 border-b border-gray-200 dark:border-gray-700">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Performance Distribution</h3>
      </div>
      <div class="p-6">
        {#if modelStats?.summary}
          <div class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
                <div class="text-sm text-gray-600 dark:text-gray-400">P50 Latency</div>
                <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">
                  {formatPercentile(modelStats.summary.percentiles?.p50)}
                </div>
              </div>
              <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
                <div class="text-sm text-gray-600 dark:text-gray-400">P90 Latency</div>
                <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">
                  {formatPercentile(modelStats.summary.percentiles?.p90)}
                </div>
              </div>
              <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
                <div class="text-sm text-gray-600 dark:text-gray-400">P95 Latency</div>
                <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">
                  {formatPercentile(modelStats.summary.percentiles?.p95)}
                </div>
              </div>
              <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
                <div class="text-sm text-gray-600 dark:text-gray-400">P99 Latency</div>
                <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">
                  {formatPercentile(modelStats.summary.percentiles?.p99)}
                </div>
              </div>
            </div>
          </div>
        {:else}
          <div class="text-center py-8">
            <span class="text-3xl mb-2 block">📈</span>
            <p class="text-gray-500 dark:text-gray-400">No performance data available</p>
          </div>
        {/if}
      </div>
    </div>
  </div>
  
  <!-- Model Details Table with Endpoint Information -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Model Inventory</h3>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full">
        <thead>
          <tr class="border-b border-gray-200 dark:border-gray-700">
            <th class="text-left px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Model</th>
            <th class="text-left px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Available On</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Requests</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Avg Latency</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Performance</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Success Rate</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
          {#each unifiedModels as model}
            {@const stats = models[model.id || model.name] || {}}
            {@const endpointInfo = modelEndpointMap.get(model.id || model.name)}
            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
              <td class="px-6 py-4">
                <div>
                  <div class="font-medium text-gray-900 dark:text-white">{model.id || model.name}</div>
                  {#if model.olla?.family}
                    <div class="text-xs text-gray-500 dark:text-gray-400">{model.olla.family}</div>
                  {/if}
                </div>
              </td>
              <td class="px-6 py-4">
                <div class="flex flex-wrap gap-1">
                  {#if model.endpoint}
                    {@const endpoint = endpoints.find(ep => ep.name === model.endpoint)}
                    {#if endpoint}
                      <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {endpoint.status === 'online' ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-300' : 'bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-300'}">
                        {endpoint.name}
                        {#if endpoint.priority >= 100}
                          <span class="ml-1 text-purple-600 dark:text-purple-400">⚡</span>
                        {/if}
                      </span>
                    {:else}
                      <span class="text-sm text-gray-500 dark:text-gray-400">{model.endpoint}</span>
                    {/if}
                  {:else}
                    <span class="text-sm text-gray-500 dark:text-gray-400">Unknown</span>
                  {/if}
                </div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{(stats.requests || 0).toLocaleString()}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{stats.avg_latency || 0}ms</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <div class="text-xs space-y-0.5">
                  {#if stats.min_latency}
                    <div class="text-gray-600 dark:text-gray-400">Min: {stats.min_latency}ms</div>
                  {/if}
                  {#if stats.max_latency}
                    <div class="text-gray-600 dark:text-gray-400">Max: {stats.max_latency}ms</div>
                  {/if}
                </div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{stats.success_rate || '100'}%</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      
      {#if unifiedModels.length === 0}
        <div class="text-center py-12">
          <span class="text-3xl mb-4 block">🤖</span>
          <p class="text-gray-500 dark:text-gray-400">No models discovered yet</p>
          <p class="text-sm text-gray-400 dark:text-gray-500 mt-2">Models will appear here once endpoints are online</p>
        </div>
      {/if}
    </div>
  </div>
</div>