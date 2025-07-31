<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const version = $derived(dashboardStore.version);
  const status = $derived(dashboardStore.status);
  
  // Mock configuration data (would come from actual config in production)
  const configuration = {
    server: {
      host: '0.0.0.0',
      port: 40114,
      readTimeout: '30s',
      writeTimeout: '0s',
      requestLogging: true,
    },
    proxy: {
      engine: 'olla',
      loadBalancer: 'priority',
      responseTimeout: '900s',
      streamBufferSize: '64KB',
      maxIdleConns: 100,
      maxConnsPerHost: 50,
    },
    security: {
      rateLimiting: true,
      globalLimit: '1000/min',
      perIPLimit: '100/min',
      maxBodySize: '50MB',
      maxHeaderSize: '1MB',
      trustProxy: true,
    },
    discovery: {
      modelDiscovery: true,
      interval: '5m',
      timeout: '30s',
      workers: 5,
    },
  };
  
  // Expanded sections
  let expandedSections = $state({
    server: true,
    proxy: false,
    security: false,
    discovery: false,
  });
  
  // Toggle section
  function toggleSection(section) {
    expandedSections[section] = !expandedSections[section];
  }
  
  // Get status indicator
  function getStatusIndicator(key, value) {
    // Special cases for certain configurations
    if (key === 'writeTimeout' && value === '0s') {
      return { color: 'green', icon: '✅', hint: 'Recommended for streaming' };
    }
    if (key === 'engine' && value === 'olla') {
      return { color: 'blue', icon: '⚡', hint: 'High performance mode' };
    }
    if (key === 'rateLimiting' && value === true) {
      return { color: 'green', icon: '🛡️', hint: 'Security enabled' };
    }
    if (key === 'trustProxy' && value === true) {
      return { color: 'yellow', icon: '⚠️', hint: 'Behind reverse proxy' };
    }
    return null;
  }
  
  onMount(() => {
    // Fetch version if not loaded
    if (!version) {
      dashboardStore.fetchVersion();
    }
  });
</script>

<div class="configuration-status">
  <div class="config-header">
    <h3 class="text-lg font-semibold text-primary">Configuration Status</h3>
  </div>
  
  <!-- Version Info -->
  {#if version}
    <div class="version-info">
      <div class="version-label">Olla Version</div>
      <div class="version-value">{version.version}</div>
      <div class="version-meta">
        <span class="meta-item">
          <span class="meta-icon">🛠️</span>
          {version.commit?.slice(0, 7) || 'unknown'}
        </span>
        <span class="meta-item">
          <span class="meta-icon">📅</span>
          {new Date(version.date).toLocaleDateString()}
        </span>
      </div>
    </div>
  {/if}
  
  <!-- Configuration Sections -->
  <div class="config-sections">
    <!-- Server Configuration -->
    <div class="config-section">
      <button 
        class="section-header"
        onclick={() => toggleSection('server')}
      >
        <div class="header-content">
          <span class="section-icon">🌐</span>
          <span class="section-title">Server</span>
        </div>
        <span class="expand-icon" class:expanded={expandedSections.server}>
          ▶
        </span>
      </button>
      
      {#if expandedSections.server}
        <div class="section-content">
          {#each Object.entries(configuration.server) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="config-item">
              <span class="config-key">{key}</span>
              <div class="config-value-row">
                <span class="config-value">{value}</span>
                {#if status}
                  <span 
                    class="status-indicator status-{status.color}"
                    title={status.hint}
                  >
                    {status.icon}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
    
    <!-- Proxy Configuration -->
    <div class="config-section">
      <button 
        class="section-header"
        onclick={() => toggleSection('proxy')}
      >
        <div class="header-content">
          <span class="section-icon">🔄</span>
          <span class="section-title">Proxy</span>
        </div>
        <span class="expand-icon" class:expanded={expandedSections.proxy}>
          ▶
        </span>
      </button>
      
      {#if expandedSections.proxy}
        <div class="section-content">
          {#each Object.entries(configuration.proxy) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="config-item">
              <span class="config-key">{key}</span>
              <div class="config-value-row">
                <span class="config-value">{value}</span>
                {#if status}
                  <span 
                    class="status-indicator status-{status.color}"
                    title={status.hint}
                  >
                    {status.icon}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
    
    <!-- Security Configuration -->
    <div class="config-section">
      <button 
        class="section-header"
        onclick={() => toggleSection('security')}
      >
        <div class="header-content">
          <span class="section-icon">🛡️</span>
          <span class="section-title">Security</span>
        </div>
        <span class="expand-icon" class:expanded={expandedSections.security}>
          ▶
        </span>
      </button>
      
      {#if expandedSections.security}
        <div class="section-content">
          {#each Object.entries(configuration.security) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="config-item">
              <span class="config-key">{key}</span>
              <div class="config-value-row">
                <span class="config-value">{value}</span>
                {#if status}
                  <span 
                    class="status-indicator status-{status.color}"
                    title={status.hint}
                  >
                    {status.icon}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
    
    <!-- Discovery Configuration -->
    <div class="config-section">
      <button 
        class="section-header"
        onclick={() => toggleSection('discovery')}
      >
        <div class="header-content">
          <span class="section-icon">🔍</span>
          <span class="section-title">Discovery</span>
        </div>
        <span class="expand-icon" class:expanded={expandedSections.discovery}>
          ▶
        </span>
      </button>
      
      {#if expandedSections.discovery}
        <div class="section-content">
          {#each Object.entries(configuration.discovery) as [key, value]}
            <div class="config-item">
              <span class="config-key">{key}</span>
              <span class="config-value">{value}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
  
  <!-- Quick Actions -->
  <div class="quick-actions">
    <h4 class="actions-title">Quick Actions</h4>
    <div class="actions-grid">
      <a href="/internal/status" target="_blank" class="action-button">
        <span class="action-icon">📊</span>
        <span class="action-text">View Full Status</span>
      </a>
      <a href="/internal/health" target="_blank" class="action-button">
        <span class="action-icon">💚</span>
        <span class="action-text">Health Check</span>
      </a>
      <a href="/olla/models" target="_blank" class="action-button">
        <span class="action-icon">🤖</span>
        <span class="action-text">Model List</span>
      </a>
      <a href="https://github.com/thushan/olla" target="_blank" rel="noopener" class="action-button">
        <span class="action-icon">📚</span>
        <span class="action-text">Documentation</span>
      </a>
    </div>
  </div>
</div>

<style>
  .configuration-status {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--bg-tertiary);
    overflow: hidden;
  }
  
  .config-header {
    padding-left: 1.5rem;
    padding-right: 1.5rem;
    padding-top: 1rem;
    padding-bottom: 1rem;
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: var(--bg-tertiary);
  }
  
  .version-info {
    padding: 1.5rem;
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: var(--bg-tertiary);
  }
  
  .version-label {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-muted);
    margin-bottom: 0.25rem;
  }
  
  .version-value {
    font-size: 1.25rem;
    line-height: 1.75rem;
    font-weight: 700;
    color: var(--text-primary);
    margin-bottom: 0.5rem;
  }
  
  .version-meta {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.75rem;
    line-height: 1rem;
  }
  
  .meta-item {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--text-secondary);
  }
  
  .meta-icon {
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  
  .config-sections > * + * {
    border-top-width: 1px;
    border-top-style: solid;
    border-top-color: var(--bg-tertiary);
  }
  
  .config-section {
    overflow: hidden;
  }
  
  .section-header {
    width: 100%;
    padding-left: 1.5rem;
    padding-right: 1.5rem;
    padding-top: 1rem;
    padding-bottom: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    transition-property: background-color, border-color, color, fill, stroke;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
  }
  
  .section-header:hover {
    background-color: var(--bg-tertiary);
  }
  
  .header-content {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  
  .section-icon {
    font-size: 1.125rem;
    line-height: 1.75rem;
  }
  
  .section-title {
    font-weight: 500;
    color: var(--text-primary);
  }
  
  .expand-icon {
    font-size: 0.875rem;
    line-height: 1.25rem;
    transition-property: transform;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
    color: var(--text-secondary);
  }
  
  .expand-icon.expanded {
    transform: rotate(90deg);
  }
  
  .section-content {
    padding-left: 1.5rem;
    padding-right: 1.5rem;
    padding-bottom: 1rem;
  }
  
  .section-content > * + * {
    margin-top: 0.75rem;
  }
  
  .config-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 0.5rem;
    padding-bottom: 0.5rem;
  }
  
  .config-key {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .config-value-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .config-value {
    font-size: 0.875rem;
    line-height: 1.25rem;
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
    color: var(--text-primary);
  }
  
  .status-indicator {
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  
  .status-green {
    color: var(--color-green);
  }
  
  .status-blue {
    color: var(--color-blue);
  }
  
  .status-yellow {
    color: var(--color-yellow);
  }
  
  .quick-actions {
    padding: 1.5rem;
  }
  
  .actions-title {
    font-weight: 500;
    color: var(--text-primary);
    margin-bottom: 0.75rem;
  }
  
  .actions-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
  }
  
  .action-button {
    padding: 0.75rem;
    border-radius: 0.5rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--bg-tertiary);
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background-color: var(--bg-primary);
    text-decoration: none;
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 200ms;
  }
  
  .action-button:hover {
    box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
  }
  
  .action-button:hover {
    border-color: var(--color-blue);
  }
  
  .action-icon {
    font-size: 1.125rem;
    line-height: 1.75rem;
  }
  
  .action-text {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-primary);
  }
</style>