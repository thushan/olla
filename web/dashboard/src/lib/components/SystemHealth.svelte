<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  const overallHealth = $derived(dashboardStore.overallHealth);
  const endpoints = $derived(dashboardStore.endpoints || []);
  const totalEndpoints = $derived(endpoints.length);
  const healthyEndpoints = $derived(endpoints.filter(e => e.status === 'online' || e.status === 'healthy').length);
  const healthPercentage = $derived(totalEndpoints > 0 ? (healthyEndpoints / totalEndpoints) * 100 : 0);
  
  // Get system metrics from process stats
  const processStats = $derived(dashboardStore.processStats || {});
  const cpuUsage = $derived(Math.round((processStats.garbage_collection?.gc_cpu_fraction || 0) * 100));
  const memoryUsage = $derived((() => {
    // Calculate memory usage percentage
    if (!processStats.memory) return 0;
    // For now, use memory pressure as a simple indicator
    const pressure = processStats.memory.memory_pressure?.toLowerCase();
    if (pressure === 'low') return 20;
    if (pressure === 'medium') return 50;
    if (pressure === 'high') return 80;
    return 0;
  })());
  const goroutines = $derived(processStats.goroutines?.count || 0);
  
  // Health status config
  const statusConfig = $derived({
    HEALTHY: { color: '#10b981', bg: 'bg-green-100 dark:bg-green-900/20', text: 'text-green-600 dark:text-green-400' },
    DEGRADED: { color: '#f59e0b', bg: 'bg-yellow-100 dark:bg-yellow-900/20', text: 'text-yellow-600 dark:text-yellow-400' },
    CRITICAL: { color: '#ef4444', bg: 'bg-red-100 dark:bg-red-900/20', text: 'text-red-600 dark:text-red-400' },
    UNKNOWN: { color: '#6b7280', bg: 'bg-gray-100 dark:bg-gray-900/20', text: 'text-gray-600 dark:text-gray-400' }
  }[overallHealth] || statusConfig.UNKNOWN);
  
  // Calculate ring dasharray
  const circumference = 2 * Math.PI * 120;
  const dashArray = $derived(`${(healthPercentage / 100) * circumference} ${circumference}`);
</script>

<div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 h-full">
  <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-6">System Health</h2>
  
  <!-- Circular Health Indicator -->
  <div class="relative flex items-center justify-center mb-6">
    <svg class="w-64 h-64 transform -rotate-90">
      <!-- Background circle -->
      <circle
        cx="128"
        cy="128"
        r="120"
        stroke="currentColor"
        stroke-width="16"
        fill="none"
        class="text-gray-200 dark:text-gray-700"
      />
      <!-- Progress circle -->
      <circle
        cx="128"
        cy="128"
        r="120"
        stroke={statusConfig.color}
        stroke-width="16"
        fill="none"
        stroke-dasharray={dashArray}
        stroke-linecap="round"
        class="transition-all duration-1000 ease-out"
        style="filter: drop-shadow(0 0 8px {statusConfig.color}40)"
      />
    </svg>
    
    <!-- Center content -->
    <div class="absolute inset-0 flex flex-col items-center justify-center">
      <div class="text-5xl font-bold {statusConfig.text} mb-2">
        {healthPercentage.toFixed(0)}%
      </div>
      <div class="px-3 py-1 rounded-full {statusConfig.bg}">
        <span class="text-sm font-medium {statusConfig.text}">{overallHealth}</span>
      </div>
      <div class="mt-2 text-sm text-gray-600 dark:text-gray-400">
        {healthyEndpoints} of {totalEndpoints} endpoints
      </div>
    </div>
  </div>
  
  <!-- System Metrics -->
  <div class="space-y-3">
    <!-- CPU Usage -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">CPU Usage</span>
        <span class="text-sm font-semibold text-gray-900 dark:text-white">{cpuUsage.toFixed(1)}%</span>
      </div>
      <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2 overflow-hidden">
        <div 
          class="h-full rounded-full transition-all duration-500 {cpuUsage > 80 ? 'bg-red-500' : cpuUsage > 60 ? 'bg-yellow-500' : 'bg-green-500'}"
          style="width: {cpuUsage}%"
        ></div>
      </div>
    </div>
    
    <!-- Memory Usage -->
    <div>
      <div class="flex items-center justify-between mb-1">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Memory Usage</span>
        <span class="text-sm font-semibold text-gray-900 dark:text-white">{memoryUsage.toFixed(1)}%</span>
      </div>
      <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2 overflow-hidden">
        <div 
          class="h-full rounded-full transition-all duration-500 {memoryUsage > 80 ? 'bg-red-500' : memoryUsage > 60 ? 'bg-yellow-500' : 'bg-blue-500'}"
          style="width: {memoryUsage}%"
        ></div>
      </div>
    </div>
    
    <!-- Goroutines -->
    <div class="flex items-center justify-between pt-2">
      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Active Goroutines</span>
      <span class="text-sm font-semibold text-gray-900 dark:text-white">{goroutines.toLocaleString()}</span>
    </div>
  </div>
</div>