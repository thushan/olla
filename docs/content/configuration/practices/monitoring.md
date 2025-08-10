---
title: "Olla Monitoring Best Practices - Observability & Performance Tracking"
description: "Complete monitoring guide for Olla deployments. Setup health endpoints, metrics collection, alerting, logging, and performance tracking for production systems."
keywords: ["olla monitoring", "observability", "performance monitoring", "health endpoints", "prometheus metrics", "logging", "alerting"]
---

# Monitoring Best Practices

This guide covers monitoring and observability for Olla deployments.

> :memo: **Default Monitoring Configuration**
> ```yaml
> # Built-in endpoints (always enabled)
> # /internal/health - Basic health check
> # /internal/status - Detailed status
> # /internal/status/endpoints - Endpoint details
> # /internal/stats/models - Model statistics
> 
> logging:
>   level: "info"
>   format: "json"
> ```
> **Key Features**:
> 
> - Health endpoints enabled by default
> - JSON logging for structured monitoring
> - No external dependencies required
> 
> **Environment Variables**: `OLLA_LOG_LEVEL`, `OLLA_LOG_FORMAT`

## Monitoring Overview

Effective monitoring helps you:

- Detect issues before users do
- Understand system performance
- Plan capacity and scaling
- Troubleshoot problems quickly
- Track SLA compliance

## Built-in Monitoring

### Health Endpoint

Basic health check:

```bash
curl http://localhost:40114/internal/health
```

Response:
```json
{
  "status": "healthy",
  "endpoints_healthy": 3,
  "endpoints_total": 3
}
```

Use for:

- Load balancer health checks
- Kubernetes liveness probes
- Basic availability monitoring

### Status Endpoint

Detailed system status:

```bash
curl http://localhost:40114/internal/status
```

Response includes:

- Endpoint health details
- Request statistics
- Model registry information
- Circuit breaker states
- Performance metrics

## Key Metrics

### Golden Signals

Monitor these four golden signals:

#### 1. Latency

Track response time percentiles:

```bash
# Get average latency
curl http://localhost:40114/internal/status | jq '.system.avg_latency'
```

Key metrics:

- P50 (median): Normal performance
- P95: Most requests
- P99: Worst-case scenarios

Alerting thresholds:

| Percentile | Good | Warning | Critical |
|------------|------|---------|----------|
| P50 | <100ms | <500ms | >1s |
| P95 | <500ms | <2s | >5s |
| P99 | <2s | <5s | >10s |

#### 2. Traffic

Monitor request rates:

```bash
# Total requests
curl http://localhost:40114/internal/status | jq '.system.total_requests'
```

Track:

- Requests per second
- Request patterns over time
- Peak vs average traffic

#### 3. Errors

Track error rates:

```bash
# Error statistics
curl http://localhost:40114/internal/status | jq '.system.total_failures'
```

Monitor:

- HTTP error codes (4xx, 5xx)
- Circuit breaker trips
- Timeout errors
- Connection failures

Alert on:

- Error rate > 1%
- 5xx errors > 0.1%
- Circuit breaker trips

#### 4. Saturation

Monitor resource usage:

- CPU utilisation
- Memory usage
- Connection pool saturation
- Queue depths

## Response Headers

Olla adds monitoring headers to responses:

```bash
curl -I http://localhost:40114/olla/ollama/v1/models

# Headers:
X-Olla-Endpoint: local-ollama
X-Olla-Model: llama3.2
X-Olla-Backend-Type: ollama
X-Olla-Request-ID: 550e8400-e29b-41d4-a716-446655440000
X-Olla-Response-Time: 125ms
```

Use these for:

- Request tracing
- Performance analysis
- Debugging routing decisions

## Logging

### Log Configuration

Configure appropriate logging:

```yaml
logging:
  level: "info"    # info for production, debug for troubleshooting
  format: "json"   # Structured logs for parsing
  output: "stdout" # Or file path
```

### Log Levels

| Level | Use Case | Volume |
|-------|----------|--------|
| debug | Development/troubleshooting | Very high |
| info | Normal operations | Moderate |
| warn | Potential issues | Low |
| error | Actual problems | Very low |

### Structured Logging

JSON format enables parsing:

```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "msg": "Request completed",
  "endpoint": "local-ollama",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "duration_ms": 125,
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## Prometheus Metrics

### Exporting Metrics

While Olla doesn't have built-in Prometheus support, you can scrape the status endpoint:

```python
# prometheus_exporter.py
import requests
import time
from prometheus_client import start_http_server, Gauge

# Define metrics
requests_total = Gauge('olla_requests_total', 'Total requests')
errors_total = Gauge('olla_errors_total', 'Total errors')
endpoints_healthy = Gauge('olla_endpoints_healthy', 'Healthy endpoints')
response_time_p50 = Gauge('olla_response_time_p50', 'P50 response time')

def collect_metrics():
    while True:
        try:
            resp = requests.get('http://localhost:40114/internal/status')
            data = resp.json()
            
            requests_total.set(data['system']['total_requests'])
            errors_total.set(data['system']['total_failures'])
            endpoints_healthy.set(len([e for e in data['endpoints'] if e['status'] == 'healthy']))
            # Note: Percentile latencies not available in status endpoint
        except:
            pass
        time.sleep(15)

if __name__ == '__main__':
    start_http_server(8000)
    collect_metrics()
```

### Grafana Dashboard

Key panels for Grafana:

1. **Request Rate**: `rate(olla_requests_total[5m])`
2. **Error Rate**: `rate(olla_errors_total[5m])`
3. **Latency**: `olla_response_time_p50`, `p95`, `p99`
4. **Endpoint Health**: `olla_endpoints_healthy`
5. **Success Rate**: `1 - (rate(errors) / rate(requests))`

## Health Monitoring

### Endpoint Health

Monitor individual endpoint health:

```bash
# Check endpoint status
curl http://localhost:40114/internal/status | jq '.endpoints'
```

Track:

- Health status per endpoint
- Last check time
- Failure counts
- Circuit breaker state

### Circuit Breaker Monitoring

Circuit breaker state affects endpoint status. When tripped, endpoints show as unhealthy in the status response.

Alert when:

- Circuit opens (immediate)
- Circuit remains open > 5 minutes
- Multiple circuits open simultaneously

## Performance Monitoring

### Latency Tracking

Monitor latency at different levels:

1. **Olla Overhead**: Time added by proxy
2. **Backend Latency**: Time spent at backend
3. **Network Latency**: Connection time

```bash
# Response headers show total time
X-Olla-Response-Time: 125ms
```

### Throughput Monitoring

Track requests per second:

```bash
# Active connections
curl http://localhost:40114/internal/status | \
  jq '.system.active_connections'
```

Historical tracking:

- Peak throughput times
- Average vs peak ratios
- Throughput per endpoint

### Resource Usage

Monitor system resources:

```bash
# Memory usage
ps aux | grep olla | awk '{print $6}'

# CPU usage
top -p $(pgrep olla) -n 1

# Connection count
netstat -an | grep 40114 | wc -l
```

## Alerting Strategy

### Critical Alerts (Page immediately)

- Service down (health check fails)
- All endpoints unhealthy
- Error rate > 5%
- P99 latency > 10s

### Warning Alerts (Notify team)

- Single endpoint unhealthy
- Error rate > 1%
- P95 latency > 5s
- Memory usage > 80%

### Info Alerts (Log for review)

- Circuit breaker trips
- Rate limit violations
- Configuration reloads
- Model discovery failures

## Log Aggregation

### ELK Stack Integration

Ship logs to Elasticsearch:

```yaml
# filebeat.yml
filebeat.inputs:
- type: container
  paths:
    - '/var/lib/docker/containers/*/*.log'
  processors:
    - decode_json_fields:
        fields: ["message"]
        target: "olla"

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

### Useful Queries

Elasticsearch queries for analysis:

```json
// Error rate by endpoint
{
  "query": {
    "bool": {
      "must": [
        {"term": {"level": "error"}},
        {"range": {"@timestamp": {"gte": "now-1h"}}}
      ]
    }
  },
  "aggs": {
    "by_endpoint": {
      "terms": {"field": "endpoint.keyword"}
    }
  }
}

// Slow requests
{
  "query": {
    "range": {
      "duration_ms": {"gte": 1000}
    }
  }
}
```

## Distributed Tracing

### Request Correlation

Use request IDs for tracing:

```bash
# Request ID in headers
X-Olla-Request-ID: 550e8400-e29b-41d4-a716-446655440000
```

Track requests across:

- Olla proxy
- Backend services
- Frontend applications

### OpenTelemetry Integration

While not built-in, you can add tracing:

```go
// Wrap handlers with OpenTelemetry
import "go.opentelemetry.io/otel"

tracer := otel.Tracer("olla")
ctx, span := tracer.Start(ctx, "proxy_request")
defer span.End()
```

## Capacity Planning

### Metrics for Planning

Track over time:

1. **Request Growth**: Month-over-month increase
2. **Peak Traffic**: Daily/weekly patterns
3. **Resource Usage**: CPU/memory trends
4. **Response Times**: Degradation patterns

### Scaling Indicators

Scale when:

- CPU > 70% sustained
- Memory > 80% sustained
- P95 latency increasing
- Error rate increasing
- Queue depth growing

## Monitoring Tools

### Command-Line Monitoring

Quick status checks:

```bash
# Watch status in real-time
watch -n 5 'curl -s http://localhost:40114/internal/status | jq .'

# Monitor logs
tail -f /var/log/olla.log | jq '.'

# Track specific errors
grep ERROR /var/log/olla.log | jq '.msg'
```

### Monitoring Scripts

Health check script:

```bash
#!/bin/bash
# check_olla.sh

STATUS=$(curl -s http://localhost:40114/internal/health | jq -r '.status')

if [ "$STATUS" != "healthy" ]; then
    echo "CRITICAL: Olla is unhealthy"
    exit 2
fi

ENDPOINTS=$(curl -s http://localhost:40114/internal/status | jq '[.endpoints[] | select(.status == "healthy")] | length')
TOTAL=$(curl -s http://localhost:40114/internal/status | jq '.endpoints | length')

if [ "$ENDPOINTS" -lt "$TOTAL" ]; then
    echo "WARNING: Only $ENDPOINTS/$TOTAL endpoints healthy"
    exit 1
fi

echo "OK: Olla healthy with $ENDPOINTS endpoints"
exit 0
```

## Troubleshooting with Monitoring

### High Latency Investigation

1. Check P50 vs P99 spread
2. Identify slow endpoints
3. Review circuit breaker states
4. Check backend health directly
5. Analyse request patterns

### Error Spike Investigation

1. Check error types (4xx vs 5xx)
2. Identify affected endpoints
3. Review recent changes
4. Check rate limit violations
5. Analyse request logs

## Monitoring Checklist

Production monitoring setup:

- [ ] Health endpoint monitoring
- [ ] Status endpoint collection
- [ ] Log aggregation configured
- [ ] Alert rules defined
- [ ] Dashboard created
- [ ] Request ID correlation
- [ ] Error rate tracking
- [ ] Latency percentiles
- [ ] Resource monitoring
- [ ] Circuit breaker alerts
- [ ] Capacity planning metrics

## Next Steps

- [Security Best Practices](security.md) - Security monitoring
- [Performance Tuning](performance.md) - Performance metrics
- [Configuration Reference](../reference.md) - Monitoring configuration