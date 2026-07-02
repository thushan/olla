# System Endpoints

Internal endpoints for health monitoring, system status, and process information.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/version` | Get Olla version information |
| GET | `/internal/health` | Health check endpoint |
| GET | `/internal/status` | System status and statistics |
| GET | `/internal/metrics` | Prometheus metrics (same data as `/internal/status` and `/internal/stats/models`) |
| GET | `/internal/status/endpoints` | Detailed endpoint status |
| GET | `/internal/status/models` | Model registry status |
| GET | `/internal/stats/models` | Model usage statistics |
| GET | `/internal/stats/translators` | Translator usage and performance statistics |
| GET | `/internal/stats/sticky` | Sticky session statistics (returns `{"enabled":false}` when sticky sessions are disabled) |
| GET | `/internal/process` | Process information and metrics |

---

## GET /version

Get version information about the running Olla instance.

### Request

```bash
curl -X GET http://localhost:40114/version
```

### Response

```json
{
  "name": "Olla",
  "version": "0.1.0",
  "edition": "community",
  "description": "High-performance proxy and load balancer for LLM infrastructure",
  "build": {
    "commit": "abc123def",
    "date": "2026-04-15T10:00:00Z",
    "go_version": "go1.24.0",
    "platform": "linux/amd64"
  },
  "capabilities": ["proxy", "load-balancing", "health-checking"],
  "capabilities_experimental": [],
  "supported_backends": ["ollama", "lm-studio", "vllm", "llamacpp"],
  "api": {
    "version": "v1",
    "endpoints": {
      "health": "/internal/health",
      "metrics": "/internal/metrics",
      "status": "/internal/status",
      "process": "/internal/process",
      "version": "/version"
    }
  },
  "links": {
    "homepage": "https://github.com/thushan/olla",
    "documentation": "https://github.com/thushan/olla#readme",
    "releases": "https://github.com/thushan/olla/releases/latest"
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Application name |
| `version` | string | Olla version |
| `edition` | string | Build edition |
| `description` | string | Application description |
| `build` | object | Build metadata |
| `build.commit` | string | Git commit hash |
| `build.date` | string | Build timestamp |
| `build.go_version` | string | Go toolchain version used to build |
| `build.platform` | string | OS/arch the binary is running on |
| `capabilities` | array | Stable capabilities advertised by this build |
| `capabilities_experimental` | array | Experimental capabilities advertised by this build |
| `supported_backends` | array | Backend types this build supports |
| `api` | object | API metadata |
| `api.version` | string | Public API version |
| `api.endpoints` | object | Map of named endpoints to their paths |
| `links` | object | Project links (homepage, documentation, releases) |

---

## GET /internal/health

Health check endpoint for monitoring Olla's availability and backend connectivity.

### Request

```bash
curl -X GET http://localhost:40114/internal/health
```

### Response

```json
{
  "status": "healthy"
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always `"healthy"` when the process is responding. The endpoint always returns HTTP 200 with this body; it confirms the process is alive but does not reflect backend health. |

> :memo: For per-endpoint health, request statistics, and proxy configuration, use [`/internal/status`](#get-internalstatus) or `/internal/status/endpoints`.

---

## GET /internal/status

Detailed system status including statistics, configuration, and model information.

### Request

```bash
curl -X GET http://localhost:40114/internal/status
```

### Response

```json
{
  "timestamp": "2026-04-15T10:30:00Z",
  "endpoints": [
    {
      "name": "local-ollama",
      "status": "healthy",
      "success_rate": "99.6%",
      "avg_latency": "120ms",
      "traffic": "1.2 GB",
      "last_check": "2 seconds ago",
      "next_check": "in 3 seconds",
      "issues": "",
      "models": { "last_updated": "2026-04-15T10:25:00Z", "count": 3 },
      "priority": 100,
      "connections": 2,
      "requests": 1200
    }
  ],
  "proxy": {
    "engine": "olla",
    "profile": "auto",
    "balancer": "least-connections"
  },
  "security": {
    "status": "normal",
    "blocked_ips": 0,
    "violations": { "rate_limits": 0, "size_limits": 0 }
  },
  "system": {
    "status": "healthy",
    "endpoints_up": "2/2",
    "success_rate": "99.2%",
    "avg_latency": "125ms",
    "total_traffic": "1.5 GB",
    "uptime": "2h30m15s",
    "version": "0.1.0",
    "commit": "abc123de",
    "active_connections": 3,
    "security_violations": 0,
    "total_requests": 1523,
    "total_failures": 12
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 timestamp at which the response was generated |
| `endpoints` | array | Per-endpoint runtime view (see below) |
| `proxy` | object | Active proxy configuration: `engine`, `profile`, `balancer` |
| `security` | object | Security posture: `status`, `blocked_ips`, `violations.{rate_limits, size_limits}` |
| `system` | object | Aggregate system summary (see below) |

**`endpoints[]` fields**: `name`, `status`, `success_rate`, `avg_latency`, `traffic`, `last_check`, `next_check`, `issues`, `models.{last_updated, count}`, `priority`, `connections`, `requests`. The `last_check` and `next_check` values are human-readable relative strings (e.g. `"2 seconds ago"`, `"in 3 seconds"`), not RFC3339 timestamps.

**`system` fields**: `status` (healthy/degraded/critical), `endpoints_up`, `success_rate`, `avg_latency`, `total_traffic`, `uptime`, `version`, `commit`, `active_connections`, `security_violations`, `total_requests`, `total_failures`.

---

## GET /internal/metrics

Prometheus text exposition of the same operational data as [`/internal/status`](#get-internalstatus) and [`/internal/stats/models`](#get-internalstatsmodels). Use this endpoint for Prometheus, Grafana Agent, or VictoriaMetrics scraping.

### Request

```bash
curl -X GET http://localhost:40114/internal/metrics
```

### Response

Content-Type: `text/plain; version=0.0.4; charset=utf-8`

```text
# HELP olla_requests_total Total proxy requests processed
# TYPE olla_requests_total counter
olla_requests_total 1523
# HELP olla_endpoints_healthy Number of healthy endpoints
# TYPE olla_endpoints_healthy gauge
olla_endpoints_healthy 2
# HELP olla_model_requests_total Total requests for a model
# TYPE olla_model_requests_total counter
olla_model_requests_total{model="llama3.2"} 1200
```

### Metrics exposed

| Metric | Type | Description |
|--------|------|-------------|
| `olla_info` | gauge | Build and proxy config (`version`, `commit`, `engine`, `profile`, `balancer` labels) |
| `olla_system_status` | gauge | Overall status: `2`=healthy, `1`=degraded, `0`=critical |
| `olla_endpoints_total` | gauge | Total configured endpoints |
| `olla_endpoints_healthy` | gauge | Healthy endpoint count |
| `olla_success_rate_percent` | gauge | Proxy success rate |
| `olla_avg_latency_ms` | gauge | Average proxy latency (ms) |
| `olla_total_traffic_bytes` | gauge | Total traffic (bytes) |
| `olla_uptime_seconds` | gauge | Process uptime |
| `olla_active_connections` | gauge | Active connections |
| `olla_requests_total` | counter | Total proxy requests |
| `olla_failures_total` | counter | Total failed requests |
| `olla_security_*` | gauge/counter | Security posture and violations |
| `olla_endpoint_up` | gauge | Endpoint health (`endpoint`, `status` labels; value `1`=up) |
| `olla_endpoint_*` | gauge/counter | Per-endpoint stats (`endpoint` label only) |
| `olla_models_*` | gauge/counter | Aggregate model stats (same summary as `/internal/stats/models`) |
| `olla_model_*` | gauge/counter | Per-model stats (`model` label) |
| `olla_model_endpoint_*` | gauge/counter | Per-model, per-endpoint stats (`model`, `endpoint` labels) |

> :memo: Translator-specific metrics remain on [`/internal/stats/translators`](#get-internalstatstranslators). Model routing and usage metrics from `/internal/stats/models` are included in `/internal/metrics`; scrape `/internal/stats/translators` separately if you need translation passthrough rates.

### Prometheus scrape config

```yaml
scrape_configs:
  - job_name: olla
    static_configs:
      - targets: ['localhost:40114']
    metrics_path: /internal/metrics
```

> :memo: When running Olla in Docker, bind the server to all interfaces (`server.host: "0.0.0.0"` or `OLLA_SERVER_HOST=0.0.0.0`) so published ports are reachable from the host.

---

## GET /internal/stats/models

Model usage statistics including per-model request counts, latency, routing effectiveness, and optional per-endpoint breakdowns. The same data is exposed in Prometheus format on [`/internal/metrics`](#get-internalmetrics) as `olla_models_*`, `olla_model_*`, and `olla_model_endpoint_*` series.

### Request

```bash
curl -X GET http://localhost:40114/internal/stats/models
curl -X GET "http://localhost:40114/internal/stats/models?include_endpoints=true&include_summary=true"
```

Query parameters: `include_endpoints=true` adds per-endpoint breakdown per model; `include_summary=true` adds aggregated endpoint summaries.

---

## GET /internal/process

Process information and resource metrics.

### Request

```bash
curl -X GET http://localhost:40114/internal/process
```

### Response

```json
{
  "timestamp": "2026-04-15T10:30:00Z",
  "memory": {
    "heap_alloc": "45.2 MB",
    "heap_sys": "72.3 MB",
    "heap_inuse": "50.1 MB",
    "heap_released": "12.0 MB",
    "stack_inuse": "1.5 MB",
    "total_alloc": "512.8 MB",
    "memory_pressure": "low"
  },
  "garbage_collection": {
    "last_gc": "2026-04-15T10:29:50Z",
    "total_gc_time": "120ms",
    "avg_gc_pause": "2.9ms",
    "gc_cpu_fraction": 0.00021,
    "num_gc_cycles": 42
  },
  "goroutines": {
    "health_status": "healthy",
    "count": 28,
    "cgo_calls": 0
  },
  "runtime": {
    "uptime": "2h30m15s",
    "go_version": "go1.24.0",
    "num_cpu": 8,
    "gomaxprocs": 8
  },
  "allocations": {
    "total_mallocs": 1532411,
    "total_frees": 1406979,
    "net_objects": 125432
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 timestamp at which the snapshot was taken |
| `memory.heap_alloc` | string | Currently allocated heap, human-readable |
| `memory.heap_sys` | string | Heap memory obtained from OS |
| `memory.heap_inuse` | string | Heap memory currently in use |
| `memory.heap_released` | string | Heap memory released back to OS |
| `memory.stack_inuse` | string | Stack memory in use |
| `memory.total_alloc` | string | Cumulative bytes allocated for heap objects |
| `memory.memory_pressure` | string | Derived pressure indicator (low/medium/high) |
| `garbage_collection.last_gc` | string | RFC3339 timestamp of last GC (omitted if none yet) |
| `garbage_collection.total_gc_time` | string | Cumulative GC time, human-readable |
| `garbage_collection.avg_gc_pause` | string | Average pause per GC cycle |
| `garbage_collection.gc_cpu_fraction` | float | Fraction of CPU time spent in GC |
| `garbage_collection.num_gc_cycles` | integer | Total GC cycles since start |
| `goroutines.health_status` | string | Derived goroutine health (healthy/elevated/critical) |
| `goroutines.count` | integer | Active goroutines |
| `goroutines.cgo_calls` | integer | Total cgo calls |
| `runtime.uptime` | string | Process uptime, human-readable |
| `runtime.go_version` | string | Go toolchain version |
| `runtime.num_cpu` | integer | Logical CPUs reported by runtime |
| `runtime.gomaxprocs` | integer | Current GOMAXPROCS |
| `allocations.total_mallocs` | integer | Cumulative malloc count |
| `allocations.total_frees` | integer | Cumulative free count |
| `allocations.net_objects` | integer | Net live objects (mallocs - frees) |

## GET /internal/stats/translators

Translator usage and performance statistics. Provides per-translator metrics and an aggregate summary, useful for monitoring API translation behaviour, passthrough efficiency, and fallback reasons.

### Request

```bash
curl -X GET http://localhost:40114/internal/stats/translators
```

### Response

```json
{
  "timestamp": "2026-02-13T10:30:00Z",
  "translators": [
    {
      "translator_name": "anthropic",
      "total_requests": 1500,
      "successful_requests": 1450,
      "failed_requests": 50,
      "success_rate": "96.7%",
      "passthrough_rate": "80.0%",
      "passthrough_requests": 1200,
      "translation_requests": 300,
      "streaming_requests": 800,
      "non_streaming_requests": 700,
      "fallback_no_compatible_endpoints": 5,
      "fallback_translator_does_not_support_passthrough": 0,
      "fallback_cannot_passthrough": 295,
      "average_latency": "245ms"
    }
  ],
  "summary": {
    "total_translators": 1,
    "active_translators": 1,
    "total_requests": 1500,
    "overall_success_rate": "96.7%",
    "total_passthrough": 1200,
    "total_translations": 300,
    "overall_passthrough_rate": "80.0%",
    "total_streaming": 800,
    "total_non_streaming": 700
  }
}
```

### Response Fields

#### Top-level

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | Current timestamp in RFC3339 format |
| `translators` | array | Per-translator statistics, sorted by request count (most active first) |
| `summary` | object | Aggregate statistics across all translators |

#### Translator Entry (`translators[]`)

| Field | Type | Description |
|-------|------|-------------|
| `translator_name` | string | Translator identifier (e.g., "anthropic") |
| `total_requests` | integer | Total requests processed by this translator |
| `successful_requests` | integer | Requests that completed successfully |
| `failed_requests` | integer | Requests that failed |
| `success_rate` | string | Human-readable success percentage |
| `passthrough_rate` | string | Human-readable passthrough percentage |
| `passthrough_requests` | integer | Requests forwarded directly in native format |
| `translation_requests` | integer | Requests that required format conversion |
| `streaming_requests` | integer | Streaming (SSE) requests |
| `non_streaming_requests` | integer | Non-streaming requests |
| `fallback_no_compatible_endpoints` | integer | Fallbacks due to no healthy endpoints available |
| `fallback_translator_does_not_support_passthrough` | integer | Fallbacks because the translator lacks passthrough capability |
| `fallback_cannot_passthrough` | integer | Fallbacks because no compatible backend declares native support |
| `average_latency` | string | Human-readable average request latency |

#### Summary

| Field | Type | Description |
|-------|------|-------------|
| `total_translators` | integer | Total number of registered translators |
| `active_translators` | integer | Translators that have processed at least one request |
| `total_requests` | integer | Total requests across all translators |
| `overall_success_rate` | string | Aggregate success percentage |
| `total_passthrough` | integer | Total passthrough requests across all translators |
| `total_translations` | integer | Total translation requests across all translators |
| `overall_passthrough_rate` | string | Aggregate passthrough percentage |
| `total_streaming` | integer | Total streaming requests across all translators |
| `total_non_streaming` | integer | Total non-streaming requests across all translators |

### Key Metrics for Monitoring

**Passthrough Rate**: A high `passthrough_rate` indicates backends are being used optimally in their native format, avoiding translation overhead.

**Fallback Reasons**: The three `fallback_*` fields help diagnose why passthrough is not being used:

- `fallback_no_compatible_endpoints`: No healthy endpoints available (operational issue, check health endpoint)
- `fallback_cannot_passthrough`: No compatible backend declares native support for the translator's format
- `fallback_translator_does_not_support_passthrough`: Expected for translators without passthrough capability

**Success Rate**: A declining `success_rate` may indicate backend issues or incompatible request formats.

---

## Rate Limits

System endpoints have elevated rate limits:

- 1000 requests per minute
- Burst size: 50 requests

This ensures monitoring systems can poll frequently without being rate-limited.
