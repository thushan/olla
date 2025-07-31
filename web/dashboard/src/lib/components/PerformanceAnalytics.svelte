<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const processStats = $derived(dashboardStore.processStats);
  const status = $derived(dashboardStore.status);
  const modelStats = $derived(dashboardStore.modelStats);
  
  // Active tab
  let activeTab = $state('system');
  
  // Calculate performance metrics
  const performanceMetrics = $derived(() => {
    if (!status?.system) {
      return {
        avgLatency: 0,
        successRate: 0,
        throughput: 0,
        errorRate: 0,
      };
    }
    
    const system = status.system;
    const total = system.total_requests || 1;
    const successful = system.successful_requests || 0;
    const failed = system.failed_requests || 0;
    
    return {
      avgLatency: system.avg_latency_ms || 0,
      successRate: ((successful / total) * 100).toFixed(1),
      throughput: Math.round(total / 60), // requests per minute
      errorRate: ((failed / total) * 100).toFixed(1),
    };
  });
  
  // Memory pressure indicator
  const memoryPressure = $derived(() => {
    if (!processStats?.memory?.memory_pressure) return 'unknown';
    const pressure = processStats.memory.memory_pressure.toLowerCase();
    
    return {
      status: pressure,
      color: pressure === 'low' ? 'green' : pressure === 'moderate' ? 'yellow' : 'red',
      icon: pressure === 'low' ? '✅' : pressure === 'moderate' ? '⚠️' : '🚨',
    };
  });
  
  // Format uptime
  function formatUptime(uptime) {
    if (!uptime) return 'Unknown';
    return uptime;
  }
  
  // Get top models by request count
  const topModels = $derived(() => {
    if (!modelStats || Object.keys(modelStats).length === 0) return [];
    
    return Object.entries(modelStats)
      .map(([model, stats]) => ({
        name: model,
        requests: stats.request_count || 0,
        avgLatency: stats.avg_latency_ms || 0,
        errors: stats.error_count || 0,
      }))
      .sort((a, b) => b.requests - a.requests)
      .slice(0, 5);
  });
</script>

<div class="performance-analytics">
  <div class="analytics-header">
    <h3 class="text-lg font-semibold text-primary">Performance Analytics</h3>
    
    <!-- Tab Navigation -->
    <div class="tab-nav">
      <button 
        class="tab-button"
        class:active={activeTab === 'system'}
        onclick={() => activeTab = 'system'}
      >
        System
      </button>
      <button 
        class="tab-button"
        class:active={activeTab === 'memory'}
        onclick={() => activeTab = 'memory'}
      >
        Memory
      </button>
      <button 
        class="tab-button"
        class:active={activeTab === 'models'}
        onclick={() => activeTab = 'models'}
      >
        Models
      </button>
    </div>
  </div>
  
  <div class="analytics-content">
    <!-- System Performance Tab -->
    {#if activeTab === 'system'}
      <div class="tab-content">
        <!-- Key Metrics -->
        <div class="metrics-row">
          <div class="metric-box">
            <div class="metric-header">
              <span class="metric-icon">⚡</span>
              <span class="metric-title">Avg Latency</span>
            </div>
            <div class="metric-value">{performanceMetrics().avgLatency}ms</div>
            <div class="metric-trend">
              <span class="trend-icon">📊</span>
              <span class="trend-text">Real-time</span>
            </div>
          </div>
          
          <div class="metric-box">
            <div class="metric-header">
              <span class="metric-icon">✅</span>
              <span class="metric-title">Success Rate</span>
            </div>
            <div class="metric-value">{performanceMetrics().successRate}%</div>
            <div class="metric-trend success">
              <span class="trend-icon">📈</span>
              <span class="trend-text">Healthy</span>
            </div>
          </div>
          
          <div class="metric-box">
            <div class="metric-header">
              <span class="metric-icon">📊</span>
              <span class="metric-title">Throughput</span>
            </div>
            <div class="metric-value">{performanceMetrics().throughput}</div>
            <div class="metric-subtitle">requests/min</div>
          </div>
          
          <div class="metric-box">
            <div class="metric-header">
              <span class="metric-icon">❌</span>
              <span class="metric-title">Error Rate</span>
            </div>
            <div class="metric-value">{performanceMetrics().errorRate}%</div>
            <div class="metric-trend" class:error={performanceMetrics().errorRate > 5}>
              <span class="trend-icon">{performanceMetrics().errorRate > 5 ? '📉' : '✅'}</span>
              <span class="trend-text">{performanceMetrics().errorRate > 5 ? 'High' : 'Low'}</span>
            </div>
          </div>
        </div>
        
        <!-- Runtime Stats -->
        {#if processStats?.runtime}
          <div class="runtime-stats">
            <h4 class="stats-title">Runtime Information</h4>
            <div class="stats-grid">
              <div class="stat-item">
                <span class="stat-label">Uptime</span>
                <span class="stat-value">{formatUptime(processStats.runtime.uptime)}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">Go Version</span>
                <span class="stat-value">{processStats.runtime.go_version}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">CPU Cores</span>
                <span class="stat-value">{processStats.runtime.num_cpu}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">GOMAXPROCS</span>
                <span class="stat-value">{processStats.runtime.gomaxprocs}</span>
              </div>
            </div>
          </div>
        {/if}
      </div>
    {/if}
    
    <!-- Memory Performance Tab -->
    {#if activeTab === 'memory'}
      <div class="tab-content">
        <div class="memory-overview">
          <div class="memory-pressure-indicator pressure-{memoryPressure().color}">
            <span class="pressure-icon">{memoryPressure().icon}</span>
            <div class="pressure-details">
              <span class="pressure-label">Memory Pressure</span>
              <span class="pressure-status">{memoryPressure().status}</span>
            </div>
          </div>
        </div>
        
        {#if processStats?.memory}
          <div class="memory-stats">
            <div class="memory-grid">
              <div class="memory-item">
                <span class="memory-label">Heap Allocated</span>
                <span class="memory-value">{processStats.memory.heap_alloc}</span>
              </div>
              <div class="memory-item">
                <span class="memory-label">Heap System</span>
                <span class="memory-value">{processStats.memory.heap_sys}</span>
              </div>
              <div class="memory-item">
                <span class="memory-label">Heap In Use</span>
                <span class="memory-value">{processStats.memory.heap_inuse}</span>
              </div>
              <div class="memory-item">
                <span class="memory-label">Stack In Use</span>
                <span class="memory-value">{processStats.memory.stack_inuse}</span>
              </div>
              <div class="memory-item">
                <span class="memory-label">Total Allocated</span>
                <span class="memory-value">{processStats.memory.total_alloc}</span>
              </div>
              <div class="memory-item">
                <span class="memory-label">Heap Released</span>
                <span class="memory-value">{processStats.memory.heap_released}</span>
              </div>
            </div>
            
            <!-- GC Stats -->
            {#if processStats.garbage_collection}
              <div class="gc-stats">
                <h4 class="stats-title">Garbage Collection</h4>
                <div class="stats-grid">
                  <div class="stat-item">
                    <span class="stat-label">Last GC</span>
                    <span class="stat-value">{new Date(processStats.garbage_collection.last_gc).toLocaleTimeString()}</span>
                  </div>
                  <div class="stat-item">
                    <span class="stat-label">GC Cycles</span>
                    <span class="stat-value">{processStats.garbage_collection.num_gc_cycles}</span>
                  </div>
                  <div class="stat-item">
                    <span class="stat-label">Total GC Time</span>
                    <span class="stat-value">{processStats.garbage_collection.total_gc_time}</span>
                  </div>
                  <div class="stat-item">
                    <span class="stat-label">GC CPU %</span>
                    <span class="stat-value">{(processStats.garbage_collection.gc_cpu_fraction * 100).toFixed(2)}%</span>
                  </div>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
    
    <!-- Model Performance Tab -->
    {#if activeTab === 'models'}
      <div class="tab-content">
        {#if topModels().length === 0}
          <div class="empty-models">
            <span class="empty-icon">📊</span>
            <p class="empty-text">No model statistics available yet</p>
          </div>
        {:else}
          <div class="models-performance">
            <h4 class="stats-title">Top Models by Request Count</h4>
            <div class="models-table">
              <div class="table-header">
                <span class="col-model">Model</span>
                <span class="col-requests">Requests</span>
                <span class="col-latency">Avg Latency</span>
                <span class="col-errors">Errors</span>
              </div>
              {#each topModels() as model}
                <div class="table-row">
                  <span class="col-model">{model.name}</span>
                  <span class="col-requests">{model.requests.toLocaleString()}</span>
                  <span class="col-latency">{model.avgLatency}ms</span>
                  <span class="col-errors" class:has-errors={model.errors > 0}>
                    {model.errors}
                  </span>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .performance-analytics {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    border: 1px solid var(--bg-tertiary);
    overflow: hidden;
    border-color: var(--bg-tertiary);
  }
  
  .analytics-header {
    padding-left: 1.5rem; padding-right: 1.5rem;
    padding-top: 1rem; padding-bottom: 1rem;
    border-bottom: 1px solid var(--bg-tertiary);
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-color: var(--bg-tertiary);
  }
  
  .tab-nav {
    display: flex;
    gap: 0.5rem;
  }
  
  .tab-button {
    padding-left: 1rem; padding-right: 1rem;
    padding-top: 0.5rem; padding-bottom: 0.5rem;
    border-radius: 0.5rem;
    font-size: 0.875rem; line-height: 1.25rem;
    font-weight: 500;
    transition: all;
    transition-duration: 200ms;
    color: var(--text-secondary);
  }
  
  .tab-button:hover {
    background-color: var(--bg-tertiary);
  }
  
  .tab-button.active {
    background-color: var(--bg-tertiary);
    color: var(--color-blue);
  }
  
  .analytics-content {
    padding: 1.5rem;
  }
  
  .tab-content {
    > * + * { margin-top: 1.5rem; }
  }
  
  .metrics-row {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 1024px) {
    .metrics-row {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }
  
  .metric-box {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .metric-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  
  .metric-icon {
    font-size: 1.125rem; line-height: 1.75rem;
  }
  
  .metric-title {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .metric-value {
    font-size: 1.5rem; line-height: 2rem;
    font-weight: 700;
    color: var(--text-primary);
  }
  
  .metric-subtitle {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .metric-trend {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    margin-top: 0.5rem;
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
  }
  
  .metric-trend.success {
    color: var(--color-green);
  }
  
  .metric-trend.error {
    color: var(--color-red);
  }
  
  .runtime-stats, .gc-stats { /* placeholder */ }
  
  .stats-title {
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 0.75rem;
  }
  
  .stats-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 1024px) {
    .stats-grid {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }
  
  .stat-item {
    padding: 0.75rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .stat-label {
    display: block;
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
    margin-bottom: 0.25rem;
  }
  
  .stat-value {
    font-size: 0.875rem; line-height: 1.25rem;
    font-weight: 500;
    color: var(--text-primary);
  }
  
  .memory-overview {
    margin-bottom: 1.5rem;
  }
  
  .memory-pressure-indicator {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .pressure-green {
    border-color: var(--color-green);
  }
  
  .pressure-yellow {
    border-color: var(--color-yellow);
  }
  
  .pressure-red {
    border-color: var(--color-red);
  }
  
  .pressure-icon {
    font-size: 1.5rem; line-height: 2rem;
  }
  
  .pressure-details {
    flex: 1 1 0%;
  }
  
  .pressure-label {
    display: block;
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .pressure-status {
    font-size: 1.125rem; line-height: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
    text-transform: capitalize;
  }
  
  .memory-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 1024px) {
    .memory-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
  
  .memory-item {
    padding: 0.75rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .memory-label {
    display: block;
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
    margin-bottom: 0.25rem;
  }
  
  .memory-value {
    font-size: 0.875rem; line-height: 1.25rem;
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
    font-weight: 500;
    color: var(--text-primary);
  }
  
  .empty-models {
    text-align: center;
    padding-top: 3rem; padding-bottom: 3rem;
  }
  
  .empty-icon {
    font-size: 1.875rem; line-height: 2.25rem;
    display: block;
    margin-bottom: 0.5rem;
  }
  
  .empty-text {
    color: var(--text-secondary);
  }
  
  .models-table {
    > * + * { margin-top: 0.5rem; }
  }
  
  .table-header {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1rem;
    padding-left: 1rem; padding-right: 1rem;
    padding-top: 0.5rem; padding-bottom: 0.5rem;
    font-size: 0.875rem; line-height: 1.25rem;
    font-weight: 500;
    color: var(--text-muted);
  }
  
  .table-row {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1rem;
    padding-left: 1rem; padding-right: 1rem;
    padding-top: 0.75rem; padding-bottom: 0.75rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .col-model {
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  
  .col-requests, .col-latency, .col-errors {
    text-align: right;
  }
  
  .col-errors.has-errors {
    color: var(--color-red);
  }
</style>