<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import ModelGalaxy from '$lib/components/ModelGalaxy.svelte';
  
  const modelStats = $derived(dashboardStore.modelStats);
  const unifiedModels = $derived(dashboardStore.unifiedModels || []);
  
  // Extract model data
  const models = $derived(modelStats?.models || {});
  const modelFamilies = $derived((() => {
    const families = new Set();
    unifiedModels.forEach(model => {
      if (model.family) families.add(model.family);
    });
    return Array.from(families);
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
  
  <!-- Model Galaxy Visualization -->
  <div>
    <ModelGalaxy />
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
  
  <!-- Model Details Table -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">All Models</h3>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full">
        <thead>
          <tr class="border-b border-gray-200 dark:border-gray-700">
            <th class="text-left px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Model</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Requests</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Avg Latency</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Min</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Max</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Success Rate</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
          {#each Object.entries(models) as [name, stats]}
            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
              <td class="px-6 py-4 whitespace-nowrap">
                <div class="font-medium text-gray-900 dark:text-white">{name}</div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{(stats.requests || 0).toLocaleString()}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{stats.avg_latency || 0}ms</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-600 dark:text-gray-400">{stats.min_latency || 0}ms</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-600 dark:text-gray-400">{stats.max_latency || 0}ms</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{stats.success_rate || '100'}%</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      
      {#if Object.keys(models).length === 0}
        <div class="text-center py-12">
          <span class="text-3xl mb-4 block">🤖</span>
          <p class="text-gray-500 dark:text-gray-400">No models have been used yet</p>
        </div>
      {/if}
    </div>
  </div>
</div>