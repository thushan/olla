<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import PerformanceMetrics from '$lib/components/PerformanceMetrics.svelte';
  
  const status = $derived(dashboardStore.status);
  const stats = $derived(dashboardStore.stats);
  const modelStats = $derived(dashboardStore.modelStats);
  
  // Calculate performance metrics
  const requestsPerSecond = $derived((() => {
    // This would need real time-series data, for now estimate from total
    const totalRequests = stats.totalRequests || 0;
    if (totalRequests === 0) return 0;
    // Rough estimate assuming system has been running for some time
    return Math.round(totalRequests / 3600); // per hour to per second
  })());
  
  const errorRate = $derived((() => {
    const total = stats.totalRequests || 0;
    const errors = stats.totalErrors || 0;
    if (total === 0) return 0;
    return ((errors / total) * 100).toFixed(2);
  })());
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div>
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">Performance</h2>
    <p class="text-gray-600 dark:text-gray-400 mt-1">Detailed performance metrics and analytics</p>
  </div>
  
  <!-- Key Metrics -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Requests/sec</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{requestsPerSecond}</p>
        </div>
        <div class="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">📈</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Error Rate</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{errorRate}%</p>
        </div>
        <div class="w-12 h-12 bg-red-100 dark:bg-red-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">⚠️</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Avg Response</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{stats.avgResponseTime || 0}ms</p>
        </div>
        <div class="w-12 h-12 bg-green-100 dark:bg-green-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">⚡</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Active Connections</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{stats.activeConnections || 0}</p>
        </div>
        <div class="w-12 h-12 bg-purple-100 dark:bg-purple-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">🔗</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Main Performance Chart -->
  <div>
    <PerformanceMetrics />
  </div>
  
  <!-- Latency Distribution -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- Latency Percentiles -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Latency Distribution</h3>
      
      {#if modelStats?.summary?.percentiles}
        <div class="space-y-4">
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-600 dark:text-gray-400">P50 (Median)</span>
            <span class="font-medium text-gray-900 dark:text-white">{modelStats.summary.percentiles.p50 || 0}ms</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-blue-500 h-2 rounded-full" style="width: 50%"></div>
          </div>
          
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-600 dark:text-gray-400">P90</span>
            <span class="font-medium text-gray-900 dark:text-white">{modelStats.summary.percentiles.p90 || 0}ms</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-yellow-500 h-2 rounded-full" style="width: 90%"></div>
          </div>
          
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-600 dark:text-gray-400">P95</span>
            <span class="font-medium text-gray-900 dark:text-white">{modelStats.summary.percentiles.p95 || 0}ms</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-orange-500 h-2 rounded-full" style="width: 95%"></div>
          </div>
          
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-600 dark:text-gray-400">P99</span>
            <span class="font-medium text-gray-900 dark:text-white">{modelStats.summary.percentiles.p99 || 0}ms</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-red-500 h-2 rounded-full" style="width: 99%"></div>
          </div>
        </div>
      {:else}
        <div class="text-center py-8">
          <span class="text-3xl mb-2 block">📊</span>
          <p class="text-gray-500 dark:text-gray-400">No latency data available</p>
        </div>
      {/if}
    </div>
    
    <!-- Throughput Stats -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Throughput Analysis</h3>
      
      <div class="space-y-4">
        <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Total Traffic</span>
            <span class="font-medium text-gray-900 dark:text-white">{status?.system?.total_traffic || '0 B'}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">Cumulative data transferred</div>
        </div>
        
        <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Success Rate</span>
            <span class="font-medium text-green-600 dark:text-green-400">{status?.system?.success_rate || '0%'}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">Successful request percentage</div>
        </div>
        
        <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Avg Latency</span>
            <span class="font-medium text-gray-900 dark:text-white">{status?.system?.avg_latency || '0ms'}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">Average response time</div>
        </div>
      </div>
    </div>
  </div>
</div>