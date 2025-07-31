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

<div class="request-stream">
  <div class="stream-header">
    <div class="stream-title">
      <h3 class="text-lg font-semibold text-primary">Live Request Stream</h3>
      <span class="connection-indicator" class:connected={wsConnected}>
        <span class="indicator-dot"></span>
        {wsConnected ? 'Connected' : 'Disconnected'}
      </span>
    </div>
    
    <div class="stream-controls">
      <button 
        class="control-button"
        class:active={autoScroll}
        onclick={() => autoScroll = !autoScroll}
      >
        {autoScroll ? '⏸️ Pause' : '▶️ Resume'} Auto-scroll
      </button>
      
      <button 
        class="control-button"
        onclick={() => dashboardStore.clearEvents()}
      >
        🗑️ Clear
      </button>
    </div>
  </div>
  
  <div class="stream-container" bind:this={streamContainer}>
    {#if events.length === 0}
      <div class="empty-state">
        <div class="empty-icon">📡</div>
        <p class="empty-text">Waiting for requests...</p>
        <p class="empty-subtext">
          {wsConnected ? 'Real-time updates enabled' : 'Connecting to server...'}
        </p>
      </div>
    {:else}
      <div class="event-list">
        {#each events as event}
          {@const eventType = formatEventType(event.type)}
          <div class="event-item {eventType.class}">
            <div class="event-header">
              <span class="event-time">{formatTime(event.timestamp)}</span>
              <span class="event-type">{eventType.label}</span>
              {#if event.duration}
                <span class="event-duration">{formatDuration(event.duration)}</span>
              {/if}
            </div>
            
            <div class="event-details">
              {#if event.endpoint}
                <span class="event-endpoint">📍 {event.endpoint}</span>
              {/if}
              
              {#if event.metadata?.model}
                <span class="event-model">🤖 {event.metadata.model}</span>
              {/if}
              
              {#if event.error}
                <span class="event-error">❌ {event.error}</span>
              {/if}
            </div>
            
            {#if event.request_id}
              <div class="event-footer">
                <span class="event-request-id">ID: {event.request_id}</span>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .request-stream {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--bg-tertiary);
    overflow: hidden;
  }
  
  .stream-header {
    padding-left: 1.5rem;
    padding-right: 1.5rem;
    padding-top: 1rem;
    padding-bottom: 1rem;
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: var(--bg-tertiary);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .stream-title {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  
  .connection-indicator {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .indicator-dot {
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 9999px;
    background-color: var(--color-red);
  }
  
  .connection-indicator.connected .indicator-dot {
    background-color: var(--color-green);
    animation: pulse 2s infinite;
  }
  
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  
  .stream-controls {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .control-button {
    padding-left: 0.75rem;
    padding-right: 0.75rem;
    padding-top: 0.25rem;
    padding-bottom: 0.25rem;
    font-size: 0.875rem;
    line-height: 1.25rem;
    border-radius: 0.5rem;
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
    color: var(--text-secondary);
  }
  
  .control-button:hover {
    background-color: var(--bg-tertiary);
  }
  
  .control-button.active {
    background-color: var(--bg-tertiary);
    color: var(--color-blue);
  }
  
  .stream-container {
    height: 24rem;
    overflow-y: auto;
  }
  
  .empty-state {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
  }
  
  .empty-icon {
    font-size: 2.25rem;
    line-height: 2.5rem;
    margin-bottom: 0.75rem;
  }
  
  .empty-text {
    color: var(--text-primary);
    font-weight: 500;
  }
  
  .empty-subtext {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .event-list {
    padding: 1rem;
  }
  
  .event-list > * + * {
    margin-top: 0.5rem;
  }
  
  .event-item {
    padding: 0.75rem;
    border-radius: 0.5rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--bg-tertiary);
    background-color: var(--bg-primary);
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
  }
  
  .event-item:hover {
    box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
    border-color: var(--color-blue);
  }
  
  .event-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  
  .event-time {
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-muted);
  }
  
  .event-type {
    font-size: 0.75rem;
    line-height: 1rem;
    font-weight: 500;
    padding-left: 0.5rem;
    padding-right: 0.5rem;
    padding-top: 0.25rem;
    padding-bottom: 0.25rem;
    border-radius: 9999px;
  }
  
  .event-success .event-type {
    background-color: rgba(var(--color-green-rgb, 8, 145, 106), 0.1);
    color: var(--color-green);
  }
  
  .event-error .event-type {
    background-color: rgba(var(--color-red-rgb, 230, 65, 0), 0.1);
    color: var(--color-red);
  }
  
  .event-warning .event-type {
    background-color: rgba(var(--color-orange-rgb, 170, 93, 0), 0.1);
    color: var(--color-orange);
  }
  
  .event-info .event-type {
    background-color: rgba(var(--color-blue-rgb, 0, 119, 170), 0.1);
    color: var(--color-blue);
  }
  
  .event-duration {
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-secondary);
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
  }
  
  .event-details {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  
  .event-endpoint, .event-model {
    color: var(--text-secondary);
  }
  
  .event-error {
    color: #ef4444;
    font-size: 0.75rem;
    line-height: 1rem;
  }
  
  .event-footer {
    margin-top: 0.5rem;
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-muted);
  }
  
  .event-request-id {
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
  }
</style>