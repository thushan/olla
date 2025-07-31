<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { fade, scale } from 'svelte/transition';
  
  // Get endpoints from dashboard store
  let endpoints = $derived(dashboardStore.status?.endpoints || []);
  
  // Sort endpoints by priority and status
  let sortedEndpoints = $derived(
    [...endpoints].sort((a, b) => {
      // First by priority (higher first)
      if (a.priority !== b.priority) return b.priority - a.priority;
      // Then by status (healthy first)
      const statusOrder = { 'healthy': 0, 'warming': 1, 'busy': 2, 'unhealthy': 3, 'offline': 4 };
      return (statusOrder[a.status] || 5) - (statusOrder[b.status] || 5);
    })
  );
  
  // Status colors and icons
  const statusConfig = {
    healthy: {
      color: 'bg-light-status-success dark:bg-dark-status-success',
      icon: '✓',
      pulse: true
    },
    warming: {
      color: 'bg-light-status-warning dark:bg-dark-status-warning',
      icon: '↻',
      pulse: true
    },
    busy: {
      color: 'bg-light-syntax-orange dark:bg-dark-syntax-orange',
      icon: '●',
      pulse: false
    },
    unhealthy: {
      color: 'bg-light-status-error dark:bg-dark-status-error',
      icon: '!',
      pulse: true
    },
    offline: {
      color: 'bg-light-text-muted dark:bg-dark-text-muted',
      icon: '✕',
      pulse: false
    }
  };
  
  // Format latency display
  function formatLatency(latency) {
    if (!latency) return '—';
    const ms = parseInt(latency);
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }
  
  // Generate mini chart data (mock for now)
  function generateChartData() {
    return Array.from({ length: 10 }, () => Math.random() * 100);
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-700">
    <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
      Endpoint Health
    </h2>
    <div class="text-sm text-gray-600 dark:text-gray-400">
      {endpoints.filter(e => e.status === 'healthy' || e.status === 'online').length} / {endpoints.length} healthy
    </div>
  </div>
  
  <div class="p-6">
    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
      {#each sortedEndpoints as endpoint (endpoint.name)}
        <div 
          class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-5 relative overflow-hidden transition-all duration-300 hover:shadow-md hover:scale-[1.02] hover:border-gray-300 dark:hover:border-gray-600"
          transition:scale={{ duration: 300, delay: sortedEndpoints.indexOf(endpoint) * 50 }}
        >
          <!-- Status beacon -->
          <div class="flex items-start gap-3 mb-4">
            <div class="relative">
              <div class="w-10 h-10 rounded-full flex items-center justify-center text-white font-bold text-sm relative {endpoint.status === 'healthy' ? 'bg-green-500' : endpoint.status === 'warming' ? 'bg-yellow-500' : endpoint.status === 'busy' ? 'bg-orange-500' : endpoint.status === 'unhealthy' ? 'bg-red-500' : 'bg-gray-500'}">
                <span>{statusConfig[endpoint.status]?.icon || '?'}</span>
                {#if statusConfig[endpoint.status]?.pulse}
                  <span class="absolute top-0 left-0 w-full h-full rounded-full animate-ping opacity-75 {endpoint.status === 'healthy' ? 'bg-green-500' : endpoint.status === 'warming' ? 'bg-yellow-500' : endpoint.status === 'unhealthy' ? 'bg-red-500' : 'bg-gray-500'}"></span>
                {/if}
              </div>
            </div>
            
            <div class="flex-1">
              <h3 class="font-semibold text-gray-900 dark:text-white">{endpoint.name}</h3>
              <div class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
                Priority: {endpoint.priority}
                {#if endpoint.priority >= 100}
                  <span class="px-2 py-0.5 text-xs rounded-full font-medium bg-purple-100 dark:bg-purple-900/20 text-purple-600 dark:text-purple-400">HIGH</span>
                {/if}
              </div>
            </div>
            
            <div class="text-right">
              <span class="text-2xl font-bold text-gray-900 dark:text-white">{endpoint.models?.count || 0}</span>
              <span class="block text-xs text-gray-500 dark:text-gray-400">models</span>
            </div>
          </div>
        
          <!-- Performance metrics -->
          <div class="grid grid-cols-3 gap-2 mb-3">
            <div class="text-center">
              <span class="block text-xs text-gray-500 dark:text-gray-400">Success Rate</span>
              <span class="block text-sm font-mono font-medium text-gray-900 dark:text-white {parseFloat(endpoint.success_rate) < 90 ? 'text-yellow-600 dark:text-yellow-400' : ''}">
                {endpoint.success_rate || '—'}
              </span>
            </div>
            
            <div class="text-center">
              <span class="block text-xs text-gray-500 dark:text-gray-400">Avg Latency</span>
              <span class="block text-sm font-mono font-medium text-gray-900 dark:text-white">
                {formatLatency(endpoint.avg_latency)}
              </span>
            </div>
            
            <div class="text-center">
              <span class="block text-xs text-gray-500 dark:text-gray-400">Connections</span>
              <span class="block text-sm font-mono font-medium text-gray-900 dark:text-white">
                {endpoint.connections || 0}
              </span>
            </div>
          </div>
        
          <!-- Mini performance chart -->
          <div class="mb-3 opacity-50 hover:opacity-100 transition-opacity">
            <svg class="w-full h-12" viewBox="0 0 100 48">
              {#each generateChartData() as value, i}
                <rect
                  x={i * 10}
                  y={48 - value * 0.48}
                  width="8"
                  height={value * 0.48}
                  class="fill-blue-500 opacity-30"
                />
              {/each}
            </svg>
          </div>
        
          <!-- Additional info -->
          <div class="flex items-center justify-between text-xs text-gray-600 dark:text-gray-400">
            <div class="flex items-center gap-1">
              <span class="text-blue-500">↕</span>
              {endpoint.traffic || '0 B'}
            </div>
            
            {#if endpoint.issues}
              <div class="flex items-center gap-1 text-yellow-600 dark:text-yellow-400">
                <span>⚠</span>
                {endpoint.issues}
              </div>
            {/if}
          </div>
        
          <!-- Circuit breaker indicator (for Olla engine) -->
          {#if endpoint.circuit_breaker}
            <div class="absolute top-2 right-2 text-xs px-2 py-1 rounded-full font-medium {endpoint.circuit_breaker.state === 'open' ? 'text-red-600 dark:text-red-400 bg-red-100 dark:bg-red-900/20' : endpoint.circuit_breaker.state === 'half-open' ? 'text-yellow-600 dark:text-yellow-400 bg-yellow-100 dark:bg-yellow-900/20' : 'text-green-600 dark:text-green-400 bg-green-100 dark:bg-green-900/20'}">
              Circuit: {endpoint.circuit_breaker.state}
            </div>
          {/if}
        </div>
      {/each}
      
      {#if endpoints.length === 0}
        <div class="col-span-full text-center py-12">
          <div class="text-6xl opacity-50 mb-4">🔌</div>
          <p class="text-lg font-medium text-gray-600 dark:text-gray-400">No endpoints configured</p>
          <p class="text-sm text-gray-500 dark:text-gray-500 mt-2">Add endpoints in your config.yaml file</p>
        </div>
      {/if}
    </div>
  </div>
</div>

