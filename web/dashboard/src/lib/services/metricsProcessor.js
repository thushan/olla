/**
 * Phase 2: Data Processor & Transformer
 * Transforms raw API data into dashboard-ready metrics with time series
 */

export class MetricsProcessor {
  constructor() {
    // Time series storage
    this.timeSeries = {
      latency: [],
      throughput: [],
      connections: [],
      memory: [],
      errors: [],
      requests: []
    };
    
    // Configuration
    this.maxHistoryPoints = 120; // 2 hours at 1min intervals
    this.historyRetentionMs = 2 * 60 * 60 * 1000; // 2 hours
    
    // Derived metrics cache
    this.derivedMetrics = {
      lastUpdate: 0,
      systemHealth: 'unknown',
      overallHealth: 'UNKNOWN',
      performanceScore: 0,
      securityScore: 0
    };
    
    // Raw data cache
    this.rawData = {
      status: null,
      endpoints: [],
      processStats: null,
      modelStats: {},
      unifiedModels: [],
      events: []
    };
  }

  /**
   * Process incoming metrics update
   */
  processUpdate(type, data, timestamp) {
    // Store raw data
    this.rawData[type === 'process' ? 'processStats' : type] = data;
    
    // Update time series based on data type
    switch (type) {
      case 'status':
        this.processSystemMetrics(data, timestamp);
        break;
      case 'endpoints':
        this.processEndpointMetrics(data, timestamp);
        break;
      case 'process':
        this.processMemoryMetrics(data, timestamp);
        break;
      case 'models':
        this.processModelMetrics(data, timestamp);
        break;
    }
    
    // Update derived metrics
    this.updateDerivedMetrics(timestamp);
    
    // Clean old data
    this.cleanupOldData(timestamp);
  }

  /**
   * Process system-level metrics
   */
  processSystemMetrics(data, timestamp) {
    if (!data?.system) return;
    
    const system = data.system;
    
    // Extract latency (handle different formats)
    let latencyMs = 0;
    if (system.avg_latency) {
      const latencyStr = system.avg_latency.toString();
      if (latencyStr.endsWith('ms')) {
        latencyMs = parseFloat(latencyStr);
      } else if (latencyStr.endsWith('s')) {
        latencyMs = parseFloat(latencyStr) * 1000;
      } else if (latencyStr.endsWith('µs')) {
        latencyMs = parseFloat(latencyStr) / 1000;
      } else {
        latencyMs = parseFloat(latencyStr); // Assume ms
      }
    }
    
    // Calculate throughput (requests per minute)
    let throughputRpm = 0;
    if (system.total_requests > 0 && system.uptime) {
      const uptimeStr = system.uptime.toString();
      let uptimeSeconds = 0;
      
      if (uptimeStr.includes('m')) {
        const parts = uptimeStr.split('m');
        const minutes = parseInt(parts[0]) || 0;
        const seconds = parts[1] ? parseInt(parts[1]) : 0;
        uptimeSeconds = minutes * 60 + seconds;
      } else if (uptimeStr.includes('s')) {
        uptimeSeconds = parseFloat(uptimeStr);
      } else {
        uptimeSeconds = parseFloat(uptimeStr);
      }
      
      if (uptimeSeconds > 0) {
        throughputRpm = (system.total_requests / uptimeSeconds) * 60;
      }
    }
    
    // Add to time series
    this.addToTimeSeries('latency', latencyMs, timestamp);
    this.addToTimeSeries('throughput', throughputRpm, timestamp);
    this.addToTimeSeries('connections', system.active_connections || 0, timestamp);
    this.addToTimeSeries('requests', system.total_requests || 0, timestamp);
    this.addToTimeSeries('errors', system.total_failures || 0, timestamp);
  }

  /**
   * Process endpoint health metrics
   */
  processEndpointMetrics(endpoints, timestamp) {
    if (!Array.isArray(endpoints)) return;
    
    // Calculate aggregate connection count
    const totalConnections = endpoints.reduce((sum, ep) => sum + (ep.connections || 0), 0);
    
    // Add to time series (this will override system connections if more accurate)
    this.addToTimeSeries('connections', totalConnections, timestamp);
  }

  /**
   * Process memory metrics from process stats
   */
  processMemoryMetrics(processStats, timestamp) {
    if (!processStats?.memory?.heap_alloc) return;
    
    // Parse memory string (e.g., "2.49 MB" -> 2.49)
    const memoryStr = processStats.memory.heap_alloc;
    let memoryMB = 0;
    
    if (memoryStr.includes('MB')) {
      memoryMB = parseFloat(memoryStr);
    } else if (memoryStr.includes('KB')) {
      memoryMB = parseFloat(memoryStr) / 1024;
    } else if (memoryStr.includes('GB')) {
      memoryMB = parseFloat(memoryStr) * 1024;
    } else {
      // Assume bytes
      memoryMB = parseFloat(memoryStr) / (1024 * 1024);
    }
    
    this.addToTimeSeries('memory', memoryMB, timestamp);
  }

  /**
   * Process model metrics
   */
  processModelMetrics(modelStats, timestamp) {
    // This will be expanded when model-specific metrics are needed
    // For now, we track this for completeness
  }

  /**
   * Add data point to time series
   */
  addToTimeSeries(metric, value, timestamp) {
    if (!this.timeSeries[metric]) {
      this.timeSeries[metric] = [];
    }
    
    const series = this.timeSeries[metric];
    
    // Don't add duplicate timestamps or invalid values
    if (series.length > 0 && series[series.length - 1].timestamp === timestamp) {
      return;
    }
    
    if (typeof value !== 'number' || isNaN(value)) {
      return;
    }
    
    series.push({
      timestamp,
      value,
      time: new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    });
    
    // Maintain max history
    if (series.length > this.maxHistoryPoints) {
      series.splice(0, series.length - this.maxHistoryPoints);
    }
  }

  /**
   * Update derived metrics
   */
  updateDerivedMetrics(timestamp) {
    const status = this.rawData.status;
    const endpoints = this.rawData.endpoints;
    const processStats = this.rawData.processStats;
    
    // System health
    this.derivedMetrics.systemHealth = status?.system?.status || 'unknown';
    
    // Overall health based on endpoints
    if (Array.isArray(endpoints) && endpoints.length > 0) {
      const healthyCount = endpoints.filter(ep => 
        ep.status === 'healthy' || ep.status === 'online'
      ).length;
      const totalCount = endpoints.length;
      
      if (healthyCount === totalCount) {
        this.derivedMetrics.overallHealth = 'HEALTHY';
      } else if (healthyCount === 0) {
        this.derivedMetrics.overallHealth = 'CRITICAL';
      } else {
        this.derivedMetrics.overallHealth = 'DEGRADED';
      }
    }
    
    // Performance score (0-100)
    let perfScore = 100;
    
    const latestLatency = this.getLatestValue('latency');
    if (latestLatency > 1000) perfScore -= 30; // High latency penalty
    else if (latestLatency > 500) perfScore -= 15;
    
    const errorRate = this.calculateErrorRate();
    if (errorRate > 5) perfScore -= 25; // High error rate penalty
    else if (errorRate > 1) perfScore -= 10;
    
    if (this.derivedMetrics.overallHealth === 'DEGRADED') perfScore -= 20;
    else if (this.derivedMetrics.overallHealth === 'CRITICAL') perfScore -= 50;
    
    this.derivedMetrics.performanceScore = Math.max(0, perfScore);
    
    // Security score (simplified for now)
    const securityViolations = status?.system?.security_violations || 0;
    this.derivedMetrics.securityScore = Math.max(0, 100 - (securityViolations * 10));
    
    this.derivedMetrics.lastUpdate = timestamp;
  }

  /**
   * Calculate current error rate
   */
  calculateErrorRate() {
    const latestRequests = this.getLatestValue('requests');
    const latestErrors = this.getLatestValue('errors');
    
    if (latestRequests === 0) return 0;
    return (latestErrors / latestRequests) * 100;
  }

  /**
   * Get latest value from time series
   */
  getLatestValue(metric) {
    const series = this.timeSeries[metric];
    if (!series || series.length === 0) return 0;
    return series[series.length - 1].value;
  }

  /**
   * Get time series data for charts
   */
  getTimeSeriesData(metric, maxPoints = 20) {
    const series = this.timeSeries[metric];
    if (!series || series.length === 0) {
      return this.generateFallbackSeries(metric, maxPoints);
    }
    
    // If we have enough data points, sample them
    if (series.length <= maxPoints) {
      return series.map((point, index) => ({
        ...point,
        x: (index / Math.max(1, series.length - 1)) * 740 // Chart width - margins
      }));
    }
    
    // Sample data points evenly
    const step = (series.length - 1) / (maxPoints - 1);
    const sampled = [];
    
    for (let i = 0; i < maxPoints; i++) {
      const index = Math.round(i * step);
      const point = series[index];
      sampled.push({
        ...point,
        x: (i / (maxPoints - 1)) * 740
      });
    }
    
    return sampled;
  }

  /**
   * Generate fallback series when no real data is available
   */
  generateFallbackSeries(metric, points) {
    const now = Date.now();
    const interval = (5 * 60 * 1000) / points; // 5 minutes
    const baseValue = this.getBaseValueForMetric(metric);
    
    return Array.from({ length: points }, (_, i) => {
      const timestamp = now - (points - i - 1) * interval;
      return {
        timestamp,
        value: baseValue,
        time: new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        x: (i / (points - 1)) * 740
      };
    });
  }

  /**
   * Get reasonable base value for metrics when no data exists
   */
  getBaseValueForMetric(metric) {
    const defaults = {
      latency: 15,
      throughput: 2.5,
      connections: 3,
      memory: 12,
      requests: 0,
      errors: 0
    };
    
    return defaults[metric] || 0;
  }

  /**
   * Get current metrics snapshot
   */
  getCurrentMetrics() {
    const status = this.rawData.status;
    const processStats = this.rawData.processStats;
    
    return {
      // Current values
      latency: this.getLatestValue('latency'),
      throughput: this.getLatestValue('throughput'),
      connections: this.getLatestValue('connections'),
      memory: this.getLatestValue('memory'),
      requests: this.getLatestValue('requests'),
      errors: this.getLatestValue('errors'),
      
      // Calculated metrics
      errorRate: this.calculateErrorRate(),
      successRate: 100 - this.calculateErrorRate(),
      
      // System status
      systemHealth: this.derivedMetrics.systemHealth,
      overallHealth: this.derivedMetrics.overallHealth,
      performanceScore: this.derivedMetrics.performanceScore,
      securityScore: this.derivedMetrics.securityScore,
      
      // Raw data access
      status: status,
      endpoints: this.rawData.endpoints,
      processStats: processStats,
      modelStats: this.rawData.modelStats,
      unifiedModels: this.rawData.unifiedModels,
      
      // Meta
      lastUpdate: this.derivedMetrics.lastUpdate,
      hasRealData: this.hasRealData()
    };
  }

  /**
   * Check if we have real data vs fallbacks
   */
  hasRealData() {
    return Object.values(this.timeSeries).some(series => series.length > 0);
  }

  /**
   * Clean up old time series data
   */
  cleanupOldData(currentTimestamp) {
    const cutoff = currentTimestamp - this.historyRetentionMs;
    
    Object.keys(this.timeSeries).forEach(metric => {
      const series = this.timeSeries[metric];
      const cutoffIndex = series.findIndex(point => point.timestamp > cutoff);
      
      if (cutoffIndex > 0) {
        series.splice(0, cutoffIndex);
      }
    });
  }

  /**
   * Reset all data
   */
  reset() {
    Object.keys(this.timeSeries).forEach(metric => {
      this.timeSeries[metric] = [];
    });
    
    this.rawData = {
      status: null,
      endpoints: [],
      processStats: null,
      modelStats: {},
      unifiedModels: [],
      events: []
    };
    
    this.derivedMetrics = {
      lastUpdate: 0,
      systemHealth: 'unknown',
      overallHealth: 'UNKNOWN',
      performanceScore: 0,
      securityScore: 0
    };
  }

  /**
   * Get debug information
   */
  getDebugInfo() {
    return {
      timeSeriesLengths: Object.entries(this.timeSeries).reduce((acc, [key, series]) => {
        acc[key] = series.length;
        return acc;
      }, {}),
      oldestTimestamp: Math.min(
        ...Object.values(this.timeSeries)
          .filter(series => series.length > 0)
          .map(series => series[0].timestamp)
      ),
      newestTimestamp: Math.max(
        ...Object.values(this.timeSeries)
          .filter(series => series.length > 0)
          .map(series => series[series.length - 1].timestamp)
      ),
      derivedMetrics: this.derivedMetrics,
      hasRealData: this.hasRealData()
    };
  }
}

// Create singleton instance
export const metricsProcessor = new MetricsProcessor();