<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { globalCache } from '$lib/utils/dataCache.js';
  
  // Get reactive references
  const status = $derived(dashboardStore.status?.system || {});
  const endpoints = $derived(dashboardStore.endpoints || []);
  const events = $derived(dashboardStore.events.slice(0, 5));
  
  // Calculate metrics with caching to prevent flickering
  const totalRequests = $derived(globalCache.update('metric:totalRequests', status.total_requests || 0));
  const activeConnections = $derived(globalCache.update('metric:activeConnections', status.active_connections || 0));
  const avgLatency = $derived(status.avg_latency || '0ms');
  const requestRate = $derived((() => {
    const uptime = status.uptime_seconds || 60;
    if (uptime > 0 && totalRequests > 0) {
      const rate = (totalRequests / uptime) * 60;
      return globalCache.update('metric:requestRate', Math.round(rate));
    }
    return globalCache.update('metric:requestRate', 0);
  })());
  
  // Format timestamp
  function formatTime(timestamp) {
    if (!timestamp) return 'Unknown';
    const date = new Date(timestamp);
    return date.toLocaleTimeString();
  }
  
  // Format duration
  function formatDuration(ms) {
    if (!ms) return '0ms';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
  
  // Get status color
  function getStatusColor(status) {
    if (status < 300) return 'text-green-600 dark:text-green-400';
    if (status < 400) return 'text-blue-600 dark:text-blue-400';
    if (status < 500) return 'text-yellow-600 dark:text-yellow-400';
    return 'text-red-600 dark:text-red-400';
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 h-full">
  <div class="mb-6">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">System Activity</h2>
  </div>
  
  <!-- Current Metrics -->
  <div class="grid grid-cols-2 gap-4 mb-6">
    <div class="bg-gray-50 dark:bg-gray-900/50 rounded-lg p-4">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Active Connections</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{activeConnections}</p>
        </div>
        <div class="text-2xl">🔗</div>
      </div>
    </div>
    
    <div class="bg-gray-50 dark:bg-gray-900/50 rounded-lg p-4">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Request Rate</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{requestRate}</p>
          <p class="text-xs text-gray-500 dark:text-gray-500">req/min</p>
        </div>
        <div class="text-2xl">📈</div>
      </div>
    </div>
  </div>
  
  <!-- Recent Activity Log -->
  <div>
    <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Recent Activity</h3>
    <div class="space-y-2 max-h-[200px] overflow-y-auto">
      {#if events.length > 0}
        {#each events as event}
          <div class="bg-gray-50 dark:bg-gray-900/50 rounded-lg p-3 text-sm">
            <div class="flex items-center justify-between mb-1">
              <span class="font-medium {getStatusColor(event.status)}">
                {event.status || 200} {event.method || 'POST'}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {formatTime(event.timestamp)}
              </span>
            </div>
            <div class="text-xs text-gray-600 dark:text-gray-400">
              {#if event.model}
                <span class="inline-block mr-2">Model: {event.model}</span>
              {/if}
              {#if event.endpoint}
                <span class="inline-block mr-2">via {event.endpoint}</span>
              {/if}
              {#if event.duration}
                <span class="inline-block">Duration: {formatDuration(event.duration)}</span>
              {/if}
            </div>
          </div>
        {/each}
      {:else}
        <div class="text-center py-8 text-gray-400 dark:text-gray-500">
          <p class="text-sm">No recent activity</p>
          <p class="text-xs mt-1">Activity will appear here when requests are processed</p>
        </div>
      {/if}
    </div>
  </div>
  
  <!-- System Status Indicators -->
  <div class="mt-6 pt-6 border-t border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between text-sm">
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full {totalRequests > 0 ? 'bg-green-500' : 'bg-gray-400'}"></div>
        <span class="text-gray-600 dark:text-gray-400">System Active</span>
      </div>
      <div class="flex items-center gap-4 text-xs text-gray-500 dark:text-gray-500">
        <span>Total: {totalRequests} requests</span>
        <span>Latency: {avgLatency}</span>
      </div>
    </div>
  </div>
</div>