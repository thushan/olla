<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import EndpointMap from '$lib/components/EndpointMap.svelte';
  
  const endpoints = $derived(dashboardStore.endpoints || []);
  const status = $derived(dashboardStore.status);
  
  // Group endpoints by status
  const endpointsByStatus = $derived((() => {
    const groups = {
      healthy: [],
      degraded: [],
      offline: [],
      unknown: []
    };
    
    endpoints.forEach(ep => {
      const status = ep.status === 'online' ? 'healthy' : ep.status;
      const group = groups[status] || groups.unknown;
      group.push(ep);
    });
    
    return groups;
  })());
  
  // Get status color
  function getStatusColor(status) {
    switch(status) {
      case 'healthy':
      case 'online':
        return 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-300';
      case 'degraded':
        return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-300';
      case 'offline':
        return 'bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-300';
      default:
        return 'bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-300';
    }
  }
  
  // Format latency
  function formatLatency(latency) {
    if (!latency) return 'N/A';
    if (typeof latency === 'string') return latency;
    return `${latency}ms`;
  }
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div>
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">Endpoints</h2>
    <p class="text-gray-600 dark:text-gray-400 mt-1">Monitor and manage your LLM inference endpoints</p>
  </div>
  
  <!-- Summary Cards -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Total Endpoints</p>
          <p class="text-2xl font-bold text-gray-900 dark:text-white mt-1">{endpoints.length}</p>
        </div>
        <div class="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">🖥️</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Healthy</p>
          <p class="text-2xl font-bold text-green-600 dark:text-green-400 mt-1">{endpointsByStatus.healthy.length}</p>
        </div>
        <div class="w-12 h-12 bg-green-100 dark:bg-green-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">✅</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Degraded</p>
          <p class="text-2xl font-bold text-yellow-600 dark:text-yellow-400 mt-1">{endpointsByStatus.degraded.length}</p>
        </div>
        <div class="w-12 h-12 bg-yellow-100 dark:bg-yellow-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">⚠️</span>
        </div>
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-400">Offline</p>
          <p class="text-2xl font-bold text-red-600 dark:text-red-400 mt-1">{endpointsByStatus.offline.length}</p>
        </div>
        <div class="w-12 h-12 bg-red-100 dark:bg-red-900/20 rounded-lg flex items-center justify-center">
          <span class="text-xl">❌</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Endpoint Map -->
  <div>
    <EndpointMap />
  </div>
  
  <!-- Endpoint List -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Endpoint Details</h3>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full">
        <thead>
          <tr class="border-b border-gray-200 dark:border-gray-700">
            <th class="text-left px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Name</th>
            <th class="text-left px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">URL</th>
            <th class="text-center px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Status</th>
            <th class="text-center px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Priority</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Latency</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Success Rate</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Models</th>
            <th class="text-right px-6 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Connections</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
          {#each endpoints as endpoint}
            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
              <td class="px-6 py-4 whitespace-nowrap">
                <div class="font-medium text-gray-900 dark:text-white">{endpoint.name}</div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap">
                <div class="text-sm text-gray-600 dark:text-gray-400 font-mono">{endpoint.url}</div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-center">
                <span class="inline-flex px-2 py-1 text-xs font-medium rounded-full {getStatusColor(endpoint.status)}">
                  {endpoint.status}
                </span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-center">
                <span class="text-sm text-gray-900 dark:text-white">{endpoint.priority || 0}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{formatLatency(endpoint.last_latency || endpoint.last_latency_ms)}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{endpoint.success_rate || 'N/A'}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{endpoint.models?.count || 0}</span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right">
                <span class="text-sm text-gray-900 dark:text-white">{endpoint.connections || 0}</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      
      {#if endpoints.length === 0}
        <div class="text-center py-12">
          <span class="text-3xl mb-4 block">🔍</span>
          <p class="text-gray-500 dark:text-gray-400">No endpoints configured</p>
        </div>
      {/if}
    </div>
  </div>
</div>