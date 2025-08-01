<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const unifiedModels = $derived(dashboardStore.unifiedModels);
  const endpoints = $derived(dashboardStore.endpoints);
  const loading = $derived(dashboardStore.loading.unifiedModels);
  
  // Group models by family
  const modelFamilies = $derived.by(() => {
    const families = new Map();
    
    for (const model of unifiedModels) {
      const family = model.family || 'Unknown';
      if (!families.has(family)) {
        families.set(family, {
          name: family,
          models: [],
          totalSize: 0,
          endpointCount: new Set(),
        });
      }
      
      const familyData = families.get(family);
      familyData.models.push(model);
      familyData.totalSize += model.size || 0;
      
      // Count unique endpoints
      if (model.available_at) {
        model.available_at.forEach(ep => familyData.endpointCount.add(ep));
      }
    }
    
    return Array.from(families.values())
      .sort((a, b) => b.models.length - a.models.length);
  });
  
  // Selected family for details
  let selectedFamily = $state(null);
  
  // Format model size
  function formatSize(bytes) {
    if (!bytes) return 'Unknown';
    const gb = bytes / (1024 * 1024 * 1024);
    return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
  }
  
  // Get endpoint name from URL
  function getEndpointName(url) {
    const endpoint = endpoints.find(ep => ep.url === url);
    return endpoint ? endpoint.name : url;
  }
  
  // Get model type badge
  function getModelTypeBadge(model) {
    if (model.type === 'chat') return { icon: '💬', label: 'Chat' };
    if (model.type === 'embedding') return { icon: '🔤', label: 'Embedding' };
    if (model.type === 'vlm') return { icon: '👁️', label: 'Vision' };
    return { icon: '🤖', label: 'LLM' };
  }
  
  onMount(() => {
    // Fetch models if not already loaded
    if (unifiedModels.length === 0) {
      dashboardStore.fetchUnifiedModels();
    }
  });
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="flex items-center justify-between p-6 border-b border-gray-200 dark:border-gray-700">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Model Universe</h3>
    <div class="flex items-center gap-3">
      <span class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        <span class="text-base">🌌</span>
        {modelFamilies.length} Families
      </span>
      <span class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        <span class="text-base">🤖</span>
        {unifiedModels.length} Models
      </span>
    </div>
  </div>
  
  {#if loading}
    <div class="py-16 text-center">
      <div class="w-8 h-8 mx-auto mb-4 border-2 border-gray-300 dark:border-gray-600 border-t-blue-500 rounded-full animate-spin"></div>
      <p class="text-gray-600 dark:text-gray-400">Discovering models across endpoints...</p>
    </div>
  {:else if modelFamilies.length === 0}
    <div class="py-16 text-center">
      <div class="text-4xl mb-3">🔭</div>
      <p class="text-gray-900 dark:text-white font-medium mb-1">No models discovered yet</p>
      <p class="text-sm text-gray-600 dark:text-gray-400">Models will appear here as endpoints come online</p>
    </div>
  {:else}
    <div class="p-6">
      <!-- Family Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
        {#each modelFamilies as family}
          <button 
            class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 text-left transition-all duration-200 bg-white dark:bg-gray-800 hover:border-blue-300 dark:hover:border-blue-600 {selectedFamily === family ? 'ring-2 ring-blue-500 border-blue-500' : ''}"
            onclick={() => selectedFamily = selectedFamily === family ? null : family}
          >
            <div class="flex items-center justify-between mb-3">
              <h4 class="font-semibold text-gray-900 dark:text-white">{family.name}</h4>
              <span class="text-sm px-2 py-1 rounded-full bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400">{family.models.length}</span>
            </div>
            
            <div class="grid grid-cols-2 gap-3 mb-3">
              <div class="text-sm">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Total Size</span>
                <span class="font-medium text-gray-600 dark:text-gray-300">{formatSize(family.totalSize)}</span>
              </div>
              <div class="text-sm">
                <span class="block text-xs text-gray-500 dark:text-gray-400 mb-1">Endpoints</span>
                <span class="font-medium text-gray-600 dark:text-gray-300">{family.endpointCount.size}</span>
              </div>
            </div>
            
            <div class="flex gap-2">
              {#each family.models.slice(0, 3) as model}
                <span class="text-xs px-2 py-1 rounded bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300">{model.name}</span>
              {/each}
              {#if family.models.length > 3}
                <span class="text-xs text-gray-500 dark:text-gray-400">+{family.models.length - 3} more</span>
              {/if}
            </div>
          </button>
        {/each}
      </div>
      
      <!-- Selected Family Details -->
      {#if selectedFamily}
        <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div class="flex items-center justify-between mb-4">
            <h4 class="text-lg font-semibold text-gray-900 dark:text-white">{selectedFamily.name} Models</h4>
            <button 
              class="w-8 h-8 rounded-lg flex items-center justify-center transition-colors duration-200 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700"
              onclick={() => selectedFamily = null}
            >
              ✕
            </button>
          </div>
          
          <div class="space-y-4">
            {#each selectedFamily.models as model}
              {@const badge = getModelTypeBadge(model)}
              <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
                <div class="flex items-start justify-between mb-2">
                  <div class="flex items-center gap-2">
                    <span class="text-lg" title={badge.label}>
                      {badge.icon}
                    </span>
                    <h5 class="font-medium text-gray-900 dark:text-white">{model.name}</h5>
                  </div>
                  <span class="text-sm font-mono text-gray-600 dark:text-gray-400">{formatSize(model.size)}</span>
                </div>
                
                {#if model.description}
                  <p class="text-sm text-gray-600 dark:text-gray-400 mb-3">{model.description}</p>
                {/if}
                
                <div class="flex gap-2 mb-3">
                  {#if model.parameter_size}
                    <span class="text-xs px-2 py-1 rounded bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300">📊 {model.parameter_size}</span>
                  {/if}
                  {#if model.quantization}
                    <span class="text-xs px-2 py-1 rounded bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300">🔢 {model.quantization}</span>
                  {/if}
                  {#if model.max_context_length}
                    <span class="text-xs px-2 py-1 rounded bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300">📏 {model.max_context_length} tokens</span>
                  {/if}
                </div>
                
                {#if model.available_at && model.available_at.length > 0}
                  <div class="text-sm">
                    <span class="block text-gray-500 dark:text-gray-400 mb-1">Available at:</span>
                    <div class="flex gap-2">
                      {#each model.available_at as endpoint}
                        <span class="text-xs px-2 py-1 rounded bg-white dark:bg-gray-800 text-blue-600 dark:text-blue-400">
                          📍 {getEndpointName(endpoint)}
                        </span>
                      {/each}
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

