import { api } from '$lib/services/api.js';

// Dashboard store using Svelte 5 runes
class DashboardStore {
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
  
  get endpointsUp() {
    if (!this.status?.endpoints) return { up: 0, total: 0 };
    const up = this.status.endpoints.filter(e => e.status === 'healthy').length;
    return { up, total: this.status.endpoints.length };
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
  
  // Methods
  async fetchStatus() {
    this.loading.status = true;
    this.errors.status = null;
    
    try {
      this.status = await api.getStatus();
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
      this.endpoints = await api.getEndpoints();
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
      this.unifiedModels = response.unified_models || [];
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
  
  setWebSocketConnected(connected) {
    this.wsConnected = connected;
  }
  
  clearEvents() {
    this.events = [];
  }
  
  // Lifecycle
  init() {
    // Start auto-refresh for critical data
    this.startAutoRefresh('status', this.fetchStatus, 5000); // 5 seconds
    this.startAutoRefresh('modelStats', this.fetchModelStats, 10000); // 10 seconds
    this.startAutoRefresh('processStats', this.fetchProcessStats, 30000); // 30 seconds
  }
  
  destroy() {
    this.stopAllAutoRefresh();
  }
}

// Export singleton instance
export const dashboardStore = new DashboardStore();