<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references from the store
  const events = $derived(dashboardStore.events);
  const wsConnected = $derived(dashboardStore.wsConnected);
  
  // Auto-scroll state
  let autoScroll = $state(true);
  let streamContainer = null;
  
  // Format event type for display
  function formatEventType(type) {
    const typeMap = {
      'proxy.success': { label: 'Success', class: 'event-success' },
      'proxy.error': { label: 'Error', class: 'event-error' },
      'circuit_breaker.open': { label: 'Circuit Open', class: 'event-warning' },
      'client.disconnected': { label: 'Disconnected', class: 'event-info' },
    };
    return typeMap[type] || { label: type, class: 'event-default' };
  }
  
  // Format duration for display
  function formatDuration(ms) {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
  
  // Format timestamp
  function formatTime(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString();
  }
  
  // Auto-scroll effect
  $effect(() => {
    if (autoScroll && streamContainer && events.length > 0) {
      streamContainer.scrollTop = streamContainer.scrollHeight;
    }
  });
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center gap-3">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Live Request Stream</h3>
      <span class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        <span class="w-2 h-2 rounded-full {wsConnected ? 'bg-green-500 animate-pulse' : 'bg-red-500'}"></span>
        {wsConnected ? 'Connected' : 'Disconnected'}
      </span>
    </div>
    
    <div class="flex items-center gap-2">
      <button 
        class="px-3 py-1 text-sm rounded-lg transition-all duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 {autoScroll ? 'bg-gray-100 dark:bg-gray-700 text-blue-600 dark:text-blue-400' : ''}"
        onclick={() => autoScroll = !autoScroll}
      >
        {autoScroll ? '⏸️ Pause' : '▶️ Resume'} Auto-scroll
      </button>
      
      <button 
        class="px-3 py-1 text-sm rounded-lg transition-all duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700"
        onclick={() => dashboardStore.clearEvents()}
      >
        🗑️ Clear
      </button>
    </div>
  </div>
  
  <div class="h-96 overflow-y-auto" bind:this={streamContainer}>
    {#if events.length === 0}
      <div class="h-full flex flex-col items-center justify-center">
        <div class="text-4xl mb-3">📡</div>
        <p class="text-gray-900 dark:text-white font-medium">Waiting for requests...</p>
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {wsConnected ? 'Real-time updates enabled' : 'Connecting to server...'}
        </p>
      </div>
    {:else}
      <div class="p-4 space-y-2">
        {#each events as event}
          {@const eventType = formatEventType(event.type)}
          <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 transition-all duration-200 hover:shadow-sm hover:border-blue-300 dark:hover:border-blue-600 {eventType.class === 'event-success' ? 'border-l-4 border-l-green-500' : eventType.class === 'event-error' ? 'border-l-4 border-l-red-500' : eventType.class === 'event-warning' ? 'border-l-4 border-l-yellow-500' : eventType.class === 'event-info' ? 'border-l-4 border-l-blue-500' : ''}">
            <div class="flex items-center gap-3 mb-2">
              <span class="text-xs text-gray-500 dark:text-gray-400">{formatTime(event.timestamp)}</span>
              <span class="text-xs font-medium px-2 py-1 rounded-full {eventType.class === 'event-success' ? 'bg-green-100 dark:bg-green-900/20 text-green-600 dark:text-green-400' : eventType.class === 'event-error' ? 'bg-red-100 dark:bg-red-900/20 text-red-600 dark:text-red-400' : eventType.class === 'event-warning' ? 'bg-yellow-100 dark:bg-yellow-900/20 text-yellow-600 dark:text-yellow-400' : eventType.class === 'event-info' ? 'bg-blue-100 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'}">{eventType.label}</span>
              {#if event.duration}
                <span class="text-xs text-gray-600 dark:text-gray-400 font-mono">{formatDuration(event.duration)}</span>
              {/if}
            </div>
            
            <div class="flex flex-wrap gap-3 text-sm">
              {#if event.endpoint}
                <span class="text-gray-600 dark:text-gray-400">📍 {event.endpoint}</span>
              {/if}
              
              {#if event.metadata?.model}
                <span class="text-gray-600 dark:text-gray-400">🤖 {event.metadata.model}</span>
              {/if}
              
              {#if event.error}
                <span class="text-red-600 dark:text-red-400 text-xs">❌ {event.error}</span>
              {/if}
            </div>
            
            {#if event.request_id}
              <div class="mt-2">
                <span class="text-xs text-gray-500 dark:text-gray-400 font-mono">ID: {event.request_id}</span>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

