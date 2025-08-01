/**
 * Phase 1: Background Metrics Collector
 * Unified data collection with WebSocket + intelligent polling
 */

import { api } from './api.js';

export class MetricsCollector {
  constructor() {
    this.ws = null;
    this.listeners = new Map();
    this.intervals = new Map();
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5;
    this.baseReconnectDelay = 1000;
    this.isConnected = false;
    
    // Collection state
    this.lastData = {
      status: null,
      endpoints: [],
      processStats: null,
      modelStats: {},
      timestamp: Date.now()
    };
    
    // Smart polling configuration
    this.pollConfig = {
      status: { interval: 2000, enabled: true },
      endpoints: { interval: 3000, enabled: true },
      processStats: { interval: 1500, enabled: true },
      modelStats: { interval: 5000, enabled: true },
      unifiedModels: { interval: 10000, enabled: true }
    };
  }

  /**
   * Start the metrics collection system
   */
  async start() {
    console.log('[MetricsCollector] Starting collection system...');
    
    // Try WebSocket first, fallback to polling
    this.connectWebSocket();
    
    // Start smart polling as backup/supplement
    this.startSmartPolling();
    
    // Initial data fetch
    await this.fetchInitialData();
  }

  /**
   * Stop all collection
   */
  stop() {
    console.log('[MetricsCollector] Stopping collection system...');
    
    // Stop WebSocket
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    
    // Stop all polling
    this.intervals.forEach((intervalId) => clearInterval(intervalId));
    this.intervals.clear();
    
    this.isConnected = false;
  }

  /**
   * WebSocket connection for real-time updates
   */
  connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsUrl = `${protocol}//${host}/internal/ws`;
    
    try {
      this.ws = new WebSocket(wsUrl);
      
      this.ws.onopen = () => {
        console.log('[MetricsCollector] WebSocket connected');
        this.isConnected = true;
        this.reconnectAttempts = 0;
        
        // Subscribe to real-time updates
        this.sendWS({ type: 'subscribe', topics: ['stats', 'metrics', 'health', 'events'] });
        
        // Reduce polling frequency when WebSocket is active
        this.adjustPollingForWebSocket(true);
        
        this.emit('websocket-connected', { connected: true });
      };
      
      this.ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          this.handleWebSocketMessage(data);
        } catch (error) {
          console.error('[MetricsCollector] Failed to parse WebSocket message:', error);
        }
      };
      
      this.ws.onerror = (error) => {
        console.error('[MetricsCollector] WebSocket error:', error);
      };
      
      this.ws.onclose = () => {
        console.log('[MetricsCollector] WebSocket disconnected');
        this.isConnected = false;
        
        // Increase polling frequency when WebSocket is down
        this.adjustPollingForWebSocket(false);
        
        this.emit('websocket-connected', { connected: false });
        
        // Auto-reconnect
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
          const delay = this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts);
          this.reconnectAttempts++;
          
          console.log(`[MetricsCollector] Reconnecting WebSocket in ${delay}ms (attempt ${this.reconnectAttempts})`);
          setTimeout(() => this.connectWebSocket(), delay);
        }
      };
    } catch (error) {
      console.error('[MetricsCollector] Failed to create WebSocket:', error);
      this.adjustPollingForWebSocket(false);
    }
  }

  /**
   * Handle incoming WebSocket messages
   */
  handleWebSocketMessage(data) {
    const timestamp = Date.now();
    
    switch (data.type) {
      case 'stats':
      case 'metrics':
        this.updateLastData('status', { system: data.payload, timestamp });
        this.emit('metrics-update', { 
          type: 'system', 
          data: data.payload, 
          timestamp,
          source: 'websocket' 
        });
        break;
        
      case 'endpoint_health':
        this.updateLastData('endpoints', data.payload);
        this.emit('metrics-update', { 
          type: 'endpoints', 
          data: data.payload, 
          timestamp,
          source: 'websocket' 
        });
        break;
        
      case 'process_metrics':
      case 'system_metrics':
        this.updateLastData('processStats', data.payload);
        this.emit('metrics-update', { 
          type: 'process', 
          data: data.payload, 
          timestamp,
          source: 'websocket' 
        });
        break;
        
      case 'proxy_event':
        this.emit('event-received', { 
          event: data.payload, 
          timestamp,
          source: 'websocket' 
        });
        break;
        
      default:
        console.log('[MetricsCollector] Unknown WebSocket message:', data.type);
    }
  }

  /**
   * Smart polling system - adjusts based on WebSocket availability
   */
  startSmartPolling() {
    Object.entries(this.pollConfig).forEach(([key, config]) => {
      if (!config.enabled) return;
      
      const pollFunction = this.createPollFunction(key);
      const intervalId = setInterval(pollFunction, config.interval);
      this.intervals.set(key, intervalId);
      
      console.log(`[MetricsCollector] Started polling ${key} every ${config.interval}ms`);
    });
  }

  /**
   * Adjust polling frequency based on WebSocket status
   */
  adjustPollingForWebSocket(wsConnected) {
    const multiplier = wsConnected ? 3 : 1; // Slower polling when WS is active
    
    Object.entries(this.pollConfig).forEach(([key, config]) => {
      if (this.intervals.has(key)) {
        clearInterval(this.intervals.get(key));
        
        const adjustedInterval = config.interval * multiplier;
        const pollFunction = this.createPollFunction(key);
        const intervalId = setInterval(pollFunction, adjustedInterval);
        this.intervals.set(key, intervalId);
        
        console.log(`[MetricsCollector] Adjusted ${key} polling to ${adjustedInterval}ms (WS: ${wsConnected})`);
      }
    });
  }

  /**
   * Create polling function for specific data type
   */
  createPollFunction(type) {
    const pollFunctions = {
      status: () => this.pollStatus(),
      endpoints: () => this.pollEndpoints(),
      processStats: () => this.pollProcessStats(),
      modelStats: () => this.pollModelStats(),
      unifiedModels: () => this.pollUnifiedModels()
    };
    
    return pollFunctions[type] || (() => {});
  }

  /**
   * Individual polling methods
   */
  async pollStatus() {
    try {
      const data = await api.getStatus();
      const timestamp = Date.now();
      
      this.updateLastData('status', data);
      this.emit('metrics-update', { 
        type: 'status', 
        data, 
        timestamp,
        source: 'polling' 
      });
    } catch (error) {
      console.error('[MetricsCollector] Failed to poll status:', error);
    }
  }

  async pollEndpoints() {
    try {
      const data = await api.getEndpoints();
      const timestamp = Date.now();
      
      this.updateLastData('endpoints', data.endpoints || []);
      this.emit('metrics-update', { 
        type: 'endpoints', 
        data: data.endpoints || [], 
        timestamp,
        source: 'polling' 
      });
    } catch (error) {
      console.error('[MetricsCollector] Failed to poll endpoints:', error);
    }
  }

  async pollProcessStats() {
    try {
      const data = await api.getProcessStats();
      const timestamp = Date.now();
      
      this.updateLastData('processStats', data);
      this.emit('metrics-update', { 
        type: 'process', 
        data, 
        timestamp,
        source: 'polling' 
      });
    } catch (error) {
      console.error('[MetricsCollector] Failed to poll process stats:', error);
    }
  }

  async pollModelStats() {
    try {
      const data = await api.getModelStats();
      const timestamp = Date.now();
      
      this.updateLastData('modelStats', data);
      this.emit('metrics-update', { 
        type: 'models', 
        data, 
        timestamp,
        source: 'polling' 
      });
    } catch (error) {
      console.error('[MetricsCollector] Failed to poll model stats:', error);
    }
  }

  async pollUnifiedModels() {
    try {
      const data = await api.getUnifiedModels();
      const timestamp = Date.now();
      
      const models = data.data || data.unified_models || data.models || [];
      this.updateLastData('unifiedModels', models);
      this.emit('metrics-update', { 
        type: 'unified-models', 
        data: models, 
        timestamp,
        source: 'polling' 
      });
    } catch (error) {
      console.error('[MetricsCollector] Failed to poll unified models:', error);
    }
  }

  /**
   * Initial data fetch on startup
   */
  async fetchInitialData() {
    console.log('[MetricsCollector] Fetching initial data...');
    
    const fetchPromises = [
      this.pollStatus(),
      this.pollEndpoints(),
      this.pollProcessStats(),
      this.pollModelStats(),
      this.pollUnifiedModels()
    ];
    
    try {
      await Promise.allSettled(fetchPromises);
      console.log('[MetricsCollector] Initial data fetch completed');
    } catch (error) {
      console.error('[MetricsCollector] Initial data fetch failed:', error);
    }
  }

  /**
   * Update last known data
   */
  updateLastData(type, data) {
    this.lastData[type] = data;
    this.lastData.timestamp = Date.now();
  }

  /**
   * Send WebSocket message
   */
  sendWS(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }

  /**
   * Event emitter functionality
   */
  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event).add(callback);
  }

  off(event, callback) {
    if (this.listeners.has(event)) {
      this.listeners.get(event).delete(callback);
    }
  }

  emit(event, data) {
    if (this.listeners.has(event)) {
      this.listeners.get(event).forEach(callback => {
        try {
          callback(data);
        } catch (error) {
          console.error(`[MetricsCollector] Error in ${event} listener:`, error);
        }
      });
    }
  }

  /**
   * Get current connection status
   */
  getConnectionStatus() {
    return {
      websocket: this.isConnected,
      polling: this.intervals.size > 0,
      lastUpdate: this.lastData.timestamp
    };
  }

  /**
   * Get last known data
   */
  getLastData() {
    return { ...this.lastData };
  }
}

// Create singleton instance
export const metricsCollector = new MetricsCollector();