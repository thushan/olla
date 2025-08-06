# Proxy Engines: Sherpa vs Olla

Olla provides two proxy engines optimised for different operational requirements. Understanding when to use each engine helps you choose the right tool for your deployment.

## Quick Comparison

| Feature | Sherpa | Olla |
|---------|--------|------|
| **Target Use** | Development, small deployments | Production, high-traffic |
| **Complexity** | Simple, maintainable | Advanced, feature-rich |
| **Performance** | 5,000-10,000 req/s | 20,000-50,000 req/s |
| **Memory Usage** | Lower baseline | Higher baseline, stable under load |
| **Circuit Breakers** | No | Yes |
| **Connection Pooling** | Shared across endpoints | Per-endpoint pools |
| **Best For** | Simple setups, debugging | High availability, scale |

## Sherpa: The Simple Engine

Sherpa is designed for simplicity and maintainability. It handles moderate workloads efficiently without complex optimisations.

### When to Use Sherpa

Choose Sherpa when:
- **Getting started** with Olla
- **Development or staging** environments
- **Small to medium** production workloads (< 10,000 req/s)
- **Fewer than 10 endpoints**
- **Team prioritises** simple, debuggable implementations
- **Predictable, moderate** traffic patterns

### Configuration

```yaml
proxy:
  engine: "sherpa"
  connection_timeout: 30s
  response_timeout: 10m
  stream_buffer_size: 8192    # 8KB buffers
```

### Performance Characteristics

- **Latency**: Minimal overhead (~1ms)
- **Throughput**: 5,000-10,000 requests/second
- **Memory**: Stable, grows linearly with connections
- **Resource Usage**: Low CPU and memory overhead

### Use Cases

**Development Environment**
```yaml
# Simple setup for local development
proxy:
  engine: "sherpa"
  load_balancer: "round_robin"

discovery:
  endpoints:
    - name: "local-1"
      url: "http://localhost:11434"
    - name: "local-2" 
      url: "http://localhost:11435"
```

**Small Production Setup**
```yaml
# Simple production for small teams
proxy:
  engine: "sherpa"
  load_balancer: "priority"

discovery:
  endpoints:
    - name: "primary-server"
      url: "http://gpu-1:11434"
      priority: 100
    - name: "backup-server"
      url: "http://gpu-2:11434"  
      priority: 50
```

## Olla: The Performance Engine

Olla is built for demanding production environments requiring maximum performance, resilience, and advanced features.

### When to Use Olla

Choose Olla when:
- **High-traffic production** workloads (> 10,000 req/s)
- **Managing 10+ endpoints** with varying performance
- **Requiring circuit breaker protection** against failures
- **Optimising for minimum latency** and maximum throughput
- **Variable endpoint performance** characteristics
- **Streaming large responses** frequently
- **Mission-critical** applications requiring high availability

### Configuration

```yaml
proxy:
  engine: "olla"
  connection_timeout: 30s
  response_timeout: 10m
  stream_buffer_size: 65536     # 64KB buffers
  
  # Olla-specific settings
  max_idle_conns: 100           # Global connection limit
  max_conns_per_host: 50        # Per-endpoint limit
  idle_conn_timeout: 90s        # Connection cleanup
```

### Advanced Features

**Circuit Breakers**
- Automatically isolate failing endpoints
- Prevent cascade failures across your infrastructure
- Configurable failure thresholds and recovery timeouts

**Per-Endpoint Connection Pools**
- Each endpoint gets dedicated connection pool
- Prevents slow endpoints affecting fast ones
- Optimised connection limits per endpoint

**Object Pooling**
- Reduces garbage collection pressure by 60-80%
- Pre-allocated buffers for streaming responses
- Memory-efficient under high load

### Performance Characteristics

- **Latency**: Sub-millisecond overhead
- **Throughput**: 20,000-50,000 requests/second  
- **Memory**: Higher baseline, stable under load
- **GC Impact**: Significantly reduced garbage collection pauses

### Use Cases

**High-Traffic Production**
```yaml
# Production setup with circuit protection
proxy:
  engine: "olla"
  load_balancer: "least_connections"
  max_idle_conns: 200
  max_conns_per_host: 100

discovery:
  endpoints:
    - name: "gpu-cluster-1"
      url: "http://10.0.1.10:11434"
    - name: "gpu-cluster-2"
      url: "http://10.0.1.11:11434"
    - name: "gpu-cluster-3"
      url: "http://10.0.1.12:11434"
```

**Mixed Performance Infrastructure**
```yaml
# Different endpoint capabilities
proxy:
  engine: "olla"               # Handle variable performance
  load_balancer: "priority"

discovery:
  endpoints:
    - name: "fast-a100"         # High-end GPU
      url: "http://a100:11434"
      priority: 100
    - name: "medium-rtx4090"    # Mid-range GPU  
      url: "http://rtx4090:11434"
      priority: 75
    - name: "slow-cpu"          # CPU fallback
      url: "http://cpu:11434"
      priority: 25
```

## Configuration Comparison

### Basic Settings

Both engines share these common settings:

```yaml
proxy:
  engine: "sherpa"              # or "olla"
  connection_timeout: 30s       # TCP connection timeout
  response_timeout: 10m         # Total request timeout
  read_timeout: 2m              # Read operation timeout
```

### Olla-Specific Settings

Olla includes additional tuning options:

```yaml
proxy:
  engine: "olla"
  
  # Connection pooling (Olla only)
  max_idle_conns: 100           # Total idle connections
  max_conns_per_host: 50        # Per-endpoint limit
  idle_conn_timeout: 90s        # Cleanup timeout
  
  # Buffer sizing (both engines, but Olla optimised)
  stream_buffer_size: 65536     # 64KB for Olla, 8KB for Sherpa
```

## Migration Between Engines

### Sherpa to Olla

Upgrade when you need higher performance:

1. **Update Configuration**
   ```yaml
   proxy:
     engine: "olla"              # Change engine
     max_idle_conns: 100         # Add Olla settings
     max_conns_per_host: 50
   ```

2. **Monitor Performance**
   - Watch for improved throughput
   - Monitor circuit breaker activity
   - Verify connection pool efficiency

3. **Test Circuit Breakers**
   - Simulate endpoint failures
   - Confirm automatic recovery
   - Validate fallback behaviour

### Olla to Sherpa

Downgrade for debugging or simplicity:

1. **Simplify Configuration**
   ```yaml
   proxy:
     engine: "sherpa"
     # Remove Olla-specific settings
   ```

2. **Adjust Expectations**
   - Lower throughput capacity
   - No circuit breaker protection
   - Shared connection pooling

## Monitoring

### Common Metrics

Both engines provide these metrics:

```bash
# Check basic proxy status
curl http://localhost:40114/internal/status

# Key metrics for both engines:
# - requests_per_second
# - average_latency
# - success_rate  
# - active_connections
```

### Olla-Specific Monitoring

Olla provides additional insights:

```bash
# Circuit breaker status
# - circuit_breakers_open
# - failure_rates_per_endpoint

# Connection pool health  
# - pool_utilisation
# - connections_per_endpoint
# - pool_efficiency
```

## Troubleshooting

### Common Issues

**Sherpa Performance Limits**
- Symptom: High latency under load
- Solution: Upgrade to Olla engine
- Prevention: Monitor request rates

**Olla Memory Usage**
- Symptom: Higher baseline memory usage
- Solution: Tune pool sizes appropriately
- Prevention: Monitor pool utilisation

**Circuit Breaker False Positives (Olla)**
- Symptom: Healthy endpoints marked as failing
- Solution: Increase failure threshold
- Prevention: Tune thresholds for your infrastructure

### Best Practices

1. **Start with Sherpa** for initial deployments
2. **Upgrade to Olla** when performance demands it  
3. **Monitor actively** regardless of engine choice
4. **Test failure scenarios** with both engines
5. **Tune configuration** based on actual traffic patterns

## Summary

Choose your proxy engine based on your operational requirements:

- **Sherpa**: Simple, reliable, perfect for getting started
- **Olla**: High-performance, resilient, built for production scale

Both engines provide identical functionality from a user perspective—the difference lies in their performance characteristics and operational features. Start simple and upgrade when you need the additional capabilities.