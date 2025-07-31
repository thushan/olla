<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const status = $derived(dashboardStore.status);
  const events = $derived(dashboardStore.events);
  
  // Radar chart dimensions
  const size = 300;
  const center = size / 2;
  const radius = size / 2 - 40;
  const levels = 5;
  
  // Security metrics for radar chart
  const radarMetrics = $derived(() => {
    if (!status?.system) {
      return {
        threatDetection: 0,
        accessControl: 0,
        rateLimiting: 0,
        dataIntegrity: 0,
        networkSecurity: 0,
        auditCompliance: 0,
      };
    }
    
    // Calculate security scores (0-100)
    const recentEvents = (events || []).filter(e => {
      const eventTime = new Date(e.timestamp);
      const tenMinutesAgo = new Date(Date.now() - 10 * 60 * 1000);
      return eventTime > tenMinutesAgo;
    });
    
    const securityViolations = status.system.security_violations || 0;
    const blockedRequests = recentEvents.filter(e => e.type === 'security.blocked').length;
    const rateLimitHits = recentEvents.filter(e => e.type === 'rate_limit.exceeded').length;
    const totalRequests = status.system.total_requests || 0;
    
    // Calculate success rate from errors
    const totalErrors = status.system.total_errors || 0;
    const successfulRequests = totalRequests > 0 ? totalRequests - totalErrors : 0;
    const successRate = totalRequests > 0 ? (successfulRequests / totalRequests) * 100 : 95;
    
    // If no data, show optimistic values
    const hasData = totalRequests > 0;
    
    return {
      threatDetection: hasData ? Math.max(0, 100 - (securityViolations * 10)) : 90,
      accessControl: hasData ? Math.max(0, 100 - (blockedRequests * 5)) : 95,
      rateLimiting: hasData ? Math.max(0, 100 - (rateLimitHits * 8)) : 92,
      dataIntegrity: hasData ? Math.min(100, Math.max(80, successRate)) : 88,
      networkSecurity: hasData ? Math.max(0, 100 - (recentEvents.filter(e => e.type?.includes('network')).length * 3)) : 91,
      auditCompliance: hasData ? Math.max(0, 100 - (recentEvents.filter(e => e.type?.includes('audit')).length * 4)) : 85,
    };
  });
  
  // Convert metrics to radar points
  const radarPoints = $derived(() => {
    const metrics = radarMetrics();
    const categories = Object.keys(metrics);
    const angleStep = (2 * Math.PI) / categories.length;
    
    return categories.map((category, index) => {
      const angle = index * angleStep - Math.PI / 2; // Start from top
      const value = metrics[category];
      const distance = (value / 100) * radius;
      
      return {
        category,
        value,
        x: center + Math.cos(angle) * distance,
        y: center + Math.sin(angle) * distance,
        labelX: center + Math.cos(angle) * (radius + 20),
        labelY: center + Math.sin(angle) * (radius + 20),
        angle,
      };
    });
  });
  
  // Generate grid levels
  const gridLevels = Array.from({ length: levels }, (_, i) => {
    const levelRadius = ((i + 1) / levels) * radius;
    const categories = Object.keys(radarMetrics());
    const angleStep = (2 * Math.PI) / categories.length;
    
    return categories.map((_, index) => {
      const angle = index * angleStep - Math.PI / 2;
      return {
        x: center + Math.cos(angle) * levelRadius,
        y: center + Math.sin(angle) * levelRadius,
      };
    });
  });
  
  // Create path for filled area
  const radarPath = $derived(() => {
    const points = radarPoints();
    if (points.length === 0) return '';
    
    const pathData = points.map((point, index) => 
      `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`
    ).join(' ') + ' Z';
    
    return pathData;
  });
  
  // Overall security score
  const securityScore = $derived(() => {
    const metrics = radarMetrics();
    const values = Object.values(metrics);
    const average = values.reduce((sum, val) => sum + val, 0) / values.length;
    return Math.round(average);
  });
  
  // Security status
  const securityStatus = $derived(() => {
    const score = securityScore();
    if (score >= 90) return { label: 'Excellent', color: 'emerald', icon: '🛡️' };
    if (score >= 75) return { label: 'Good', color: 'green', icon: '✅' };
    if (score >= 60) return { label: 'Fair', color: 'yellow', icon: '⚠️' };
    if (score >= 40) return { label: 'Poor', color: 'orange', icon: '🔶' };
    return { label: 'Critical', color: 'red', icon: '🚨' };
  });
  
  // Format category names for display
  function formatCategoryName(category) {
    return category
      .replace(/([A-Z])/g, ' $1')
      .replace(/^./, str => str.toUpperCase())
      .trim();
  }
  
  // Animated mount
  let mounted = $state(false);
  onMount(() => {
    setTimeout(() => mounted = true, 100);
  });
</script>

<div class="bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-900 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Security Radar</h3>
      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 px-3 py-1.5 rounded-full text-sm font-medium bg-gradient-to-r {securityStatus().color === 'emerald' ? 'from-emerald-100 to-emerald-200 dark:from-emerald-900/30 dark:to-emerald-800/30 text-emerald-700 dark:text-emerald-300' : securityStatus().color === 'green' ? 'from-green-100 to-green-200 dark:from-green-900/30 dark:to-green-800/30 text-green-700 dark:text-green-300' : securityStatus().color === 'yellow' ? 'from-yellow-100 to-yellow-200 dark:from-yellow-900/30 dark:to-yellow-800/30 text-yellow-700 dark:text-yellow-300' : securityStatus().color === 'orange' ? 'from-orange-100 to-orange-200 dark:from-orange-900/30 dark:to-orange-800/30 text-orange-700 dark:text-orange-300' : 'from-red-100 to-red-200 dark:from-red-900/30 dark:to-red-800/30 text-red-700 dark:text-red-300'}">
          <span>{securityStatus().icon}</span>
          <span>{securityStatus().label}</span>
        </div>
        <div class="text-2xl font-bold text-gray-900 dark:text-white">
          {securityScore()}%
        </div>
      </div>
    </div>
  </div>
  
  <div class="p-6">
    <div class="flex flex-col lg:flex-row gap-8 items-center">
      <!-- Radar Chart -->
      <div class="flex-1 flex justify-center">
        <div class="relative">
          <svg width="{size}" height="{size}" class="transform transition-all duration-1000 {mounted ? 'scale-100 opacity-100' : 'scale-95 opacity-0'}">
            <!-- Grid circles -->
            {#each Array.from({length: levels}) as _, i}
              <circle
                cx="{center}"
                cy="{center}"
                r="{((i + 1) / levels) * radius}"
                fill="none"
                stroke="currentColor"
                stroke-width="1"
                class="text-gray-200 dark:text-gray-700 opacity-30"
              />
            {/each}
            
            <!-- Grid axes -->
            {#each radarPoints() as point}
              <line
                x1="{center}"
                y1="{center}"
                x2="{center + Math.cos(point.angle) * radius}"
                y2="{center + Math.sin(point.angle) * radius}"
                stroke="currentColor"
                stroke-width="1"
                class="text-gray-200 dark:text-gray-700 opacity-30"
              />
            {/each}
            
            <!-- Filled area with gradient -->
            <defs>
              <radialGradient id="radarGradient" cx="50%" cy="50%" r="50%">
                <stop offset="0%" stop-color="rgb(59, 130, 246)" stop-opacity="0.4"/>
                <stop offset="50%" stop-color="rgb(59, 130, 246)" stop-opacity="0.2"/>
                <stop offset="100%" stop-color="rgb(59, 130, 246)" stop-opacity="0.1"/>
              </radialGradient>
              <filter id="glow">
                <feGaussianBlur stdDeviation="3" result="coloredBlur"/>
                <feMerge>
                  <feMergeNode in="coloredBlur"/>
                  <feMergeNode in="SourceGraphic"/>
                </feMerge>
              </filter>
            </defs>
            
            <!-- Animated path -->
            <path
              d="{radarPath()}"
              fill="url(#radarGradient)"
              stroke="rgb(59, 130, 246)"
              stroke-width="2"
              filter="url(#glow)"
              class="transition-all duration-1000 {mounted ? 'opacity-100' : 'opacity-0'}"
              style="animation: drawPath 2s ease-in-out;"
            />
            
            <!-- Data points -->
            {#each radarPoints() as point, index}
              <circle
                cx="{point.x}"
                cy="{point.y}"
                r="4"
                fill="rgb(59, 130, 246)"
                stroke="white"
                stroke-width="2"
                class="transition-all duration-300 hover:r-6 cursor-pointer filter drop-shadow-sm {mounted ? 'opacity-100' : 'opacity-0'}"
                style="animation: fadeInPoint {0.5 + index * 0.1}s ease-out;"
              />
            {/each}
            
            <!-- Category labels -->
            {#each radarPoints() as point}
              <text
                x="{point.labelX}"
                y="{point.labelY}"
                text-anchor="middle"
                dominant-baseline="middle"
                class="text-xs font-medium fill-gray-700 dark:fill-gray-300 transition-all duration-300 {mounted ? 'opacity-100' : 'opacity-0'}"
              >
                {formatCategoryName(point.category)}
              </text>
            {/each}
          </svg>
        </div>
      </div>
      
      <!-- Metrics Breakdown -->
      <div class="flex-1 space-y-4">
        <h4 class="text-md font-semibold text-gray-900 dark:text-white mb-4">Security Metrics</h4>
        {#each radarPoints() as point, index}
          <div class="group p-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-gradient-to-r from-white to-gray-50 dark:from-gray-800 dark:to-gray-700 transition-all duration-300 hover:shadow-md hover:scale-105">
            <div class="flex items-center justify-between mb-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {formatCategoryName(point.category)}
              </span>
              <span class="text-sm font-bold text-gray-900 dark:text-white">
                {Math.round(point.value)}%
              </span>
            </div>
            <div class="w-full h-2 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-1000 ease-out bg-gradient-to-r {point.value >= 80 ? 'from-emerald-400 to-emerald-500' : point.value >= 60 ? 'from-blue-400 to-blue-500' : point.value >= 40 ? 'from-yellow-400 to-yellow-500' : 'from-red-400 to-red-500'}"
                style="width: {mounted ? point.value : 0}%; animation-delay: {index * 0.1}s;"
              ></div>
            </div>
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>

<style>
  @keyframes drawPath {
    0% {
      stroke-dasharray: 1000;
      stroke-dashoffset: 1000;
    }
    100% {
      stroke-dasharray: 1000;
      stroke-dashoffset: 0;
    }
  }
  
  @keyframes fadeInPoint {
    0% {
      opacity: 0;
      transform: scale(0);
    }
    100% {
      opacity: 1;
      transform: scale(1);
    }
  }
</style>