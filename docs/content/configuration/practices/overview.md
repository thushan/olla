---
title: "Olla Best Practices Overview - Production Deployment Guide"
description: "Essential best practices for deploying and operating Olla in production. Engine selection, rate limiting, timeouts, security, monitoring and operational guidelines."
keywords: ["olla best practices", "production deployment", "operational guidelines", "performance tuning", "configuration recommendations", "deployment patterns"]
---

# Best Practices Overview

This guide provides recommended practices for deploying and operating Olla in production environments.

## Quick Checklist

Essential items for production deployment:

- [ ] Use the Olla engine for high-performance requirements
- [ ] Configure appropriate rate limits
- [ ] Set request size limits
- [ ] Enable health checking with reasonable intervals
- [ ] Use priority load balancing for cost optimisation
- [ ] Configure proper timeouts for your use case
- [ ] Set logging to appropriate level (info or warn)
- [ ] Monitor circuit breaker trips
- [ ] Use model unification for same-type endpoints
- [ ] Implement graceful shutdown handling

## Configuration Recommendations

### Engine Selection

Choose the right proxy engine for your needs:

```yaml
# Development or moderate load
proxy:
  engine: "sherpa"
  
# Production high-throughput
proxy:
  engine: "olla"
```

**Sherpa** is recommended for:

- Development environments
- Small to medium deployments
- When simplicity is preferred
- Load under 100 requests/second

**Olla** is recommended for:

- Production environments
- High-throughput requirements
- When performance is critical
- Load over 100 requests/second

### Load Balancer Strategy

Select based on your requirements:

| Strategy | Use When |
|----------|----------|
| **priority** | You have preferred endpoints (local vs cloud) |
| **round-robin** | All endpoints are equal, want even distribution |
| **least-connections** | Optimising for response time |

```yaml
# Cost optimisation - prefer local
proxy:
  load_balancer: "priority"

# Even distribution
proxy:
  load_balancer: "round-robin"

# Performance optimisation
proxy:
  load_balancer: "least-connections"
```

### Timeout Configuration

Set appropriate timeouts for your use case:

```yaml
server:
  read_timeout: 30s      # Time to read request
  write_timeout: 0s      # Must be 0 for streaming
  shutdown_timeout: 30s  # Graceful shutdown time

proxy:
  connection_timeout: 30s  # Backend connection timeout
```

**Important**: Always set `write_timeout: 0s` to support LLM streaming responses.

### Health Check Intervals

Balance between detection speed and overhead:

```yaml
endpoints:
  - url: "http://localhost:11434"
    check_interval: 5s   # Critical endpoints
    check_timeout: 2s
    
  - url: "http://backup:11434"
    check_interval: 30s  # Less critical endpoints
    check_timeout: 5s
```

Guidelines:

- **Critical endpoints**: 5-10 second intervals
- **Secondary endpoints**: 15-30 second intervals
- **Backup endpoints**: 30-60 second intervals

### Rate Limiting

Protect your infrastructure:

```yaml
server:
  rate_limits:
    # Overall system capacity
    global_requests_per_minute: 10000
    
    # Per-client limits
    per_ip_requests_per_minute: 100
    
    # Monitoring endpoints
    health_requests_per_minute: 5000
    
    # Burst allowance
    burst_size: 50
```

Sizing guidelines:

- Set global limit to 80% of tested capacity
- Per-IP limits prevent single client abuse
- Health endpoints need higher limits for monitoring
- Burst size handles temporary spikes

### Request Size Limits

Prevent resource exhaustion:

```yaml
server:
  request_limits:
    max_body_size: 52428800   # 50MB
    max_header_size: 524288   # 512KB
```

Recommendations by use case:

- **Chat applications**: 10-50MB
- **Code generation**: 50-100MB
- **Document processing**: 100MB+
- **API gateway**: 5-10MB

## Deployment Patterns

### Single Instance

For simple deployments:

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "single-instance"
        type: "ollama"
        priority: 100
```

### Active-Passive Failover

Primary with automatic failover:

```yaml
proxy:
  load_balancer: "priority"

discovery:
  static:
    endpoints:
      - url: "http://primary:11434"
        name: "primary"
        priority: 100
        check_interval: 5s
        
      - url: "http://backup:11434"
        name: "backup"
        priority: 10
        check_interval: 10s
```

### Active-Active Load Balancing

Distribute load across multiple endpoints:

```yaml
proxy:
  load_balancer: "least-connections"

discovery:
  static:
    endpoints:
      - url: "http://node1:11434"
        name: "node-1"
        priority: 100
        
      - url: "http://node2:11434"
        name: "node-2"
        priority: 100
        
      - url: "http://node3:11434"
        name: "node-3"
        priority: 100
```

### Geographic Distribution

Multi-region deployment:

```yaml
proxy:
  load_balancer: "priority"
  # Automatic failover between regions

discovery:
  static:
    endpoints:
      # Primary region
      - url: "http://us-east-1:11434"
        name: "us-east-primary"
        priority: 100
        check_interval: 5s
        
      # Secondary region
      - url: "http://us-west-1:11434"
        name: "us-west-backup"
        priority: 50
        check_interval: 10s
        
      # Disaster recovery
      - url: "http://eu-west-1:11434"
        name: "eu-west-dr"
        priority: 10
        check_interval: 30s
```

## Operational Guidelines

### Monitoring

Key metrics to track:

1. **Request Rate**: Monitor throughput trends
2. **Response Times**: Track P50, P95, P99 latencies
3. **Error Rates**: Watch for increases
4. **Circuit Breaker**: Track trip frequency
5. **Endpoint Health**: Monitor availability
6. **Model Discovery**: Track model availability

### Logging

Configure appropriate log levels:

```yaml
# Development
logging:
  level: "debug"
  format: "text"

# Production
logging:
  level: "info"  # or "warn" for less verbosity
  format: "json"
```

### Graceful Shutdown

Handle shutdown properly:

```yaml
server:
  shutdown_timeout: 30s  # Wait for requests to complete
```

Ensure your deployment:

1. Sends SIGTERM for shutdown
2. Waits for shutdown_timeout
3. Only sends SIGKILL if necessary

### Resource Allocation

Memory considerations:

- **Sherpa engine**: ~50-100MB base + request overhead
- **Olla engine**: ~100-200MB base + connection pools
- **Model registry**: ~10MB per 1000 models

CPU considerations:

- Primarily I/O bound
- 1-2 cores sufficient for most deployments
- Scale horizontally for higher throughput

## Security Considerations

### Network Security

1. **Bind Address**: Use `localhost` unless network access needed
2. **TLS Termination**: Use reverse proxy for HTTPS
3. **Firewall Rules**: Restrict access to Olla port

### Rate Limiting

Always configure rate limits in production:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 60
```

### Request Validation

Set appropriate size limits:

```yaml
server:
  request_limits:
    max_body_size: 10485760  # 10MB
    max_header_size: 131072  # 128KB
```

## Common Pitfalls

### 1. Forgetting write_timeout

**Problem**: Streaming responses timeout

**Solution**:
```yaml
server:
  write_timeout: 0s  # Required for streaming
```

### 2. Aggressive Health Checks

**Problem**: Overloading endpoints with health checks

**Solution**: Use appropriate intervals
```yaml
check_interval: 30s  # Not every second
```

### 3. No Rate Limiting

**Problem**: Single client can overwhelm system

**Solution**: Always configure rate limits

### 4. Wrong Engine Choice

**Problem**: Poor performance with Sherpa under high load

**Solution**: Use Olla engine for production

### 5. Missing Model Unification

**Problem**: Models appear/disappear randomly

**Solution**:
```yaml
model_registry:
  enable_unifier: true
```

## Performance Tuning

### Connection Pooling

The Olla engine maintains connection pools:

```yaml
proxy:
  engine: "olla"
  connection_timeout: 30s
```

### Retry Strategy

Automatic retry on connection failures is built-in as of v0.0.16:

```yaml
proxy:
  # Note: Retry is automatic and built-in for connection failures
  engine: "olla"  # Circuit breaker integration
  load_balancer: "priority"  # Failover to next endpoint
```

The automatic retry mechanism intelligently:
- Only retries connection failures (not application errors)
- Automatically tries different endpoints
- Marks failed endpoints as unhealthy
- Uses exponential backoff for health checks

### Model Discovery

Optimise discovery frequency:

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: 5m  # Not too frequent
    concurrent_workers: 5  # Parallel discovery
```

## Next Steps

- [Security Best Practices](security.md) - Secure your deployment
- [Performance Tuning](performance.md) - Optimise for your workload
- [Monitoring Guide](monitoring.md) - Track system health
- [Configuration Reference](../reference.md) - Complete configuration options