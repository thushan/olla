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
from prometheus_client import start_http_server, Gauge, Histogram

# Define metrics
requests_total = Gauge('olla_requests_total', 'Total requests')
errors_total = Gauge('olla_errors_total', 'Total errors')
endpoints_healthy = Gauge('olla_endpoints_healthy', 'Healthy endpoints')
response_time_p50 = Gauge('olla_response_time_p50', 'P50 response time')

# Provider metrics (from extracted data)
tokens_per_second = Histogram('olla_tokens_per_second', 'Token generation speed', 
                              ['endpoint', 'model'])
prompt_tokens = Histogram('olla_prompt_tokens', 'Prompt token count',
                          ['endpoint', 'model'])
completion_tokens = Histogram('olla_completion_tokens', 'Completion token count',
                              ['endpoint', 'model'])

def collect_metrics():
    while True:
        try:
            resp = requests.get('http://localhost:40114/internal/status')
            data = resp.json()
            
            requests_total.set(data['system']['total_requests'])
            errors_total.set(data['system']['total_failures'])
            endpoints_healthy.set(len([e for e in data['endpoints'] if e['status'] == 'healthy']))
            
            # Extract provider metrics if available
            for endpoint_name, stats in data.get('proxy', {}).get('endpoints', {}).items():
                if 'avg_tokens_per_second' in stats:
                    tokens_per_second.labels(
                        endpoint=endpoint_name,
                        model=stats.get('primary_model', 'unknown')
                    ).observe(stats['avg_tokens_per_second'])
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
6. **Token Generation Speed**: `olla_tokens_per_second` (from provider metrics)
7. **Token Usage**: `olla_prompt_tokens` + `olla_completion_tokens`
8. **Translator Passthrough Rate**: `olla_translator_passthrough_requests / olla_translator_total_requests` (from translator metrics)
9. **Translator Fallback Reasons**: Breakdown of `olla_translator_fallback_*` counters
10. **Translator Latency**: `olla_translator_avg_latency_ms` per translator

## Provider Metrics

Olla automatically extracts performance metrics from LLM provider responses:

### Available Metrics

| Metric | Description | Source |
|--------|-------------|--------|
| `tokens_per_second` | Generation speed | Ollama, LM Studio |
| `prompt_tokens` | Input token count | All providers |
| `completion_tokens` | Output token count | All providers |
| `total_duration_ms` | End-to-end time | Ollama |
| `eval_duration_ms` | Generation time | Ollama |
| `time_per_token_ms` | Per-token latency | Calculated |

### Accessing Provider Metrics

1. **Debug Logs**: Metrics appear in debug-level logs
2. **Status Endpoint**: Aggregated in `/internal/status`
3. **Custom Extraction**: Parse from debug logs

See [Provider Metrics Documentation](../../concepts/provider-metrics.md) for configuration details.

## Translator Metrics

Olla tracks comprehensive metrics for API translation requests, providing visibility into passthrough vs translation usage, fallback behaviour, and performance.

### Available Translator Metrics

Translator metrics are collected per-translator (e.g., "anthropic") and include:

| Metric | Type | Description |
|--------|------|-------------|
| `total_requests` | Counter | Total requests processed |
| `successful_requests` | Counter | Requests that completed successfully |
| `failed_requests` | Counter | Requests that failed |
| `passthrough_requests` | Counter | Requests forwarded directly (native format) |
| `translation_requests` | Counter | Requests that required format conversion |
| `streaming_requests` | Counter | Streaming (SSE) requests |
| `non_streaming_requests` | Counter | Non-streaming requests |
| `fallback_no_compatible_endpoints` | Counter | Fallbacks due to no healthy endpoints |
| `fallback_translator_does_not_support_passthrough` | Counter | Fallbacks due to translator lacking passthrough |
| `fallback_cannot_passthrough` | Counter | Fallbacks due to no backends with native support |
| `avg_latency_ms` | Gauge | Average request latency in milliseconds |
| `total_latency_ms` | Counter | Cumulative latency across all requests |

### Key Metrics to Track

**Passthrough Efficiency**: Monitor the ratio of `passthrough_requests` to `translation_requests`. A high passthrough rate indicates backends are being used optimally.

**Fallback Reasons**: Track `fallback_*` counters to understand why passthrough isn't being used:

- `fallback_no_compatible_endpoints` - No healthy endpoints available (operational issue)
- `fallback_cannot_passthrough` - Backends don't declare native Anthropic support (configuration issue)
- `fallback_translator_does_not_support_passthrough` - Expected for translators without passthrough capability

**Success Rate**: Compare `successful_requests` vs `failed_requests` to detect translation issues.

### Response Header Observability

The `X-Olla-Mode: passthrough` response header is included when passthrough mode is active. This allows external monitoring tools to track mode usage:

```bash
# Check which mode was used for a request
curl -sI -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model":"llama4:latest","max_tokens":10,"messages":[{"role":"user","content":"Hi"}]}' \
  | grep X-Olla-Mode
```

### Implementation Details

Translator metrics are collected using thread-safe `xsync` counters in `internal/adapter/stats/translator_collector.go`. Metrics are recorded at all decision points in the translation handler (`internal/app/handlers/handler_translation.go`), including:

- Early exits (body read errors, transform errors)
- Endpoint lookup failures
- Passthrough mode selection
- Translation mode fallback with reason tracking
- Request completion (success or failure)

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
- [ ] Translator metrics tracking (passthrough/translation rates, fallback reasons)
- [ ] `X-Olla-Mode` header monitoring for passthrough efficiency

## Next Steps

- [Security Best Practices](security.md) - Security monitoring
- [Performance Tuning](performance.md) - Performance metrics
- [Configuration Reference](../reference.md) - Monitoring configuration