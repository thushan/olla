<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const unifiedModels = $derived(dashboardStore.unifiedModels);
  const endpoints = $derived(dashboardStore.endpoints);
  const loading = $derived(dashboardStore.loading.unifiedModels);
  
  // Group models by family
  const modelFamilies = $derived(() => {
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

<div class="model-universe">
  <div class="universe-header">
    <h3 class="text-lg font-semibold text-primary">Model Universe</h3>
    <div class="header-stats">
      <span class="stat-badge">
        <span class="stat-icon">🌌</span>
        {modelFamilies().length} Families
      </span>
      <span class="stat-badge">
        <span class="stat-icon">🤖</span>
        {unifiedModels.length} Models
      </span>
    </div>
  </div>
  
  {#if loading}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <p class="loading-text">Discovering models across endpoints...</p>
    </div>
  {:else if modelFamilies().length === 0}
    <div class="empty-state">
      <div class="empty-icon">🔭</div>
      <p class="empty-text">No models discovered yet</p>
      <p class="empty-subtext">Models will appear here as endpoints come online</p>
    </div>
  {:else}
    <div class="universe-content">
      <!-- Family Grid -->
      <div class="family-grid">
        {#each modelFamilies() as family}
          <button 
            class="family-card"
            class:selected={selectedFamily === family}
            onclick={() => selectedFamily = selectedFamily === family ? null : family}
          >
            <div class="family-header">
              <h4 class="family-name">{family.name}</h4>
              <span class="family-count">{family.models.length}</span>
            </div>
            
            <div class="family-stats">
              <div class="family-stat">
                <span class="stat-label">Total Size</span>
                <span class="stat-value">{formatSize(family.totalSize)}</span>
              </div>
              <div class="family-stat">
                <span class="stat-label">Endpoints</span>
                <span class="stat-value">{family.endpointCount.size}</span>
              </div>
            </div>
            
            <div class="family-models-preview">
              {#each family.models.slice(0, 3) as model}
                <span class="model-preview">{model.name}</span>
              {/each}
              {#if family.models.length > 3}
                <span class="model-more">+{family.models.length - 3} more</span>
              {/if}
            </div>
          </button>
        {/each}
      </div>
      
      <!-- Selected Family Details -->
      {#if selectedFamily}
        <div class="family-details">
          <div class="details-header">
            <h4 class="details-title">{selectedFamily.name} Models</h4>
            <button 
              class="close-button"
              onclick={() => selectedFamily = null}
            >
              ✕
            </button>
          </div>
          
          <div class="models-list">
            {#each selectedFamily.models as model}
              {@const badge = getModelTypeBadge(model)}
              <div class="model-item">
                <div class="model-header">
                  <div class="model-name-row">
                    <span class="model-type-icon" title={badge.label}>
                      {badge.icon}
                    </span>
                    <h5 class="model-name">{model.name}</h5>
                  </div>
                  <span class="model-size">{formatSize(model.size)}</span>
                </div>
                
                {#if model.description}
                  <p class="model-description">{model.description}</p>
                {/if}
                
                <div class="model-metadata">
                  {#if model.parameter_size}
                    <span class="metadata-tag">📊 {model.parameter_size}</span>
                  {/if}
                  {#if model.quantization}
                    <span class="metadata-tag">🔢 {model.quantization}</span>
                  {/if}
                  {#if model.max_context_length}
                    <span class="metadata-tag">📏 {model.max_context_length} tokens</span>
                  {/if}
                </div>
                
                {#if model.available_at && model.available_at.length > 0}
                  <div class="model-endpoints">
                    <span class="endpoints-label">Available at:</span>
                    <div class="endpoints-list">
                      {#each model.available_at as endpoint}
                        <span class="endpoint-tag">
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

<style>
  .model-universe {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    border: 1px solid var(--bg-tertiary);
    overflow: hidden;
    border-color: var(--bg-tertiary);
  }
  
  .universe-header {
    padding-left: 1.5rem; padding-right: 1.5rem;
    padding-top: 1rem; padding-bottom: 1rem;
    border-bottom: 1px solid var(--bg-tertiary);
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-color: var(--bg-tertiary);
  }
  
  .header-stats {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  
  .stat-badge {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .stat-icon {
    font-size: 1rem; line-height: 1.5rem;
  }
  
  .loading-state, .empty-state {
    padding-top: 4rem; padding-bottom: 4rem;
    text-align: center;
  }
  
  .loading-spinner {
    width: 2rem;
    height: 2rem;
    margin-left: auto; margin-right: auto;
    margin-bottom: 1rem;
    border: 2px solid var(--bg-tertiary);
    border-radius: 9999px;
    animation: spin 1s linear infinite;
    border-color: var(--color-blue);
    border-top-color: transparent;
  }
  
  .loading-text {
    color: var(--text-secondary);
  }
  
  .empty-icon {
    font-size: 2.25rem; line-height: 2.5rem;
    margin-bottom: 0.75rem;
  }
  
  .empty-text {
    color: var(--text-primary);
    font-weight: 500;
    margin-bottom: 0.25rem;
  }
  
  .empty-subtext {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .universe-content {
    padding: 1.5rem;
  }
  
  .family-grid {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 768px) {
    .family-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  
  @media (min-width: 1024px) {
    .family-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
  
  .family-card {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    text-align: left;
    transition: all;
    transition-duration: 200ms;

    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .family-card:hover {
    border-color: var(--color-blue);
  }
  
  .family-card.selected {

    ring-color: var(--color-blue);
    border-color: var(--color-blue);
  }
  
  .family-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.75rem;
  }
  
  .family-name {
    font-weight: 600;
    color: var(--text-primary);
  }
  
  .family-count {
    font-size: 0.875rem; line-height: 1.25rem;
    padding-left: 0.5rem; padding-right: 0.5rem;
    padding-top: 0.25rem; padding-bottom: 0.25rem;
    border-radius: 9999px;
    background-color: var(--bg-tertiary);
    color: var(--text-secondary);
  }
  
  .family-stats {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
    margin-bottom: 0.75rem;
  }
  
  .family-stat {
    font-size: 0.875rem; line-height: 1.25rem;
  }
  
  .stat-label {
    display: block;
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
    margin-bottom: 0.25rem;
  }
  
  .stat-value {
    font-weight: 500;
    color: var(--text-secondary);
  }
  
  .family-models-preview {
    display: flex;
    gap: 0.5rem;
  }
  
  .model-preview {
    font-size: 0.75rem; line-height: 1rem;
    padding-left: 0.5rem; padding-right: 0.5rem;
    padding-top: 0.25rem; padding-bottom: 0.25rem;
    border-radius: 0.25rem;
    background-color: var(--bg-secondary);
    color: var(--text-secondary);
  }
  
  .model-more {
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
  }
  
  .family-details {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .details-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
  }
  
  .details-title {
    font-size: 1.125rem; line-height: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  
  .close-button {
    width: 2rem;
    height: 2rem;
    border-radius: 0.5rem;
    display: flex;
    align-items: center;
    justify-content: center;
    transition-property: background-color, border-color, color, fill, stroke;
    transition-duration: 200ms;
    color: var(--text-secondary);
  }
  
  .models-list {
    > * + * { margin-top: 1rem; }
  }
  
  .model-item {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-secondary);
    border-color: var(--bg-tertiary);
  }
  
  .model-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  
  .model-name-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .model-type-icon {
    font-size: 1.125rem; line-height: 1.75rem;
  }
  
  .model-name {
    font-weight: 500;
    color: var(--text-primary);
  }
  
  .model-size {
    font-size: 0.875rem; line-height: 1.25rem;
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
    color: var(--text-secondary);
  }
  
  .model-description {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
    margin-bottom: 0.75rem;
  }
  
  .model-metadata {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  
  .metadata-tag {
    font-size: 0.75rem; line-height: 1rem;
    padding-left: 0.5rem; padding-right: 0.5rem;
    padding-top: 0.25rem; padding-bottom: 0.25rem;
    border-radius: 0.25rem;
    background-color: var(--bg-primary);
    color: var(--text-secondary);
  }
  
  .model-endpoints {
    font-size: 0.875rem; line-height: 1.25rem;
  }
  
  .endpoints-label {
    color: var(--text-muted);
    margin-bottom: 0.25rem;
    display: block;
  }
  
  .endpoints-list {
    display: flex;
    gap: 0.5rem;
  }
  
  .endpoint-tag {
    font-size: 0.75rem; line-height: 1rem;
    padding-left: 0.5rem; padding-right: 0.5rem;
    padding-top: 0.25rem; padding-bottom: 0.25rem;
    border-radius: 0.25rem;
    background-color: var(--bg-primary);
    color: var(--color-blue);
  }
</style>