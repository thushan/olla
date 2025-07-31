<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { onMount } from 'svelte';
  
  let healthPercentage = $derived(
    dashboardStore.endpointsUp.total > 0 
      ? (dashboardStore.endpointsUp.up / dashboardStore.endpointsUp.total) * 100 
      : 0
  );
  
  let healthStatus = $derived(
    healthPercentage >= 90 ? 'healthy' : 
    healthPercentage >= 50 ? 'degraded' : 
    'critical'
  );
  
  let healthColor = $derived(
    healthStatus === 'healthy' ? 'text-success' :
    healthStatus === 'degraded' ? 'text-warning' :
    'text-error'
  );
  
  // Animated counter for metrics
  function animateValue(start, end, duration) {
    let startTimestamp = null;
    const step = (timestamp) => {
      if (!startTimestamp) startTimestamp = timestamp;
      const progress = Math.min((timestamp - startTimestamp) / duration, 1);
      const current = Math.floor(progress * (end - start) + start);
      return current;
    };
    return step;
  }
  
  // Format large numbers
  function formatNumber(num) {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
  }
  
  onMount(() => {
    dashboardStore.init();
    return () => dashboardStore.destroy();
  });
</script>

<div class="hero-status">
  <!-- Main health indicator -->
  <div class="health-ring-container">
    <div class="health-ring">
      <svg class="health-svg" viewBox="0 0 200 200">
        <!-- Background circle -->
        <circle
          cx="100"
          cy="100"
          r="90"
          fill="none"
          stroke="currentColor"
          stroke-width="12"
          class="text-tertiary opacity-20"
        />
        <!-- Progress circle -->
        <circle
          cx="100"
          cy="100"
          r="90"
          fill="none"
          stroke="currentColor"
          stroke-width="12"
          stroke-linecap="round"
          class={healthColor}
          stroke-dasharray={`${healthPercentage * 5.65} 565`}
          stroke-dashoffset="0"
          transform="rotate(-90 100 100)"
          style="transition: stroke-dasharray 1s ease-in-out"
        />
      </svg>
      <div class="health-content">
        <div class="health-percentage metric-value text-5xl font-bold {healthColor}">
          {Math.round(healthPercentage)}%
        </div>
        <div class="health-label text-secondary">
          System Health
        </div>
        <div class="health-status text-sm font-medium {healthColor}">
          {healthStatus.toUpperCase()}
        </div>
      </div>
    </div>
  </div>
  
  <!-- Metric cards -->
  <div class="metrics-grid">
    <!-- Requests/sec -->
    <div class="metric-card gradient-primary">
      <div class="metric-icon">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
        </svg>
      </div>
      <div class="metric-content">
        <div class="metric-value text-2xl font-bold text-white">
          {formatNumber(dashboardStore.totalRequests || 0)}
        </div>
        <div class="metric-label text-white/80">Total Requests</div>
        <div class="metric-sparkline">
          <!-- Placeholder for sparkline -->
          <svg class="w-full h-8" viewBox="0 0 100 32">
            <path
              d="M0,16 L10,12 L20,18 L30,8 L40,14 L50,10 L60,20 L70,15 L80,12 L90,18 L100,14"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              class="text-white/50"
            />
          </svg>
        </div>
      </div>
    </div>
    
    <!-- Success Rate -->
    <div class="metric-card gradient-success">
      <div class="metric-icon">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
        </svg>
      </div>
      <div class="metric-content">
        <div class="metric-value text-2xl font-bold text-white">
          {dashboardStore.successRate || '0%'}
        </div>
        <div class="metric-label text-white/80">Success Rate</div>
        <div class="metric-trend text-sm text-white/60">
          <span class="text-white">↑ 2.3%</span> from last hour
        </div>
      </div>
    </div>
    
    <!-- Active Connections -->
    <div class="metric-card bg-secondary">
      <div class="metric-icon">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0"></path>
        </svg>
      </div>
      <div class="metric-content">
        <div class="metric-value text-2xl font-bold">
          {formatNumber(dashboardStore.activeConnections || 0)}
        </div>
        <div class="metric-label text-secondary">
          Active Connections
        </div>
        <div class="connection-bar">
          <div class="connection-fill" style="width: {Math.min(dashboardStore.activeConnections / 100 * 100, 100)}%"></div>
        </div>
      </div>
    </div>
    
    <!-- Security Status -->
    <div class="metric-card {dashboardStore.securityViolations > 0 ? 'gradient-warning' : 'bg-secondary'}">
      <div class="metric-icon">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"></path>
        </svg>
      </div>
      <div class="metric-content">
        <div class="metric-value text-2xl font-bold {dashboardStore.securityViolations > 0 ? 'text-white' : ''}">
          {formatNumber(dashboardStore.securityViolations || 0)}
        </div>
        <div class="metric-label {dashboardStore.securityViolations > 0 ? 'text-white/80' : 'text-secondary'}">
          Security Violations
        </div>
        <div class="metric-status text-sm {dashboardStore.securityViolations > 0 ? 'text-white' : 'text-success'}">
          {dashboardStore.securityViolations > 0 ? 'ACTIVE THREATS' : 'SECURE'}
        </div>
      </div>
    </div>
  </div>
</div>

<style>
  .hero-status {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 1.5rem;
    margin-bottom: 2rem;
  }
  
  @media (min-width: 1024px) {
    .hero-status {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  
  .health-ring-container {
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .health-ring {
    position: relative;
    width: 12rem;
    height: 12rem;
  }
  
  @media (min-width: 1024px) {
    .health-ring {
      width: 14rem;
      height: 14rem;
    }
  }
  
  .health-svg {
    width: 100%;
    height: 100%;
  }
  
  .health-content {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    left: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
  }
  
  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 640px) {
    .metrics-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  
  .metric-card {
    border-radius: 0.75rem;
    padding: 1.5rem;
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 300ms;
    position: relative;
    overflow: hidden;
  }
  
  .metric-card:hover {
    transform: scale(1.05);
  }
  
  .metric-card::before {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    left: 0;
    opacity: 0;
    transition-property: opacity;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 300ms;
    background: radial-gradient(circle at center, rgba(255,255,255,0.1) 0%, transparent 70%);
  }
  
  .metric-card:hover::before {
    opacity: 1;
  }
  
  .metric-icon {
    margin-bottom: 0.75rem;
    color: rgba(255, 255, 255, 0.8);
  }
  
  .metric-content {
    position: relative;
    z-index: 10;
  }
  
  .connection-bar {
    margin-top: 0.5rem;
    height: 0.5rem;
    border-radius: 9999px;
    overflow: hidden;
    background-color: rgba(0, 0, 0, 0.2);
  }
  
  .dark .connection-bar {
    background-color: rgba(255, 255, 255, 0.2);
  }
  
  .connection-fill {
    height: 100%;
    transition-property: all;
    transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
    transition-duration: 500ms;
    background-color: var(--color-blue);
  }
  
  /* Pulse animation for critical status */
  @keyframes pulse-critical {
    0%, 100% {
      opacity: 1;
    }
    50% {
      opacity: 0.5;
    }
  }
</style>