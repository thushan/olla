// Non-reactive cache for utilities
class ReactiveCache {
  constructor(options = {}) {
    this.noiseThreshold = options.noiseThreshold || 0.2; // 20% change threshold
    this.ignoreZeroTimeout = options.ignoreZeroTimeout || 5000; // 5 seconds
    this.cache = new Map();
    this.zeroTimestamps = new Map();
  }
  
  // Get cached value or default
  get(key, defaultValue = null) {
    const entry = this.cache.get(key);
    return entry ? entry.value : defaultValue;
  }
  
  // Update value with noise filtering
  update(key, newValue) {
    const now = Date.now();
    const entry = this.cache.get(key);
    
    // First time seeing this key
    if (!entry) {
      this.cache.set(key, { value: newValue, lastUpdate: now });
      return newValue;
    }
    
    const oldValue = entry.value;
    
    // Handle zero values with timeout
    if (newValue === 0 && oldValue !== 0) {
      const zeroTime = this.zeroTimestamps.get(key);
      
      if (!zeroTime) {
        // First zero, start timeout
        this.zeroTimestamps.set(key, now);
        return oldValue; // Keep old value
      } else if (now - zeroTime < this.ignoreZeroTimeout) {
        // Within timeout, ignore zero
        return oldValue;
      } else {
        // Timeout expired, accept zero
        this.zeroTimestamps.delete(key);
      }
    } else if (newValue !== 0) {
      // Clear zero timestamp if value is non-zero
      this.zeroTimestamps.delete(key);
    }
    
    // Check noise threshold for numeric values
    if (typeof oldValue === 'number' && typeof newValue === 'number' && oldValue !== 0) {
      const change = Math.abs((newValue - oldValue) / oldValue);
      if (change < this.noiseThreshold) {
        return oldValue; // Keep old value if change is within noise threshold
      }
    }
    
    // Update cache
    this.cache.set(key, { value: newValue, lastUpdate: now });
    
    return newValue;
  }
  
  // Clear stale entries
  clearStale(maxAge = 300000) { // 5 minutes default
    const now = Date.now();
    const newCache = new Map();
    
    for (const [key, entry] of this.cache) {
      if (now - entry.lastUpdate < maxAge) {
        newCache.set(key, entry);
      }
    }
    
    this.cache = newCache;
  }
  
  // Clear all entries
  clear() {
    this.cache = new Map();
    this.zeroTimestamps.clear();
  }
}

// Create state for endpoint details
let endpointDetailsCache = {};

// Helper to get endpoint cache key
export function endpointCacheKey(endpoint, field) {
  return `endpoint:${endpoint.name}:${field}`;
}

// Update endpoint detail
export function updateEndpointDetail(endpointName, field, value) {
  endpointDetailsCache[`${endpointName}:${field}`] = value;
}

// Get endpoint detail
export function getEndpointDetail(endpointName, field, defaultValue = null) {
  return endpointDetailsCache[`${endpointName}:${field}`] || defaultValue;
}

// Create and export singleton cache instance
export const globalCache = new ReactiveCache({
  noiseThreshold: 0.2,
  ignoreZeroTimeout: 5000
});

// State for metrics with noise filtering
let metricsCache = {
  totalRequests: 0,
  activeConnections: 0,
  requestRate: 0,
  avgLatency: 0,
  successRate: 100
};

// Get metrics with caching
export function getCachedMetric(key) {
  return metricsCache[key] || 0;
}

// Update metric with noise filtering
export function updateMetric(key, value) {
  const oldValue = metricsCache[key] || 0;
  
  // Apply noise filtering for numeric values
  if (typeof value === 'number' && typeof oldValue === 'number') {
    // Ignore single zero values
    if (value === 0 && oldValue !== 0) {
      return oldValue; // Keep old value
    }
    
    // Check threshold for non-zero changes
    if (oldValue !== 0) {
      const change = Math.abs((value - oldValue) / oldValue);
      if (change < 0.2) { // 20% threshold
        return oldValue;
      }
    }
  }
  
  // Update
  metricsCache[key] = value;
  
  return value;
}

// Batch update metrics
export function updateMetrics(updates) {
  const newMetrics = { ...metricsCache };
  
  Object.entries(updates).forEach(([key, value]) => {
    const oldValue = newMetrics[key] || 0;
    
    // Apply same filtering logic
    if (typeof value === 'number' && typeof oldValue === 'number') {
      if (value === 0 && oldValue !== 0) {
        // Keep old value
      } else if (oldValue !== 0) {
        const change = Math.abs((value - oldValue) / oldValue);
        if (change >= 0.2) {
          newMetrics[key] = value;
        }
      } else {
        newMetrics[key] = value;
      }
    } else {
      newMetrics[key] = value;
    }
  });
  
  metricsCache = newMetrics;
}

// Set up periodic cleanup (this should be called from a component if needed)
export function startCacheCleanup() {
  const interval = setInterval(() => {
    globalCache.clearStale();
  }, 60000); // Every minute
  
  return () => clearInterval(interval);
}