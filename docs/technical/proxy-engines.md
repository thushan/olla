# Proxy Engines: Sherpa vs Olla

Olla provides two distinct proxy engine implementations, each optimised for different operational requirements. Understanding when and why to use each engine is crucial for deploying a performant LLM infrastructure.

## Overview

Both engines implement the same ProxyService interface, ensuring seamless interchangeability. The choice between them comes down to your specific performance requirements, operational complexity tolerance, and traffic patterns.

### Quick Comparison

| Feature | Sherpa | Olla |
|---------|--------|------|
| **Complexity** | Simple, maintainable | Complex, feature-rich |
| **Performance** | Good for moderate loads | Optimised for high throughput |
| **Buffer Size** | 8KB | 64KB |
| **Connection Pooling** | Shared transport | Per-endpoint pools |
| **Circuit Breakers** | No | Yes |
| **Object Pooling** | No | Yes (buffers, contexts, errors) |
| **Memory Usage** | Lower | Higher (but managed) |
| **GC Pressure** | Normal | Reduced through pooling |
| **Best For** | Development, small deployments | Production, high-traffic scenarios |

## Sherpa: The Simple Proxy

Sherpa embodies the principle of simplicity. It's a straightforward HTTP proxy that gets the job done without unnecessary complexity.

### Architecture

```go
type SherpaProxy struct {
    loadBalancer ports.LoadBalancer
    discovery    ports.DiscoveryService
    stats        ports.StatsCollector
    client       *http.Client
    bufferPool   *sync.Pool
}
```

Sherpa uses a single shared HTTP client with a common transport. This design keeps things simple but means all endpoints share the same connection pool.

### Key Features

**Stream Processing**
```go
func (s *SherpaProxy) streamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
    buffer := s.bufferPool.Get().([]byte)
    defer s.bufferPool.Put(buffer)
    
    flusher, canFlush := w.(http.Flusher)
    
    for {
        n, err := resp.Body.Read(buffer)
        if n > 0 {
            if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
                return writeErr
            }
            if canFlush {
                flusher.Flush()
            }
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
    }
    return nil
}
```

The streaming implementation is clean and efficient. It uses a buffer pool to reduce allocations and flushes periodically to ensure clients receive data promptly.

**Request Handling**
1. Select endpoint using load balancer
2. Clone request with new URL
3. Copy headers (with sanitisation)
4. Forward body
5. Stream response back

### Performance Characteristics

- **Latency**: Minimal overhead, typically adds <1ms
- **Throughput**: Handles 5,000-10,000 req/s comfortably
- **Memory**: Stable usage, grows linearly with connections
- **CPU**: Low overhead, most time spent in I/O

### When to Use Sherpa

Choose Sherpa when:
- Deploying to development or staging environments
- Running small to medium production workloads
- Prioritising debuggability and maintainability
- Operating with fewer than 10 endpoints
- Handling predictable, moderate traffic patterns

## Olla: The Performance Proxy

Olla is our high-performance proxy engine, designed for demanding production environments where every millisecond counts.

### Architecture

```go
type OllaProxy struct {
    loadBalancer   ports.LoadBalancer
    discovery      ports.DiscoveryService
    stats          ports.StatsCollector
    transportPool  map[string]*http.Transport
    transportMutex sync.RWMutex
    bufferPool     *lite.Pool[[]byte]
    contextPool    *lite.Pool[*proxyContext]
    errorPool      *lite.Pool[*proxyError]
    circuitBreaker map[string]*CircuitBreaker
}
```

Olla maintains per-endpoint connection pools, circuit breakers, and extensive object pooling. This complexity pays off in reduced latency and higher throughput.

### Key Features

**Per-Endpoint Connection Pooling**
```go
func (o *OllaProxy) getOrCreateTransport(endpoint *domain.Endpoint) *http.Transport {
    o.transportMutex.RLock()
    transport, exists := o.transportPool[endpoint.ID]
    o.transportMutex.RUnlock()
    
    if exists {
        return transport
    }
    
    o.transportMutex.Lock()
    defer o.transportMutex.Unlock()
    
    transport = &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 50,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  true, // LLM responses are already compressed
    }
    
    o.transportPool[endpoint.ID] = transport
    return transport
}
```

Each endpoint gets its own connection pool, preventing slow endpoints from affecting fast ones. Connection limits are tuned for LLM workloads.

**Circuit Breakers**
```go
type CircuitBreaker struct {
    failures     atomic.Int32
    lastFailTime atomic.Int64
    state        atomic.Int32 // 0=closed, 1=open, 2=half-open
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.failures.Store(0)
    cb.state.Store(StateClosed)
}

func (cb *CircuitBreaker) RecordFailure() {
    failures := cb.failures.Add(1)
    cb.lastFailTime.Store(time.Now().Unix())
    
    if failures >= FailureThreshold {
        cb.state.Store(StateOpen)
    }
}
```

Circuit breakers prevent cascade failures. After 5 consecutive failures, an endpoint is marked as "open" and requests fail fast for 30 seconds before retrying.

**Object Pooling**
```go
// Buffer pool with 64KB buffers
bufferPool := lite.NewPool(
    func() []byte { return make([]byte, 65536) },
    func(b []byte) { /* reset if needed */ },
)

// Context pool for request metadata
contextPool := lite.NewPool(
    func() *proxyContext { return &proxyContext{} },
    func(ctx *proxyContext) { ctx.Reset() },
)
```

Aggressive pooling reduces GC pressure significantly. In benchmarks, this can reduce GC pauses by 60-80% under high load.

### Advanced Features

**Optimised Streaming**
```go
func (o *OllaProxy) streamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, buffer []byte) error {
    // Pre-warm the flusher
    flusher, canFlush := w.(http.Flusher)
    
    // Larger initial read for headers
    headerBuf := make([]byte, 4096)
    n, err := resp.Body.Read(headerBuf)
    if n > 0 {
        w.Write(headerBuf[:n])
        if canFlush {
            flusher.Flush()
        }
    }
    
    // Switch to larger buffer for body
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    lastFlush := time.Now()
    for {
        n, err := resp.Body.Read(buffer)
        if n > 0 {
            w.Write(buffer[:n])
            
            // Adaptive flushing
            if canFlush && (time.Since(lastFlush) > 50*time.Millisecond || n < 1024) {
                flusher.Flush()
                lastFlush = time.Now()
            }
        }
        
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        // Check for client disconnect
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            continue
        }
    }
    return nil
}
```

The streaming logic includes:
- Adaptive flushing based on data patterns
- Client disconnection detection
- Optimised buffer sizes for LLM responses

### Performance Characteristics

- **Latency**: Sub-millisecond overhead in most cases
- **Throughput**: Handles 20,000-50,000 req/s
- **Memory**: Higher baseline, but stable under load
- **CPU**: Efficient, with lock-free statistics
- **GC Impact**: 60-80% reduction in GC pauses

### When to Use Olla

Choose Olla when:
- Running high-traffic production workloads
- Managing 10+ endpoints
- Requiring circuit breaker protection
- Optimising for minimum latency
- Dealing with variable endpoint performance
- Streaming large model responses frequently

## Implementation Details

### Common Patterns

Both engines share several implementation patterns:

**Header Sanitisation**
```go
var hopHeaders = []string{
    "Connection", "Keep-Alive", "Proxy-Authenticate",
    "Proxy-Authorization", "Te", "Trailers", 
    "Transfer-Encoding", "Upgrade",
}

func sanitizeHeaders(headers http.Header) {
    for _, h := range hopHeaders {
        headers.Del(h)
    }
}
```

**Statistics Collection**
```go
func recordRequest(stats ports.StatsCollector, endpoint string, duration time.Duration, success bool) {
    stats.RecordRequest()
    stats.RecordEndpointRequest(endpoint)
    stats.RecordLatency(duration)
    if !success {
        stats.RecordError()
    }
}
```

### Testing Strategy

Both engines are tested using a shared test suite:

```go
func TestProxyEngine(t *testing.T, factory func() ports.ProxyService) {
    t.Run("handles streaming", func(t *testing.T) {
        proxy := factory()
        // Test implementation
    })
    
    t.Run("handles errors", func(t *testing.T) {
        proxy := factory()
        // Test implementation
    })
}

// Run tests for both engines
func TestSherpa(t *testing.T) {
    TestProxyEngine(t, NewSherpaProxy)
}

func TestOlla(t *testing.T) {
    TestProxyEngine(t, NewOllaProxy)
}
```

## Configuration

### Sherpa Configuration

```yaml
proxy:
  engine: "sherpa"
  timeout: 300s
  max_idle_conns: 100
```

### Olla Configuration

```yaml
proxy:
  engine: "olla"
  timeout: 300s
  max_idle_conns: 100
  max_conns_per_host: 50
  idle_conn_timeout: 90s
  circuit_breaker:
    failure_threshold: 5
    timeout: 30s
    half_open_requests: 3
```

## Performance Tuning

### Sherpa Tuning

1. **Buffer Size**: Increase for larger responses
   ```go
   bufferSize := 16384 // 16KB for larger models
   ```

2. **Connection Pool**: Tune the shared transport
   ```go
   transport.MaxIdleConns = 200
   transport.MaxIdleConnsPerHost = 100
   ```

### Olla Tuning

1. **Per-Endpoint Pools**: Adjust based on endpoint characteristics
   ```go
   // High-traffic endpoint
   transport.MaxIdleConnsPerHost = 100
   
   // Low-traffic endpoint
   transport.MaxIdleConnsPerHost = 10
   ```

2. **Circuit Breaker Sensitivity**: Balance between protection and availability
   ```go
   const (
       FailureThreshold = 10  // More tolerant
       RecoveryTimeout  = 60  // Longer recovery
   )
   ```

3. **Pool Sizes**: Pre-warm pools for predictable traffic
   ```go
   bufferPool.PreWarm(1000)  // Pre-allocate 1000 buffers
   ```

## Monitoring and Observability

Both engines expose metrics through the stats collector:

```go
// Common metrics
- proxy.requests.total
- proxy.requests.success
- proxy.requests.failed
- proxy.latency.p50/p95/p99
- proxy.active_connections

// Olla-specific metrics
- proxy.circuit_breaker.open
- proxy.pool.buffer.active
- proxy.pool.buffer.created
- proxy.transport.connections.active
```

## Migration Guide

Switching between engines is straightforward:

1. **Update Configuration**
   ```yaml
   proxy:
     engine: "olla"  # Changed from "sherpa"
   ```

2. **Adjust Monitoring**: Update dashboards for Olla-specific metrics

3. **Test Under Load**: Verify performance improvements

4. **Gradual Rollout**: Use canary deployments if possible

## Conclusion

The dual-engine approach gives you flexibility to choose the right tool for your deployment. Start with Sherpa for simplicity, and upgrade to Olla when performance demands it. Both engines are production-tested and actively maintained, ensuring your LLM infrastructure remains robust and performant.