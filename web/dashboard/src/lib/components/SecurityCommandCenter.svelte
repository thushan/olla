<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const status = $derived(dashboardStore.status);
  const events = $derived(dashboardStore.events);
  
  // Security metrics
  const securityMetrics = $derived.by(() => {
    if (!status || !status.system) {
      return {
        violations: 0,
        blockedRequests: 0,
        rateLimitHits: 0,
        suspiciousPatterns: 0,
      };
    }
    
    // Calculate from events
    const recentEvents = (events || []).filter(e => {
      const eventTime = new Date(e.timestamp);
      const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000);
      return eventTime > fiveMinutesAgo;
    });
    
    return {
      violations: status.system.security_violations || 0,
      blockedRequests: recentEvents.filter(e => e.type === 'security.blocked').length,
      rateLimitHits: recentEvents.filter(e => e.type === 'rate_limit.exceeded').length,
      suspiciousPatterns: recentEvents.filter(e => e.type === 'security.suspicious').length,
    };
  });
  
  // Threat level calculation
  const threatLevel = $derived.by(() => {
    const metrics = securityMetrics;
    const score = 
      metrics.violations * 10 +
      metrics.blockedRequests * 5 +
      metrics.rateLimitHits * 2 +
      metrics.suspiciousPatterns * 3;
    
    if (score === 0) return { level: 'low', label: 'Secure', color: 'green' };
    if (score < 10) return { level: 'medium', label: 'Caution', color: 'yellow' };
    if (score < 50) return { level: 'high', label: 'Warning', color: 'orange' };
    return { level: 'critical', label: 'Critical', color: 'red' };
  });
  
  // Recent security events
  const securityEvents = $derived.by(() => {
    return (events || [])
      .filter(e => 
        e.type?.includes('security') || 
        e.type?.includes('rate_limit') ||
        e.type === 'proxy.error'
      )
      .slice(0, 10);
  });
  
  // Mock rate limit config (would come from actual config in production)
  const rateLimitConfig = {
    global: { limit: 1000, window: '1m', current: 0 },
    perIP: { limit: 100, window: '1m', current: 0 },
    health: { limit: 1000, window: '1m', current: 0 },
  };
  
  // Format event for display
  function formatSecurityEvent(event) {
    const time = new Date(event.timestamp).toLocaleTimeString();
    
    switch (event.type) {
      case 'security.blocked':
        return {
          icon: '🛡️',
          message: `Blocked request from ${event.metadata?.clientIP || 'unknown'}`,
          severity: 'high',
          time,
        };
      case 'rate_limit.exceeded':
        return {
          icon: '⏱️',
          message: `Rate limit exceeded for ${event.metadata?.clientIP || 'unknown'}`,
          severity: 'medium',
          time,
        };
      case 'security.suspicious':
        return {
          icon: '👁️',
          message: `Suspicious pattern detected: ${event.metadata?.pattern || 'unknown'}`,
          severity: 'high',
          time,
        };
      case 'proxy.error':
        if (event.error?.includes('forbidden') || event.error?.includes('unauthorized')) {
          return {
            icon: '🚫',
            message: `Unauthorized access attempt to ${event.endpoint || 'unknown'}`,
            severity: 'medium',
            time,
          };
        }
        return null;
      default:
        return {
          icon: '📍',
          message: event.metadata?.message || event.type,
          severity: 'low',
          time,
        };
    }
  }
</script>

<div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
  <div class="p-6 border-b border-gray-200 dark:border-gray-700">
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Security Command Center</h3>
      <div class="flex items-center gap-2 px-3 py-1 rounded-full text-sm font-medium {threatLevel.level === 'low' ? 'bg-green-100 dark:bg-green-900/20 text-green-600 dark:text-green-400' : threatLevel.level === 'medium' ? 'bg-yellow-100 dark:bg-yellow-900/20 text-yellow-600 dark:text-yellow-400' : threatLevel.level === 'high' ? 'bg-orange-100 dark:bg-orange-900/20 text-orange-600 dark:text-orange-400' : 'bg-red-100 dark:bg-red-900/20 text-red-600 dark:text-red-400'}">
        <span>🛡️</span>
        <span>Threat Level: {threatLevel.label}</span>
      </div>
    </div>
  </div>
  
  <div class="p-6 space-y-6">
    <!-- Metrics Grid -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-4 bg-white dark:bg-gray-800">
        <div class="text-2xl">🚨</div>
        <div class="flex-1">
          <div class="text-2xl font-bold text-gray-900 dark:text-white">{securityMetrics.violations}</div>
          <div class="text-sm text-gray-600 dark:text-gray-400">Security Violations</div>
        </div>
      </div>
      
      <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-4 bg-white dark:bg-gray-800">
        <div class="text-2xl">🚫</div>
        <div class="flex-1">
          <div class="text-2xl font-bold text-gray-900 dark:text-white">{securityMetrics.blockedRequests}</div>
          <div class="text-sm text-gray-600 dark:text-gray-400">Blocked Requests</div>
        </div>
      </div>
      
      <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-4 bg-white dark:bg-gray-800">
        <div class="text-2xl">⏱️</div>
        <div class="flex-1">
          <div class="text-2xl font-bold text-gray-900 dark:text-white">{securityMetrics.rateLimitHits}</div>
          <div class="text-sm text-gray-600 dark:text-gray-400">Rate Limit Hits</div>
        </div>
      </div>
      
      <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center gap-4 bg-white dark:bg-gray-800">
        <div class="text-2xl">👁️</div>
        <div class="flex-1">
          <div class="text-2xl font-bold text-gray-900 dark:text-white">{securityMetrics.suspiciousPatterns}</div>
          <div class="text-sm text-gray-600 dark:text-gray-400">Suspicious Patterns</div>
        </div>
      </div>
    </div>
    
    <!-- Rate Limits Status -->
    <div>
      <h4 class="font-semibold text-gray-900 dark:text-white mb-3">Rate Limit Status</h4>
      <div class="space-y-4">
        <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div class="flex items-center justify-between mb-2">
            <span class="font-medium text-gray-900 dark:text-white">Global Limit</span>
            <span class="text-sm text-gray-600 dark:text-gray-400 font-mono">{rateLimitConfig.global.limit}/{rateLimitConfig.global.window}</span>
          </div>
          <div class="space-y-1">
            <div class="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div 
                class="h-full bg-blue-500 rounded-full transition-all duration-500"
                style="width: {(rateLimitConfig.global.current / rateLimitConfig.global.limit) * 100}%"
              ></div>
            </div>
            <span class="text-xs text-gray-600 dark:text-gray-400">{rateLimitConfig.global.current} / {rateLimitConfig.global.limit}</span>
          </div>
        </div>
        
        <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div class="flex items-center justify-between mb-2">
            <span class="font-medium text-gray-900 dark:text-white">Per IP Limit</span>
            <span class="text-sm text-gray-600 dark:text-gray-400 font-mono">{rateLimitConfig.perIP.limit}/{rateLimitConfig.perIP.window}</span>
          </div>
          <div class="space-y-1">
            <div class="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div 
                class="h-full bg-blue-500 rounded-full transition-all duration-500"
                style="width: {(rateLimitConfig.perIP.current / rateLimitConfig.perIP.limit) * 100}%"
              ></div>
            </div>
            <span class="text-xs text-gray-600 dark:text-gray-400">{rateLimitConfig.perIP.current} / {rateLimitConfig.perIP.limit}</span>
          </div>
        </div>
        
        <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div class="flex items-center justify-between mb-2">
            <span class="font-medium text-gray-900 dark:text-white">Health Check Limit</span>
            <span class="text-sm text-gray-600 dark:text-gray-400 font-mono">{rateLimitConfig.health.limit}/{rateLimitConfig.health.window}</span>
          </div>
          <div class="space-y-1">
            <div class="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div 
                class="h-full bg-blue-500 rounded-full transition-all duration-500"
                style="width: {(rateLimitConfig.health.current / rateLimitConfig.health.limit) * 100}%"
              ></div>
            </div>
            <span class="text-xs text-gray-600 dark:text-gray-400">{rateLimitConfig.health.current} / {rateLimitConfig.health.limit}</span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- Security Events -->
    <div>
      <h4 class="font-semibold text-gray-900 dark:text-white mb-3">Recent Security Events</h4>
      {#if securityEvents.length === 0}
        <div class="text-center py-8">
          <span class="text-2xl block mb-2">✅</span>
          <p class="text-gray-600 dark:text-gray-400">No security events in the last 5 minutes</p>
        </div>
      {:else}
        <div class="space-y-3">
          {#each securityEvents as event}
            {@const formatted = formatSecurityEvent(event)}
            {#if formatted}
              <div class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 flex items-start gap-3 bg-white dark:bg-gray-800 {formatted.severity === 'low' ? 'border-l-4 border-l-blue-500' : formatted.severity === 'medium' ? 'border-l-4 border-l-yellow-500' : 'border-l-4 border-l-red-500'}">
                <span class="text-lg">{formatted.icon}</span>
                <div class="flex-1">
                  <p class="text-sm text-gray-900 dark:text-white">{formatted.message}</p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">{formatted.time}</span>
                </div>
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

