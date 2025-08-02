# Statistics and Monitoring

Olla implements a centralised statistics collection system designed for high-concurrency environments. Using thread-safe counters and maps, the system tracks requests, connections, and security events without impacting performance.

## Architecture Overview

The statistics system uses a single collector that all components report to:

```go
type Collector struct {
    logger logger.StyledLogger
    
    // Endpoint-specific statistics
    endpoints *xsync.Map[string, *endpointData]
    
    // Global counters using xsync.Counter for lock-free operations
    totalRequests      *xsync.Counter
    successfulRequests *xsync.Counter
    failedRequests     *xsync.Counter
    totalLatency       *xsync.Counter
    
    // Security tracking
    rateLimitViolations *xsync.Counter
    sizeLimitViolations *xsync.Counter
    uniqueRateLimitedIPs map[string]int64
    
    // Model statistics
    modelCollector *ModelCollector
}
```

## Statistics Collection

### Request Tracking

Every request is recorded with its outcome and latency:

```go
func (c *Collector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
    latencyMs := latency.Milliseconds()
    
    c.totalRequests.Inc()
    
    if status == StatusSuccess {
        c.successfulRequests.Inc()
        c.totalLatency.Add(latencyMs)
    } else {
        c.failedRequests.Inc()
    }
    
    // Update endpoint-specific stats
    if endpoint != nil {
        c.updateEndpointStats(endpoint, status, latencyMs, bytes, now)
    }
}
```

### Connection Tracking

Active connections are tracked per endpoint:

```go
func (c *Collector) RecordConnection(endpoint *domain.Endpoint, delta int) {
    data := c.getOrInitEndpoint(endpoint, now)
    
    if delta > 0 {
        atomic.AddInt64(&data.activeConnections, int64(delta))
    } else if delta < 0 {
        // Ensure we don't go negative
        for {
            current := atomic.LoadInt64(&data.activeConnections)
            newVal := current + int64(delta)
            if newVal < 0 {
                newVal = 0
            }
            if atomic.CompareAndSwapInt64(&data.activeConnections, current, newVal) {
                break
            }
        }
    }
}
```

### Per-Endpoint Statistics

Each endpoint maintains its own statistics:

```go
type endpointData struct {
    totalRequests      *xsync.Counter
    successfulRequests *xsync.Counter
    failedRequests     *xsync.Counter
    totalBytes         *xsync.Counter
    totalLatency       *xsync.Counter
    name               string
    url                string
    activeConnections  int64  // Atomic operations
    minLatency         int64  // Atomic for CAS operations
    maxLatency         int64  // Atomic for CAS operations
    lastUsed           int64  // Timestamp
}
```

## Model-Level Statistics

The system tracks statistics per model across endpoints:

```go
type ModelCollector struct {
    models *xsync.Map[string, *modelStats]
}

type modelStats struct {
    totalRequests      *xsync.Counter
    successfulRequests *xsync.Counter
    failedRequests     *xsync.Counter
    totalBytes         *xsync.Counter
    totalLatency       *xsync.Counter
    lastLatencies      []int64  // Circular buffer for percentiles
    latencyIndex       int32
    latencyMu          sync.RWMutex
}
```

### Percentile Calculation

Model statistics include P95 and P99 latency calculations:

```go
func (ms *modelStats) getPercentiles() (p95, p99 int64) {
    ms.latencyMu.RLock()
    defer ms.latencyMu.RUnlock()
    
    // Copy and sort latencies
    latencies := make([]int64, 0, len(ms.lastLatencies))
    for _, l := range ms.lastLatencies {
        if l > 0 {
            latencies = append(latencies, l)
        }
    }
    
    if len(latencies) == 0 {
        return 0, 0
    }
    
    sort.Slice(latencies, func(i, j int) bool {
        return latencies[i] < latencies[j]
    })
    
    p95Index := int(float64(len(latencies)) * 0.95)
    p99Index := int(float64(len(latencies)) * 0.99)
    
    return latencies[p95Index], latencies[p99Index]
}
```

## Automatic Cleanup

To prevent memory growth, the collector automatically cleans up old data:

```go
const (
    MaxTrackedEndpoints = 50           // Maximum endpoints to track
    EndpointTTL         = 1 * time.Hour // Remove after 1 hour of inactivity
    CleanupInterval     = 5 * time.Minute
)

func (c *Collector) cleanup(now int64) {
    cutoff := now - int64(EndpointTTL)
    var toRemove []string
    
    // Remove inactive endpoints
    c.endpoints.Range(func(url string, data *endpointData) bool {
        if atomic.LoadInt64(&data.lastUsed) < cutoff {
            toRemove = append(toRemove, url)
        }
        return true
    })
    
    for _, url := range toRemove {
        c.endpoints.Delete(url)
    }
    
    // If still too many, remove oldest
    if count > MaxTrackedEndpoints {
        // Sort by last used time and remove oldest
    }
}
```

## Security Statistics

The collector tracks security-related events:

```go
func (c *Collector) RecordSecurityViolation(violation ports.SecurityViolation) {
    switch violation.ViolationType {
    case constants.ViolationRateLimit:
        c.rateLimitViolations.Inc()
        c.recordRateLimitedIP(violation.ClientID)
    case constants.ViolationSizeLimit:
        c.sizeLimitViolations.Inc()
    }
}
```

## Accessing Statistics

### Global Statistics

```go
func (c *Collector) GetProxyStats() ports.ProxyStats {
    total := c.totalRequests.Value()
    successful := c.successfulRequests.Value()
    failed := c.failedRequests.Value()
    
    var avgLatency int64
    if successful > 0 {
        avgLatency = c.totalLatency.Value() / successful
    }
    
    return ports.ProxyStats{
        TotalRequests:      total,
        SuccessfulRequests: successful,
        FailedRequests:     failed,
        AverageLatency:     avgLatency,
    }
}
```

### Endpoint Statistics

```go
func (c *Collector) GetEndpointStats() map[string]ports.EndpointStats {
    stats := make(map[string]ports.EndpointStats)
    
    c.endpoints.Range(func(url string, data *endpointData) bool {
        stats[url] = ports.EndpointStats{
            Name:               data.name,
            URL:                data.url,
            ActiveConnections:  atomic.LoadInt64(&data.activeConnections),
            TotalRequests:      data.totalRequests.Value(),
            SuccessfulRequests: data.successfulRequests.Value(),
            FailedRequests:     data.failedRequests.Value(),
            TotalBytes:         data.totalBytes.Value(),
            AverageLatency:     avgLatency,
            MinLatency:         minLatency,
            MaxLatency:         maxLatency,
            SuccessRate:        successRate,
        }
        return true
    })
    
    return stats
}
```

### Model Statistics

```go
type ModelStats struct {
    TotalRequests      int64
    SuccessfulRequests int64
    FailedRequests     int64
    TotalBytes         int64
    AverageLatency     int64
    P95Latency         int64
    P99Latency         int64
    ErrorRate          float64
}
```

## Integration with HTTP Handlers

Statistics are exposed through HTTP endpoints:

```go
// GET /internal/status
{
    "stats": {
        "total_requests": 15234,
        "successful_requests": 15100,
        "failed_requests": 134,
        "average_latency": 234
    }
}

// GET /internal/status/endpoints
{
    "endpoints": [
        {
            "name": "gpu-1",
            "url": "http://10.0.1.10:11434",
            "active_connections": 5,
            "total_requests": 5234,
            "success_rate": 99.2,
            "average_latency": 189
        }
    ]
}
```

## Performance Considerations

### Lock-Free Operations

The system uses `xsync.Counter` for lock-free atomic operations:
- No mutex contention on hot paths
- Safe for concurrent access
- Minimal CPU cache line bouncing

### Memory Management

- Automatic cleanup prevents unbounded growth
- Conservative limits (50 endpoints max)
- 1-hour TTL for inactive endpoints
- Circular buffer for model latencies (last 1000 values)

### Efficiency

- No allocations in hot paths
- Pre-allocated structures where possible
- Batch operations for cleanup

## Limitations

- No persistence - statistics are in-memory only
- No time-series data - only current values and totals
- No histogram implementation - simple min/max/average
- No export to external systems (Prometheus, etc.)
- Fixed retention periods

## Usage

The stats collector is initialised at startup:

```go
statsCollector := stats.NewCollector(logger)

// Record a request
statsCollector.RecordRequest(endpoint, "success", latency, bytes)

// Track connections
statsCollector.RecordConnection(endpoint, 1)  // Increment
defer statsCollector.RecordConnection(endpoint, -1)  // Decrement

// Get statistics
proxyStats := statsCollector.GetProxyStats()
endpointStats := statsCollector.GetEndpointStats()
```

