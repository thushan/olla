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
>     check_interval: 5s
>     check_timeout: 2s
> ```
> **Supported Settings**:
> 
> - `check_interval` _(default: 5s)_ - Time between health checks
> - `check_timeout` _(default: 2s)_ - Maximum time to wait for response
> - `check_path` _(auto-detected)_ - Health check endpoint path
> 
> **Note**: Both `check_interval` and `check_timeout` are optional with sensible defaults (5s and 2s respectively), so you don't need to specify them for basic setups.
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
| **Busy** | Responding slowly (above latency threshold) | ✅ Yes | Reduced traffic weight |
| **Warming** | Coming back online | ✅ Yes | Limited test traffic |
| **Offline** | Network/connection errors | ❌ No | Awaiting recovery |
| **Unhealthy** | Failing health checks | ❌ No | No traffic routed |
| **Unknown** | Not yet checked | ❌ No | Awaiting first check |
| **ConfigError** | Reachable but credentials rejected (401/403) | ❌ No | Operator must fix auth config; circuit breaker not tripped |
| **RateLimited** | Endpoint returned 429 | ❌ No | Scheduler honours Retry-After before next probe; circuit breaker not tripped |

## Configuration

### Basic Health Check Setup

```yaml
discovery:
  static:
    endpoints:
      # Minimal configuration - uses profile defaults
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        check_interval: 5s           # How often to check
        check_timeout: 2s            # Timeout per check
        # health_check_url: "/" (automatically set based on type)

      # Custom health check URL
      - url: "http://localhost:8080"
        name: "custom-endpoint"
        type: "vllm"
        health_check_url: "/health"  # Override profile default
        check_interval: 10s
```

### Health Check URL Configuration

The `health_check_url` field is **optional**. When not specified, Olla automatically uses profile-specific defaults based on the endpoint type.

**Profile Defaults:**

| Platform | Default `health_check_url` | Default `model_url` | Expected Response |
|----------|---------------------------|-------------------|-------------------|
| Ollama | `/` | `/api/tags` | 200 with "Ollama is running" |
| llama.cpp | `/health` | `/v1/models` | 200 with JSON status |
| LM Studio | `/v1/models` | `/api/v0/models` | 200 with model list |
| vLLM | `/health` | `/v1/models` | 200 with JSON status |
| SGLang | `/health` | `/v1/models` | 200 with JSON status |
| OpenAI-compatible | `/v1/models` | `/v1/models` | 200 with model list |
| Auto (or unknown) | `/` | `/v1/models` | 200 OK |

### URL Path Behaviour

Both `health_check_url` and `model_url` support:

1. **Relative paths** (recommended) - automatically joined with the endpoint base URL:
   ```yaml
   url: "http://localhost:8080/api/"
   health_check_url: "/health"
   # Results in: http://localhost:8080/api/health
   ```

2. **Absolute URLs** - used as-is for external health monitoring:
   ```yaml
   url: "http://localhost:11434"
   health_check_url: "http://monitoring.local:9090/health/ollama"
   # Health checks go to the monitoring service
   ```

When using relative paths, base path prefixes in the endpoint URL are **automatically preserved**.

### Check Intervals

Configure how frequently health checks are eligible to run:

```yaml
endpoints:
  - url: "http://localhost:11434"
    check_interval: 5s    # Due for check every 5s
    
  - url: "http://remote:11434"
    check_interval: 30s   # Due for check every 30s
```

!!! note "Effective check frequency"
    The health checker runs a background poller every 30 seconds. An endpoint becomes eligible for a check once its `check_interval` has elapsed since the last check, but the poller will only pick it up on its next 30-second tick. This means the effective minimum check granularity is approximately 30 seconds. The `check_interval` value still controls backoff calculation during failures and determines when the endpoint is next scheduled.

**Recommendations**:

- **Local endpoints**: 5-30 seconds (higher values match the poller granularity)
- **LAN endpoints**: 15-30 seconds
- **Remote/Cloud**: 30-60 seconds
- **Critical endpoints**: 5-10 seconds

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

When an endpoint fails, Olla widens the gap between successive health checks rather than hammering a broken backend. The multiplier doubles on each consecutive failure (`2 → 4 → 8 → ...`), capped at `12`. The resulting wait is also clamped to 60 seconds, whichever bound is hit first.

| Consecutive failures | Multiplier applied to `check_interval` | Wait (for `check_interval: 5s`) |
|---|---|---|
| 1 (first failure) | 1 (normal interval) | 5s |
| 2 | 2 | 10s |
| 3 | 4 | 20s |
| 4 | 8 | 40s |
| 5+ | 12 (max) | 60s _(capped)_ |

> The first failure deliberately retries at the normal interval. Most transient errors clear on the very next check, so adding backoff there would slow recovery for the common case. The cap means even a long-dead endpoint is probed at least once per minute.

The HTTP client also has its own attempt-level retry loop (max 2 retries per probe, base delay 100 ms, capped at 2 s, with ±25 % jitter); this fires before any per-endpoint backoff kicks in.

### Recovery

A single successful health check resets the backoff multiplier to `1` and returns the endpoint to its normal `check_interval`. There is no half-open phase: as soon as one probe succeeds, the endpoint is eligible to receive full traffic again. On the same transition, Olla triggers a model-discovery refresh so the unified catalogue reflects what the recovered backend now serves.

### HTTP Circuit Breaker (separate from endpoint state)

The HTTP transport carries its own circuit breaker that trips after `3` consecutive failures (per upstream URL) and stays open for `30 s` before allowing another attempt. This is independent of the per-endpoint adaptive interval above and only affects the in-process HTTP client; it is not configurable from YAML.

### Automatic Model Discovery on Recovery

When an endpoint recovers from an unhealthy state, Olla automatically:

1. **Detects Recovery**: Health check transitions from unhealthy to healthy
2. **Triggers Discovery**: Automatically initiates model discovery
3. **Updates Catalog**: Refreshes the unified model catalog with latest models
4. **Resumes Routing**: Endpoint is immediately available for request routing

This ensures the model catalog stays up-to-date even if models were added/removed while the endpoint was down.

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

For endpoints that support model listing, the `model_url` field is also optional and uses profile defaults:

```yaml
endpoints:
  # Uses profile default model_url
  - url: "http://localhost:11434"
    type: "ollama"
    # model_url: "/api/tags" (automatically set)

  # Custom model discovery URL
  - url: "http://localhost:8080"
    type: "llamacpp"
    model_url: "/v1/models"  # Override if needed

  # External model registry
  - url: "http://localhost:11434"
    type: "ollama"
    model_url: "http://registry.local/models/ollama"
    # Absolute URL to external registry
```

## Connection Failure Handling

### Automatic Retry on Connection Failures

When a request fails due to connection issues, Olla automatically:

1. **Detects Failure**: Identifies connection refused, reset, or timeout errors
2. **Marks Unhealthy**: Immediately updates endpoint status to unhealthy
3. **Retries Request**: Automatically tries the next available healthy endpoint
4. **Updates Health**: Triggers exponential backoff for failed endpoint

This happens transparently without dropping the user request. The retry behaviour is automatic and built-in as of v0.0.16.

Retry only applies before the client has received any of the response. If a backend dies part-way through a streamed response, Olla cannot move the request to another endpoint: it sends the client a provider-format error event (OpenAI `data: {"error":...}`, Anthropic `event: error`, Ollama `{"error":"..."}`) and drops the connection, so the truncated reply is never mistaken for a complete one. See [Retry Behaviour](../configuration/reference.md#retry-behaviour).

Connection errors that trigger automatic retry:
- **Connection Refused**: Backend service is down
- **Connection Reset**: Backend crashed or restarted
- **Connection Timeout**: Backend is overloaded
- **Network Unreachable**: Network connectivity issues

## Circuit Breaker Integration

Health checks work with the circuit breaker to prevent cascade failures:

### Circuit States

```
     Closed (Normal)
          │
          ├─── N failures ──▶ Open (No Traffic)
          │                        │
          │                        │ 30s timeout
          │                        ▼
          └──── 1 success ◀── Half-Open (Test Traffic)
```

- **Closed**: Normal operation, all requests pass through
- **Open**: Endpoint marked unhealthy, no requests sent
- **Half-Open**: Testing recovery with limited requests

### Circuit Breaker Behaviour

There are two independent circuit breakers. The health-checker circuit breaker governs health probes; the Olla proxy engine has a separate per-endpoint circuit breaker that governs live request traffic (Sherpa has none).

1. **Failure Threshold**: 3 consecutive failures (health checker) or 5 consecutive transport failures (Olla proxy engine)
2. **Open Duration**: Circuit stays open for 30 seconds
3. **Half-Open Test**: Allows one test request through
4. **Recovery**: First successful request closes the circuit
5. **What counts as a failure differs by breaker**: the Olla proxy circuit breaker counts only transport-level errors (connection refused, reset, timeout) and ignores backend HTTP 5xx responses; the health-checker circuit breaker counts an unhealthy health-probe response (including a 5xx health check) as a failure in addition to transport errors. Neither counts 401/403 (ConfigError) or 429 (RateLimited).

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

# Network endpoints (e.g. another machine on the LAN) - allow for network latency
- url: "http://gpu-server.internal:8080"
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

### 6. Leverage Profile Defaults

Minimise configuration by relying on profile defaults:

```yaml
endpoints:
  # Minimal - uses all profile defaults
  - url: "http://localhost:11434"
    name: "local-ollama"
    type: "ollama"
    # health_check_url: "/" (automatic)
    # model_url: "/api/tags" (automatic)

  # Only override what you need
  - url: "http://localhost:8080"
    name: "llamacpp"
    type: "llamacpp"
    check_interval: 10s  # Custom interval only
    # health_check_url: "/health" (automatic)
    # model_url: "/v1/models" (automatic)
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
- `/internal/stats/translators` - Translator usage and performance statistics

These can be integrated with external monitoring systems to track:

1. Endpoint availability over time
2. Health check latency trends
3. Failure rates by endpoint
4. Circuit breaker state changes
5. Translator passthrough efficiency and fallback reasons

## Next Steps

- [Load Balancing](load-balancing.md) - How health affects routing
- [Circuit Breaker](../development/circuit-breaker.md) - Failure protection details
- [Monitoring](../configuration/practices/monitoring.md) - Complete monitoring setup