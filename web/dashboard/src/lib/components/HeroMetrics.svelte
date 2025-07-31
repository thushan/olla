<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const totalRequests = $derived(dashboardStore.stats.totalRequests);
  const totalErrors = $derived(dashboardStore.stats.totalErrors);
  const avgResponseTime = $derived(dashboardStore.stats.avgResponseTime);
  const activeConnections = $derived(dashboardStore.stats.activeConnections);
  
  // Calculate metrics
  const successRate = $derived(
    totalRequests > 0 ? ((totalRequests - totalErrors) / totalRequests * 100).toFixed(1) : '100'
  );
  
  const errorRate = $derived(
    totalRequests > 0 ? (totalErrors / totalRequests * 100).toFixed(1) : '0'
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
  
  // Metrics configuration
  const metrics = $derived([
    {
      label: 'Total Requests',
      value: formatNumber(totalRequests),
      change: '+12.5%',
      trend: 'up',
      icon: '📊',
      color: 'blue',
      bgGradient: 'from-blue-500 to-blue-600',
    },
    {
      label: 'Success Rate',
      value: `${successRate}%`,
      change: errorRate > 5 ? '-2.3%' : '+0.8%',
      trend: errorRate > 5 ? 'down' : 'up',
      icon: '✨',
      color: 'green',
      bgGradient: 'from-green-500 to-green-600',
    },
    {
      label: 'Avg Response',
      value: formatResponseTime(avgResponseTime),
      change: avgResponseTime > 500 ? '+15ms' : '-8ms',
      trend: avgResponseTime > 500 ? 'down' : 'up',
      icon: '⚡',
      color: 'purple',
      bgGradient: 'from-purple-500 to-purple-600',
    },
    {
      label: 'Active Connections',
      value: formatNumber(activeConnections),
      change: '+5',
      trend: 'up',
      icon: '🔗',
      color: 'orange',
      bgGradient: 'from-orange-500 to-orange-600',
    },
  ]);
</script>

<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
  {#each metrics as metric, i}
    <div 
      class="group relative overflow-hidden rounded-2xl bg-white dark:bg-gray-800 p-6 shadow-lg hover:shadow-xl transition-all duration-300 hover:-translate-y-1"
      style="animation-delay: {i * 100}ms"
    >
      <!-- Background decoration -->
      <div class="absolute top-0 right-0 -mt-4 -mr-4 w-24 h-24 rounded-full bg-gradient-to-br {metric.bgGradient} opacity-10 group-hover:scale-110 transition-transform duration-300"></div>
      
      <!-- Icon -->
      <div class="relative flex items-center justify-center w-12 h-12 rounded-xl bg-gradient-to-br {metric.bgGradient} shadow-lg mb-4 group-hover:scale-110 transition-transform duration-300">
        <span class="text-2xl filter drop-shadow-md">{metric.icon}</span>
      </div>
      
      <!-- Content -->
      <div class="relative">
        <p class="text-sm font-medium text-gray-600 dark:text-gray-400 mb-1">{metric.label}</p>
        <div class="flex items-baseline gap-2">
          <h3 class="text-3xl font-bold text-gray-900 dark:text-white">{metric.value}</h3>
          <div class="flex items-center gap-1 text-sm">
            {#if metric.trend === 'up'}
              <svg class="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
              </svg>
              <span class="text-green-600 dark:text-green-400 font-medium">{metric.change}</span>
            {:else}
              <svg class="w-4 h-4 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 17h8m0 0V9m0 8l-8-8-4 4-6-6" />
              </svg>
              <span class="text-red-600 dark:text-red-400 font-medium">{metric.change}</span>
            {/if}
          </div>
        </div>
      </div>
      
      <!-- Hover effect line -->
      <div class="absolute bottom-0 left-0 w-full h-1 bg-gradient-to-r {metric.bgGradient} transform scale-x-0 group-hover:scale-x-100 transition-transform duration-300"></div>
    </div>
  {/each}
</div>