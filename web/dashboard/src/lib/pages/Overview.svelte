<script>
  console.log('[Overview] Component initializing');
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import HeroMetrics from '$lib/components/HeroMetrics.svelte';
  import SystemHealth from '$lib/components/SystemHealth.svelte';
  import ActiveQueries from '$lib/components/ActiveQueries.svelte';
  
  const status = $derived(dashboardStore.status);
  const endpoints = $derived(dashboardStore.endpoints || []);
  const stats = $derived(dashboardStore.stats);
  
  // Calculate key metrics
  const totalEndpoints = $derived(endpoints.length);
  const healthyEndpoints = $derived(endpoints.filter(e => e.status === 'healthy' || e.status === 'online').length);
  const successRate = $derived((() => {
    if (!status?.system) return 0;
    const total = status.system.total_requests || 0;
    const failed = status.system.total_failures || 0;
    if (total === 0) return 100;
    return ((total - failed) / total * 100).toFixed(1);
  })());
  
  const avgLatency = $derived(status?.system?.avg_latency || '0ms');
  const activeConnections = $derived(status?.system?.active_connections || 0);
  const securityViolations = $derived(status?.system?.security_violations || 0);
  
  // Debug info
  const lastUpdate = $derived(status?.timestamp || null);
  const isLoading = $derived(dashboardStore.loading.status);
  
  // Add debug logging
  $effect(() => {
    console.log('[Overview] Status updated:', status?.timestamp);
    console.log('[Overview] Loading:', isLoading);
    console.log('[Overview] Endpoints:', endpoints.length);
  });
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div class="flex items-center justify-between">
    <div>
      <h2 class="text-2xl font-bold text-gray-900 dark:text-white">System Overview</h2>
      <p class="text-gray-600 dark:text-gray-400 mt-1">Real-time monitoring of your Olla proxy infrastructure</p>
    </div>
    <div class="text-right">
      <div class="flex items-center gap-2">
        <div class={`w-2 h-2 rounded-full ${isLoading ? 'bg-blue-500 animate-pulse' : 'bg-green-500'}`}></div>
        <span class="text-sm text-gray-600 dark:text-gray-400">
          {#if lastUpdate}
            Last updated: {new Date(lastUpdate).toLocaleTimeString()}
          {:else}
            No data yet
          {/if}
        </span>
      </div>
      <div class="text-xs text-gray-500 dark:text-gray-500 mt-1">
        {endpoints.length} endpoints, {activeConnections} active connections
      </div>
    </div>
  </div>
  
  <!-- Key Metrics Grid -->
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Total Requests</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{(stats.totalRequests || 0).toLocaleString()}</p>
        </div>
        <div class="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">📊</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Success Rate</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{successRate}%</p>
        </div>
        <div class="w-12 h-12 bg-green-100 dark:bg-green-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">✅</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Endpoints</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{healthyEndpoints}/{totalEndpoints}</p>
        </div>
        <div class="w-12 h-12 bg-purple-100 dark:bg-purple-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">🖥️</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Avg Latency</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{avgLatency}</p>
        </div>
        <div class="w-12 h-12 bg-yellow-100 dark:bg-yellow-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">⚡</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Main Content Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <!-- System Health -->
    <div class="lg:col-span-1">
      <SystemHealth />
    </div>
    
    <!-- Active Queries -->
    <div class="lg:col-span-2">
      <ActiveQueries />
    </div>
  </div>
  
  <!-- Hero Metrics -->
  <div>
    <HeroMetrics />
  </div>
  
  <!-- Quick Stats -->
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">Connection Status</h3>
      <div class="space-y-3">
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Active</span>
          <span class="font-medium text-gray-900 dark:text-white">{activeConnections}</span>
        </div>
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Security Violations</span>
          <span class="font-medium {securityViolations > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'}">{securityViolations}</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">Traffic Volume</h3>
      <div class="space-y-3">
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Total</span>
          <span class="font-medium text-gray-900 dark:text-white">{status?.system?.total_traffic || '0 B'}</span>
        </div>
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Errors</span>
          <span class="font-medium text-gray-900 dark:text-white">{stats.totalErrors || 0}</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">System Status</h3>
      <div class="space-y-3">
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Health</span>
          <span class="font-medium capitalize {
            dashboardStore.overallHealth === 'HEALTHY' ? 'text-green-600 dark:text-green-400' :
            dashboardStore.overallHealth === 'DEGRADED' ? 'text-yellow-600 dark:text-yellow-400' :
            'text-red-600 dark:text-red-400'
          }">{dashboardStore.overallHealth}</span>
        </div>
        <div class="flex justify-between items-center">
          <span class="text-sm text-gray-600 dark:text-gray-400">Uptime</span>
          <span class="font-medium text-gray-900 dark:text-white">{status?.system?.uptime || 'Unknown'}</span>
        </div>
      </div>
    </div>
  </div>
</div>