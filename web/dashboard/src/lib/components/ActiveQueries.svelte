<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Use $props for component properties if needed
  let { maxHeight = 400 } = $props();
  
  // Get reactive references using $derived
  const connections = $derived(dashboardStore.connections);
  const endpoints = $derived(dashboardStore.endpoints);
  const models = $derived(dashboardStore.unifiedModels);
  
  // Animation states
  let animationStates = $state({});
  
  // Derived values for summary stats
  const activeCount = $derived(connections.filter(c => c.status === 'active').length);
  const pendingCount = $derived(connections.filter(c => c.status === 'pending').length);
  const uniqueModels = $derived([...new Set(connections.map(c => c.model))].length);
  
  // Format duration
  function formatDuration(startTime) {
    if (!startTime) return '0ms';
    const duration = Date.now() - new Date(startTime).getTime();
    if (duration < 1000) return `${Math.round(duration)}ms`;
    if (duration < 60000) return `${(duration / 1000).toFixed(1)}s`;
    return `${Math.floor(duration / 60000)}m ${Math.floor((duration % 60000) / 1000)}s`;
  }
  
  // Get endpoint info with memoization
  const endpointMap = $derived.by(() => {
    const map = new Map();
    endpoints.forEach(ep => {
      map.set(ep.name, {
        type: ep.type || ep.backend_type || 'unknown',
        icon: getEndpointIcon(ep.type || ep.backend_type || 'unknown')
      });
    });
    return map;
  });
  
  function getEndpointIcon(type) {
    const icons = {
      'ollama': '🦙',
      'lm-studio': '🎨',
      'openai': '🤖',
      'vllm': '⚡',
      'other': '🔌'
    };
    return icons[type] || icons.other;
  }
  
  function getEndpointInfo(endpointName) {
    return endpointMap.get(endpointName) || { type: 'unknown', icon: '🔌' };
  }
  
  // Get model info with memoization
  const modelMap = $derived.by(() => {
    const map = new Map();
    models.forEach(m => {
      map.set(m.id, m);
    });
    return map;
  });
  
  function getModelInfo(modelId) {
    return modelMap.get(modelId) || { id: modelId, name: modelId };
  }
  
  // Get status color
  function getStatusColor(connection) {
    if (connection.error) return 'text-red-600 dark:text-red-400';
    if (connection.status === 'active') return 'text-green-600 dark:text-green-400';
    if (connection.status === 'pending') return 'text-yellow-600 dark:text-yellow-400';
    return 'text-gray-600 dark:text-gray-400';
  }
  
  // Track animations using $effect
  $effect(() => {
    const currentIds = new Set(connections.map(c => c.id));
    const existingIds = new Set(Object.keys(animationStates));
    
    // Find new connections
    const newIds = [...currentIds].filter(id => !existingIds.has(id));
    
    // Add animations for new connections
    if (newIds.length > 0) {
      const newStates = { ...animationStates };
      newIds.forEach(id => {
        newStates[id] = { isNew: true, timestamp: Date.now() };
      });
      animationStates = newStates;
      
      // Remove animation flag after delay
      setTimeout(() => {
        const updatedStates = { ...animationStates };
        newIds.forEach(id => {
          if (updatedStates[id]) {
            updatedStates[id] = { ...updatedStates[id], isNew: false };
          }
        });
        animationStates = updatedStates;
      }, 500);
    }
    
    // Clean up old animation states
    const staleIds = [...existingIds].filter(id => !currentIds.has(id));
    if (staleIds.length > 0) {
      const cleanedStates = { ...animationStates };
      staleIds.forEach(id => delete cleanedStates[id]);
      animationStates = cleanedStates;
    }
  });
  
  // Use $inspect for debugging in development
  $inspect('Active Queries', { connections: connections.length, active: activeCount, pending: pendingCount });
</script>

<div class="bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-900 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Active Queries</h3>
      <div class="flex items-center gap-2">
        <div class="flex items-center gap-1">
          <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
          <span class="text-sm text-gray-600 dark:text-gray-400">{connections.length} active</span>
        </div>
      </div>
    </div>
  </div>
  
  <div class="p-6">
    {#if connections.length === 0}
      <div class="text-center py-12">
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-full bg-gray-100 dark:bg-gray-800 mb-4">
          <span class="text-2xl">💤</span>
        </div>
        <p class="text-gray-500 dark:text-gray-400">No active queries</p>
        <p class="text-sm text-gray-400 dark:text-gray-500 mt-1">Queries will appear here when processing</p>
      </div>
    {:else}
      <div class="space-y-3 overflow-y-auto" style="max-height: {maxHeight}px;">
        {#each connections as connection (connection.id)}
          {@const endpointInfo = getEndpointInfo(connection.endpoint)}
          {@const modelInfo = getModelInfo(connection.model || 'unknown')}
          {@const animState = animationStates[connection.id]}
          <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 
                      transition-all duration-300 hover:shadow-md
                      {animState?.isNew ? 'animate-slideIn' : ''}">
            <div class="flex items-start justify-between mb-2">
              <div class="flex items-center gap-3">
                <div class="flex items-center gap-1">
                  <span class="text-lg">{getEndpointIcon(connection.endpointType)}</span>
                  <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {connection.displayName || connection.endpoint}
                  </span>
                </div>
                <span class={`text-xs font-medium ${getStatusColor(connection)}`}>
                  {#if connection.connectionCount && connection.connectionCount > 1}
                    {connection.connectionCount} active
                  {:else}
                    {connection.status || 'active'}
                  {/if}
                </span>
              </div>
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {formatDuration(connection.started_at)}
              </div>
            </div>
            
            <div class="space-y-1">
              {#if connection.isEndpointSummary}
                <div class="flex items-center gap-2 text-sm">
                  <span class="text-gray-600 dark:text-gray-400">Endpoint Status:</span>
                  <span class="font-medium text-gray-900 dark:text-white truncate">
                    {connection.connectionCount} active connection{connection.connectionCount > 1 ? 's' : ''}
                  </span>
                </div>
                
                <div class="flex items-center gap-4 text-xs text-gray-500 dark:text-gray-400">
                  <span>Total Requests: {connection.requests}</span>
                  <span>Success Rate: {connection.successRate}</span>
                  <span>Avg Latency: {connection.avgLatency}</span>
                </div>
                
                {#if connection.traffic && connection.traffic !== '0 B'}
                  <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>Traffic: {connection.traffic}</span>
                  </div>
                {/if}
                
                <div class="text-xs text-gray-400 dark:text-gray-500 italic">
                  Note: Individual request details (model names, tokens) are only available when requests complete.
                </div>
              {:else}
                <div class="flex items-center gap-2 text-sm">
                  <span class="text-gray-600 dark:text-gray-400">Model:</span>
                  <span class="font-medium text-gray-900 dark:text-white truncate">
                    {modelInfo.name || modelInfo.id || 'Unknown'}
                  </span>
                </div>
                
                {#if connection.method || connection.path}
                  <div class="flex items-center gap-2 text-sm">
                    <span class="text-gray-600 dark:text-gray-400">Request:</span>
                    <code class="text-xs bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded">
                      {connection.method || 'POST'} {connection.path || '/api/chat'}
                    </code>
                  </div>
                {/if}
                
                {#if connection.isReal}
                  <div class="flex items-center gap-4 text-xs text-gray-500 dark:text-gray-400">
                    <span>Requests: {connection.requests}</span>
                    <span>Success Rate: {connection.successRate}</span>
                    <span>Latency: {connection.avgLatency}</span>
                  </div>
                {/if}
              {/if}
              
              {#if connection.error}
                <div class="mt-2 text-xs text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20 rounded p-2">
                  {connection.error}
                </div>
              {/if}
            </div>
            
            {#if connection.status === 'active' && connection.isReal}
              <div class="mt-3">
                <div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400 mb-1">
                  <span>Active Connection</span>
                  <span>Live</span>
                </div>
                <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-1 overflow-hidden">
                  <div class="bg-green-500 h-full rounded-full animate-pulse" style="width: 100%"></div>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
    
    <!-- Summary Stats -->
    {#if connections.length > 0}
      <div class="mt-6 pt-6 border-t border-gray-200 dark:border-gray-700">
        <div class="grid grid-cols-3 gap-4 text-center">
          <div>
            <div class="text-lg font-semibold text-gray-900 dark:text-white">
              {activeCount}
            </div>
            <div class="text-xs text-gray-600 dark:text-gray-400">Processing</div>
          </div>
          <div>
            <div class="text-lg font-semibold text-gray-900 dark:text-white">
              {pendingCount}
            </div>
            <div class="text-xs text-gray-600 dark:text-gray-400">Queued</div>
          </div>
          <div>
            <div class="text-lg font-semibold text-gray-900 dark:text-white">
              {uniqueModels}
            </div>
            <div class="text-xs text-gray-600 dark:text-gray-400">Models</div>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  @keyframes slideIn {
    from {
      opacity: 0;
      transform: translateY(-10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
  
  .animate-slideIn {
    animation: slideIn 0.3s ease-out;
  }
</style>