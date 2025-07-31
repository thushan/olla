<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { onMount, onDestroy } from 'svelte';
  
  const events = $derived(dashboardStore.events.slice(0, 10));
  const stats = $derived(dashboardStore.status?.system || {});
  let animatingRequests = $state([]);
  let lastRequestCount = $state(0);
  let animationInterval;
  
  // Simulate request flow based on stats changes
  $effect(() => {
    const currentRequests = stats.total_requests || 0;
    if (currentRequests > lastRequestCount && animatingRequests.length < 8) {
      const diff = Math.min(currentRequests - lastRequestCount, 5);
      
      for (let i = 0; i < diff; i++) {
        setTimeout(() => {
          const reqId = `req-${Date.now()}-${Math.random()}`;
          animatingRequests = [...animatingRequests, {
            id: reqId,
            startTime: Date.now()
          }];
          
          // Remove after animation
          setTimeout(() => {
            animatingRequests = animatingRequests.filter(r => r.id !== reqId);
          }, 2000);
        }, i * 200);
      }
    }
    lastRequestCount = currentRequests;
  });
  
  // Also add periodic animations if there are active connections
  onMount(() => {
    animationInterval = setInterval(() => {
      const activeConnections = stats.active_connections || 0;
      if (activeConnections > 0 && animatingRequests.length < 3) {
        const reqId = `periodic-${Date.now()}`;
        animatingRequests = [...animatingRequests, {
          id: reqId,
          startTime: Date.now()
        }];
        
        setTimeout(() => {
          animatingRequests = animatingRequests.filter(r => r.id !== reqId);
        }, 2000);
      }
    }, 3000);
  });
  
  onDestroy(() => {
    if (animationInterval) clearInterval(animationInterval);
  });
  
  // Format timestamp
  function formatTime(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('en-US', { 
      hour12: false,
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  }
  
  // Get event color
  function getEventColor(event) {
    if (event.error) return 'border-red-500 bg-red-50 dark:bg-red-900/10';
    if (event.duration > 1000) return 'border-yellow-500 bg-yellow-50 dark:bg-yellow-900/10';
    return 'border-green-500 bg-green-50 dark:bg-green-900/10';
  }
  
  // Get method badge color
  function getMethodColor(method) {
    const colors = {
      GET: 'bg-blue-100 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300',
      POST: 'bg-green-100 text-green-700 dark:bg-green-900/20 dark:text-green-300',
      PUT: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300',
      DELETE: 'bg-red-100 text-red-700 dark:bg-red-900/20 dark:text-red-300',
    };
    return colors[method] || 'bg-gray-100 text-gray-700 dark:bg-gray-900/20 dark:text-gray-300';
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 h-full">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Request Flow</h2>
      <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">Real-time request monitoring</p>
    </div>
    <div class="flex items-center gap-2">
      <div class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
      <span class="text-xs font-medium text-gray-700 dark:text-gray-300">Live</span>
    </div>
  </div>
  
  <!-- Flow Visualization -->
  <div class="relative h-24 mb-6 bg-gray-50 dark:bg-gray-900/50 rounded-xl overflow-hidden">
    <div class="absolute inset-0 flex items-center justify-between px-8">
      <!-- Client -->
      <div class="flex flex-col items-center">
        <div class="w-16 h-16 rounded-full bg-gradient-to-br from-blue-500 to-blue-600 flex items-center justify-center shadow-lg">
          <span class="text-2xl">👤</span>
        </div>
        <span class="text-xs font-medium text-gray-700 dark:text-gray-300 mt-2">Client</span>
      </div>
      
      <!-- Flow Lines -->
      <div class="flex-1 mx-8 relative">
        <div class="absolute inset-0 flex items-center">
          <div class="w-full h-0.5 bg-gray-300 dark:bg-gray-600"></div>
        </div>
        
        <!-- Animated Requests -->
        {#each animatingRequests as request}
          <div 
            class="absolute top-1/2 transform -translate-y-1/2 w-4 h-4 bg-gradient-to-r from-blue-500 to-purple-500 rounded-full shadow-lg"
            style="animation: flow 2s linear forwards; left: 0;"
          ></div>
        {/each}
      </div>
      
      <!-- Olla -->
      <div class="flex flex-col items-center">
        <div class="w-16 h-16 rounded-full bg-gradient-to-br from-purple-500 to-purple-600 flex items-center justify-center shadow-lg">
          <span class="text-white font-bold">O</span>
        </div>
        <span class="text-xs font-medium text-gray-700 dark:text-gray-300 mt-2">Olla</span>
      </div>
      
      <!-- Flow Lines -->
      <div class="flex-1 mx-8 relative">
        <div class="absolute inset-0 flex items-center">
          <div class="w-full h-0.5 bg-gray-300 dark:bg-gray-600"></div>
        </div>
      </div>
      
      <!-- Endpoints -->
      <div class="flex flex-col items-center">
        <div class="w-16 h-16 rounded-full bg-gradient-to-br from-green-500 to-green-600 flex items-center justify-center shadow-lg">
          <span class="text-2xl">🖥️</span>
        </div>
        <span class="text-xs font-medium text-gray-700 dark:text-gray-300 mt-2">Endpoints</span>
      </div>
    </div>
  </div>
  
  <!-- Recent Requests -->
  <div class="space-y-2 max-h-80 overflow-y-auto scrollbar-thin">
    {#if events.length === 0}
      <div class="text-center py-8 text-gray-500 dark:text-gray-400">
        <span class="text-3xl mb-2 block">📭</span>
        <p class="text-sm">No recent requests</p>
      </div>
    {:else}
      {#each events as event, i}
        <div 
          class="border-l-2 {getEventColor(event)} p-3 rounded-r-lg hover:shadow-md transition-all duration-200"
          style="animation: slideIn 0.3s ease-out {i * 50}ms backwards"
        >
          <div class="flex items-start justify-between">
            <div class="flex-1">
              <div class="flex items-center gap-2 mb-1">
                <span class="px-2 py-0.5 text-xs font-medium rounded {getMethodColor(event.method)}">
                  {event.method}
                </span>
                <span class="text-sm font-medium text-gray-900 dark:text-white truncate">
                  {event.path}
                </span>
              </div>
              <div class="flex items-center gap-4 text-xs text-gray-600 dark:text-gray-400">
                <span>→ {event.endpoint}</span>
                <span>{event.model}</span>
                <span>{event.duration}ms</span>
              </div>
            </div>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {formatTime(event.timestamp)}
            </span>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  @keyframes flow {
    from {
      left: 0;
    }
    to {
      left: calc(100% - 1rem);
    }
  }
  
  @keyframes slideIn {
    from {
      opacity: 0;
      transform: translateX(-10px);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }
</style>