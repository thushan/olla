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

<div class="endpoint-grid">
  <div class="grid-header">
    <h2 class="text-xl font-semibold text-primary">
      Endpoint Health
    </h2>
    <div class="grid-stats text-sm text-secondary">
      {endpoints.filter(e => e.status === 'healthy').length} / {endpoints.length} healthy
    </div>
  </div>
  
  <div class="endpoints-container">
    {#each sortedEndpoints as endpoint (endpoint.name)}
      <div 
        class="endpoint-card"
        transition:scale={{ duration: 300, delay: sortedEndpoints.indexOf(endpoint) * 50 }}
      >
        <!-- Status beacon -->
        <div class="endpoint-header">
          <div class="status-beacon">
            <div class="beacon {statusConfig[endpoint.status]?.color} {statusConfig[endpoint.status]?.pulse ? 'beacon-pulse' : ''}">
              <span class="beacon-icon">{statusConfig[endpoint.status]?.icon || '?'}</span>
            </div>
          </div>
          
          <div class="endpoint-info">
            <h3 class="endpoint-name">{endpoint.name}</h3>
            <div class="endpoint-priority">
              Priority: {endpoint.priority}
              {#if endpoint.priority >= 100}
                <span class="priority-badge">HIGH</span>
              {/if}
            </div>
          </div>
          
          <div class="endpoint-models">
            <span class="model-count">{endpoint.models?.count || 0}</span>
            <span class="model-label">models</span>
          </div>
        </div>
        
        <!-- Performance metrics -->
        <div class="endpoint-metrics">
          <div class="metric">
            <span class="metric-label">Success Rate</span>
            <span class="metric-value {parseFloat(endpoint.success_rate) < 90 ? 'text-warning' : ''}">
              {endpoint.success_rate || '—'}
            </span>
          </div>
          
          <div class="metric">
            <span class="metric-label">Avg Latency</span>
            <span class="metric-value">
              {formatLatency(endpoint.avg_latency)}
            </span>
          </div>
          
          <div class="metric">
            <span class="metric-label">Connections</span>
            <span class="metric-value">
              {endpoint.connections || 0}
            </span>
          </div>
        </div>
        
        <!-- Mini performance chart -->
        <div class="endpoint-chart">
          <svg class="w-full h-12" viewBox="0 0 100 48">
            {#each generateChartData() as value, i}
              <rect
                x={i * 10}
                y={48 - value * 0.48}
                width="8"
                height={value * 0.48}
                class="fill-current opacity-30"
                style="fill: var(--color-blue);"
              />
            {/each}
          </svg>
        </div>
        
        <!-- Additional info -->
        <div class="endpoint-footer">
          <div class="endpoint-traffic">
            <span class="traffic-icon">↕</span>
            {endpoint.traffic || '0 B'}
          </div>
          
          {#if endpoint.issues}
            <div class="endpoint-issues">
              <span class="issue-icon">⚠</span>
              {endpoint.issues}
            </div>
          {/if}
        </div>
        
        <!-- Circuit breaker indicator (for Olla engine) -->
        {#if endpoint.circuit_breaker}
          <div class="circuit-breaker circuit-breaker-{endpoint.circuit_breaker.state}">
            Circuit: {endpoint.circuit_breaker.state}
          </div>
        {/if}
      </div>
    {/each}
    
    {#if endpoints.length === 0}
      <div class="empty-state">
        <div class="empty-icon">🔌</div>
        <p class="empty-text">No endpoints configured</p>
        <p class="empty-hint">Add endpoints in your config.yaml file</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .endpoint-grid {
    margin-bottom: 2rem;
  }
  
  .grid-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1.5rem;
  }
  
  .endpoints-container {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 768px) {
    .endpoints-container {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  
  @media (min-width: 1280px) {
    .endpoints-container {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
  
  @media (min-width: 1536px) {
    .endpoints-container {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }
  
  .endpoint-card {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    padding: 1.25rem;
    border-width: 1px;
    border-style: solid;
    border-color: transparent;
    position: relative;
    overflow: hidden;
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 300ms;
  }
  
  .endpoint-card:hover {
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
    transform: scale(1.02);
    border-color: var(--bg-tertiary);
  }
  
  .endpoint-header {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }
  
  .status-beacon {
    position: relative;
  }
  
  .beacon {
    width: 2.5rem;
    height: 2.5rem;
    border-radius: 9999px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
    font-weight: 700;
    font-size: 0.875rem;
    line-height: 1.25rem;
    position: relative;
  }
  
  .beacon-pulse::before {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    left: 0;
    border-radius: 9999px;
    animation: ping 1s cubic-bezier(0, 0, 0.2, 1) infinite;
    opacity: 0.75;
    background: inherit;
  }
  
  .endpoint-info {
    flex: 1 1 0%;
  }
  
  .endpoint-name {
    font-weight: 600;
    color: var(--text-primary);
  }
  
  .endpoint-priority {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .priority-badge {
    padding-left: 0.5rem;
    padding-right: 0.5rem;
    padding-top: 0.125rem;
    padding-bottom: 0.125rem;
    font-size: 0.75rem;
    line-height: 1rem;
    border-radius: 9999px;
    font-weight: 500;
    background-color: rgba(153, 76, 195, 0.2);
    color: var(--color-purple);
  }
  
  .endpoint-models {
    text-align: right;
  }
  
  .model-count {
    font-size: 1.5rem;
    line-height: 2rem;
    font-weight: 700;
    color: var(--text-primary);
  }
  
  .model-label {
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-muted);
    display: block;
  }
  
  .endpoint-metrics {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  
  .metric {
    text-align: center;
  }
  
  .metric-label {
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-muted);
    display: block;
  }
  
  .metric-value {
    font-size: 0.875rem;
    line-height: 1.25rem;
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
    font-weight: 500;
    color: var(--text-primary);
    display: block;
  }
  
  .endpoint-chart {
    margin-bottom: 0.75rem;
    opacity: 0.5;
    transition-property: opacity;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
  }
  
  .endpoint-chart:hover {
    opacity: 1;
  }
  
  .endpoint-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.75rem;
    line-height: 1rem;
    color: var(--text-secondary);
  }
  
  .endpoint-traffic {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  
  .traffic-icon {
    color: var(--color-blue);
  }
  
  .endpoint-issues {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--color-yellow);
  }
  
  .circuit-breaker {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    font-size: 0.75rem;
    line-height: 1rem;
    padding-left: 0.5rem;
    padding-right: 0.5rem;
    padding-top: 0.25rem;
    padding-bottom: 0.25rem;
    border-radius: 9999px;
    font-weight: 500;
  }
  
  .circuit-breaker-open {
    color: var(--color-red);
    background-color: rgba(230, 65, 0, 0.2);
  }
  
  .dark .circuit-breaker-open {
    background-color: rgba(255, 99, 99, 0.2);
  }
  
  .circuit-breaker-half-open {
    color: var(--color-yellow);
    background-color: rgba(201, 103, 101, 0.2);
  }
  
  .dark .circuit-breaker-half-open {
    background-color: rgba(255, 203, 139, 0.2);
  }
  
  .circuit-breaker-closed {
    color: var(--color-green);
    background-color: rgba(8, 145, 106, 0.2);
  }
  
  .dark .circuit-breaker-closed {
    background-color: rgba(34, 218, 110, 0.2);
  }
  
  .empty-state {
    grid-column: 1 / -1;
    text-align: center;
    padding-top: 3rem;
    padding-bottom: 3rem;
  }
  
  .empty-icon {
    font-size: 3.75rem;
    line-height: 1;
    margin-bottom: 1rem;
    opacity: 0.5;
  }
  
  .empty-text {
    font-size: 1.125rem;
    line-height: 1.75rem;
    font-weight: 500;
    color: var(--text-secondary);
  }
  
  .empty-hint {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-muted);
    margin-top: 0.5rem;
  }
</style>