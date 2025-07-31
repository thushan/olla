<script>
  import { onMount } from 'svelte';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive references
  const status = $derived(dashboardStore.status);
  const events = $derived(dashboardStore.events);
  
  // Security metrics
  const securityMetrics = $derived(() => {
    if (!status || !status.system) {
      return {
        violations: 0,
        blockedRequests: 0,
        rateLimitHits: 0,
        suspiciousPatterns: 0,
      };
    }
    
    // Calculate from events
    const recentEvents = events.filter(e => {
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
  const threatLevel = $derived(() => {
    const metrics = securityMetrics();
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
  const securityEvents = $derived(() => {
    return events
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

<div class="security-center">
  <div class="center-header">
    <div class="header-content">
      <h3 class="text-lg font-semibold text-primary">Security Command Center</h3>
      <div class="threat-indicator threat-{threatLevel().level}">
        <span class="threat-icon">🛡️</span>
        <span class="threat-label">Threat Level: {threatLevel().label}</span>
      </div>
    </div>
  </div>
  
  <div class="security-content">
    <!-- Metrics Grid -->
    <div class="metrics-grid">
      <div class="metric-card">
        <div class="metric-icon">🚨</div>
        <div class="metric-details">
          <div class="metric-value">{securityMetrics().violations}</div>
          <div class="metric-label">Security Violations</div>
        </div>
      </div>
      
      <div class="metric-card">
        <div class="metric-icon">🚫</div>
        <div class="metric-details">
          <div class="metric-value">{securityMetrics().blockedRequests}</div>
          <div class="metric-label">Blocked Requests</div>
        </div>
      </div>
      
      <div class="metric-card">
        <div class="metric-icon">⏱️</div>
        <div class="metric-details">
          <div class="metric-value">{securityMetrics().rateLimitHits}</div>
          <div class="metric-label">Rate Limit Hits</div>
        </div>
      </div>
      
      <div class="metric-card">
        <div class="metric-icon">👁️</div>
        <div class="metric-details">
          <div class="metric-value">{securityMetrics().suspiciousPatterns}</div>
          <div class="metric-label">Suspicious Patterns</div>
        </div>
      </div>
    </div>
    
    <!-- Rate Limits Status -->
    <div class="rate-limits-section">
      <h4 class="section-title">Rate Limit Status</h4>
      <div class="rate-limits-grid">
        <div class="rate-limit-item">
          <div class="limit-header">
            <span class="limit-name">Global Limit</span>
            <span class="limit-config">{rateLimitConfig.global.limit}/{rateLimitConfig.global.window}</span>
          </div>
          <div class="limit-progress">
            <div class="progress-bar">
              <div 
                class="progress-fill"
                style="width: {(rateLimitConfig.global.current / rateLimitConfig.global.limit) * 100}%"
              ></div>
            </div>
            <span class="limit-usage">{rateLimitConfig.global.current} / {rateLimitConfig.global.limit}</span>
          </div>
        </div>
        
        <div class="rate-limit-item">
          <div class="limit-header">
            <span class="limit-name">Per IP Limit</span>
            <span class="limit-config">{rateLimitConfig.perIP.limit}/{rateLimitConfig.perIP.window}</span>
          </div>
          <div class="limit-progress">
            <div class="progress-bar">
              <div 
                class="progress-fill"
                style="width: {(rateLimitConfig.perIP.current / rateLimitConfig.perIP.limit) * 100}%"
              ></div>
            </div>
            <span class="limit-usage">{rateLimitConfig.perIP.current} / {rateLimitConfig.perIP.limit}</span>
          </div>
        </div>
        
        <div class="rate-limit-item">
          <div class="limit-header">
            <span class="limit-name">Health Check Limit</span>
            <span class="limit-config">{rateLimitConfig.health.limit}/{rateLimitConfig.health.window}</span>
          </div>
          <div class="limit-progress">
            <div class="progress-bar">
              <div 
                class="progress-fill"
                style="width: {(rateLimitConfig.health.current / rateLimitConfig.health.limit) * 100}%"
              ></div>
            </div>
            <span class="limit-usage">{rateLimitConfig.health.current} / {rateLimitConfig.health.limit}</span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- Security Events -->
    <div class="events-section">
      <h4 class="section-title">Recent Security Events</h4>
      {#if securityEvents().length === 0}
        <div class="no-events">
          <span class="no-events-icon">✅</span>
          <p class="no-events-text">No security events in the last 5 minutes</p>
        </div>
      {:else}
        <div class="security-events-list">
          {#each securityEvents() as event}
            {@const formatted = formatSecurityEvent(event)}
            {#if formatted}
              <div class="security-event severity-{formatted.severity}">
                <span class="event-icon">{formatted.icon}</span>
                <div class="event-content">
                  <p class="event-message">{formatted.message}</p>
                  <span class="event-time">{formatted.time}</span>
                </div>
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .security-center {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    border: 1px solid var(--bg-tertiary);
    overflow: hidden;
    border-color: var(--bg-tertiary);
  }
  
  .center-header {
    padding-left: 1.5rem; padding-right: 1.5rem;
    padding-top: 1rem; padding-bottom: 1rem;
    border-bottom: 1px solid var(--bg-tertiary);
    border-color: var(--bg-tertiary);
  }
  
  .header-content {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .threat-indicator {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding-left: 0.75rem; padding-right: 0.75rem;
    padding-top: 0.25rem; padding-bottom: 0.25rem;
    border-radius: 9999px;
    font-size: 0.875rem; line-height: 1.25rem;
    font-weight: 500;
  }
  
  .threat-low {
    background-color: rgba(var(--color-green-rgb, 8, 145, 106), 0.1);
    color: var(--color-green);
  }
  
  .threat-medium {
    background-color: rgba(var(--color-yellow-rgb, 201, 103, 101), 0.1);
    color: var(--color-yellow);
  }
  
  .threat-high {
    background-color: rgba(var(--color-orange-rgb, 170, 93, 0), 0.1);
    color: var(--color-orange);
  }
  
  .threat-critical {
    background-color: rgba(var(--color-red-rgb, 230, 65, 0), 0.1);
    color: var(--color-red);
  }
  
  .security-content {
    padding: 1.5rem;
    /* Child spacing handled by gap */
  }
  
  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  
  @media (min-width: 1024px) {
    .metrics-grid {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }
  
  .metric-card {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    display: flex;
    align-items: center;
    gap: 1rem;
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .metric-icon {
    font-size: 1.5rem; line-height: 2rem;
  }
  
  .metric-details {
    flex: 1 1 0%;
  }
  
  .metric-value {
    font-size: 1.5rem; line-height: 2rem;
    font-weight: 700;
    color: var(--text-primary);
  }
  
  .metric-label {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .rate-limits-section {
    /* Child spacing handled by gap */
  }
  
  .section-title {
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 0.75rem;
  }
  
  .rate-limits-grid {
    /* Child spacing handled by gap */
  }
  
  .rate-limit-item {
    padding: 1rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .limit-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  
  .limit-name {
    font-weight: 500;
    color: var(--text-primary);
  }
  
  .limit-config {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-secondary);
    font-family: 'JetBrains Mono', 'Consolas', 'Monaco', monospace;
  }
  
  .limit-progress {
    /* Child spacing handled by gap */
  }
  
  .progress-bar {
    width: 100%;
    height: 0.5rem;
    border-radius: 9999px;
    overflow: hidden;
    background-color: var(--bg-tertiary);
  }
  
  .progress-fill {
    height: 100%;
    border-radius: 9999px;
    transition: all;
    transition-duration: 500ms;
    background-color: var(--color-blue);
  }
  
  .limit-usage {
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-secondary);
  }
  
  .events-section {
    /* Child spacing handled by gap */
  }
  
  .no-events {
    text-align: center;
    padding-top: 2rem; padding-bottom: 2rem;
  }
  
  .no-events-icon {
    font-size: 1.5rem; line-height: 2rem;
    display: block;
    margin-bottom: 0.5rem;
  }
  
  .no-events-text {
    color: var(--text-secondary);
  }
  
  .security-events-list {
    /* Child spacing handled by gap */
  }
  
  .security-event {
    padding: 0.75rem;
    border-radius: 0.5rem;
    border: 1px solid var(--bg-tertiary);
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    background-color: var(--bg-primary);
    border-color: var(--bg-tertiary);
  }
  
  .severity-low {
    border-left-width: 3px;
    border-left-color: var(--color-blue);
  }
  
  .severity-medium {
    border-left-width: 3px;
    border-left-color: var(--color-yellow);
  }
  
  .severity-high {
    border-left-width: 3px;
    border-left-color: var(--color-red);
  }
  
  .event-icon {
    font-size: 1.125rem; line-height: 1.75rem;
  }
  
  .event-content {
    flex: 1 1 0%;
  }
  
  .event-message {
    font-size: 0.875rem; line-height: 1.25rem;
    color: var(--text-primary);
  }
  
  .event-time {
    font-size: 0.75rem; line-height: 1rem;
    color: var(--text-muted);
  }
</style>