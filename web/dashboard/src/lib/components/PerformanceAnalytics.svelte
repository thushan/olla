<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const processStats = $derived(dashboardStore.processStats);
  const status = $derived(dashboardStore.status);
  const modelStats = $derived(dashboardStore.modelStats);
  
  // Active tab
  let activeTab = $state('system');
  
  // Calculate performance metrics
  const performanceMetrics = $derived.by(() => {
    if (!status?.system) {
      return {
        avgLatency: 0,
        successRate: 0,
        throughput: 0,
        errorRate: 0,
      };
    }
    
    const system = status.system;
    const total = system.total_requests || 1;
    const successful = system.successful_requests || 0;
    const failed = system.failed_requests || 0;
    
    return {
      avgLatency: system.avg_latency_ms || 0,
      successRate: ((successful / total) * 100).toFixed(1),
      throughput: Math.round(total / 60), // requests per minute
      errorRate: ((failed / total) * 100).toFixed(1),
    };
  });
  
  // Memory pressure indicator
  const memoryPressure = $derived.by(() => {
    if (!processStats?.memory?.memory_pressure) return 'unknown';
    const pressure = processStats.memory.memory_pressure.toLowerCase();
    
    return {
      status: pressure,
      color: pressure === 'low' ? 'green' : pressure === 'moderate' ? 'yellow' : 'red',
      icon: pressure === 'low' ? '✅' : pressure === 'moderate' ? '⚠️' : '🚨',
    };
  });
  
  // Format uptime
  function formatUptime(uptime) {
    if (!uptime) return 'Unknown';
    return uptime;
  }
  
  // Get top models by request count
  const topModels = $derived.by(() => {
    if (!modelStats || Object.keys(modelStats).length === 0) return [];
    
    return Object.entries(modelStats)
      .map(([model, stats]) => ({
        name: model,
        requests: stats.request_count || 0,
        avgLatency: stats.avg_latency_ms || 0,
        errors: stats.error_count || 0,
      }))
      .sort((a, b) => b.requests - a.requests)
      .slice(0, 5);
  });
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-700">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Performance Analytics</h3>
    
    <!-- Tab Navigation -->
    <div class="flex gap-2">
      <button 
        class="px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 {activeTab === 'system' ? 'bg-gray-100 dark:bg-gray-700 text-blue-600 dark:text-blue-400' : ''}"
        onclick={() => activeTab = 'system'}
      >
        System
      </button>
      <button 
        class="px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 {activeTab === 'memory' ? 'bg-gray-100 dark:bg-gray-700 text-blue-600 dark:text-blue-400' : ''}"
        onclick={() => activeTab = 'memory'}
      >
        Memory
      </button>
      <button 
        class="px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 {activeTab === 'models' ? 'bg-gray-100 dark:bg-gray-700 text-blue-600 dark:text-blue-400' : ''}"
        onclick={() => activeTab = 'models'}
      >
        Models
      </button>
    </div>
  </div>
  
  <div class="p-6">
    <!-- System Performance Tab -->
    {#if activeTab === 'system'}
      <div class="space-y-6">
        <!-- Key Metrics -->
        <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
            <div class="flex items-center gap-2 mb-2">
              <span class="text-lg">⚡</span>
              <span class="text-sm text-gray-600 dark:text-gray-400">Avg Latency</span>
            </div>
            <div class="text-2xl font-bold text-gray-900 dark:text-white">{performanceMetrics.avgLatency}ms</div>
            <div class="flex items-center gap-1 mt-2 text-xs text-gray-500 dark:text-gray-400">
              <span>📊</span>
              <span>Real-time</span>
            </div>
          </div>
          
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
            <div class="flex items-center gap-2 mb-2">
              <span class="text-lg">✅</span>
              <span class="text-sm text-gray-600 dark:text-gray-400">Success Rate</span>
            </div>
            <div class="text-2xl font-bold text-gray-900 dark:text-white">{performanceMetrics.successRate}%</div>
            <div class="flex items-center gap-1 mt-2 text-xs text-green-600 dark:text-green-400">
              <span>📈</span>
              <span>Healthy</span>
            </div>
          </div>
          
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
            <div class="flex items-center gap-2 mb-2">
              <span class="text-lg">📊</span>
              <span class="text-sm text-gray-600 dark:text-gray-400">Throughput</span>
            </div>
            <div class="text-2xl font-bold text-gray-900 dark:text-white">{performanceMetrics.throughput}</div>
            <div class="text-sm text-gray-600 dark:text-gray-400">requests/min</div>
          </div>
          
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
            <div class="flex items-center gap-2 mb-2">
              <span class="text-lg">❌</span>
              <span class="text-sm text-gray-600 dark:text-gray-400">Error Rate</span>
            </div>
            <div class="text-2xl font-bold text-gray-900 dark:text-white">{performanceMetrics.errorRate}%</div>
            <div class="flex items-center gap-1 mt-2 text-xs {performanceMetrics.errorRate > 5 ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'}">
              <span>{performanceMetrics.errorRate > 5 ? '📉' : '✅'}</span>
              <span>{performanceMetrics.errorRate > 5 ? 'High' : 'Low'}</span>
            </div>
          </div>
        </div>
        
        <!-- Runtime Stats -->
        {#if processStats?.runtime}
          <div>
            <h4 class="font-semibold text-gray-900 dark:text-white mb-3">Runtime Information</h4>
            <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Uptime</span>
                <span class="text-sm font-medium text-gray-900 dark:text-white">{formatUptime(processStats.runtime.uptime)}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Go Version</span>
                <span class="text-sm font-medium text-gray-900 dark:text-white">{processStats.runtime.go_version}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">CPU Cores</span>
                <span class="text-sm font-medium text-gray-900 dark:text-white">{processStats.runtime.num_cpu}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">GOMAXPROCS</span>
                <span class="text-sm font-medium text-gray-900 dark:text-white">{processStats.runtime.gomaxprocs}</span>
              </div>
            </div>
          </div>
        {/if}
      </div>
    {/if}
    
    <!-- Memory Performance Tab -->
    {#if activeTab === 'memory'}
      <div class="space-y-6">
        <div>
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-3 bg-white dark:bg-gray-800 {memoryPressure.color === 'green' ? 'border-green-500' : memoryPressure.color === 'yellow' ? 'border-yellow-500' : 'border-red-500'}">
            <span class="text-2xl">{memoryPressure.icon}</span>
            <div class="flex-1">
              <span class="block text-sm text-gray-600 dark:text-gray-400">Memory Pressure</span>
              <span class="text-lg font-semibold text-gray-900 dark:text-white capitalize">{memoryPressure.status}</span>
            </div>
          </div>
        </div>
        
        {#if processStats?.memory}
          <div>
            <div class="grid grid-cols-2 lg:grid-cols-3 gap-4">
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Heap Allocated</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.heap_alloc}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Heap System</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.heap_sys}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Heap In Use</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.heap_inuse}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Stack In Use</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.stack_inuse}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Total Allocated</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.total_alloc}</span>
              </div>
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Heap Released</span>
                <span class="text-sm font-mono font-medium text-gray-900 dark:text-white">{processStats.memory.heap_released}</span>
              </div>
            </div>
            
            <!-- GC Stats -->
            {#if processStats.garbage_collection}
              <div class="mt-6">
                <h4 class="font-semibold text-gray-900 dark:text-white mb-3">Garbage Collection</h4>
                <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
                  <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                    <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Last GC</span>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{new Date(processStats.garbage_collection.last_gc).toLocaleTimeString()}</span>
                  </div>
                  <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                    <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">GC Cycles</span>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{processStats.garbage_collection.num_gc_cycles}</span>
                  </div>
                  <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                    <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Total GC Time</span>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{processStats.garbage_collection.total_gc_time}</span>
                  </div>
                  <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                    <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">GC CPU %</span>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{(processStats.garbage_collection.gc_cpu_fraction * 100).toFixed(2)}%</span>
                  </div>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
    
    <!-- Model Performance Tab -->
    {#if activeTab === 'models'}
      <div class="space-y-6">
        {#if topModels.length === 0}
          <div class="text-center py-12">
            <span class="text-6xl block mb-2">📊</span>
            <p class="text-gray-600 dark:text-gray-400">No model statistics available yet</p>
          </div>
        {:else}
          <div>
            <h4 class="font-semibold text-gray-900 dark:text-white mb-3">Top Models by Request Count</h4>
            <div class="space-y-2">
              <div class="grid grid-cols-4 gap-4 px-4 py-2 text-sm font-medium text-gray-500 dark:text-gray-400">
                <span>Model</span>
                <span class="text-right">Requests</span>
                <span class="text-right">Avg Latency</span>
                <span class="text-right">Errors</span>
              </div>
              {#each topModels as model}
                <div class="grid grid-cols-4 gap-4 px-4 py-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                  <span class="truncate text-gray-900 dark:text-white">{model.name}</span>
                  <span class="text-right text-gray-900 dark:text-white">{model.requests.toLocaleString()}</span>
                  <span class="text-right text-gray-900 dark:text-white">{model.avgLatency}ms</span>
                  <span class="text-right {model.errors > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'}">
                    {model.errors}
                  </span>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

