// Data caching utilities to prevent UI flickering and maintain stability

export class StableDataCache {
  constructor(options = {}) {
    this.cache = new Map();
    this.lastUpdate = new Map();
    this.noiseThreshold = options.noiseThreshold || 0.2; // 20% change threshold
    this.staleTimeout = options.staleTimeout || 30000; // 30 seconds
    this.ignoreZeroTimeout = options.ignoreZeroTimeout || 5000; // 5 seconds
  }

  // Get cached value or default
  get(key, defaultValue = null) {
    const cached = this.cache.get(key);
    if (!cached) return defaultValue;
    
    // Check if data is stale
    const lastUpdate = this.lastUpdate.get(key) || 0;
    if (Date.now() - lastUpdate > this.staleTimeout) {
      return defaultValue;
    }
    
    return cached.value;
  }

  // Update cache with noise filtering
  update(key, newValue, options = {}) {
    const cached = this.cache.get(key);
    const now = Date.now();
    
    // If no cached value, accept the new value
    if (!cached) {
      this.cache.set(key, { value: newValue, zeroCount: 0 });
      this.lastUpdate.set(key, now);
      return newValue;
    }
    
    // Handle zero values specially
    if (this.isEffectivelyZero(newValue)) {
      const timeSinceUpdate = now - (this.lastUpdate.get(key) || 0);
      
      // If we recently had a non-zero value, ignore the zero
      if (!this.isEffectivelyZero(cached.value) && timeSinceUpdate < this.ignoreZeroTimeout) {
        cached.zeroCount = (cached.zeroCount || 0) + 1;
        
        // Only accept zero after multiple consecutive zeros
        if (cached.zeroCount < 3) {
          return cached.value; // Return old value
        }
      }
    } else {
      // Reset zero count on non-zero value
      cached.zeroCount = 0;
    }
    
    // Check if change is significant (for numeric values)
    if (typeof newValue === 'number' && typeof cached.value === 'number') {
      const changeRatio = Math.abs(newValue - cached.value) / (cached.value || 1);
      
      // Ignore small fluctuations
      if (changeRatio < this.noiseThreshold && !options.forceUpdate) {
        return cached.value;
      }
    }
    
    // Update cache
    this.cache.set(key, { value: newValue, zeroCount: 0 });
    this.lastUpdate.set(key, now);
    return newValue;
  }

  // Check if value is effectively zero
  isEffectivelyZero(value) {
    if (value === 0 || value === null || value === undefined) return true;
    if (value === '0' || value === 'N/A' || value === '') return true;
    if (typeof value === 'object' && Object.keys(value).length === 0) return true;
    if (Array.isArray(value) && value.length === 0) return true;
    return false;
  }

  // Clear stale entries
  clearStale() {
    const now = Date.now();
    for (const [key, timestamp] of this.lastUpdate.entries()) {
      if (now - timestamp > this.staleTimeout) {
        this.cache.delete(key);
        this.lastUpdate.delete(key);
      }
    }
  }

  // Clear all cache
  clear() {
    this.cache.clear();
    this.lastUpdate.clear();
  }
}

// Singleton instance for global use
export const globalCache = new StableDataCache();

// Helper to create endpoint-specific cache key
export function endpointCacheKey(endpoint, field) {
  return `endpoint:${endpoint.name}:${field}`;
}

// Helper to create model-specific cache key
export function modelCacheKey(model, field) {
  return `model:${model.id || model.name}:${field}`;
}