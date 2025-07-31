<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const status = $derived(dashboardStore.status);
  const processStats = $derived(dashboardStore.processStats);
  const events = $derived(dashboardStore.events);
  
  // Active chart view
  let activeChart = $state('latency');
  
  // Chart dimensions
  const chartWidth = 400;
  const chartHeight = 200;
  const margin = { top: 20, right: 20, bottom: 40, left: 60 };
  const graphWidth = chartWidth - margin.left - margin.right;
  const graphHeight = chartHeight - margin.top - margin.bottom;
  
  // Generate mock time series data (in production, this would come from real metrics)
  const generateTimeSeriesData = (baseValue, variance = 0.2, points = 20) => {
    const now = Date.now();
    return Array.from({ length: points }, (_, i) => {
      const timestamp = now - (points - i - 1) * 5000; // 5 second intervals
      const randomFactor = 1 + (Math.random() - 0.5) * variance;
      return {
        timestamp,
        value: Math.max(0, baseValue * randomFactor),
        x: (i / (points - 1)) * graphWidth,
        time: new Date(timestamp).toLocaleTimeString(),
      };
    });
  };
  
  // Performance time series data
  const performanceData = $derived(() => {
    const avgLatency = status?.system?.avg_latency_ms || 50;
    const totalRequests = status?.system?.total_requests || 100;
    const activeConnections = status?.system?.active_connections || 10;
    const memoryUsage = processStats?.memory?.heap_alloc ? 
      parseInt(processStats.memory.heap_alloc.replace(/[^\d]/g, '')) / 1024 / 1024 : 25; // MB
    
    return {
      latency: generateTimeSeriesData(avgLatency, 0.3),
      throughput: generateTimeSeriesData(totalRequests / 60, 0.4), // requests per minute
      connections: generateTimeSeriesData(activeConnections, 0.2),
      memory: generateTimeSeriesData(memoryUsage, 0.15),
    };
  });
  
  // Get current chart data
  const currentChartData = $derived(() => performanceData()[activeChart]);
  
  // Calculate Y scale
  const yScale = $derived(() => {
    const data = currentChartData();
    if (!data || data.length === 0) return { min: 0, max: 100 };
    
    const values = data.map(d => d.value);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const padding = (max - min) * 0.1 || 10;
    
    return {
      min: Math.max(0, min - padding),
      max: max + padding,
    };
  });
  
  // Convert data point to SVG coordinates
  const getY = (value) => {
    const scale = yScale();
    return graphHeight - ((value - scale.min) / (scale.max - scale.min)) * graphHeight;
  };
  
  // Generate SVG path for line chart
  const chartPath = $derived(() => {
    const data = currentChartData();
    if (!data || data.length === 0) return '';
    
    return data.map((point, index) => 
      `${index === 0 ? 'M' : 'L'} ${point.x} ${getY(point.value)}`
    ).join(' ');
  });
  
  // Generate area path for gradient fill
  const areaPath = $derived(() => {
    const data = currentChartData();
    if (!data || data.length === 0) return '';
    
    const linePath = chartPath();
    const firstPoint = data[0];
    const lastPoint = data[data.length - 1];
    
    return `${linePath} L ${lastPoint.x} ${graphHeight} L ${firstPoint.x} ${graphHeight} Z`;
  });
  
  // Chart configurations
  const chartConfigs = {
    latency: {
      title: 'Response Latency',
      unit: 'ms',
      color: 'blue',
      icon: '⚡',
      gradient: 'from-blue-500/30 to-blue-600/10',
    },
    throughput: {
      title: 'Request Throughput',
      unit: 'req/min',
      color: 'emerald',
      icon: '📊',
      gradient: 'from-emerald-500/30 to-emerald-600/10',
    },
    connections: {
      title: 'Active Connections',
      unit: 'conn',
      color: 'purple',
      icon: '🔗',
      gradient: 'from-purple-500/30 to-purple-600/10',
    },
    memory: {
      title: 'Memory Usage',
      unit: 'MB',
      color: 'orange',
      icon: '💾',
      gradient: 'from-orange-500/30 to-orange-600/10',
    },
  };
  
  // Performance insights
  const performanceInsights = $derived(() => {
    if (!status?.system) return [];
    
    const insights = [];
    const system = status.system;
    const avgLatency = system.avg_latency_ms || 0;
    const successRate = ((system.successful_requests || 0) / (system.total_requests || 1)) * 100;
    
    if (avgLatency > 1000) {
      insights.push({
        type: 'warning',
        icon: '⚠️',
        message: 'High response latency detected',
        suggestion: 'Consider scaling or optimizing backend services',
      });
    }
    
    if (successRate < 95) {
      insights.push({
        type: 'error',
        icon: '🚨',
        message: `Success rate is ${successRate.toFixed(1)}%`,
        suggestion: 'Investigate error causes and improve reliability',
      });
    }
    
    if (system.active_connections > 100) {
      insights.push({
        type: 'info',
        icon: '📈',
        message: 'High connection count detected',
        suggestion: 'Monitor for potential connection pooling optimizations',
      });
    }
    
    if (insights.length === 0) {
      insights.push({
        type: 'success',
        icon: '✅',
        message: 'Performance metrics look healthy',
        suggestion: 'All systems operating within normal parameters',
      });
    }
    
    return insights;
  });
  
  // Animated mount
  let mounted = $state(false);
  onMount(() => {
    setTimeout(() => mounted = true, 200);
  });
</script>

<div class="bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-900 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Performance Metrics</h3>
      
      <!-- Chart Type Selector -->
      <div class="flex gap-1 p-1 bg-gray-100 dark:bg-gray-700 rounded-lg">
        {#each Object.entries(chartConfigs) as [key, config]}
          <button
            class="flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all duration-200 {activeChart === key ? `bg-${config.color}-500 text-white shadow-sm` : 'text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-600'}"
            onclick={() => activeChart = key}
          >
            <span>{config.icon}</span>
            <span class="hidden sm:inline">{config.title}</span>
          </button>
        {/each}
      </div>
    </div>
  </div>
  
  <div class="p-6 space-y-6">
    <!-- Interactive Chart -->
    <div class="relative">
      <div class="flex items-center justify-between mb-4">
        <h4 class="text-md font-semibold text-gray-900 dark:text-white flex items-center gap-2">
          <span>{chartConfigs[activeChart].icon}</span>
          {chartConfigs[activeChart].title}
        </h4>
        
        <!-- Current value -->
        {#if currentChartData().length > 0}
          <div class="text-right">
            <div class="text-2xl font-bold text-gray-900 dark:text-white">
              {currentChartData()[currentChartData().length - 1].value.toFixed(1)}
            </div>
            <div class="text-sm text-gray-600 dark:text-gray-400">
              {chartConfigs[activeChart].unit}
            </div>
          </div>
        {/if}
      </div>
      
      <!-- Chart Container -->
      <div class="relative bg-gradient-to-br from-gray-50 to-white dark:from-gray-700 dark:to-gray-800 rounded-lg p-4 border border-gray-200 dark:border-gray-600">
        <svg width="{chartWidth}" height="{chartHeight}" class="w-full h-auto">
          <!-- Gradient definitions -->
          <defs>
            <linearGradient id="chartGradient-{activeChart}" x1="0%" y1="0%" x2="0%" y2="100%">
              <stop offset="0%" stop-color="{chartConfigs[activeChart].color === 'blue' ? 'rgb(59, 130, 246)' : chartConfigs[activeChart].color === 'emerald' ? 'rgb(16, 185, 129)' : chartConfigs[activeChart].color === 'purple' ? 'rgb(139, 92, 246)' : 'rgb(249, 115, 22)'}" stop-opacity="0.3"/>
              <stop offset="100%" stop-color="{chartConfigs[activeChart].color === 'blue' ? 'rgb(59, 130, 246)' : chartConfigs[activeChart].color === 'emerald' ? 'rgb(16, 185, 129)' : chartConfigs[activeChart].color === 'purple' ? 'rgb(139, 92, 246)' : 'rgb(249, 115, 22)'}" stop-opacity="0.05"/>
            </linearGradient>
            
            <filter id="glow-{activeChart}">
              <feGaussianBlur stdDeviation="2" result="coloredBlur"/>
              <feMerge>
                <feMergeNode in="coloredBlur"/>
                <feMergeNode in="SourceGraphic"/>
              </feMerge>
            </filter>
          </defs>
          
          <!-- Chart area -->
          <g transform="translate({margin.left}, {margin.top})">
            <!-- Grid lines -->
            {#each Array.from({length: 5}) as _, i}
              <line
                x1="0"
                y1="{(i / 4) * graphHeight}"
                x2="{graphWidth}"
                y2="{(i / 4) * graphHeight}"
                stroke="currentColor"
                stroke-width="1"
                class="text-gray-200 dark:text-gray-600 opacity-50"
                stroke-dasharray="2,2"
              />
            {/each}
            
            <!-- Area fill -->
            <path
              d="{areaPath()}"
              fill="url(#chartGradient-{activeChart})"
              class="transition-all duration-1000 {mounted ? 'opacity-100' : 'opacity-0'}"
            />
            
            <!-- Line -->
            <path
              d="{chartPath()}"
              fill="none"
              stroke="{chartConfigs[activeChart].color === 'blue' ? 'rgb(59, 130, 246)' : chartConfigs[activeChart].color === 'emerald' ? 'rgb(16, 185, 129)' : chartConfigs[activeChart].color === 'purple' ? 'rgb(139, 92, 246)' : 'rgb(249, 115, 22)'}"
              stroke-width="3"
              filter="url(#glow-{activeChart})"
              class="transition-all duration-1000 {mounted ? 'opacity-100' : 'opacity-0'}"
              style="stroke-dasharray: 1000; stroke-dashoffset: {mounted ? 0 : 1000}; transition: stroke-dashoffset 2s ease-in-out;"
            />
            
            <!-- Data points -->
            {#each currentChartData() as point, index}
              <circle
                cx="{point.x}"
                cy="{getY(point.value)}"
                r="4"
                fill="{chartConfigs[activeChart].color === 'blue' ? 'rgb(59, 130, 246)' : chartConfigs[activeChart].color === 'emerald' ? 'rgb(16, 185, 129)' : chartConfigs[activeChart].color === 'purple' ? 'rgb(139, 92, 246)' : 'rgb(249, 115, 22)'}"
                stroke="white"
                stroke-width="2"
                class="transition-all duration-300 hover:r-6 cursor-pointer opacity-0 {mounted ? 'animate-fadeIn' : ''}"
                style="animation-delay: {index * 0.05}s;"
              >
                <title>{point.time}: {point.value.toFixed(1)} {chartConfigs[activeChart].unit}</title>
              </circle>
            {/each}
            
            <!-- Y-axis labels -->
            {#each Array.from({length: 5}) as _, i}
              <text
                x="-10"
                y="{(i / 4) * graphHeight + 5}"
                text-anchor="end"
                class="text-xs fill-gray-600 dark:fill-gray-400"
              >
                {(yScale().max - (i / 4) * (yScale().max - yScale().min)).toFixed(0)}
              </text>
            {/each}
          </g>
          
          <!-- X-axis -->
          <line
            x1="{margin.left}"
            y1="{chartHeight - margin.bottom}"
            x2="{chartWidth - margin.right}"
            y2="{chartHeight - margin.bottom}"
            stroke="currentColor"
            class="text-gray-300 dark:text-gray-600"
          />
          
          <!-- Y-axis -->
          <line
            x1="{margin.left}"
            y1="{margin.top}"
            x2="{margin.left}"
            y2="{chartHeight - margin.bottom}"
            stroke="currentColor"
            class="text-gray-300 dark:text-gray-600"
          />
        </svg>
      </div>
    </div>
    
    <!-- Performance Insights -->
    <div>
      <h4 class="text-md font-semibold text-gray-900 dark:text-white mb-4">Performance Insights</h4>
      <div class="space-y-3">
        {#each performanceInsights() as insight}
          <div class="flex items-start gap-3 p-4 rounded-lg border {insight.type === 'success' ? 'border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-900/20' : insight.type === 'warning' ? 'border-yellow-200 dark:border-yellow-800 bg-yellow-50 dark:bg-yellow-900/20' : insight.type === 'error' ? 'border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20' : 'border-blue-200 dark:border-blue-800 bg-blue-50 dark:bg-blue-900/20'}">
            <span class="text-lg">{insight.icon}</span>
            <div class="flex-1">
              <p class="text-sm font-medium text-gray-900 dark:text-white">{insight.message}</p>
              <p class="text-xs text-gray-600 dark:text-gray-400 mt-1">{insight.suggestion}</p>
            </div>
          </div>
        {/each}
      </div>
    </div>
    
    <!-- Quick Stats -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      {#each Object.entries(chartConfigs) as [key, config]}
        {@const data = performanceData()[key]}
        {@const currentValue = data[data.length - 1]?.value || 0}
        <div class="group p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-700 transition-all duration-300 hover:shadow-md hover:scale-105 cursor-pointer"
             onclick={() => activeChart = key}>
          <div class="flex items-center gap-3 mb-2">
            <div class="text-xl group-hover:scale-110 transition-transform duration-200">
              {config.icon}
            </div>
            <div class="flex-1">
              <div class="text-lg font-bold text-gray-900 dark:text-white">
                {currentValue.toFixed(1)}
              </div>
              <div class="text-xs text-gray-600 dark:text-gray-400">
                {config.unit}
              </div>
            </div>
          </div>
          <div class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {config.title}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: scale(0);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }
  
  .animate-fadeIn {
    animation: fadeIn 0.5s ease-out forwards;
  }
</style>