---
title: "Olla Performance Best Practices - Optimization & Tuning Guide"
description: "Complete performance optimization guide for Olla. Engine selection, connection pooling, memory tuning, streaming configuration and production optimizations."
keywords: ["olla performance", "performance optimization", "performance tuning", "connection pooling", "memory optimization", "streaming performance"]
---

# Performance Best Practices

This guide covers performance optimisation techniques for Olla deployments.

> :memo: **Default Configuration**
> ```yaml
> proxy:
>   engine: "sherpa"              # Simple, maintainable engine
>   load_balancer: "priority"     # Fastest routing decisions
>   stream_buffer_size: 8192      # 8KB buffer (balanced)
>   connection_timeout: 30s       # Connection reuse duration
> 
> server:
>   write_timeout: 0s             # Required for streaming
>   read_timeout: 30s             # Request timeout
> ```
> **Performance Defaults**:
> 
> - Sherpa engine for simplicity (use "olla" for production)
> - Priority load balancer for lowest CPU usage
> - 8KB buffer size for balanced performance
> 
> **Environment Variables**: `OLLA_PROXY_ENGINE`, `OLLA_PROXY_LOAD_BALANCER`

## Performance Overview

Olla is designed for high-performance LLM request routing with:

- Sub-millisecond routing decisions
- Minimal memory overhead
- Efficient connection pooling
- Lock-free statistics collection
- Zero-copy streaming where possible

## Engine Selection

### Sherpa vs Olla Engine

Choose the right engine for your performance needs:

| Metric | Sherpa | Olla |
|--------|--------|------|
| Throughput | Moderate | High |
| Latency Overhead | Low (1-5ms) | Very Low (<1ms) |
| Memory Usage | 50-100MB | 100-200MB |
| Connection Pooling | Basic | Advanced |
| Best For | Development | Production |

```yaml
# High-performance configuration
proxy:
  engine: "olla"  # Use Olla engine for production
```

## Load Balancer Optimisation

### Strategy Performance Impact

Different strategies have different performance characteristics:

| Strategy | CPU Usage | Memory | Latency | Best For |
|----------|-----------|---------|---------|----------|
| priority | Lowest | Lowest | Fastest | Static preferences |
| round-robin | Low | Low | Fast | Even distribution |
| least-connections | Medium | Medium | Medium | Dynamic load |

```yaml
# Fastest routing decision
proxy:
  load_balancer: "priority"

# Best response times under load
proxy:
  load_balancer: "least-connections"
```

## Connection Management

### Connection Pooling

The Olla engine maintains persistent connections:

```yaml
proxy:
  engine: "olla"
  connection_timeout: 30s  # Reuse connections for 30s
```

Benefits:

- Eliminates TCP handshake overhead
- Reduces latency by 10-50ms per request
- Lower CPU usage
- Better throughput

### Connection Reuse

The Olla engine maintains persistent connections for better performance:

```yaml
# Connection reuse configuration
proxy:
  connection_timeout: 60s  # Keep connections alive for reuse
```

## Timeout Tuning

### Critical Timeout Settings

```yaml
server:
  read_timeout: 30s       # Time to read request
  write_timeout: 0s       # MUST be 0 for streaming
  idle_timeout: 120s      # Keep-alive duration
  shutdown_timeout: 30s   # Graceful shutdown

proxy:
  connection_timeout: 30s # Backend connection timeout
```

**Important**: `write_timeout: 0s` is required for LLM streaming.

### Health Check Optimisation

Balance detection speed vs overhead:

```yaml
endpoints:
  - url: "http://localhost:11434"
    check_interval: 30s    # Not too frequent
    check_timeout: 2s      # Fast failure detection
```

Too frequent checks waste resources:

- 5s interval = 12 checks/minute/endpoint
- 30s interval = 2 checks/minute/endpoint
- With 10 endpoints, that's 120 vs 20 checks/minute

## Memory Optimisation

### Memory Usage Patterns

Typical memory usage:

| Component | Sherpa | Olla |
|-----------|--------|------|
| Base | 30MB | 50MB |
| Per Connection | 4KB | 8KB |
| Per Model | 1KB | 1KB |
| Request Buffer | 64KB | 128KB |

### Reducing Memory Usage

```yaml
# Memory-conscious configuration
server:
  request_limits:
    max_body_size: 5242880    # 5MB instead of 50MB
    max_header_size: 65536    # 64KB instead of 512KB

model_registry:
  unification:
    stale_threshold: 1h       # Aggressive cleanup
    cleanup_interval: 5m      # Frequent cleanup
```

### Buffer Tuning

The `stream_buffer_size` setting is crucial for balancing latency and throughput:

```yaml
# Low-latency configuration (chat applications)
proxy:
  engine: "sherpa"
  profile: "streaming"
  stream_buffer_size: 2048   # 2KB - Fastest first token (~5ms)
  
# Balanced configuration (general purpose)
proxy:
  engine: "sherpa"
  profile: "auto"
  stream_buffer_size: 8192   # 8KB - Good balance (~20ms)

# High-throughput configuration (batch processing)
proxy:
  engine: "olla"
  profile: "standard"
  stream_buffer_size: 65536  # 64KB - Maximum throughput (~150ms)
```

#### Buffer Size Trade-offs

| Buffer Size | First Token | Throughput | Memory/Req | Best For |
|-------------|-------------|------------|------------|----------|
| 2KB | ~5ms | Lower | 2KB | Interactive chat |
| 4KB | ~10ms | Moderate | 4KB | Responsive UIs |
| 8KB | ~20ms | Good | 8KB | General purpose |
| 16KB | ~40ms | Better | 16KB | API serving |
| 64KB | ~150ms | Best | 64KB | Batch operations |

See [Stream Buffer Size](../../concepts/proxy-engines.md#stream-buffer-size) for detailed analysis.

## CPU Optimisation

### Reducing CPU Usage

1. **Use Priority Load Balancing**: Least CPU intensive
2. **Increase Health Check Intervals**: Reduce background work
3. **Disable Request Logging**: Significant CPU savings
4. **Use JSON Logging**: More efficient than text

```yaml
# CPU-optimised configuration
proxy:
  load_balancer: "priority"      # Lowest CPU usage

server:
  request_logging: false          # Disable for performance

logging:
  level: "warn"                   # Reduce logging
  format: "json"                  # More efficient

discovery:
  model_discovery:
    enabled: false                # Disable if not needed
```

## Network Optimisation

### Reduce Network Latency

1. **Colocate Olla with Backends**: Same machine or network
2. **Use Internal Networks**: Avoid internet routing
3. **Enable Keep-Alive**: Reuse connections

```yaml
discovery:
  static:
    endpoints:
      # Use localhost when possible
      - url: "http://localhost:11434"
        name: "local"
        priority: 100
        
      # Use internal IPs over public
      - url: "http://10.0.1.10:11434"
        name: "internal"
        priority: 90
```

### TCP Tuning

System-level optimisations:

```bash
# Increase socket buffer sizes
sysctl -w net.core.rmem_max=134217728
sysctl -w net.core.wmem_max=134217728

# TCP optimisation
sysctl -w net.ipv4.tcp_nodelay=1
sysctl -w net.ipv4.tcp_quickack=1
```

## Streaming Performance

### Optimise for Streaming

```yaml
proxy:
  engine: "olla"
  profile: "streaming"  # Optimised for token streaming

server:
  write_timeout: 0s     # Required for streaming
```

### Chunk Size Tuning

The streaming profile uses optimal chunk sizes:

- Small chunks (4KB): Lower latency
- Large chunks (64KB): Better throughput

## Caching Strategies

### Model Registry Caching

```yaml
model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    stale_threshold: 24h      # Cache models for 24h
    cleanup_interval: 30m     # Infrequent cleanup
```

### Health Check Caching

Health check results are cached between intervals:

```yaml
endpoints:
  - check_interval: 60s  # Cache health for 60s
    check_timeout: 5s
```

## Benchmarking

### Load Testing

Use tools like `hey` or `ab`:

```bash
# Test throughput
hey -n 10000 -c 100 http://localhost:40114/internal/health

# Test streaming
hey -n 100 -c 10 -m POST \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2","prompt":"test"}' \
  http://localhost:40114/olla/ollama/api/generate
```

### Key Metrics to Monitor

| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| P50 Latency | <10ms | <50ms | >100ms |
| P99 Latency | <100ms | <500ms | >1s |
| CPU Usage | <50% | <80% | >90% |
| Memory | <200MB | <500MB | >1GB |

## Production Optimisations

### High-Throughput Configuration

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  request_logging: false       # Disable logging
  
  request_limits:
    max_body_size: 52428800    # 50MB
    max_header_size: 524288    # 512KB

proxy:
  engine: "olla"               # High-performance engine
  profile: "auto"              # Dynamic selection
  load_balancer: "least-connections"
  connection_timeout: 60s      # Long connection reuse
  max_retries: 2               # Limit retries

discovery:
  model_discovery:
    enabled: true
    interval: 15m              # Infrequent updates
    concurrent_workers: 10     # Parallel discovery

logging:
  level: "error"               # Minimal logging
  format: "json"
```

### Low-Latency Configuration

```yaml
server:
  host: "localhost"
  port: 40114
  read_timeout: 10s            # Fast failure
  
proxy:
  engine: "olla"
  profile: "streaming"         # Optimise for streaming
  load_balancer: "priority"    # Fastest decisions
  connection_timeout: 120s     # Reuse connections
  max_retries: 1               # Fast failure

discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local"
        priority: 100
        check_interval: 60s    # Reduce overhead
        check_timeout: 1s      # Fast detection

logging:
  level: "error"
  format: "json"
```

## Scaling Strategies

### Vertical Scaling

When to scale up:

- CPU consistently >80%
- Memory pressure
- Network saturation

### Horizontal Scaling

Deploy multiple Olla instances:

```nginx
upstream olla_cluster {
    least_conn;
    server olla1:40114;
    server olla2:40114;
    server olla3:40114;
}

server {
    location / {
        proxy_pass http://olla_cluster;
    }
}
```

### Load Distribution

Distribute by:

1. **Geographic Region**: Deploy per region
2. **Model Type**: Dedicated instances per model
3. **Priority**: Separate production/development

## Performance Troubleshooting

### High CPU Usage

Causes and solutions:

1. **Frequent Health Checks**: Increase intervals
2. **Debug Logging**: Switch to warn/error level
3. **Request Logging**: Disable if not needed
4. **Wrong Engine**: Switch to Olla engine

### High Memory Usage

Causes and solutions:

1. **Large Request Limits**: Reduce max_body_size
2. **Connection Leaks**: Check timeout settings
3. **Model Registry**: Reduce stale_threshold
4. **Memory Leaks**: Update to latest version

### High Latency

Causes and solutions:

1. **Network Distance**: Colocate services
2. **Cold Connections**: Increase connection_timeout
3. **Overloaded Backends**: Add more endpoints
4. **Wrong Load Balancer**: Try least-connections

## Monitoring Performance

### Key Metrics

Monitor these for performance:

```bash
# Average latency
curl http://localhost:40114/internal/status | jq '.system.avg_latency'

# Total requests
curl http://localhost:40114/internal/status | jq '.system.total_requests'

# Active connections
curl http://localhost:40114/internal/status | jq '.system.active_connections'
```

### Performance Dashboards

Track over time:

1. Request rate (req/s)
2. Response time (P50, P95, P99)
3. Error rate (%)
4. CPU usage (%)
5. Memory usage (MB)
6. Network I/O (MB/s)

## Next Steps

- [Monitoring Guide](monitoring.md) - Track performance metrics
- [Configuration Reference](../reference.md) - All performance settings
- [Proxy Engines](../../concepts/proxy-engines.md) - Detailed engine comparison