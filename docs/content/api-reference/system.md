# System Endpoints

Internal endpoints for health monitoring, system status, and process information.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/version` | Get Olla version information |
| GET | `/internal/health` | Health check endpoint |
| GET | `/internal/status` | System status and statistics |
| GET | `/internal/status/endpoints` | Detailed endpoint status |
| GET | `/internal/status/models` | Model registry status |
| GET | `/internal/stats/models` | Model usage statistics |
| GET | `/internal/stats/translators` | Translator usage and performance statistics |
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
  "version": "0.1.0",
  "build": {
    "version": "0.1.0",
    "commit": "abc123def",
    "date": "2024-01-15",
    "go_version": "go1.24.0"
  }
}
```

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
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "uptime": "2h30m15s",
  "endpoints": [
    {
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "status": "healthy",
      "latency": "1.2ms",
      "last_check": "2024-01-15T10:29:45Z"
    },
    {
      "name": "local-lm-studio",
      "url": "http://localhost:11234", 
      "status": "healthy",
      "latency": "0.8ms",
      "last_check": "2024-01-15T10:29:45Z"
    }
  ]
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Overall health status (healthy/degraded/unhealthy) |
| `timestamp` | string | Current timestamp in RFC3339 format |
| `uptime` | string | Time since Olla started |
| `endpoints` | array | Status of each configured endpoint |
| `endpoints[].name` | string | Endpoint identifier |
| `endpoints[].url` | string | Backend URL |
| `endpoints[].status` | string | Endpoint health (healthy/unhealthy) |
| `endpoints[].latency` | string | Last health check latency |
| `endpoints[].last_check` | string | Timestamp of last health check |

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
  "version": "0.1.0",
  "build": {
    "version": "0.1.0",
    "commit": "abc123def",
    "date": "2024-01-15",
    "go_version": "go1.24.0"
  },
  "config": {
    "proxy_engine": "sherpa",
    "load_balancer": "least-connections",
    "endpoints_configured": 2,
    "models_discovered": 5
  },
  "statistics": {
    "requests_total": 1523,
    "requests_active": 3,
    "requests_failed": 12,
    "average_latency": "125ms",
    "p95_latency": "450ms",
    "p99_latency": "850ms"
  },
  "endpoints": {
    "local-ollama": {
      "healthy": true,
      "models": 3,
      "requests": 1200,
      "errors": 5,
      "average_latency": "120ms"
    },
    "local-lm-studio": {
      "healthy": true,
      "models": 2,
      "requests": 323,
      "errors": 7,
      "average_latency": "135ms"
    }
  },
  "models": {
    "total": 5,
    "by_provider": {
      "ollama": ["llama3.2:latest", "mistral:latest", "codellama:latest"],
      "lm-studio": ["phi-3-mini", "gemma-2b"]
    }
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Olla version |
| `build` | object | Build information |
| `config` | object | Current configuration |
| `statistics` | object | Request statistics |
| `endpoints` | object | Per-endpoint statistics |
| `models` | object | Model information |

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
  "pid": 12345,
  "started_at": "2024-01-15T08:00:00Z",
  "uptime_seconds": 9015,
  "memory": {
    "alloc_mb": 45.2,
    "total_alloc_mb": 512.8,
    "sys_mb": 72.3,
    "heap_alloc_mb": 45.2,
    "heap_objects": 125432,
    "gc_runs": 42,
    "gc_pause_ms": 0.125
  },
  "cpu": {
    "goroutines": 28,
    "threads": 12,
    "cpu_percent": 2.5
  },
  "connections": {
    "active": 3,
    "idle": 12,
    "total_created": 1523
  },
  "runtime": {
    "go_version": "go1.24.0",
    "os": "linux",
    "arch": "amd64",
    "max_procs": 8
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `pid` | integer | Process ID |
| `started_at` | string | Process start time |
| `uptime_seconds` | integer | Uptime in seconds |
| `memory` | object | Memory statistics |
| `memory.alloc_mb` | float | Currently allocated memory |
| `memory.total_alloc_mb` | float | Total allocated memory |
| `memory.sys_mb` | float | System memory |
| `memory.heap_alloc_mb` | float | Heap allocated memory |
| `memory.heap_objects` | integer | Number of heap objects |
| `memory.gc_runs` | integer | Number of GC runs |
| `memory.gc_pause_ms` | float | Last GC pause duration |
| `cpu` | object | CPU statistics |
| `cpu.goroutines` | integer | Active goroutines |
| `cpu.threads` | integer | OS threads |
| `cpu.cpu_percent` | float | CPU usage percentage |
| `connections` | object | Connection pool stats |
| `runtime` | object | Runtime information |

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
| `fallback_cannot_passthrough` | integer | Fallbacks because no backend declares native support |
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

- `fallback_no_compatible_endpoints` -- No healthy endpoints available (operational issue, check health endpoint)
- `fallback_cannot_passthrough` -- Backends do not declare native support for the translator's format (configuration issue)
- `fallback_translator_does_not_support_passthrough` -- Expected for translators without passthrough capability

**Success Rate**: A declining `success_rate` may indicate backend issues or incompatible request formats.

---

## Rate Limits

System endpoints have elevated rate limits:

- 1000 requests per minute
- Burst size: 50 requests

This ensures monitoring systems can poll frequently without being rate-limited.