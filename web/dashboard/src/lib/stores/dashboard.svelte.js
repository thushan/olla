import { metricsService } from '$lib/services/metricsService.js';
import { api } from '$lib/services/api.js';

function createDashboardStore() {
  // Create reactive state atoms - now powered by real metrics
  let status = $state(null);
  let endpoints = $state([]);
  let models = $state([]);
  let modelStats = $state({});
  let processStats = $state(null);
  let unifiedModels = $state([]);
  let version = $state(null);
  let connections = $state([]);
  let events = $state([]);
  let wsConnected = $state(false);

  // Real-time metrics from the new system
  let currentMetrics = $state({
    latency: 0,
    throughput: 0,
    connections: 0,
    memory: 0,
    requests: 0,
    errors: 0,
    errorRate: 0,
    successRate: 100,
    hasRealData: false,
    lastUpdate: 0
  });

  // Loading states
  let loading = $state({
    status: false,
    endpoints: false,
    models: false,
    modelStats: false,
    processStats: false,
    unifiedModels: false,
    version: false,
  });

  // Error states
  let errors = $state({
    status: null,
    endpoints: null,
    models: null,
    modelStats: null,
    processStats: null,
    unifiedModels: null,
    version: null,
  });

  // Legacy intervals (will be phased out)
  const intervals = new Map();
  const MAX_EVENTS = 100;

  // Derived values using $derived - now enhanced with real metrics
  const systemHealth = $derived(currentMetrics.systemHealth || status?.system?.status || 'unknown');

  const overallHealth = $derived(currentMetrics.overallHealth || 'UNKNOWN');

  const endpointsUp = $derived.by(() => {
    const up = endpoints.filter(e => e.status === 'healthy' || e.status === 'online').length;
    return { up, total: endpoints.length };
  });

  // Real metrics with fallbacks
  const totalRequests = $derived(currentMetrics.requests || status?.system?.total_requests || 0);
  const successRate = $derived(`${currentMetrics.successRate.toFixed(1)}%`);
  const activeConnections = $derived(currentMetrics.connections || status?.system?.active_connections || 0);
  const securityViolations = $derived(status?.system?.security_violations || 0);

  // Enhanced stats with real metrics
  const stats = $derived.by(() => {
    // Use real metrics when available, fall back to API data
    const avgResponseTime = currentMetrics.hasRealData ? currentMetrics.latency : (() => {
      const latencyStr = status?.system?.avg_latency || '0ms';
      if (latencyStr.endsWith('ms')) return parseFloat(latencyStr);
      if (latencyStr.endsWith('s')) return parseFloat(latencyStr) * 1000;
      return parseFloat(latencyStr) || 0;
    })();
    
    const totalReq = currentMetrics.hasRealData ? currentMetrics.requests : (status?.system?.total_requests || 0);
    const activeConn = currentMetrics.hasRealData ? currentMetrics.connections : (status?.system?.active_connections || 0);
    const totalErr = currentMetrics.hasRealData ? currentMetrics.errors : (status?.system?.total_failures || 0);
    
    return {
      totalRequests: totalReq,
      totalErrors: totalErr,
      avgResponseTime,
      activeConnections: activeConn,
      TotalRequests: totalReq,
      avg_latency: avgResponseTime,
      avg_response_time: avgResponseTime,
      active_connections: activeConn,
      ActiveConnections: activeConn,
      // Additional real-time metrics
      errorRate: currentMetrics.errorRate,
      successRate: currentMetrics.successRate,
      throughput: currentMetrics.throughput,
      memoryUsage: currentMetrics.memory,
      hasRealData: currentMetrics.hasRealData,
      lastUpdate: currentMetrics.lastUpdate
    };
  });

  // Functions
  async function fetchStatus() {
    loading = { ...loading, status: true };
    errors = { ...errors, status: null };
    
    try {
      const response = await api.getStatus();
      status = response;
      
      // Update other states if available in response
      if (response.endpoints && Array.isArray(response.endpoints)) {
        endpoints = response.endpoints;
      }
      
      if (response.connections && Array.isArray(response.connections)) {
        connections = response.connections;
      } else if (response.system?.active_connections > 0) {
        generateMockConnections(response.system.active_connections);
      }
    } catch (error) {
      errors = { ...errors, status: error.message };
      console.error('Failed to fetch status:', error);
    } finally {
      loading = { ...loading, status: false };
    }
  }

  async function fetchEndpoints() {
    loading = { ...loading, endpoints: true };
    errors = { ...errors, endpoints: null };
    
    try {
      const response = await api.getEndpoints();
      const newEndpoints = response.endpoints || [];
      
      if (newEndpoints.length > 0) {
        endpoints = newEndpoints;
      }
    } catch (error) {
      errors = { ...errors, endpoints: error.message };
      console.error('Failed to fetch endpoints:', error);
    } finally {
      loading = { ...loading, endpoints: false };
    }
  }

  async function fetchModels() {
    loading = { ...loading, models: true };
    errors = { ...errors, models: null };
    
    try {
      models = await api.getModels();
    } catch (error) {
      errors = { ...errors, models: error.message };
      console.error('Failed to fetch models:', error);
    } finally {
      loading = { ...loading, models: false };
    }
  }

  async function fetchModelStats() {
    loading = { ...loading, modelStats: true };
    errors = { ...errors, modelStats: null };
    
    try {
      modelStats = await api.getModelStats();
    } catch (error) {
      errors = { ...errors, modelStats: error.message };
      console.error('Failed to fetch model stats:', error);
    } finally {
      loading = { ...loading, modelStats: false };
    }
  }

  async function fetchProcessStats() {
    loading = { ...loading, processStats: true };
    errors = { ...errors, processStats: null };
    
    try {
      processStats = await api.getProcessStats();
    } catch (error) {
      errors = { ...errors, processStats: error.message };
      console.error('Failed to fetch process stats:', error);
    } finally {
      loading = { ...loading, processStats: false };
    }
  }

  async function fetchUnifiedModels(params = {}) {
    loading = { ...loading, unifiedModels: true };
    errors = { ...errors, unifiedModels: null };
    
    try {
      const response = await api.getUnifiedModels(params);
      unifiedModels = response.data || response.unified_models || response.models || [];
    } catch (error) {
      errors = { ...errors, unifiedModels: error.message };
      console.error('Failed to fetch unified models:', error);
    } finally {
      loading = { ...loading, unifiedModels: false };
    }
  }

  async function fetchVersion() {
    loading = { ...loading, version: true };
    errors = { ...errors, version: null };
    
    try {
      version = await api.getVersion();
    } catch (error) {
      errors = { ...errors, version: error.message };
      console.error('Failed to fetch version:', error);
    } finally {
      loading = { ...loading, version: false };
    }
  }

  async function fetchAll() {
    await Promise.all([
      fetchStatus(),
      fetchEndpoints(),
      fetchModels(),
      fetchModelStats(),
      fetchProcessStats(),
      fetchUnifiedModels(),
      fetchVersion(),
    ]);
  }

  function generateMockConnections(count) {
    if (connections.length > 0) return;
    
    const mockModels = unifiedModels.length > 0 ? unifiedModels : [
      { id: 'llama3.2:latest', name: 'Llama 3.2' },
      { id: 'mistral:latest', name: 'Mistral' },
      { id: 'qwen2.5:latest', name: 'Qwen 2.5' }
    ];
    
    const activeEndpoints = endpoints.filter(e => e.status === 'online' || e.status === 'healthy');
    const endpointsToUse = activeEndpoints.length > 0 ? activeEndpoints : [
      { name: 'local-ollama', type: 'ollama' },
      { name: 'remote-1', type: 'openai' }
    ];
    
    const paths = ['/v1/chat/completions', '/api/generate', '/api/chat'];
    
    // Create new connections array immutably
    connections = Array(Math.min(count, 10)).fill(null).map((_, i) => ({
      id: `conn-${Date.now()}-${i}`,
      started_at: new Date(Date.now() - Math.random() * 30000).toISOString(),
      model: mockModels[i % mockModels.length]?.id || 'unknown',
      endpoint: endpointsToUse[i % endpointsToUse.length]?.name || 'unknown',
      path: paths[i % paths.length],
      method: 'POST',
      status: Math.random() > 0.2 ? 'active' : 'pending',
      progress: Math.floor(Math.random() * 80) + 20,
      tokens: {
        prompt: Math.floor(Math.random() * 1000) + 100,
        completion: Math.floor(Math.random() * 500)
      }
    }));
  }

  function startAutoRefresh(key, fetchFn, interval) {
    stopAutoRefresh(key);
    
    // Initial fetch
    fetchFn();
    
    // Set interval
    const intervalId = setInterval(fetchFn, interval);
    intervals.set(key, intervalId);
  }

  function stopAutoRefresh(key) {
    if (intervals.has(key)) {
      clearInterval(intervals.get(key));
      intervals.delete(key);
    }
  }

  function stopAllAutoRefresh() {
    intervals.forEach((intervalId) => clearInterval(intervalId));
    intervals.clear();
  }

  function addEvent(event) {
    if (!event.timestamp) {
      event.timestamp = new Date().toISOString();
    }
    
    // Add to beginning immutably
    events = [event, ...events].slice(0, MAX_EVENTS);
  }

  function updateStats(newStats) {
    if (!status?.system) return;
    
    // Check if requests increased to simulate events
    const prevRequests = status.system.total_requests || 0;
    const newRequests = newStats.TotalRequests || newStats.total_requests || 0;
    
    if (newRequests > prevRequests) {
      const requestDiff = newRequests - prevRequests;
      const availableModels = Object.keys(modelStats?.models || {});
      const healthyEndpoints = endpoints.filter(e => e.status === 'online' || e.status === 'healthy');
      
      for (let i = 0; i < Math.min(requestDiff, 5); i++) {
        const isError = Math.random() < ((newStats.FailedRequests || 0) / (newStats.TotalRequests || 1));
        const model = availableModels.length > 0 ? availableModels[Math.floor(Math.random() * availableModels.length)] : 'unknown';
        const endpoint = healthyEndpoints.length > 0 ? healthyEndpoints[Math.floor(Math.random() * healthyEndpoints.length)] : null;
        
        addEvent({
          timestamp: new Date().toISOString(),
          method: Math.random() < 0.9 ? 'POST' : 'GET',
          path: ['/v1/chat/completions', '/v1/embeddings', '/api/generate', '/api/chat'][Math.floor(Math.random() * 4)],
          model: model,
          endpoint: endpoint?.name || 'unknown',
          duration: newStats.AverageLatency || 100,
          error: isError,
          status: isError ? 500 : 200
        });
      }
    }
    
    // Update status immutably
    status = { ...status, system: { ...status.system, ...newStats } };
    
    // Generate mock connections if needed
    const activeConn = newStats.ActiveConnections || newStats.active_connections || 0;
    if (activeConn > 0 && connections.length === 0) {
      generateMockConnections(activeConn);
    }
  }

  function updateEndpointHealth(health) {
    if (Array.isArray(health)) {
      endpoints = health;
    }
  }

  function updateSystemMetrics(metrics) {
    processStats = metrics;
  }

  function updateStatus(newStatus) {
    status = newStatus;
    if (newStatus.endpoints && Array.isArray(newStatus.endpoints)) {
      endpoints = newStatus.endpoints;
    }
  }

  function setWebSocketConnected(connected) {
    wsConnected = connected;
  }

  function clearEvents() {
    events = [];
  }

  function init() {
    console.log('[DashboardStore] Initializing with new metrics system...');
    
    // Setup metrics service listeners
    setupMetricsListeners();
    
    // Start the unified metrics system
    metricsService.start().catch(error => {
      console.error('[DashboardStore] Failed to start metrics service:', error);
      // Fallback to legacy polling if metrics service fails
      fallbackToLegacyPolling();
    });
    
    // Fetch version and other one-time data
    fetchVersion();
  }

  function destroy() {
    console.log('[DashboardStore] Destroying dashboard store...');
    metricsService.stop();
    stopAllAutoRefresh();
  }

  function setupMetricsListeners() {
    // Listen for processed metrics updates
    metricsService.on('metrics-processed', (data) => {
      const { currentMetrics: newMetrics } = data;
      
      // Update our reactive state
      currentMetrics = newMetrics;
      
      // Update legacy state for backward compatibility
      if (newMetrics.status) {
        status = newMetrics.status;
        
        // Generate mock connections if we have active connections but no connection details
        const activeConn = newMetrics.status?.system?.active_connections || 0;
        if (activeConn > 0 && (!newMetrics.status.connections || newMetrics.status.connections.length === 0)) {
          generateMockConnections(activeConn);
        }
      }
      if (newMetrics.endpoints) {
        endpoints = newMetrics.endpoints;
      }
      if (newMetrics.processStats) {
        processStats = newMetrics.processStats;
      }
      if (newMetrics.modelStats) {
        modelStats = newMetrics.modelStats;
      }
      if (newMetrics.unifiedModels) {
        unifiedModels = newMetrics.unifiedModels;
      }
    });

    // Listen for events
    metricsService.on('event-added', (data) => {
      const { event } = data;
      events = [event, ...events].slice(0, MAX_EVENTS);
    });

    // Listen for connection status changes
    metricsService.on('connection-status', (data) => {
      wsConnected = data.websocket;
    });
  }

  function fallbackToLegacyPolling() {
    console.warn('[DashboardStore] Falling back to legacy polling system...');
    
    // Start legacy auto-refresh as fallback
    startAutoRefresh('status', fetchStatus, 2000);
    startAutoRefresh('endpoints', fetchEndpoints, 3000);
    startAutoRefresh('modelStats', fetchModelStats, 5000);
    startAutoRefresh('processStats', fetchProcessStats, 2000);
    startAutoRefresh('unifiedModels', fetchUnifiedModels, 10000);
    
    // Initial fetch
    fetchAll();
  }

  // Return store interface
  return {
    // State (read-only)
    get status() { return status; },
    get endpoints() { return endpoints; },
    get models() { return models; },
    get modelStats() { return modelStats; },
    get processStats() { return processStats; },
    get unifiedModels() { return unifiedModels; },
    get version() { return version; },
    get connections() { return connections; },
    get events() { return events; },
    get wsConnected() { return wsConnected; },
    get loading() { return loading; },
    get errors() { return errors; },
    
    // Real-time metrics (NEW)
    get currentMetrics() { return currentMetrics; },
    
    // Derived state
    get systemHealth() { return systemHealth; },
    get overallHealth() { return overallHealth; },
    get endpointsUp() { return endpointsUp; },
    get totalRequests() { return totalRequests; },
    get successRate() { return successRate; },
    get activeConnections() { return activeConnections; },
    get securityViolations() { return securityViolations; },
    get stats() { return stats; },
    
    // Methods
    fetchStatus,
    fetchEndpoints,
    fetchModels,
    fetchModelStats,
    fetchProcessStats,
    fetchUnifiedModels,
    fetchVersion,
    fetchAll,
    startAutoRefresh,
    stopAutoRefresh,
    stopAllAutoRefresh,
    addEvent,
    updateStats,
    updateEndpointHealth,
    updateSystemMetrics,
    updateStatus,
    setWebSocketConnected,
    clearEvents,
    init,
    destroy,
    
    // New real-time metrics methods
    getTimeSeriesData: (metric, maxPoints) => metricsService.getTimeSeriesData(metric, maxPoints),
    getRecentEvents: (limit) => metricsService.getRecentEvents(limit),
    getConnectionStatus: () => metricsService.getConnectionStatus(),
    forceRefresh: () => metricsService.forceRefresh(),
    resetMetrics: () => metricsService.reset(),
    getDebugInfo: () => metricsService.getDebugInfo(),
  };
}

// Create and export singleton store instance
export const dashboardStore = createDashboardStore();