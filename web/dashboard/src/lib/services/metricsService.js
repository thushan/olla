/**
 * Unified Metrics Service
 * Coordinates the MetricsCollector and MetricsProcessor
 */

import { metricsCollector } from './metricsCollector.js';
import { metricsProcessor } from './metricsProcessor.js';

export class MetricsService {
  constructor() {
    this.listeners = new Map();
    this.isStarted = false;
    this.eventBuffer = [];
    this.maxEventBuffer = 100;
    
    // Bind collector events
    this.setupCollectorListeners();
  }

  /**
   * Start the complete metrics system
   */
  async start() {
    if (this.isStarted) {
      console.warn('[MetricsService] Already started');
      return;
    }
    
    console.log('[MetricsService] Starting unified metrics system...');
    
    try {
      await metricsCollector.start();
      this.isStarted = true;
      
      console.log('[MetricsService] Metrics system started successfully');
      this.emit('system-started', { timestamp: Date.now() });
    } catch (error) {
      console.error('[MetricsService] Failed to start metrics system:', error);
      throw error;
    }
  }

  /**
   * Stop the metrics system
   */
  stop() {
    if (!this.isStarted) return;
    
    console.log('[MetricsService] Stopping metrics system...');
    
    metricsCollector.stop();
    this.isStarted = false;
    
    this.emit('system-stopped', { timestamp: Date.now() });
  }

  /**
   * Setup listeners for collector events
   */
  setupCollectorListeners() {
    // Handle metrics updates
    metricsCollector.on('metrics-update', (data) => {
      const { type, data: rawData, timestamp, source } = data;
      
      // Process the data through the processor
      metricsProcessor.processUpdate(type, rawData, timestamp);
      
      // Emit processed update
      this.emit('metrics-processed', {
        type,
        source,
        timestamp,
        currentMetrics: metricsProcessor.getCurrentMetrics()
      });
    });

    // Handle WebSocket connection changes
    metricsCollector.on('websocket-connected', (data) => {
      this.emit('connection-status', {
        websocket: data.connected,
        timestamp: Date.now()
      });
    });

    // Handle events
    metricsCollector.on('event-received', (data) => {
      this.addEvent(data.event, data.timestamp);
    });
  }

  /**
   * Add event to buffer and emit
   */
  addEvent(event, timestamp) {
    const processedEvent = {
      ...event,
      timestamp: timestamp || Date.now(),
      id: `event-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
    };
    
    // Add to buffer
    this.eventBuffer.unshift(processedEvent);
    
    // Maintain buffer size
    if (this.eventBuffer.length > this.maxEventBuffer) {
      this.eventBuffer = this.eventBuffer.slice(0, this.maxEventBuffer);
    }
    
    // Emit event
    this.emit('event-added', {
      event: processedEvent,
      totalEvents: this.eventBuffer.length
    });
  }

  /**
   * Get current metrics snapshot
   */
  getCurrentMetrics() {
    return metricsProcessor.getCurrentMetrics();
  }

  /**
   * Get time series data for charts
   */
  getTimeSeriesData(metric, maxPoints = 20) {
    return metricsProcessor.getTimeSeriesData(metric, maxPoints);
  }

  /**
   * Get recent events
   */
  getRecentEvents(limit = 50) {
    return this.eventBuffer.slice(0, limit);
  }

  /**
   * Get connection status
   */
  getConnectionStatus() {
    return {
      ...metricsCollector.getConnectionStatus(),
      isStarted: this.isStarted,
      hasRealData: metricsProcessor.hasRealData()
    };
  }

  /**
   * Get debug information
   */
  getDebugInfo() {
    return {
      service: {
        isStarted: this.isStarted,
        eventBufferSize: this.eventBuffer.length
      },
      collector: metricsCollector.getConnectionStatus(),
      processor: metricsProcessor.getDebugInfo()
    };
  }

  /**
   * Force refresh of all metrics
   */
  async forceRefresh() {
    if (!this.isStarted) {
      throw new Error('Metrics service not started');
    }
    
    console.log('[MetricsService] Force refreshing all metrics...');
    
    try {
      await metricsCollector.fetchInitialData();
      this.emit('force-refresh-completed', { timestamp: Date.now() });
    } catch (error) {
      console.error('[MetricsService] Force refresh failed:', error);
      this.emit('force-refresh-failed', { error: error.message, timestamp: Date.now() });
      throw error;
    }
  }

  /**
   * Reset all data and restart collection
   */
  async reset() {
    console.log('[MetricsService] Resetting metrics system...');
    
    this.stop();
    metricsProcessor.reset();
    this.eventBuffer = [];
    
    await this.start();
    
    this.emit('system-reset', { timestamp: Date.now() });
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
          console.error(`[MetricsService] Error in ${event} listener:`, error);
        }
      });
    }
  }

  /**
   * Subscribe to all metric updates with a single callback
   */
  subscribe(callback) {
    this.on('metrics-processed', callback);
    this.on('event-added', callback);
    this.on('connection-status', callback);
    
    // Return unsubscribe function
    return () => {
      this.off('metrics-processed', callback);
      this.off('event-added', callback);
      this.off('connection-status', callback);
    };
  }
}

// Create singleton instance
export const metricsService = new MetricsService();