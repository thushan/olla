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

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Configuration Status</h3>
  </div>
  
  <!-- Version Info -->
  {#if version}
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Olla Version</div>
      <div class="text-xl font-bold text-gray-900 dark:text-white mb-2">{version.version}</div>
      <div class="flex items-center gap-3 text-xs">
        <span class="flex items-center gap-1 text-gray-600 dark:text-gray-400">
          <span class="text-sm">🛠️</span>
          {version.commit?.slice(0, 7) || 'unknown'}
        </span>
        <span class="flex items-center gap-1 text-gray-600 dark:text-gray-400">
          <span class="text-sm">📅</span>
          {new Date(version.date).toLocaleDateString()}
        </span>
      </div>
    </div>
  {/if}
  
  <!-- Configuration Sections -->
  <div>
    <!-- Server Configuration -->
    <div class="overflow-hidden">
      <button 
        class="w-full px-6 py-4 flex items-center justify-between transition-colors duration-200 hover:bg-gray-50 dark:hover:bg-gray-700 border-b border-gray-200 dark:border-gray-700"
        onclick={() => toggleSection('server')}
      >
        <div class="flex items-center gap-3">
          <span class="text-lg">🌐</span>
          <span class="font-medium text-gray-900 dark:text-white">Server</span>
        </div>
        <span class="text-sm text-gray-600 dark:text-gray-400 transition-transform duration-200 {expandedSections.server ? 'rotate-90' : ''}">
          ▶
        </span>
      </button>
      
      {#if expandedSections.server}
        <div class="px-6 pb-4 space-y-3">
          {#each Object.entries(configuration.server) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="flex items-center justify-between py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{key}</span>
              <div class="flex items-center gap-2">
                <span class="text-sm font-mono text-gray-900 dark:text-white">{value}</span>
                {#if status}
                  <span 
                    class="text-sm {status.color === 'green' ? 'text-green-600 dark:text-green-400' : status.color === 'blue' ? 'text-blue-600 dark:text-blue-400' : status.color === 'yellow' ? 'text-yellow-600 dark:text-yellow-400' : 'text-gray-600 dark:text-gray-400'}"
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
    <div class="overflow-hidden">
      <button 
        class="w-full px-6 py-4 flex items-center justify-between transition-colors duration-200 hover:bg-gray-50 dark:hover:bg-gray-700 border-b border-gray-200 dark:border-gray-700"
        onclick={() => toggleSection('proxy')}
      >
        <div class="flex items-center gap-3">
          <span class="text-lg">🔄</span>
          <span class="font-medium text-gray-900 dark:text-white">Proxy</span>
        </div>
        <span class="text-sm text-gray-600 dark:text-gray-400 transition-transform duration-200 {expandedSections.proxy ? 'rotate-90' : ''}">
          ▶
        </span>
      </button>
      
      {#if expandedSections.proxy}
        <div class="px-6 pb-4 space-y-3">
          {#each Object.entries(configuration.proxy) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="flex items-center justify-between py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{key}</span>
              <div class="flex items-center gap-2">
                <span class="text-sm font-mono text-gray-900 dark:text-white">{value}</span>
                {#if status}
                  <span 
                    class="text-sm {status.color === 'green' ? 'text-green-600 dark:text-green-400' : status.color === 'blue' ? 'text-blue-600 dark:text-blue-400' : status.color === 'yellow' ? 'text-yellow-600 dark:text-yellow-400' : 'text-gray-600 dark:text-gray-400'}"
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
    <div class="overflow-hidden">
      <button 
        class="w-full px-6 py-4 flex items-center justify-between transition-colors duration-200 hover:bg-gray-50 dark:hover:bg-gray-700 border-b border-gray-200 dark:border-gray-700"
        onclick={() => toggleSection('security')}
      >
        <div class="flex items-center gap-3">
          <span class="text-lg">🛡️</span>
          <span class="font-medium text-gray-900 dark:text-white">Security</span>
        </div>
        <span class="text-sm text-gray-600 dark:text-gray-400 transition-transform duration-200 {expandedSections.security ? 'rotate-90' : ''}">
          ▶
        </span>
      </button>
      
      {#if expandedSections.security}
        <div class="px-6 pb-4 space-y-3">
          {#each Object.entries(configuration.security) as [key, value]}
            {@const status = getStatusIndicator(key, value)}
            <div class="flex items-center justify-between py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{key}</span>
              <div class="flex items-center gap-2">
                <span class="text-sm font-mono text-gray-900 dark:text-white">{value}</span>
                {#if status}
                  <span 
                    class="text-sm {status.color === 'green' ? 'text-green-600 dark:text-green-400' : status.color === 'blue' ? 'text-blue-600 dark:text-blue-400' : status.color === 'yellow' ? 'text-yellow-600 dark:text-yellow-400' : 'text-gray-600 dark:text-gray-400'}"
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
    <div class="overflow-hidden">
      <button 
        class="w-full px-6 py-4 flex items-center justify-between transition-colors duration-200 hover:bg-gray-50 dark:hover:bg-gray-700 border-b border-gray-200 dark:border-gray-700"
        onclick={() => toggleSection('discovery')}
      >
        <div class="flex items-center gap-3">
          <span class="text-lg">🔍</span>
          <span class="font-medium text-gray-900 dark:text-white">Discovery</span>
        </div>
        <span class="text-sm text-gray-600 dark:text-gray-400 transition-transform duration-200 {expandedSections.discovery ? 'rotate-90' : ''}">
          ▶
        </span>
      </button>
      
      {#if expandedSections.discovery}
        <div class="px-6 pb-4 space-y-3">
          {#each Object.entries(configuration.discovery) as [key, value]}
            <div class="flex items-center justify-between py-2">
              <span class="text-sm text-gray-600 dark:text-gray-400">{key}</span>
              <span class="text-sm font-mono text-gray-900 dark:text-white">{value}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
  
  <!-- Quick Actions -->
  <div class="p-6">
    <h4 class="font-medium text-gray-900 dark:text-white mb-3">Quick Actions</h4>
    <div class="grid grid-cols-2 gap-3">
      <a href="/internal/status" target="_blank" class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-2 bg-white dark:bg-gray-800 text-decoration-none transition-all duration-200 hover:shadow-sm hover:border-blue-300 dark:hover:border-blue-600">
        <span class="text-lg">📊</span>
        <span class="text-sm text-gray-900 dark:text-white">View Full Status</span>
      </a>
      <a href="/internal/health" target="_blank" class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-2 bg-white dark:bg-gray-800 text-decoration-none transition-all duration-200 hover:shadow-sm hover:border-blue-300 dark:hover:border-blue-600">
        <span class="text-lg">💚</span>
        <span class="text-sm text-gray-900 dark:text-white">Health Check</span>
      </a>
      <a href="/olla/models" target="_blank" class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-2 bg-white dark:bg-gray-800 text-decoration-none transition-all duration-200 hover:shadow-sm hover:border-blue-300 dark:hover:border-blue-600">
        <span class="text-lg">🤖</span>
        <span class="text-sm text-gray-900 dark:text-white">Model List</span>
      </a>
      <a href="https://github.com/thushan/olla" target="_blank" rel="noopener" class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-2 bg-white dark:bg-gray-800 text-decoration-none transition-all duration-200 hover:shadow-sm hover:border-blue-300 dark:hover:border-blue-600">
        <span class="text-lg">📚</span>
        <span class="text-sm text-gray-900 dark:text-white">Documentation</span>
      </a>
    </div>
  </div>
</div>

