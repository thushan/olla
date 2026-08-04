# System Endpoints

Internal endpoints for health monitoring, system status, and process information.

> :memo: For a glanceable view of most of this data, see the [Admin Dashboard](../configuration/dashboard.md), a read-only web UI served at `/internal/ui/`.

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
      "id": "7k1la9",
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "status": "healthy",
      "priority": 100,
      "success_rate": "99.6%",
      "avg_latency": "120ms",
      "avg_latency_ms": 120,
      "min_latency_ms": 85,
      "max_latency_ms": 240,
      "traffic": "1.2 GB",
      "connections": 2,
      "requests": 1200,
      "last_check": "2 seconds ago",
      "health_check_at": "2026-04-15T10:29:58Z",
      "next_check": "in 3 seconds",
      "next_check_at": "2026-04-15T10:30:03Z",
      "issues": "",
      "models": { "last_updated": "2026-04-15T10:25:00Z", "count": 3 }
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
    "start_time": "2026-04-15T08:00:00Z",
    "version": "0.1.0",
    "commit": "abc123de",
    "active_connections": 3,
    "security_violations": 0,
    "total_requests": 1523,
    "total_failures": 12,
    "has_traffic": true
  }
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 timestamp at which the response was generated. Excluded from the ETag hash so it does not churn every poll |
| `endpoints` | array | Per-endpoint runtime view (see below) |
| `proxy` | object | Active proxy configuration: `engine`, `profile`, `balancer` |
| `security` | object | Security posture: `status`, `blocked_ips`, `violations.{rate_limits, size_limits}` |
| `system` | object | Aggregate system summary (see below) |

**`endpoints[]` fields**:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Stable opaque identifier derived from the sanitised URL (scheme+host+port+path). Unchanged by credential rotation; siblings that share a sanitised form get a `-N` positional suffix |
| `name` | string | Endpoint name from config |
| `url` | string | Sanitised endpoint URL. Userinfo, query string, and fragment are stripped before this is surfaced, so credentials configured via `auth:` never appear here |
| `status` | string | Endpoint health: `healthy`, `unhealthy`, `offline` |
| `priority` | integer | Configured priority (higher is preferred) |
| `success_rate` | string | Human-readable success percentage, or `"N/A"` when the endpoint has handled no traffic |
| `avg_latency` | string | Human-readable average latency |
| `avg_latency_ms` | integer \| omitted | Average proxy latency in milliseconds. Omitted when the endpoint has no traffic |
| `min_latency_ms` | integer | Minimum proxy latency in milliseconds. `0` when the endpoint has no traffic |
| `max_latency_ms` | integer | Maximum proxy latency in milliseconds. `0` when the endpoint has no traffic |
| `traffic` | string | Human-readable total bytes proxied (`"0 B"` with no traffic) |
| `connections` | integer | Active connections to this endpoint |
| `requests` | integer | Total proxy requests routed to this endpoint |
| `last_check` | string | Human-readable relative time since the last health check (e.g. `"2 seconds ago"`) |
| `health_check_at` | string \| omitted | RFC3339 timestamp of the last health check. Omitted until the first check runs |
| `next_check` | string | Human-readable relative time until the next scheduled health check |
| `next_check_at` | string \| omitted | RFC3339 timestamp of the next scheduled health check |
| `issues` | string | Comma-separated issue summary (`unavailable`, `unstable`, `low success rate`, `high latency`), or empty when healthy |
| `models.last_updated` | string | RFC3339 timestamp of the last model discovery sync |
| `models.count` | integer | Number of models discovered on this endpoint |

The `last_check`, `next_check`, and relative strings on every status payload render with one-second granularity and are excluded from the ETag hash; the corresponding `*_at` absolute timestamps are the stable fields to diff against.

**`system` fields**:

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Aggregate status: `healthy`, `degraded`, `critical`. With no proxy traffic, derived purely from endpoint health so a healthy fresh boot no longer reports `critical` |
| `endpoints_up` | string | Healthy / total endpoint count, human-readable |
| `success_rate` | string | Aggregate proxy success percentage, or `"N/A"` when `has_traffic` is false |
| `avg_latency` | string | Human-readable average proxy latency |
| `total_traffic` | string | Human-readable total bytes proxied |
| `uptime` | string | Human-readable process uptime (relative, excluded from the ETag hash) |
| `start_time` | string | RFC3339 process start time (absolute, hashed) |
| `version` | string | Olla build version |
| `commit` | string | Olla build commit |
| `active_connections` | integer | Total active connections across all endpoints |
| `security_violations` | integer | Total rate-limit + size-limit violations |
| `total_requests` | integer | Total proxy requests since startup |
| `total_failures` | integer | Total failed proxy requests since startup |
| `has_traffic` | boolean | Always present. `false` on a fresh boot or when `total_requests` is zero; lets clients branch on the no-traffic state without parsing `success_rate` |

### Conditional requests and compression

The `/internal/status`, `/internal/status/endpoints`, and `/internal/status/models` JSON routes all participate in conditional GET:

- **Weak ETag.** Every `200` response carries an `ETag` header of the form `W/"<base36>"`, a 32-bit FNV-1a hash rendered in base36. It is a **weak** validator per RFC 7232: the hash covers the operator-visible state but deliberately excludes every per-second relative string (`uptime`, `last_check`, `next_check`, `last_seen`, `last_model_sync`) and the top-level `timestamp`. Two payloads a second apart under unchanged state hash identically, but the hash is not a byte-for-byte content identity.
- **`If-None-Match` uses weak comparison.** Echo the previously seen `ETag` value verbatim (including the `W/` prefix) and a matching value returns a bare `304` with no body and no `Content-Encoding`. The `*` wildcard and a list of validators are accepted; oversized `If-None-Match` headers (over 1 KiB) are treated as no-match to bound parsing.
- **gzip.** Send `Accept-Encoding: gzip` with a non-zero q-value and responses above 256 bytes are compressed, with `Content-Encoding: gzip` set and `Content-Length` dropped. `Vary: Accept-Encoding` is set on **every** response from a gzip-wrapped route, including identity responses, so a shared cache cannot serve a gzipped representation to a client that did not negotiate one. `304` responses and `text/event-stream` responses are never gzipped.
- **Embedded dashboard assets use a separate strong ETag.** Files under `/internal/ui/` carry a quoted SHA-256 prefix (`"<hex>"`, no `W/`) derived from the embedded bytes; this is a true content identity because `embed.FS` is immutable for the life of the process. Do not confuse the two schemes: status JSON is weak, dashboard assets are strong.

---

## GET /internal/status/endpoints

Per-endpoint runtime view, focused on health, latency, and model discovery. Carries the same `id`, `url`, and weak-ETag semantics as `/internal/status` (see [Conditional requests and compression](#conditional-requests-and-compression)).

### Request

```bash
curl -X GET http://localhost:40114/internal/status/endpoints
```

### Response

```json
{
  "timestamp": "2026-04-15T10:30:00Z",
  "total_count": 3,
  "healthy_count": 3,
  "routable_count": 3,
  "endpoints": [
    {
      "id": "7k1la9",
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "type": "ollama",
      "status": "healthy",
      "priority": 100,
      "model_count": 3,
      "request_count": 1200,
      "success_rate": "99.6%",
      "min_latency_ms": 85,
      "max_latency_ms": 240,
      "avg_latency_ms": 120,
      "active_connections": 2,
      "health_check": "2 seconds ago",
      "health_check_at": "2026-04-15T10:29:58Z",
      "response_time": "85ms",
      "next_check_at": "2026-04-15T10:30:03Z",
      "last_model_sync": "5 minutes ago",
      "last_model_sync_at": "2026-04-15T10:25:00Z",
      "issues": ""
    }
  ]
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 timestamp at which the snapshot was taken |
| `total_count` | integer | Configured endpoint count |
| `healthy_count` | integer | Healthy endpoint count |
| `routable_count` | integer | Endpoints eligible to receive traffic |
| `endpoints[]` | array | Per-endpoint summary (see below) |

**`endpoints[]` fields**:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Stable opaque identifier derived from the sanitised URL; identical to the `id` surfaced on `/internal/status` and `/internal/status/models` for the same endpoint from the same snapshot |
| `name` | string | Endpoint name from config |
| `url` | string | Sanitised endpoint URL (userinfo, query, and fragment stripped) |
| `type` | string | Backend profile type (e.g. `ollama`, `vllm`, `lm-studio`) |
| `status` | string | `healthy`, `unhealthy`, or `offline` |
| `priority` | integer | Configured priority (higher is preferred) |
| `model_count` | integer | Number of models discovered on this endpoint |
| `request_count` | integer | Total proxy requests routed to this endpoint |
| `success_rate` | string | Human-readable success percentage, or `"N/A"` with no traffic |
| `min_latency_ms` | integer | Minimum proxy latency in milliseconds. `0` with no traffic |
| `max_latency_ms` | integer | Maximum proxy latency in milliseconds. `0` with no traffic |
| `avg_latency_ms` | integer \| omitted | Average proxy latency in milliseconds. Omitted when the endpoint has no traffic |
| `active_connections` | integer | Active connections to this endpoint |
| `health_check` | string | Human-readable relative time since the last health check. Omitted until the first check runs |
| `health_check_at` | string \| omitted | RFC3339 timestamp of the last health check |
| `response_time` | string \| omitted | Human-readable latency of the most recent health probe. Omitted when no probe has completed |
| `next_check_at` | string \| omitted | RFC3339 timestamp of the next scheduled health check |
| `last_model_sync` | string \| omitted | Human-readable relative time since the last model discovery sync |
| `last_model_sync_at` | string \| omitted | RFC3339 timestamp of the last model discovery sync |
| `issues` | string \| omitted | Issue summary (`unavailable`, `unstable`, `low success rate`), or omitted when healthy |

Endpoints are sorted by priority (descending), then health (healthy first), then name, then `id`, so equal-priority endpoints do not reshuffle between polls under map iteration randomisation.

---

## GET /internal/status/models

Discovered model inventory, optionally grouped by family. Carries the same weak-ETag semantics as the other status routes (see [Conditional requests and compression](#conditional-requests-and-compression)).

### Request

```bash
curl -X GET http://localhost:40114/internal/status/models
curl -X GET "http://localhost:40114/internal/status/models?detailed=true&group=family"
```

Query parameters:

- `detailed=true` together with `group=family` adds a `model_groups` array grouping models by family with per-group endpoint lists.
- Without those parameters, only `recent_models` and `models_by_family` are returned.

### Response

```json
{
  "timestamp": "2026-04-15T10:30:00Z",
  "total_models": 12,
  "total_families": 3,
  "total_endpoints": 3,
  "models_by_family": {
    "llama": ["llama3.2", "llama3.2:1b"],
    "qwen": ["qwen2.5:7b"],
    "unknown": ["custom-model"]
  },
  "recent_models": [
    {
      "name": "llama3.2",
      "type": "llm",
      "family": "llama",
      "params": "3B",
      "quant": "Q4_0",
      "size": "1.8 GB",
      "last_seen": "5 minutes ago",
      "last_seen_at": "2026-04-15T10:25:00Z",
      "endpoints": ["local-ollama"],
      "endpoint_ids": ["7k1la9"],
      "aliases": ["llama3"],
      "capabilities": ["text_generation", "chat"]
    }
  ]
}
```

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 timestamp at which the snapshot was taken |
| `total_models` | integer | Distinct model count across all endpoints |
| `total_families` | integer | Distinct model family count |
| `total_endpoints` | integer | Endpoints that reported at least one model |
| `models_by_family` | object | Family name to sorted list of model names. Models with no reported family are grouped under `"unknown"` |
| `recent_models` | array | Up to 10 models sorted by descending `last_seen_at` |
| `model_groups` | array \| omitted | Per-family groups with detailed model records. Only present when `detailed=true&group=family` |

**`recent_models[]` / `model_groups[].models[]` fields**:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Model name as reported by the backend |
| `type` | string \| omitted | Model type (`llm`, `embeddings`) when the backend reports one |
| `family` | string \| omitted | Inferred model family |
| `params` | string \| omitted | Parameter size label |
| `quant` | string \| omitted | Quantisation label |
| `size` | string \| omitted | Human-readable size on disk |
| `last_seen` | string | Human-readable relative time since the model was last seen (excluded from the ETag hash) |
| `last_seen_at` | string \| omitted | RFC3339 timestamp the model was last seen (absolute, hashed). When the same model is hosted on several endpoints, the newest `last_seen_at` wins |
| `endpoints` | []string | Sorted display names of every endpoint hosting this model |
| `endpoint_ids` | []string \| omitted | Stable IDs of every endpoint hosting this model, positionally paired with `endpoints`. Use this slice (not the names) as the click-through key to `/internal/status/endpoints`, since two endpoints can share a display name |
| `aliases` | []string \| omitted | Other identifiers for the same unified model (alias names plus the canonical unified ID). Empty when the registry has no unified view or the model has a single identifier |
| `capabilities` | []string \| omitted | Inferred capabilities (`text_generation`, `chat`, `vision`, `multimodal`, `embeddings`, `vector_search`, `long_context`, `high_precision`) |

A model hosted on several endpoints has its scalar metadata (`family`, `params`, `quant`, `capabilities`, `size`) merged prefer-non-empty: the endpoint with the lowest sorted URL populates each empty field, and conflicting non-empty values keep the one already on the summary. This keeps the ETag stable across polls despite randomised map iteration.

**`model_groups[]` fields**:

| Field | Type | Description |
|-------|------|-------------|
| `family` | string | Family name, or `"unknown"` |
| `models` | array | Detailed model records in this family |
| `endpoints` | []string | Sorted display names of every endpoint hosting any model in this family |
| `model_count` | integer | Number of models in this family |

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
