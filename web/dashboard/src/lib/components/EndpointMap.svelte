<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { onMount } from 'svelte';
  
  const endpoints = $derived(dashboardStore.endpoints);
  let selectedEndpoint = $state(null);
  let hoveredEndpoint = $state(null);
  
  // Group endpoints by type
  const endpointGroups = $derived(() => {
    const groups = {};
    endpoints.forEach(ep => {
      const type = ep.backend_type || 'unknown';
      if (!groups[type]) groups[type] = [];
      groups[type].push(ep);
    });
    return groups;
  });
  
  // Get status color
  function getStatusColor(status) {
    switch(status) {
      case 'online': return 'from-green-400 to-green-600';
      case 'offline': return 'from-red-400 to-red-600';
      case 'degraded': return 'from-yellow-400 to-yellow-600';
      default: return 'from-gray-400 to-gray-600';
    }
  }
  
  // Get status glow
  function getStatusGlow(status) {
    switch(status) {
      case 'online': return 'shadow-green-500/50';
      case 'offline': return 'shadow-red-500/50';
      case 'degraded': return 'shadow-yellow-500/50';
      default: return 'shadow-gray-500/50';
    }
  }
  
  // Format latency
  function formatLatency(ms) {
    if (!ms) return '--';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 h-full">
  <div class="flex items-center justify-between mb-6">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Endpoint Network</h2>
    <div class="flex items-center gap-4">
      <div class="flex items-center gap-2">
        <div class="w-3 h-3 rounded-full bg-green-500"></div>
        <span class="text-xs text-gray-600 dark:text-gray-400">Online</span>
      </div>
      <div class="flex items-center gap-2">
        <div class="w-3 h-3 rounded-full bg-yellow-500"></div>
        <span class="text-xs text-gray-600 dark:text-gray-400">Degraded</span>
      </div>
      <div class="flex items-center gap-2">
        <div class="w-3 h-3 rounded-full bg-red-500"></div>
        <span class="text-xs text-gray-600 dark:text-gray-400">Offline</span>
      </div>
    </div>
  </div>
  
  <!-- Endpoint Visualization -->
  <div class="relative h-96 bg-gray-50 dark:bg-gray-900/50 rounded-xl overflow-hidden">
    <!-- Grid background -->
    <div class="absolute inset-0 bg-grid-pattern opacity-5"></div>
    
    <!-- Central Hub -->
    <div class="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2">
      <div class="relative">
        <div class="w-24 h-24 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-2xl animate-pulse">
          <span class="text-white font-bold text-2xl">OLLA</span>
        </div>
        <div class="absolute inset-0 rounded-full bg-blue-500 animate-ping opacity-20"></div>
      </div>
    </div>
    
    <!-- Endpoints -->
    {#each Object.entries(endpointGroups) as [type, typeEndpoints], groupIndex}
      {#each typeEndpoints as endpoint, index}
        {@const angle = (360 / endpoints.length) * (groupIndex * typeEndpoints.length + index)}
        {@const radius = 140}
        {@const x = Math.cos((angle - 90) * Math.PI / 180) * radius}
        {@const y = Math.sin((angle - 90) * Math.PI / 180) * radius}
        
        <!-- Connection Line -->
        <svg class="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 pointer-events-none">
          <line
            x1="0"
            y1="0"
            x2={x}
            y2={y}
            stroke="currentColor"
            stroke-width="2"
            stroke-dasharray="5,5"
            class="{endpoint.status === 'online' ? 'text-green-500' : 'text-gray-300 dark:text-gray-600'} opacity-50"
          >
            {#if endpoint.status === 'online'}
              <animate
                attributeName="stroke-dashoffset"
                values="0;10"
                dur="1s"
                repeatCount="indefinite"
              />
            {/if}
          </line>
        </svg>
        
        <!-- Endpoint Node -->
        <button
          class="absolute transform -translate-x-1/2 -translate-y-1/2 group"
          style="top: 50%; left: 50%; transform: translate({x}px, {y}px) translate(-50%, -50%);"
          on:click={() => selectedEndpoint = selectedEndpoint?.name === endpoint.name ? null : endpoint}
          on:mouseenter={() => hoveredEndpoint = endpoint}
          on:mouseleave={() => hoveredEndpoint = null}
        >
          <div class="relative">
            <div class="w-16 h-16 rounded-xl bg-gradient-to-br {getStatusColor(endpoint.status)} flex items-center justify-center shadow-lg {getStatusGlow(endpoint.status)} group-hover:scale-110 transition-all duration-300">
              <span class="text-2xl font-semibold text-white">
                {type === 'ollama' ? '🦙' : type === 'lm-studio' ? '🎨' : type === 'openai' ? '🤖' : '🔌'}
              </span>
            </div>
            {#if endpoint.status === 'online'}
              <div class="absolute -top-1 -right-1 w-3 h-3 bg-green-500 rounded-full animate-pulse"></div>
            {/if}
          </div>
          
          <!-- Tooltip -->
          {#if hoveredEndpoint === endpoint}
            <div class="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-gray-900 text-white text-xs rounded-lg whitespace-nowrap z-10">
              <div class="font-semibold">{endpoint.name}</div>
              <div class="text-gray-300">{endpoint.url}</div>
              <div class="text-gray-400">Latency: {formatLatency(endpoint.last_latency_ms)}</div>
            </div>
          {/if}
        </button>
      {/each}
    {/each}
  </div>
  
  <!-- Selected Endpoint Details -->
  {#if selectedEndpoint}
    <div class="mt-4 p-4 bg-gray-50 dark:bg-gray-900/50 rounded-xl">
      <div class="flex items-center justify-between mb-2">
        <h3 class="font-semibold text-gray-900 dark:text-white">{selectedEndpoint.name}</h3>
        <span class="px-2 py-1 text-xs font-medium rounded-full {selectedEndpoint.status === 'online' ? 'bg-green-100 text-green-700 dark:bg-green-900/20 dark:text-green-400' : 'bg-red-100 text-red-700 dark:bg-red-900/20 dark:text-red-400'}">
          {selectedEndpoint.status}
        </span>
      </div>
      <div class="grid grid-cols-2 gap-4 text-sm">
        <div>
          <span class="text-gray-600 dark:text-gray-400">Type:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.backend_type}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Priority:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.priority}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Models:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.models?.length || 0}</span>
        </div>
        <div>
          <span class="text-gray-600 dark:text-gray-400">Health Checks:</span>
          <span class="ml-2 font-medium text-gray-900 dark:text-white">{selectedEndpoint.health_checks || 0}</span>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .bg-grid-pattern {
    background-image: 
      linear-gradient(rgba(0, 0, 0, 0.1) 1px, transparent 1px),
      linear-gradient(90deg, rgba(0, 0, 0, 0.1) 1px, transparent 1px);
    background-size: 20px 20px;
  }
  
  .dark .bg-grid-pattern {
    background-image: 
      linear-gradient(rgba(255, 255, 255, 0.1) 1px, transparent 1px),
      linear-gradient(90deg, rgba(255, 255, 255, 0.1) 1px, transparent 1px);
  }
</style>