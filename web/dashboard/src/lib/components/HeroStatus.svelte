<script>
  console.log('[HeroStatus] Component initializing');
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const overallHealth = $derived(dashboardStore.overallHealth);
  const endpoints = $derived(dashboardStore.endpoints || []);
  const totalEndpoints = $derived(endpoints.length);
  const healthyEndpoints = $derived(endpoints.filter(e => e.status === 'online' || e.status === 'healthy').length);
  const totalRequests = $derived(dashboardStore.status?.system?.total_requests || 0);
  const totalErrors = $derived(dashboardStore.status?.system?.total_errors || 0);
  const avgResponseTime = $derived(dashboardStore.status?.system?.avg_response_time || 0);
  const activeConnections = $derived(dashboardStore.status?.system?.active_connections || 0);
  
  // Calculate health percentage
  const healthPercentage = $derived(totalEndpoints > 0 ? (healthyEndpoints / totalEndpoints) * 100 : 0);
  
  // Get health color
  const healthColor = $derived(
    overallHealth === 'HEALTHY' ? 'text-green-600 dark:text-green-400' :
    overallHealth === 'DEGRADED' ? 'text-yellow-600 dark:text-yellow-400' :
    'text-red-600 dark:text-red-400'
  );
  
  // Get ring color
  const ringColor = $derived(
    overallHealth === 'HEALTHY' ? '#10b981' :
    overallHealth === 'DEGRADED' ? '#f59e0b' :
    '#ef4444'
  );
  
  // Format numbers
  function formatNumber(num) {
    if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
    if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
    return num.toString();
  }
  
  // Format response time
  function formatResponseTime(ms) {
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
    return `${Math.round(ms)}ms`;
  }
  
  // Calculate success rate
  const successRate = $derived(
    totalRequests > 0 ? ((totalRequests - totalErrors) / totalRequests * 100).toFixed(1) : '100.0'
  );
</script>

<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
  <!-- Health Overview Card -->
  <div class="lg:col-span-1 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">System Health</h2>
    
    <div class="flex items-center justify-center mb-6">
      <div class="relative w-40 h-40">
        <svg class="w-full h-full -rotate-90">
          <circle
            cx="80"
            cy="80"
            r="70"
            stroke="currentColor"
            stroke-width="12"
            fill="none"
            class="text-gray-200 dark:text-gray-700"
          />
          <circle
            cx="80"
            cy="80"
            r="70"
            stroke={ringColor}
            stroke-width="12"
            fill="none"
            stroke-dasharray={`${healthPercentage * 4.4} 440`}
            stroke-linecap="round"
            class="transition-all duration-500"
          />
        </svg>
        <div class="absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-3xl font-bold {healthColor}">{healthPercentage.toFixed(0)}%</span>
          <span class="text-sm text-gray-600 dark:text-gray-400">{overallHealth}</span>
        </div>
      </div>
    </div>
    
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <span class="text-sm text-gray-600 dark:text-gray-400">Active Endpoints</span>
        <span class="text-sm font-medium text-gray-900 dark:text-white">{healthyEndpoints}/{totalEndpoints}</span>
      </div>
      <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
        <div 
          class="h-2 rounded-full transition-all duration-500"
          style="width: {healthPercentage}%; background-color: {ringColor}"
        ></div>
      </div>
    </div>
  </div>
  
  <!-- Stats Grid -->
  <div class="lg:col-span-2 grid grid-cols-2 sm:grid-cols-4 gap-4">
    <!-- Total Requests -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-4">
      <div class="flex items-center gap-3 mb-2">
        <div class="w-10 h-10 rounded-lg bg-blue-100 dark:bg-blue-900/20 flex items-center justify-center">
          <span class="text-blue-600 dark:text-blue-400">📊</span>
        </div>
        <div class="flex-1">
          <p class="text-2xl font-bold text-gray-900 dark:text-white">{formatNumber(totalRequests)}</p>
          <p class="text-xs text-gray-600 dark:text-gray-400">Total Requests</p>
        </div>
      </div>
    </div>
    
    <!-- Success Rate -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-4">
      <div class="flex items-center gap-3 mb-2">
        <div class="w-10 h-10 rounded-lg bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
          <span class="text-green-600 dark:text-green-400">✓</span>
        </div>
        <div class="flex-1">
          <p class="text-2xl font-bold text-gray-900 dark:text-white">{successRate}%</p>
          <p class="text-xs text-gray-600 dark:text-gray-400">Success Rate</p>
        </div>
      </div>
    </div>
    
    <!-- Response Time -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-4">
      <div class="flex items-center gap-3 mb-2">
        <div class="w-10 h-10 rounded-lg bg-purple-100 dark:bg-purple-900/20 flex items-center justify-center">
          <span class="text-purple-600 dark:text-purple-400">⚡</span>
        </div>
        <div class="flex-1">
          <p class="text-2xl font-bold text-gray-900 dark:text-white">{formatResponseTime(avgResponseTime)}</p>
          <p class="text-xs text-gray-600 dark:text-gray-400">Avg Response</p>
        </div>
      </div>
    </div>
    
    <!-- Active Connections -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-4">
      <div class="flex items-center gap-3 mb-2">
        <div class="w-10 h-10 rounded-lg bg-orange-100 dark:bg-orange-900/20 flex items-center justify-center">
          <span class="text-orange-600 dark:text-orange-400">🔗</span>
        </div>
        <div class="flex-1">
          <p class="text-2xl font-bold text-gray-900 dark:text-white">{formatNumber(activeConnections)}</p>
          <p class="text-xs text-gray-600 dark:text-gray-400">Connections</p>
        </div>
      </div>
    </div>
  </div>
</div>