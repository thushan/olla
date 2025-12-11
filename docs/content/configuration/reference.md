---
title: Configuration Reference - Complete Olla Setup Guide
description: Complete configuration reference for Olla LLM proxy. Server settings, proxy engines, endpoints, load balancing, rate limits, and environment variables.
keywords: olla configuration, yaml config, proxy settings, load balancer config, rate limiting, endpoint configuration, environment variables
---

# Configuration Reference

Complete reference for all Olla configuration options.

> :memo: **Default Configuration**
> ```yaml
> server:
>   host: "localhost"
>   port: 40114
> 
> proxy:
>   engine: "sherpa"
>   load_balancer: "priority"
> 
> discovery:
>   model_discovery:
>     enabled: true
>     interval: 5m
> 
> logging:
>   level: "info"
>   format: "json"
> ```
> **Minimal Setup**: Olla starts with sensible defaults - just run `olla` and it works!
> 
> **Environment Variables**: All settings support `OLLA_` prefix (e.g., `OLLA_SERVER_PORT=8080`)

## Configuration Structure

```yaml
server:         # HTTP server configuration
proxy:          # Proxy engine settings
discovery:      # Endpoint discovery
model_registry: # Model management
logging:        # Logging configuration
engineering:    # Debug features
```

## Server Configuration

HTTP server and security settings.

### Basic Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"localhost"` | Network interface to bind |
| `port` | int | `40114` | TCP port to listen on |
| `request_logging` | bool | `false` | Enable request logging |

Example:

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  request_logging: true
```

### Timeouts

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `read_timeout` | duration | `20s` | Time to read request |
| `write_timeout` | duration | `0s` | Response write timeout (must be 0 for streaming) |
| `idle_timeout` | duration | `120s` | Keep-alive timeout |
| `shutdown_timeout` | duration | `10s` | Graceful shutdown timeout |

Example:

```yaml
server:
  read_timeout: 30s
  write_timeout: 0s      # Required for streaming
  idle_timeout: 120s
  shutdown_timeout: 30s
```

### Request Limits

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `request_limits.max_body_size` | int64 | `52428800` | Max request body (bytes) |
| `request_limits.max_header_size` | int64 | `524288` | Max header size (bytes) |

Example:

```yaml
server:
  request_limits:
    max_body_size: 104857600    # 100MB
    max_header_size: 1048576     # 1MB
```

### Rate Limits {#rate-limiting}

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rate_limits.global_requests_per_minute` | int | `0` | Global rate limit (0=disabled) |
| `rate_limits.per_ip_requests_per_minute` | int | `0` | Per-IP rate limit (0=disabled) |
| `rate_limits.health_requests_per_minute` | int | `0` | Health endpoint limit |
| `rate_limits.burst_size` | int | `50` | Token bucket burst size |
| `rate_limits.cleanup_interval` | duration | `1m` | Rate limiter cleanup |
| `rate_limits.trust_proxy_headers` | bool | `false` | Trust X-Forwarded-For |
| `rate_limits.trusted_proxy_cidrs` | []string | `[]` | Trusted proxy CIDRs |

Example:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    health_requests_per_minute: 5000
    burst_size: 50
    cleanup_interval: 1m
    trust_proxy_headers: true
    trusted_proxy_cidrs:
      - "10.0.0.0/8"
      - "172.16.0.0/12"
```

## Proxy Configuration

Proxy engine and request handling settings.

### Basic Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `engine` | string | `"sherpa"` | Proxy engine (`sherpa` or `olla`) |
| `profile` | string | `"auto"` | Proxy profile (`auto`, `streaming`, `standard`) |
| `load_balancer` | string | `"priority"` | Load balancer strategy |

Example:

```yaml
proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "least-connections"
```

### Connection Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `connection_timeout` | duration | `30s` | Backend connection timeout |
| `response_timeout` | duration | `0s` | Response timeout (0=disabled) |
| `read_timeout` | duration | `0s` | Read timeout (0=disabled) |

Example:

```yaml
proxy:
  connection_timeout: 45s
  response_timeout: 0s    # Disable for streaming
  read_timeout: 0s
```

### Retry Behaviour

As of v0.0.16, the retry mechanism is automatic and built-in for connection failures. When a connection error occurs (e.g., connection refused, network unreachable, timeout), Olla will automatically:

1. Mark the failed endpoint as unhealthy
2. Try the next available healthy endpoint 
3. Continue until a successful connection is made or all endpoints have been tried
4. Use exponential backoff for unhealthy endpoints to prevent overwhelming them

**Note**: The fields `max_retries` and `retry_backoff` that may still appear in the configuration are deprecated and ignored. The retry behaviour is now automatic and cannot be configured.

### Streaming Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `stream_buffer_size` | int | `4096` | Stream buffer size (bytes) |

Example:

```yaml
proxy:
  stream_buffer_size: 8192
```

### Profile Filtering

Control which inference profiles are loaded at startup. See [Filter Concepts](filters.md) for pattern details.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `profile_filter.include` | []string | `[]` | Profiles to include (glob patterns) |
| `profile_filter.exclude` | []string | `[]` | Profiles to exclude (glob patterns) |

Example:

```yaml
proxy:
  profile_filter:
    include:
      - "ollama"        # Include Ollama
      - "openai*"       # Include all OpenAI variants
    exclude:
      - "*test*"        # Exclude test profiles
      - "*debug*"       # Exclude debug profiles
```

## Discovery Configuration

Endpoint discovery and health checking.

### Discovery Type

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"static"` | Discovery type (only `static` supported) |
| `refresh_interval` | duration | `5m` | Discovery refresh interval |

Example:

```yaml
discovery:
  type: "static"
  refresh_interval: 10m
```

### Static Endpoints

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `static.endpoints[].url` | string | Yes | Endpoint base URL |
| `static.endpoints[].name` | string | Yes | Unique endpoint name |
| `static.endpoints[].type` | string | Yes | Backend type (`ollama`, `lm-studio`, `llamacpp`, `vllm`, `sglang`, `lemonade`, `litellm`, `openai`) |
| `static.endpoints[].priority` | int | No | Selection priority (higher=preferred) |
| `static.endpoints[].preserve_path` | bool | No | Preserve base path in URL when proxying (default: `false`) |
| `static.endpoints[].health_check_url` | string | No | Health check path (optional, uses profile default if not specified) |
| `static.endpoints[].model_url` | string | No | Model discovery path (optional, uses profile default if not specified) |
| `static.endpoints[].check_interval` | duration | No | Health check interval |
| `static.endpoints[].check_timeout` | duration | No | Health check timeout |
| `static.endpoints[].model_filter` | object | No | Model filtering for this endpoint |

#### URL Configuration

The `health_check_url` and `model_url` fields are **optional**. When not specified, Olla uses profile-specific defaults based on the endpoint type:

**Profile Defaults:**

| Endpoint Type | Default `health_check_url` | Default `model_url` |
|--------------|---------------------------|-------------------|
| `ollama` | `/` | `/api/tags` |
| `llamacpp` | `/health` | `/v1/models` |
| `lm-studio` | `/v1/models` | `/api/v0/models` |
| `vllm` | `/health` | `/v1/models` |
| `sglang` | `/health` | `/v1/models` |
| `openai` | `/v1/models` | `/v1/models` |
| `auto` (or unknown) | `/` | `/v1/models` |

**Both fields support:**

1. **Relative paths** (recommended) - joined with the endpoint base URL:
   ```yaml
   url: "http://localhost:8080/api/"
   health_check_url: "/health"     # Becomes: http://localhost:8080/api/health
   model_url: "/v1/models"         # Becomes: http://localhost:8080/api/v1/models
   ```

2. **Absolute URLs** - used as-is for external services:
   ```yaml
   url: "http://localhost:11434"
   health_check_url: "http://monitoring.local:9090/health"  # Different host
   model_url: "http://registry.local/models"                # Different host
   ```

When using relative paths, any base path prefix in the endpoint URL is **automatically preserved** (e.g., `http://localhost:8080/api/` + `/v1/models` = `http://localhost:8080/api/v1/models`).

#### Endpoint Model Filtering

Filter models at the endpoint level during discovery. See [Filter Concepts](filters.md) for pattern syntax.

| Field | Type | Description |
|-------|------|-------------|
| `model_filter.include` | []string | Models to include (glob patterns) |
| `model_filter.exclude` | []string | Models to exclude (glob patterns) |

#### Path Preservation

The `preserve_path` field controls how Olla handles base paths in endpoint URLs during proxying. This is particularly important for endpoints that serve multiple services or use path-based routing.

**Default Behaviour (preserve_path: false)**
When `preserve_path` is `false` (default), Olla strips the base path from the endpoint URL before proxying:

- Endpoint URL: `http://localhost:8080/api/v1`
- Request to Olla: `/v1/chat/completions`
- Proxied to: `http://localhost:8080/v1/chat/completions` (base path `/api/v1` is replaced)

**Path Preservation (preserve_path: true)**
When `preserve_path` is `true`, Olla preserves the base path:

- Endpoint URL: `http://localhost:8080/api/v1`
- Request to Olla: `/v1/chat/completions`
- Proxied to: `http://localhost:8080/api/v1/v1/chat/completions` (base path is preserved)

**When to Use Path Preservation:**

- Docker Model Runner endpoints with base paths
- APIs deployed behind path-based routers
- Services that require specific URL structures
- Multi-service endpoints using path differentiation

Example:

```yaml
discovery:
  static:
    endpoints:
      # Minimal configuration - uses profile defaults
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        # health_check_url: "/" (default for ollama)
        # model_url: "/api/tags" (default for ollama)

      # Custom health check URL
      - url: "http://localhost:8080"
        name: "llamacpp-server"
        type: "llamacpp"
        priority: 90
        health_check_url: "/health"
        # model_url: "/v1/models" (default for llamacpp)

      # Endpoint with base path - URLs are preserved
      - url: "http://localhost:8080/api/"
        name: "vllm-gateway"
        type: "vllm"
        priority: 80
        # health_check_url: "/health" -> http://localhost:8080/api/health
        # model_url: "/v1/models" -> http://localhost:8080/api/v1/models

      # External health check on different host
      - url: "http://localhost:11434"
        name: "monitored-ollama"
        type: "ollama"
        health_check_url: "http://monitoring.local:9090/health/ollama"
        # Absolute URL used as-is

      # Docker Model Runner with base path
      - url: "http://localhost:8080/api/models/llama"
        name: "docker-llama"
        type: "openai"
        preserve_path: true  # Keep /api/models/llama in requests

      # Endpoint with model filtering
      - url: "http://remote:11434"
        name: "remote-ollama"
        type: "ollama"
        priority: 50
        check_interval: 60s
        model_filter:
          include:
            - "llama*"          # Only Llama models
            - "mistral*"        # And Mistral models
```

### Model Discovery

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model_discovery.enabled` | bool | `true` | Enable model discovery |
| `model_discovery.interval` | duration | `5m` | Discovery interval |
| `model_discovery.timeout` | duration | `30s` | Discovery timeout |
| `model_discovery.concurrent_workers` | int | `5` | Parallel workers |
| `model_discovery.retry_attempts` | int | `3` | Retry attempts |
| `model_discovery.retry_backoff` | duration | `5s` | Retry backoff |

Example:

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: 10m
    timeout: 30s
    concurrent_workers: 10
    retry_attempts: 3
    retry_backoff: 5s
```

## Model Registry Configuration

Model management and unification settings.

### Registry Type

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"memory"` | Registry type (only `memory` supported) |
| `enable_unifier` | bool | `true` | Enable model unification |
| `routing_strategy.type` | string | `"strict"` | Model routing strategy (strict/optimistic/discovery) |

Example:

```yaml
model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: strict  # Default: only route to endpoints with the model
```

### Model Routing Strategy

Controls how requests are routed when models aren't available on all endpoints:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `routing_strategy.type` | string | `"strict"` | Strategy: `strict`, `optimistic`, or `discovery` |
| `routing_strategy.options.fallback_behavior` | string | `"compatible_only"` | Fallback: `compatible_only`, `all`, or `none` |
| `routing_strategy.options.discovery_timeout` | duration | `2s` | Timeout for discovery refresh |
| `routing_strategy.options.discovery_refresh_on_miss` | bool | `false` | Refresh discovery when model not found |

Example configurations:

```yaml
# Production - strict routing
model_registry:
  routing_strategy:
    type: strict

# Development - optimistic with fallback
model_registry:
  routing_strategy:
    type: optimistic
    options:
      fallback_behavior: compatible_only

# Dynamic environments - discovery mode
model_registry:
  routing_strategy:
    type: discovery
    options:
      discovery_refresh_on_miss: true
      discovery_timeout: 2s
```

### Unification Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `unification.enabled` | bool | `true` | Enable unification |
| `unification.stale_threshold` | duration | `24h` | Model retention time |
| `unification.cleanup_interval` | duration | `10m` | Cleanup frequency |
| `unification.cache_ttl` | duration | `5m` | Cache TTL |

Example:

```yaml
model_registry:
  unification:
    enabled: true
    stale_threshold: 12h
    cleanup_interval: 15m
    cache_ttl: 10m
```

### Custom Unification Rules

| Field | Type | Description |
|-------|------|-------------|
| `unification.custom_rules[].platform` | string | Platform to apply rules |
| `unification.custom_rules[].name_patterns` | map | Name pattern mappings |
| `unification.custom_rules[].family_overrides` | map | Family overrides |

Example:

```yaml
model_registry:
  unification:
    custom_rules:
      - platform: "ollama"
        name_patterns:
          "llama3.*": "llama3"
          "mistral.*": "mistral"
        family_overrides:
          "llama3": "meta-llama"
```

## Routing Configuration

Model routing strategy settings for handling requests when models aren't available on all endpoints.

### Model Routing Strategy

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `routing.model_routing.type` | string | `"strict"` | Routing strategy (`strict`, `optimistic`, `discovery`) |
| `routing.model_routing.options.fallback_behavior` | string | `"compatible_only"` | Fallback behavior (`compatible_only`, `all`, `none`) |
| `routing.model_routing.options.discovery_refresh_on_miss` | bool | `false` | Refresh discovery when model not found |
| `routing.model_routing.options.discovery_timeout` | duration | `2s` | Discovery refresh timeout |

#### Strategy Types

- **`strict`**: Only routes to endpoints known to have the model
- **`optimistic`**: Falls back to healthy endpoints when model not found
- **`discovery`**: Refreshes model discovery before routing decisions

Example:

```yaml
routing:
  model_routing:
    type: strict
    options:
      fallback_behavior: compatible_only
      discovery_refresh_on_miss: false
      discovery_timeout: 2s
```

### Response Headers

Routing decisions are exposed via response headers:

| Header | Description |
|--------|-------------|
| `X-Olla-Routing-Strategy` | Strategy used (strict/optimistic/discovery) |
| `X-Olla-Routing-Decision` | Action taken (routed/fallback/rejected) |
| `X-Olla-Routing-Reason` | Human-readable reason for decision |

## Logging Configuration

Application logging settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `"info"` | Log level (`debug`, `info`, `warn`, `error`) |
| `format` | string | `"json"` | Log format (`json` or `text`) |
| `output` | string | `"stdout"` | Output destination |

Example:

```yaml
logging:
  level: "info"
  format: "json"
  output: "stdout"
```

Log levels:

- `debug`: Detailed debugging information
- `info`: Normal operational messages
- `warn`: Warning conditions
- `error`: Error conditions only

## Engineering Configuration

Debug and development features.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `show_nerdstats` | bool | `false` | Show memory stats on shutdown |

Example:

```yaml
engineering:
  show_nerdstats: true
```

When enabled, displays:

- Memory allocation statistics
- Garbage collection metrics
- Goroutine counts
- Runtime information

## Environment Variables

All configuration can be overridden via environment variables.

Pattern: `OLLA_<SECTION>_<KEY>` in uppercase with underscores.

Examples:

```bash
# Server settings
OLLA_SERVER_HOST=0.0.0.0
OLLA_SERVER_PORT=8080
OLLA_SERVER_REQUEST_LOGGING=true

# Proxy settings
OLLA_PROXY_ENGINE=olla
OLLA_PROXY_LOAD_BALANCER=round-robin
OLLA_PROXY_PROFILE=auto

# Logging
OLLA_LOGGING_LEVEL=debug
OLLA_LOGGING_FORMAT=text

# Rate limits
OLLA_SERVER_RATE_LIMITS_GLOBAL_REQUESTS_PER_MINUTE=1000
```

## Duration Format

Duration values use Go duration syntax:

- `s` - seconds (e.g., `30s`)
- `m` - minutes (e.g., `5m`)
- `h` - hours (e.g., `2h`)
- `ms` - milliseconds (e.g., `500ms`)
- `us` - microseconds (e.g., `100us`)

Examples:

- `30s` - 30 seconds
- `5m` - 5 minutes
- `1h30m` - 1 hour 30 minutes
- `500ms` - 500 milliseconds

## Default Configuration

Complete default configuration:

```yaml
server:
  host: "localhost"
  port: 40114
  read_timeout: 20s
  write_timeout: 0s
  idle_timeout: 120s
  shutdown_timeout: 10s
  request_logging: false
  request_limits:
    max_body_size: 52428800    # 50MB
    max_header_size: 524288     # 512KB
  rate_limits:
    global_requests_per_minute: 0
    per_ip_requests_per_minute: 0
    health_requests_per_minute: 0
    burst_size: 50
    cleanup_interval: 1m
    trust_proxy_headers: false
    trusted_proxy_cidrs: []

proxy:
  engine: "sherpa"
  profile: "auto"
  load_balancer: "priority"
  connection_timeout: 30s
  response_timeout: 0s
  read_timeout: 0s
  # DEPRECATED as of v0.0.16 - retry is now automatic
  # max_retries: 3
  # retry_backoff: 1s
  stream_buffer_size: 4096

discovery:
  type: "static"
  refresh_interval: 5m
  model_discovery:
    enabled: true
    interval: 5m
    timeout: 30s
    concurrent_workers: 5
    retry_attempts: 3
    retry_backoff: 5s
  static:
    endpoints: []

model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "strict"
    options:
      fallback_behavior: "compatible_only"
      discovery_timeout: 2s
      discovery_refresh_on_miss: false
  unification:
    enabled: true
    stale_threshold: 24h
    cleanup_interval: 10m
    cache_ttl: 5m
    custom_rules: []

logging:
  level: "info"
  format: "json"
  output: "stdout"

engineering:
  show_nerdstats: false
```

## Validation

Olla validates configuration on startup:

- Required fields are checked
- URLs must be valid
- Durations must parse correctly
- Endpoints must have unique names
- Ports must be in valid range (1-65535)
- CIDR blocks must be valid

## Next Steps

- [Configuration Examples](examples.md) - Common configurations
- [Best Practices](practices/overview.md) - Production recommendations
- [Environment Variables](#environment-variables) - Override configuration