<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const models = $derived(dashboardStore.models);
  const modelStats = $derived(dashboardStore.modelStats);
  const unifiedModels = $derived(dashboardStore.unifiedModels);
  const endpoints = $derived(dashboardStore.endpoints);
  
  // Galaxy dimensions
  const galaxySize = 400;
  const center = galaxySize / 2;
  const maxRadius = galaxySize / 2 - 60;
  
  // Selected model for details
  let selectedModel = $state(null);
  let hoveredModel = $state(null);
  
  // Animation state
  let mounted = $state(false);
  let rotationAngle = $state(0);
  
  // Model categories and their colors
  const modelCategories = {
    'llama': { color: '#8B5CF6', icon: '🦙', label: 'Llama' },
    'gpt': { color: '#10B981', icon: '🤖', label: 'GPT' },
    'claude': { color: '#F59E0B', icon: '🧠', label: 'Claude' },
    'mistral': { color: '#EF4444', icon: '⚡', label: 'Mistral' },
    'gemini': { color: '#3B82F6', icon: '💎', label: 'Gemini' },
    'codellama': { color: '#EC4899', icon: '💻', label: 'Code Llama' },
    'default': { color: '#6B7280', icon: '🌟', label: 'Other' },
  };
  
  // Determine model category
  function getModelCategory(modelName) {
    const name = modelName.toLowerCase();
    for (const [category, _] of Object.entries(modelCategories)) {
      if (name.includes(category)) return category;
    }
    return 'default';
  }
  
  // Galaxy nodes - combination of models and endpoints
  const galaxyNodes = $derived.by(() => {
    const nodes = [];
    
    // Add unified models as major nodes
    if (unifiedModels && unifiedModels.length > 0) {
      unifiedModels.forEach((model, index) => {
        const category = getModelCategory(model.name);
        const stats = modelStats[model.name] || {};
        const requestCount = stats.request_count || 0;
        const avgLatency = stats.avg_latency_ms || 0;
        
        // Size based on request count (min: 8, max: 24)
        const size = Math.min(24, Math.max(8, 8 + (requestCount / 100) * 16));
        
        // Position in spiral pattern
        const angle = (index / unifiedModels.length) * 2 * Math.PI + rotationAngle * 0.01;
        const radiusBase = 60 + (index % 3) * 80;
        const radius = radiusBase + Math.sin(rotationAngle * 0.02 + index) * 20;
        
        nodes.push({
          id: `model-${model.name}`,
          name: model.name,
          type: 'model',
          category,
          x: center + Math.cos(angle) * radius,
          y: center + Math.sin(angle) * radius,
          size,
          requestCount,
          avgLatency,
          color: modelCategories[category].color,
          icon: modelCategories[category].icon,
          endpoints: model.endpoints || [],
        });
      });
    }
    
    // Add endpoints as smaller satellite nodes
    if (endpoints && endpoints.length > 0) {
      endpoints.forEach((endpoint, index) => {
        const angle = (index / endpoints.length) * 2 * Math.PI + rotationAngle * 0.005;
        const radius = maxRadius - 30;
        
        nodes.push({
          id: `endpoint-${endpoint.name}`,
          name: endpoint.name,
          type: 'endpoint',
          category: 'endpoint',
          x: center + Math.cos(angle) * radius,
          y: center + Math.sin(angle) * radius,
          size: endpoint.status === 'online' ? 6 : 4,
          color: endpoint.status === 'online' ? '#10B981' : '#EF4444',
          icon: endpoint.status === 'online' ? '🟢' : '🔴',
          status: endpoint.status,
          url: endpoint.url,
        });
      });
    }
    
    return nodes;
  });
  
  // Connection lines between related nodes
  const galaxyConnections = $derived.by(() => {
    const connections = [];
    const nodes = galaxyNodes || [];
    const modelNodes = nodes.filter(n => n.type === 'model');
    const endpointNodes = nodes.filter(n => n.type === 'endpoint');
    
    // Connect models to their endpoints
    modelNodes.forEach(model => {
      if (model.endpoints && model.endpoints.length > 0) {
        model.endpoints.forEach(endpointName => {
          const endpoint = endpointNodes.find(e => e.name === endpointName);
          if (endpoint) {
            connections.push({
              from: model,
              to: endpoint,
              strength: model.requestCount > 50 ? 'strong' : 'weak',
            });
          }
        });
      }
    });
    
    return connections;
  });
  
  // Get model details for selected model
  const modelDetails = $derived.by(() => {
    if (!selectedModel) return null;
    
    const stats = modelStats[selectedModel.name] || {};
    const endpoints = selectedModel.endpoints || [];
    
    return {
      name: selectedModel.name,
      category: modelCategories[selectedModel.category].label,
      requestCount: stats.request_count || 0,
      avgLatency: stats.avg_latency_ms || 0,
      errorCount: stats.error_count || 0,
      successRate: stats.request_count ? 
        ((stats.request_count - (stats.error_count || 0)) / stats.request_count * 100).toFixed(1) : '100',
      endpoints,
      lastUsed: stats.last_request_time || 'Never',
    };
  });
  
  // Format number with units
  function formatNumber(num) {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
  }
  
  // Handle node click
  function handleNodeClick(node) {
    if (node.type === 'model') {
      selectedModel = selectedModel?.id === node.id ? null : node;
    }
  }
  
  // Animation loop
  onMount(() => {
    mounted = true;
    
    const animate = () => {
      rotationAngle += 0.5;
      requestAnimationFrame(animate);
    };
    animate();
  });
</script>

<div class="bg-gradient-to-br from-slate-900 via-purple-900 to-slate-900 rounded-xl shadow-sm border border-gray-700 overflow-hidden text-white">
  <div class="p-6 border-b border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-white flex items-center gap-2">
        <span>🌌</span>
        Model Galaxy
      </h3>
      
      <!-- Legend -->
      <div class="flex items-center gap-4 text-sm">
        <div class="flex items-center gap-2">
          <div class="w-3 h-3 rounded-full bg-purple-500"></div>
          <span class="text-gray-300">Models</span>
        </div>
        <div class="flex items-center gap-2">
          <div class="w-2 h-2 rounded-full bg-green-500"></div>
          <span class="text-gray-300">Endpoints</span>
        </div>
      </div>
    </div>
  </div>
  
  <div class="p-6">
    <div class="flex flex-col lg:flex-row gap-6">
      <!-- Galaxy Visualization -->
      <div class="flex-1 flex justify-center">
        <div class="relative">
          <svg width="{galaxySize}" height="{galaxySize}" class="transition-all duration-1000 {mounted ? 'opacity-100' : 'opacity-0'}">
            <!-- Starfield background -->
            <defs>
              <radialGradient id="galaxyGradient" cx="50%" cy="50%" r="50%">
                <stop offset="0%" stop-color="rgba(139, 92, 246, 0.1)"/>
                <stop offset="50%" stop-color="rgba(59, 130, 246, 0.05)"/>
                <stop offset="100%" stop-color="rgba(0, 0, 0, 0)"/>
              </radialGradient>
              
              <!-- Glow filters -->
              <filter id="modelGlow">
                <feGaussianBlur stdDeviation="4" result="coloredBlur"/>
                <feMerge>
                  <feMergeNode in="coloredBlur"/>
                  <feMergeNode in="SourceGraphic"/>
                </feMerge>
              </filter>
              
              <filter id="endpointGlow">
                <feGaussianBlur stdDeviation="2" result="coloredBlur"/>
                <feMerge>
                  <feMergeNode in="coloredBlur"/>
                  <feMergeNode in="SourceGraphic"/>
                </feMerge>
              </filter>
            </defs>
            
            <!-- Galaxy background -->
            <circle
              cx="{center}"
              cy="{center}"
              r="{maxRadius}"
              fill="url(#galaxyGradient)"
              class="opacity-30"
            />
            
            <!-- Orbital rings -->
            {#each Array.from({length: 3}) as _, i}
              <circle
                cx="{center}"
                cy="{center}"
                r="{60 + i * 80}"
                fill="none"
                stroke="rgba(139, 92, 246, 0.2)"
                stroke-width="1"
                stroke-dasharray="4,8"
                class="opacity-40"
                style="transform-origin: {center}px {center}px; animation: rotate {30 + i * 10}s linear infinite;"
              />
            {/each}
            
            <!-- Connection lines -->
            {#each galaxyConnections as connection}
              <line
                x1="{connection.from.x}"
                y1="{connection.from.y}"
                x2="{connection.to.x}"
                y2="{connection.to.y}"
                stroke="rgba(139, 92, 246, 0.3)"
                stroke-width="{connection.strength === 'strong' ? 2 : 1}"
                class="transition-all duration-300"
                style="opacity: {hoveredModel?.id === connection.from.id || selectedModel?.id === connection.from.id ? 0.8 : 0.2};"
              />
            {/each}
            
            <!-- Nodes -->
            {#each galaxyNodes as node, index}
              <g 
                class="cursor-pointer transition-all duration-300 {node.type === 'model' ? 'hover:scale-110' : 'hover:scale-125'}"
                style="animation: float {3 + (index % 3)}s ease-in-out infinite alternate; animation-delay: {index * 0.2}s;"
                onclick={() => handleNodeClick(node)}
                onmouseenter={() => hoveredModel = node}
                onmouseleave={() => hoveredModel = null}
              >
                <!-- Node glow -->
                <circle
                  cx="{node.x}"
                  cy="{node.y}"
                  r="{node.size + 4}"
                  fill="{node.color}"
                  opacity="0.3"
                  filter="{node.type === 'model' ? 'url(#modelGlow)' : 'url(#endpointGlow)'}"
                  class="transition-all duration-300 {selectedModel?.id === node.id ? 'opacity-60 scale-150' : ''}"
                />
                
                <!-- Main node -->
                <circle
                  cx="{node.x}"
                  cy="{node.y}"
                  r="{node.size}"
                  fill="{node.color}"
                  stroke="rgba(255, 255, 255, 0.8)"
                  stroke-width="{selectedModel?.id === node.id ? 3 : node.type === 'model' ? 2 : 1}"
                  class="transition-all duration-300"
                />
                
                <!-- Node icon (for models) -->
                {#if node.type === 'model'}
                  <text
                    x="{node.x}"
                    y="{node.y + 2}"
                    text-anchor="middle"
                    dominant-baseline="middle"
                    class="text-xs pointer-events-none"
                  >
                    {node.icon}
                  </text>
                {/if}
                
                <!-- Node label -->
                <text
                  x="{node.x}"
                  y="{node.y + node.size + 15}"
                  text-anchor="middle"
                  class="text-xs fill-gray-300 pointer-events-none {hoveredModel?.id === node.id || selectedModel?.id === node.id ? 'opacity-100' : 'opacity-0'} transition-opacity duration-300"
                >
                  {node.name.length > 12 ? node.name.substring(0, 12) + '...' : node.name}
                </text>
                
                <!-- Request count indicator (for models with activity) -->
                {#if node.type === 'model' && node.requestCount > 0}
                  <circle
                    cx="{node.x + node.size - 2}"
                    cy="{node.y - node.size + 2}"
                    r="4"
                    fill="#10B981"
                    stroke="white"
                    stroke-width="1"
                  />
                  <text
                    x="{node.x + node.size - 2}"
                    y="{node.y - node.size + 2}"
                    text-anchor="middle"
                    dominant-baseline="middle"
                    class="text-xs fill-white font-bold pointer-events-none"
                  >
                    {formatNumber(node.requestCount).substring(0, 1)}
                  </text>
                {/if}
              </g>
            {/each}
            
            <!-- Center core -->
            <circle
              cx="{center}"
              cy="{center}"
              r="8"
              fill="url(#galaxyGradient)"
              stroke="rgba(139, 92, 246, 0.8)"
              stroke-width="2"
              filter="url(#modelGlow)"
              class="opacity-60"
            />
            <text
              x="{center}"
              y="{center + 2}"
              text-anchor="middle"
              dominant-baseline="middle"
              class="text-xs fill-white pointer-events-none"
            >
              🌟
            </text>
          </svg>
        </div>
      </div>
      
      <!-- Model Details Panel -->
      <div class="w-full lg:w-80 space-y-4">
        {#if selectedModel && modelDetails}
          <div class="p-4 rounded-lg bg-gradient-to-br from-gray-800 to-gray-900 border border-gray-600">
            <div class="flex items-start justify-between mb-4">
              <div>
                <h4 class="text-lg font-semibold text-white flex items-center gap-2">
                  <span>{selectedModel.icon}</span>
                  {modelDetails.name}
                </h4>
                <p class="text-sm text-gray-400">{modelDetails.category}</p>
              </div>
              <button
                class="text-gray-400 hover:text-white transition-colors"
                onclick={() => selectedModel = null}
              >
                ✕
              </button>
            </div>
            
            <!-- Performance Metrics -->
            <div class="space-y-3">
              <div class="grid grid-cols-2 gap-3">
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{formatNumber(modelDetails.requestCount)}</div>
                  <div class="text-xs text-gray-400">Requests</div>
                </div>
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{modelDetails.avgLatency}ms</div>
                  <div class="text-xs text-gray-400">Avg Latency</div>
                </div>
              </div>
              
              <div class="grid grid-cols-2 gap-3">
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{modelDetails.successRate}%</div>
                  <div class="text-xs text-gray-400">Success Rate</div>
                </div>
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{modelDetails.errorCount}</div>
                  <div class="text-xs text-gray-400">Errors</div>
                </div>
              </div>
            </div>
            
            <!-- Endpoints -->
            {#if modelDetails.endpoints.length > 0}
              <div class="mt-4">
                <h5 class="text-sm font-medium text-gray-400 mb-2">Connected Endpoints</h5>
                <div class="space-y-1">
                  {#each modelDetails.endpoints as endpoint}
                    <div class="text-xs px-2 py-1 bg-gray-700 rounded text-gray-300">
                      {endpoint}
                    </div>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {:else}
          <!-- Galaxy Overview -->
          <div class="p-4 rounded-lg bg-gradient-to-br from-gray-800 to-gray-900 border border-gray-600">
            <h4 class="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <span>🌌</span>
              Galaxy Overview
            </h4>
            
            <div class="space-y-4">
              <!-- Quick Stats -->
              <div class="grid grid-cols-2 gap-3">
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{(galaxyNodes || []).filter(n => n.type === 'model').length}</div>
                  <div class="text-xs text-gray-400">Models</div>
                </div>
                <div class="p-3 rounded-lg bg-gray-800/50 border border-gray-700">
                  <div class="text-lg font-bold text-white">{(galaxyNodes || []).filter(n => n.type === 'endpoint').length}</div>
                  <div class="text-xs text-gray-400">Endpoints</div>
                </div>
              </div>
              
              <!-- Model Categories -->
              <div>
                <h5 class="text-sm font-medium text-gray-400 mb-2">Model Categories</h5>
                <div class="space-y-2">
                  {#each Object.entries(modelCategories) as [key, category]}
                    {@const count = (galaxyNodes || []).filter(n => n.category === key).length}
                    {#if count > 0}
                      <div class="flex items-center justify-between text-xs">
                        <div class="flex items-center gap-2">
                          <div class="w-3 h-3 rounded-full" style="background-color: {category.color}"></div>
                          <span class="text-gray-300">{category.label}</span>
                        </div>
                        <span class="text-gray-400">{count}</span>
                      </div>
                    {/if}
                  {/each}
                </div>
              </div>
              
              <div class="text-xs text-gray-500 italic">
                Click on a model to view detailed metrics
              </div>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  @keyframes rotate {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }
  
  @keyframes float {
    from {
      transform: translateY(0px);
    }
    to {
      transform: translateY(-3px);
    }
  }
</style>