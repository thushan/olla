# Technical Overview

## Architecture

Olla follows Hexagonal Architecture (Ports & Adapters) for clean separation of concerns:

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP Layer                           │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │   Handlers  │  │  Middleware  │  │   Security    │ │
│  └──────┬──────┘  └──────┬───────┘  └───────┬───────┘ │
└─────────┼────────────────┼──────────────────┼─────────┘
          │                │                  │
┌─────────▼────────────────▼──────────────────▼─────────┐
│                    Core Domain                          │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │   Endpoint  │  │ LoadBalancer │  │    Model      │ │
│  │   Profile   │  │   Strategy   │  │   Registry    │ │
│  └─────────────┘  └──────────────┘  └───────────────┘ │
└────────────────────────────────────────────────────────┘
          │                │                  │
┌─────────▼────────────────▼──────────────────▼─────────┐
│                    Adapters                             │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │Proxy Engine │  │Health Checker│  │Stats Collector│ │
│  │(Sherpa/Olla)│  │   Circuit    │  │  Lock-free    │ │
│  └─────────────┘  │   Breaker    │  └───────────────┘ │
│                   └──────────────┘                     │
└────────────────────────────────────────────────────────┘
```

## Dual Proxy Engines

### Sherpa Engine
- **Design**: Simple, maintainable implementation
- **Transport**: Single shared `http.Transport`
- **Buffering**: 8KB buffers with basic pooling
- **Best for**: Development, moderate loads

### Olla Engine
- **Design**: High-performance with advanced features
- **Transport**: Per-endpoint connection pools
- **Buffering**: 64KB buffers with object pooling
- **Circuit Breakers**: Automatic failure isolation
- **Best for**: Production, high loads

## Request Flow

```
1. Request arrives at HTTP handler
   ↓
2. Security middleware (rate limiting, size checks)
   ↓
3. Inspector chain analyzes request
   - Extract model name
   - Determine compatible platforms
   - Identify required capabilities
   ↓
4. Load balancer selects endpoint
   - Check health status
   - Apply balancing strategy
   - Consider priorities
   ↓
5. Proxy engine forwards request
   - Connection pooling (Olla)
   - Circuit breaker checks (Olla)
   - Add tracking headers
   ↓
6. Stream response back
   - Buffered streaming
   - Timeout handling
   - Stats collection
```

## Key Components

### Load Balancing Strategies

**Priority Balancer** (Recommended)
```go
// Selects highest priority healthy endpoint
endpoints := []Endpoint{
    {Name: "primary", Priority: 100},   // Always tried first
    {Name: "backup", Priority: 50},     // Fallback
}
```

**Round Robin**
- Even distribution across all healthy endpoints
- Ignores priorities

**Least Connections**
- Routes to endpoint with fewest active connections
- Best for varying request durations

### Health Checking

Continuous monitoring with exponential backoff:
```
Healthy → Unhealthy: After 3 consecutive failures
Unhealthy → Healthy: After 1 success
Check interval: 30s (healthy) → 60s → 120s (unhealthy)
```

### Circuit Breakers (Olla Engine)

Prevents cascade failures:
```
Closed (normal) → Open (failing) → Half-Open (testing)
                    ↑                    ↓
                    └────────────────────┘
```

- **Closed**: Normal operation
- **Open**: Rejecting requests (after 5 failures)
- **Half-Open**: Testing recovery

### Model Registry

Unified model management across platforms:
```
Original Name → Normalized Name → Unified Model
llama3.2:latest → llama/3.2:latest → llama/3.2
Llama-3.2-3B-Q4 → llama/3.2:3b-q4 → llama/3.2
```

Features:
- Automatic model discovery
- Capability detection (vision, embeddings, code)
- Cross-platform normalization

### Statistics Collection

Lock-free atomic operations for high performance:
```go
// No mutexes, just atomic operations
atomic.AddInt64(&stats.totalRequests, 1)
atomic.StoreInt64(&stats.lastLatency, latencyMs)
```

Tracks:
- Request counts (success/failure)
- Response times (p50, p95, p99)
- Active connections
- Model usage

## Performance Optimizations

### Connection Pooling (Olla)
- Per-endpoint pools reduce connection overhead
- Configurable limits (default: 100 idle, 50 per host)
- Automatic cleanup of stale connections

### Object Pooling (Olla)
- Reusable buffers minimize GC pressure
- Pooled contexts and error objects
- 64KB buffers optimized for streaming

### Streaming Optimization
- Large buffers for LLM token streams
- Immediate flushing for low latency
- Graceful handling of slow consumers

## Configuration

### Proxy Engine Selection
```yaml
proxy:
  engine: olla  # or "sherpa"
  
  # Olla-specific
  max_idle_conns: 100
  max_conns_per_host: 50
  idle_conn_timeout: 90s
```

### Timeout Configuration
```yaml
server:
  read_timeout: 20s       # Initial request read
  write_timeout: 0s       # Disabled for streaming
  
proxy:
  connection_timeout: 40s # Backend connection
  response_timeout: 900s  # Total request time
```

### Security Settings
```yaml
server:
  request_limits:
    max_body_size: 50MB
    max_header_size: 512KB
  rate_limits:
    per_ip_requests_per_minute: 100
    global_requests_per_minute: 1000
```

## Monitoring

### Metrics Endpoints
- `/internal/status` - System overview
- `/internal/stats/models` - Model usage stats
- `/internal/process` - Runtime metrics

### Response Headers
```
X-Olla-Endpoint: workstation-ollama
X-Olla-Model: llama3.2:3b
X-Olla-Backend-Type: ollama
X-Olla-Request-ID: req_abc123
X-Olla-Response-Time: 1234ms
```

### Health Checks
```bash
# Simple health check
GET /internal/health
HTTP/1.1 200 OK

# Detailed status
GET /internal/status
{
  "endpoints": [...],
  "proxy": {
    "total_requests": 1234,
    "avg_latency_ms": 156
  }
}
```

## Security Considerations

### Rate Limiting
- Token bucket algorithm
- Per-IP and global limits
- Separate limits for health endpoints

### Request Validation
- Maximum body size enforcement
- Header size limits
- Content-type validation

### Network Security
- Trusted proxy support (X-Forwarded-For)
- Internal endpoints restricted to local access
- No authentication required (deploy behind firewall)

## Deployment Patterns

### Single Instance
```
Apps → Olla → [Multiple Backends]
```

### High Availability
```
       ┌→ Olla Instance 1 ┐
Apps → LB                 → [Backends]
       └→ Olla Instance 2 ┘
```

### Multi-Region
```
Region A: Olla → [Local Backends + Remote Backup]
Region B: Olla → [Local Backends + Remote Backup]
```

## Performance Benchmarks

Typical performance characteristics:

| Metric | Sherpa | Olla |
|--------|--------|------|
| Overhead | ~0.5ms | ~0.3ms |
| Concurrent connections | 1000s | 10,000s |
| Memory per connection | ~50KB | ~30KB |
| Streaming throughput | 100MB/s | 200MB/s |

## Troubleshooting

### Common Issues

**High latency**
- Check circuit breaker status
- Verify connection pool settings
- Monitor backend health

**Memory usage**
- Tune connection pool sizes
- Adjust buffer sizes
- Check for connection leaks

**Failed requests**
- Review health check logs
- Verify network connectivity
- Check rate limits

### Debug Mode
```bash
export OLLA_LOG_LEVEL=debug
./olla -config config.yaml
```

Shows:
- Request routing decisions
- Health check results
- Connection pool stats
- Circuit breaker state changes