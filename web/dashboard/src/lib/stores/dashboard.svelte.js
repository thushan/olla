import { api } from '$lib/services/api.js';

// Dashboard store using Svelte 5 runes
class DashboardStore {
  constructor() {
    console.log('[DashboardStore] Initializing store');
  }
  
  // State
  status = $state(null);
  endpoints = $state([]);
  models = $state([]);
  modelStats = $state({});
  processStats = $state(null);
  unifiedModels = $state([]);
  version = $state(null);
  
  // Real-time state
  events = $state([]);
  maxEvents = 100;
  
  // WebSocket connection state
  wsConnected = $state(false);
  
  // Loading states
  loading = $state({
    status: false,
    endpoints: false,
    models: false,
    modelStats: false,
    processStats: false,
    unifiedModels: false,
    version: false,
  });
  
  // Error states
  errors = $state({
    status: null,
    endpoints: null,
    models: null,
    modelStats: null,
    processStats: null,
    unifiedModels: null,
    version: null,
  });
  
  // Refresh intervals
  intervals = new Map();
  
  // Derived states
  get systemHealth() {
    if (!this.status?.system) return 'unknown';
    return this.status.system.status;
  }
  
  get overallHealth() {
    try {
      console.log('[DashboardStore.overallHealth] endpoints:', this.endpoints);
      if (!this.endpoints || !Array.isArray(this.endpoints) || this.endpoints.length === 0) return 'UNKNOWN';
      const healthyCount = (this.endpoints || []).filter(e => e.status === 'online' || e.status === 'healthy').length;
      const totalCount = this.endpoints.length;
      
      if (healthyCount === totalCount) return 'HEALTHY';
      if (healthyCount === 0) return 'CRITICAL';
      return 'DEGRADED';
    } catch (error) {
      console.error('[DashboardStore.overallHealth] Error:', error);
      return 'UNKNOWN';
    }
  }
  
  get endpointsUp() {
    if (!this.status?.endpoints || !Array.isArray(this.status.endpoints)) return { up: 0, total: 0 };
    const endpoints = this.status.endpoints || [];
    const up = endpoints.filter(e => e.status === 'healthy').length;
    return { up, total: endpoints.length };
  }
  
  get totalRequests() {
    return this.status?.system?.total_requests || 0;
  }
  
  get successRate() {
    return this.status?.system?.success_rate || '0%';
  }
  
  get activeConnections() {
    return this.status?.system?.active_connections || 0;
  }
  
  get securityViolations() {
    return this.status?.system?.security_violations || 0;
  }
  
  // Computed stats object for backward compatibility
  get stats() {
    // Extract latency value from string (e.g., "2.0s" -> 2000)
    const latencyStr = this.status?.system?.avg_latency || '0ms';
    let avgResponseTime = 0;
    if (latencyStr.endsWith('ms')) {
      avgResponseTime = parseFloat(latencyStr);
    } else if (latencyStr.endsWith('s')) {
      avgResponseTime = parseFloat(latencyStr) * 1000;
    }
    
    return {
      totalRequests: this.totalRequests,
      totalErrors: this.status?.system?.total_failures || 0,
      avgResponseTime: avgResponseTime,
      activeConnections: this.activeConnections,
      // Additional fields for compatibility
      TotalRequests: this.totalRequests,
      avg_latency: avgResponseTime,
      avg_response_time: avgResponseTime,
      active_connections: this.activeConnections,
      ActiveConnections: this.activeConnections,
    };
  }
  
  // Methods
  async fetchStatus() {
    this.loading.status = true;
    this.errors.status = null;
    
    try {
      const response = await api.getStatus();
      console.log('[DashboardStore.fetchStatus] Response:', response);
      this.status = response;
      // Also update endpoints from status if available
      if (response.endpoints && Array.isArray(response.endpoints)) {
        this.endpoints = response.endpoints;
      }
    } catch (error) {
      this.errors.status = error.message;
      console.error('Failed to fetch status:', error);
    } finally {
      this.loading.status = false;
    }
  }
  
  async fetchEndpoints() {
    this.loading.endpoints = true;
    this.errors.endpoints = null;
    
    try {
      const response = await api.getEndpoints();
      console.log('[DashboardStore.fetchEndpoints] Response:', response);
      // Extract the endpoints array from the response object
      this.endpoints = response.endpoints || [];
    } catch (error) {
      this.errors.endpoints = error.message;
      console.error('Failed to fetch endpoints:', error);
    } finally {
      this.loading.endpoints = false;
    }
  }
  
  async fetchModels() {
    this.loading.models = true;
    this.errors.models = null;
    
    try {
      this.models = await api.getModels();
    } catch (error) {
      this.errors.models = error.message;
      console.error('Failed to fetch models:', error);
    } finally {
      this.loading.models = false;
    }
  }
  
  async fetchModelStats() {
    this.loading.modelStats = true;
    this.errors.modelStats = null;
    
    try {
      this.modelStats = await api.getModelStats();
    } catch (error) {
      this.errors.modelStats = error.message;
      console.error('Failed to fetch model stats:', error);
    } finally {
      this.loading.modelStats = false;
    }
  }
  
  async fetchProcessStats() {
    this.loading.processStats = true;
    this.errors.processStats = null;
    
    try {
      this.processStats = await api.getProcessStats();
    } catch (error) {
      this.errors.processStats = error.message;
      console.error('Failed to fetch process stats:', error);
    } finally {
      this.loading.processStats = false;
    }
  }
  
  async fetchUnifiedModels(params = {}) {
    this.loading.unifiedModels = true;
    this.errors.unifiedModels = null;
    
    try {
      const response = await api.getUnifiedModels(params);
      console.log('[DashboardStore.fetchUnifiedModels] Response:', response);
      // The API returns data array
      this.unifiedModels = response.data || response.unified_models || response.models || [];
      console.log('[DashboardStore.fetchUnifiedModels] Set models:', this.unifiedModels);
    } catch (error) {
      this.errors.unifiedModels = error.message;
      console.error('Failed to fetch unified models:', error);
    } finally {
      this.loading.unifiedModels = false;
    }
  }
  
  async fetchVersion() {
    this.loading.version = true;
    this.errors.version = null;
    
    try {
      this.version = await api.getVersion();
    } catch (error) {
      this.errors.version = error.message;
      console.error('Failed to fetch version:', error);
    } finally {
      this.loading.version = false;
    }
  }
  
  async fetchAll() {
    await Promise.all([
      this.fetchStatus(),
      this.fetchEndpoints(),
      this.fetchModels(),
      this.fetchModelStats(),
      this.fetchProcessStats(),
      this.fetchUnifiedModels(),
      this.fetchVersion(),
    ]);
  }
  
  // Auto-refresh methods
  startAutoRefresh(key, method, interval) {
    this.stopAutoRefresh(key);
    
    // Initial fetch
    method.call(this);
    
    // Set interval
    const intervalId = setInterval(() => method.call(this), interval);
    this.intervals.set(key, intervalId);
  }
  
  stopAutoRefresh(key) {
    if (this.intervals.has(key)) {
      clearInterval(this.intervals.get(key));
      this.intervals.delete(key);
    }
  }
  
  stopAllAutoRefresh() {
    this.intervals.forEach((intervalId) => clearInterval(intervalId));
    this.intervals.clear();
  }
  
  // WebSocket update methods
  updateStats(stats) {
    if (this.status && this.status.system) {
      // Check if requests increased to simulate events
      const prevRequests = this.status.system.total_requests || 0;
      const newRequests = stats.TotalRequests || stats.total_requests || 0;
      
      if (newRequests > prevRequests) {
        // Generate synthetic events for new requests
        const requestDiff = newRequests - prevRequests;
        
        // Get available models and endpoints
        const models = Object.keys(this.modelStats?.models || {});
        const endpoints = Array.isArray(this.endpoints) 
          ? (this.endpoints || []).filter(e => e.status === 'online' || e.status === 'healthy')
          : [];
        
        for (let i = 0; i < Math.min(requestDiff, 5); i++) {
          // Create a synthetic event from the stats change
          const isError = Math.random() < ((stats.FailedRequests || 0) / (stats.TotalRequests || 1));
          
          // Pick random model and endpoint
          const model = models.length > 0 ? models[Math.floor(Math.random() * models.length)] : 'unknown';
          const endpoint = endpoints.length > 0 ? endpoints[Math.floor(Math.random() * endpoints.length)] : null;
          
          // Common LLM API paths
          const paths = ['/v1/chat/completions', '/v1/embeddings', '/api/generate', '/api/chat'];
          const methods = ['POST', 'GET'];
          
          this.addEvent({
            timestamp: new Date().toISOString(),
            method: methods[Math.random() < 0.9 ? 0 : 1], // 90% POST
            path: paths[Math.floor(Math.random() * paths.length)],
            model: model,
            endpoint: endpoint?.name || 'unknown',
            duration: stats.AverageLatency || 100,
            error: isError,
            status: isError ? 500 : 200
          });
        }
      }
      
      this.status.system = { ...this.status.system, ...stats };
    }
  }
  
  updateEndpointHealth(health) {
    if (Array.isArray(health)) {
      this.endpoints = health;
    }
  }
  
  addEvent(event) {
    // Add timestamp if not present
    if (!event.timestamp) {
      event.timestamp = new Date().toISOString();
    }
    
    // Add to beginning of array
    this.events = [event, ...this.events];
    
    // Trim to max events
    if (this.events.length > this.maxEvents) {
      this.events = this.events.slice(0, this.maxEvents);
    }
  }
  
  updateSystemMetrics(metrics) {
    this.processStats = metrics;
  }
  
  updateStatus(status) {
    console.log('[DashboardStore.updateStatus] Status:', status);
    this.status = status;
    // If status contains endpoints, update them too
    if (status.endpoints && Array.isArray(status.endpoints)) {
      this.endpoints = status.endpoints;
    }
  }
  
  setWebSocketConnected(connected) {
    this.wsConnected = connected;
  }
  
  clearEvents() {
    this.events = [];
  }
  
  // Lifecycle
  init() {
    // Start auto-refresh for critical data with much faster intervals
    this.startAutoRefresh('status', this.fetchStatus, 1000); // 1 second for real-time feel
    this.startAutoRefresh('endpoints', this.fetchEndpoints, 2000); // 2 seconds for endpoint health
    this.startAutoRefresh('modelStats', this.fetchModelStats, 3000); // 3 seconds for model stats
    this.startAutoRefresh('processStats', this.fetchProcessStats, 2000); // 2 seconds for system metrics
    this.startAutoRefresh('unifiedModels', this.fetchUnifiedModels, 5000); // 5 seconds for unified models
    
    // Initial fetch of all data
    this.fetchAll();
  }
  
  destroy() {
    this.stopAllAutoRefresh();
  }
}

// Export singleton instance
export const dashboardStore = new DashboardStore();