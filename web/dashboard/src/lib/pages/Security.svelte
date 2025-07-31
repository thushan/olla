<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import SecurityRadar from '$lib/components/SecurityRadar.svelte';
  
  const status = $derived(dashboardStore.status);
  const security = $derived(status?.security || {});
  const securityViolations = $derived(status?.system?.security_violations || 0);
  
  // Extract security metrics
  const rateLimitViolations = $derived(security.violations?.rate_limits || 0);
  const sizeLimitViolations = $derived(security.violations?.size_limits || 0);
  const blockedIPs = $derived(security.blocked_ips || 0);
  
  // Calculate security status
  const securityStatus = $derived((() => {
    if (securityViolations === 0 && blockedIPs === 0) return 'secure';
    if (securityViolations < 10 && blockedIPs < 5) return 'warning';
    return 'critical';
  })());
  
  // Get status color
  function getStatusColor(status) {
    switch(status) {
      case 'secure':
        return 'text-green-600 dark:text-green-400';
      case 'warning':
        return 'text-yellow-600 dark:text-yellow-400';
      case 'critical':
        return 'text-red-600 dark:text-red-400';
      default:
        return 'text-gray-600 dark:text-gray-400';
    }
  }
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div>
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">Security</h2>
    <p class="text-gray-600 dark:text-gray-400 mt-1">Monitor security violations and threat detection</p>
  </div>
  
  <!-- Security Status Alert -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-4">
        <div class="w-16 h-16 rounded-full bg-gradient-to-br {
          securityStatus === 'secure' ? 'from-green-400 to-green-600' :
          securityStatus === 'warning' ? 'from-yellow-400 to-yellow-600' :
          'from-red-400 to-red-600'
        } flex items-center justify-center shadow-lg">
          <span class="text-2xl text-white">
            {securityStatus === 'secure' ? '🛡️' :
             securityStatus === 'warning' ? '⚠️' : '🚨'}
          </span>
        </div>
        <div>
          <h3 class="text-xl font-semibold {getStatusColor(securityStatus)} capitalize">
            {securityStatus} Status
          </h3>
          <p class="text-gray-600 dark:text-gray-400 mt-1">
            {securityStatus === 'secure' ? 'No active security threats detected' :
             securityStatus === 'warning' ? 'Minor security events detected' :
             'Multiple security violations detected'}
          </p>
        </div>
      </div>
      <div class="text-right">
        <div class="text-sm text-gray-600 dark:text-gray-400">Total Violations</div>
        <div class="text-2xl font-bold text-gray-900 dark:text-white">{securityViolations}</div>
      </div>
    </div>
  </div>
  
  <!-- Security Metrics Grid -->
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between mb-4">
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Rate Limit Violations</h4>
        <span class="text-xl">🚦</span>
      </div>
      <div class="text-3xl font-bold text-gray-900 dark:text-white">{rateLimitViolations}</div>
      <div class="text-sm text-gray-600 dark:text-gray-400 mt-2">
        Requests exceeding rate limits
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between mb-4">
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Size Limit Violations</h4>
        <span class="text-xl">📏</span>
      </div>
      <div class="text-3xl font-bold text-gray-900 dark:text-white">{sizeLimitViolations}</div>
      <div class="text-sm text-gray-600 dark:text-gray-400 mt-2">
        Oversized request attempts
      </div>
    </div>
    
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm p-6 border border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between mb-4">
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Blocked IPs</h4>
        <span class="text-xl">🚫</span>
      </div>
      <div class="text-3xl font-bold text-gray-900 dark:text-white">{blockedIPs}</div>
      <div class="text-sm text-gray-600 dark:text-gray-400 mt-2">
        Currently blocked addresses
      </div>
    </div>
  </div>
  
  <!-- Security Radar Visualization -->
  <div>
    <SecurityRadar />
  </div>
  
  <!-- Security Configuration -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Security Configuration</h3>
    </div>
    <div class="p-6">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Rate Limits</h4>
          <div class="space-y-3">
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Global Requests/min</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">1000</span>
            </div>
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Per IP Requests/min</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">100</span>
            </div>
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Burst Size</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">50</span>
            </div>
          </div>
        </div>
        
        <div>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Request Limits</h4>
          <div class="space-y-3">
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Max Body Size</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">50 MB</span>
            </div>
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Max Header Size</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">512 KB</span>
            </div>
            <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
              <span class="text-sm text-gray-600 dark:text-gray-400">Health Check Limit</span>
              <span class="font-mono text-sm text-gray-900 dark:text-white">1000/min</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Recent Violations -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
    <div class="p-6 border-b border-gray-200 dark:border-gray-700">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Recent Security Events</h3>
    </div>
    <div class="p-6">
      {#if securityViolations === 0}
        <div class="text-center py-8">
          <span class="text-3xl mb-2 block">🛡️</span>
          <p class="text-gray-500 dark:text-gray-400">No security violations detected</p>
        </div>
      {:else}
        <div class="text-center py-8">
          <span class="text-3xl mb-2 block">📋</span>
          <p class="text-gray-500 dark:text-gray-400">Security event log coming soon</p>
        </div>
      {/if}
    </div>
  </div>
</div>