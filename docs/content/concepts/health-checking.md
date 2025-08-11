---
title: "Olla Health Checking - Automated Endpoint Monitoring"
description: "Learn how Olla automatically monitors LLM endpoint health with configurable intervals, adaptive backoff, and circuit breaker integration for reliable request routing."
keywords: ["health checking", "endpoint monitoring", "circuit breaker", "olla health", "availability detection", "automatic recovery", "health status"]
---

# Health Checking

> :memo: **Default Configuration**
> ```yaml
> endpoints:
>   - url: "http://localhost:11434"
>     check_interval: 30s
>     check_timeout: 5s
> ```
> **Supported Settings**:
> 
> - `check_interval` _(default: 30s)_ - Time between health checks
> - `check_timeout` _(default: 5s)_ - Maximum time to wait for response
> - `check_path` _(auto-detected)_ - Health check endpoint path
> 
> **Environment Variables**: Per-endpoint settings not supported via env vars

Olla continuously monitors the health of all configured endpoints to ensure requests are only routed to available backends. The health checking system is automatic and requires minimal configuration.

## Overview

Health checks serve multiple purposes:

- **Availability Detection**: Identify when endpoints come online or go offline
- **Performance Monitoring**: Track endpoint latency and response times
- **Intelligent Routing**: Ensure requests only go to healthy endpoints
- **Automatic Recovery**: Detect when failed endpoints recover

## How It Works

### Health Check Cycle

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Scheduler │────▶│ Health Check │────▶│Update Status│
│  (interval) │     │   Request    │     │   & Route   │
└─────────────┘     └──────────────┘     └─────────────┘
        ▲                                        │
        └────────────────────────────────────────┘
```

1. **Scheduler** triggers checks based on configured intervals
2. **Health Check** sends HTTP request to endpoint's health URL
3. **Status Update** marks endpoint as healthy/unhealthy
4. **Route Update** adds/removes endpoint from routing pool

### Health States

Endpoints can be in one of these states:

| State | Description | Routable | Behaviour |
|-------|-------------|----------|-----------|
| **Healthy** | Passing health checks | ✅ Yes | Normal routing |
| **Degraded** | Slow but responding | ✅ Yes | Reduced traffic weight |
| **Recovering** | Coming back online | ✅ Yes | Limited test traffic |
| **Unhealthy** | Failing health checks | ❌ No | No traffic routed |
| **Unknown** | Not yet checked | ❌ No | Awaiting first check |

## Configuration

### Basic Health Check Setup

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        health_check_url: "/"        # Health endpoint
        check_interval: 5s           # How often to check
        check_timeout: 2s            # Timeout per check
```

### Health Check URLs by Platform

Different platforms use different health endpoints:

| Platform | Default Health URL | Expected Response |
|----------|-------------------|-------------------|
| Ollama | `/` | 200 with "Ollama is running" |
| LM Studio | `/v1/models` | 200 with model list |
| vLLM | `/health` | 200 with JSON status |
| OpenAI-compatible | `/v1/models` | 200 with model list |

### Check Intervals

Configure how frequently health checks run:

```yaml
endpoints:
  - url: "http://localhost:11434"
    check_interval: 5s    # Fast checks for local
    
  - url: "http://remote:11434"
    check_interval: 30s   # Slower for remote
```

**Recommendations**:

- **Local endpoints**: 2-5 seconds
- **LAN endpoints**: 5-10 seconds  
- **Remote/Cloud**: 15-30 seconds
- **Critical endpoints**: 2-3 seconds

### Check Timeouts

Set appropriate timeouts based on endpoint characteristics:

```yaml
endpoints:
  - url: "http://fast-server:11434"
    check_timeout: 1s     # Fast server
    
  - url: "http://slow-server:11434"
    check_timeout: 5s     # Allow more time
```

## Adaptive Health Checking

### Backoff Strategy

When an endpoint fails, Olla implements exponential backoff:

1. **First failure**: Check again after `check_interval`
2. **Second failure**: Wait `check_interval * 2`
3. **Third failure**: Wait `check_interval * 4`
4. **Max backoff**: Capped at 5 minutes

This reduces load on failing endpoints while still detecting recovery.

### Fast Recovery Detection

When an unhealthy endpoint might be recovering:

1. **Half-Open State**: Send limited test traffic
2. **Success Threshold**: After 2 successful checks, mark healthy
3. **Full Traffic**: Resume normal routing

## Health Check Types

### HTTP GET Health Checks

The default health check method:

```yaml
endpoints:
  - url: "http://localhost:11434"
    health_check_url: "/"
    # Sends: GET http://localhost:11434/
    # Expects: 200-299 status code
```

### Model Discovery Health Checks

For endpoints that support model listing:

```yaml
endpoints:
  - url: "http://localhost:11434"
    type: "ollama"
    model_url: "/api/tags"
    # Health check also validates model availability
```

## Circuit Breaker Integration

Health checks work with the circuit breaker to prevent cascade failures:

### Circuit States

```
     Closed (Normal)
          │
          ├─── 3 failures ──▶ Open (No Traffic)
          │                        │
          │                        │ 30s timeout
          │                        ▼
          └──── 2 successes ◀── Half-Open (Test Traffic)
```

- **Closed**: Normal operation, all requests pass through
- **Open**: Endpoint marked unhealthy, no requests sent
- **Half-Open**: Testing recovery with limited requests

### Circuit Breaker Behaviour

The circuit breaker activates after consecutive failures:

1. **Failure Threshold**: 3 consecutive failures trigger opening
2. **Open Duration**: Circuit stays open for 30 seconds
3. **Half-Open Test**: Send 3 test requests
4. **Recovery**: 2 successful tests close the circuit

## Monitoring Health Status

### Health Status Endpoint

Check overall system health:

```bash
curl http://localhost:40114/internal/health
```

Response:

```json
{
  "status": "healthy",
  "endpoints": {
    "healthy": 3,
    "unhealthy": 1,
    "total": 4
  },
  "uptime": "2h15m",
  "version": "1.0.0"
}
```

### Endpoint Status

View detailed endpoint health:

```bash
curl http://localhost:40114/internal/status/endpoints
```

Response:

```json
{
  "endpoints": [
    {
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "status": "healthy",
      "last_check": "2024-01-15T10:30:45Z",
      "last_latency": "15ms",
      "consecutive_failures": 0,
      "uptime_percentage": 99.9
    },
    {
      "name": "remote-ollama",
      "status": "unhealthy",
      "last_check": "2024-01-15T10:30:40Z",
      "consecutive_failures": 6,
      "error": "connection timeout"
    }
  ]
}
```

### Model Statistics

Monitor model performance across endpoints:

```bash
curl http://localhost:40114/internal/stats/models
```

Metrics include:

- Request counts per model
- Model availability across endpoints
- Average check latency
- Endpoints by status

## Troubleshooting

### Endpoint Always Unhealthy

**Issue**: Endpoint never becomes healthy

**Diagnosis**:

```bash
# Test health endpoint directly
curl -v http://localhost:11434/

# Check Olla logs
docker logs olla | grep health
```

**Solutions**:

1. Verify health check URL is correct
2. Increase `check_timeout` for slow endpoints
3. Check if endpoint requires authentication
4. Verify network connectivity

### Flapping Health Status

**Issue**: Endpoint rapidly switching between healthy/unhealthy

**Solutions**:

1. Increase `check_interval` to reduce check frequency:
   ```yaml
   check_interval: 10s  # From 2s
   ```

2. Increase `check_timeout` for variable latency:
   ```yaml
   check_timeout: 5s    # From 1s
   ```

3. Check endpoint logs for intermittent issues

### High Health Check Load

**Issue**: Health checks consuming too many resources

**Solutions**:

1. Increase intervals for stable endpoints:
   ```yaml
   check_interval: 30s  # For very stable endpoints
   ```

2. Use different intervals for different endpoint types:
   ```yaml
   # Critical, local
   - url: "http://localhost:11434"
     check_interval: 5s
   
   # Stable, remote  
   - url: "http://remote:11434"
     check_interval: 60s
   ```

### False Positives

**Issue**: Endpoint marked healthy but requests fail

**Solutions**:

1. Verify health check URL actually validates service:
   ```yaml
   # Bad: Just checks if port is open
   health_check_url: "/"
   
   # Good: Checks if models are loaded
   health_check_url: "/api/tags"
   ```

2. Add model discovery to validate functionality:
   ```yaml
   model_url: "/api/tags"
   # This ensures models are actually available
   ```

## Best Practices

### 1. Use Appropriate Health Endpoints

Choose health check URLs that validate actual functionality:

- ❌ `/` - Only checks if server responds
- ✅ `/api/tags` - Verifies models are available
- ✅ `/v1/models` - Confirms API is operational

### 2. Set Realistic Timeouts

Balance between quick failure detection and false positives:

```yaml
# Local endpoints - fast timeout
- url: "http://localhost:11434"
  check_timeout: 1s

# Remote endpoints - allow for network latency
- url: "https://api.example.com"
  check_timeout: 5s
```

### 3. Configure Check Intervals

Match check frequency to endpoint stability:

```yaml
# Development - frequent checks
check_interval: 2s

# Production - balanced
check_interval: 10s

# Stable external APIs - less frequent
check_interval: 30s
```

### 4. Monitor Health Metrics

Track health check performance:

- Success rate should be > 95%
- Check latency should be consistent
- Watch for patterns in failures

### 5. Use Priority with Health

Combine health checking with priority routing:

```yaml
endpoints:
  # Primary - check frequently
  - url: "http://primary:11434"
    priority: 100
    check_interval: 5s
    
  # Backup - check less often
  - url: "http://backup:11434"
    priority: 50
    check_interval: 15s
```

## Advanced Configuration

### Custom Health Check Headers

While Olla doesn't support custom headers in configuration, you can use a reverse proxy:

```nginx
# nginx configuration
location /health {
    proxy_pass http://backend/health;
    proxy_set_header Authorization "Bearer token";
}
```

### Health Check Scripting

For complex health validation, use an external script:

```bash
#!/bin/bash
# custom-health-check.sh

# Check if Ollama is running
curl -s http://localhost:11434/ > /dev/null || exit 1

# Check if specific model is loaded
curl -s http://localhost:11434/api/tags | grep -q "llama3" || exit 1

# Check disk space
df -h | grep -q "9[0-9]%" && exit 1

exit 0
```

Run periodically and update Olla configuration based on results.

## Integration with Monitoring

Olla provides health and status information through its internal endpoints:

- `/internal/health` - Overall system health
- `/internal/status` - Detailed status information
- `/internal/status/endpoints` - Endpoint health details
- `/internal/stats/models` - Model usage statistics

These can be integrated with external monitoring systems to track:

1. Endpoint availability over time
2. Health check latency trends
3. Failure rates by endpoint
4. Circuit breaker state changes

## Next Steps

- [Load Balancing](load-balancing.md) - How health affects routing
- [Circuit Breaker](../development/circuit-breaker.md) - Failure protection details
- [Monitoring](../configuration/practices/monitoring.md) - Complete monitoring setup