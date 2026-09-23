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
>   engine: "olla"
>   load_balancer: "least-connections"
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
> **Environment Variables**: A curated set of settings support `OLLA_` env var overrides (e.g., `OLLA_SERVER_PORT=8080`). See [Environment Variables](#environment-variables) for the full list.

## Configuration Structure

```yaml
server:         # HTTP server configuration
proxy:          # Proxy engine settings
discovery:      # Endpoint discovery
model_registry: # Model management
translators:    # API translation (e.g., Anthropic ↔ OpenAI)
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
| `request_logging` | bool | `true` | Enable request logging |

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
| `read_timeout` | duration | `30s` | Time to read request |
| `read_header_timeout` | duration | `10s` | Max time to read request headers. Protects against Slowloris attacks; unset defaults to 10s. |
| `write_timeout` | duration | `0s` | Response write timeout (must be 0 for streaming) |
| `idle_timeout` | duration | `0s` | Keep-alive timeout (0 = use read_timeout) |
| `shutdown_timeout` | duration | `10s` | Graceful shutdown timeout |

Example:

```yaml
server:
  read_timeout: 30s
  read_header_timeout: 10s
  write_timeout: 0s      # Required for streaming
  idle_timeout: 120s
  shutdown_timeout: 30s
```

### Request Limits

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `request_limits.max_body_size` | int64 | `104857600` | Max request body (100MB) |
| `request_limits.max_header_size` | int64 | `1048576` | Max header size (1MB) |

Example:

```yaml
server:
  request_limits:
    max_body_size: 52428800     # 50MB (typical production value)
    max_header_size: 524288     # 512KB
```

### Rate Limits {#rate-limiting}

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rate_limits.global_requests_per_minute` | int | `1000` | Global rate limit (0=disabled) |
| `rate_limits.per_ip_requests_per_minute` | int | `100` | Per-IP rate limit (0=disabled) |
| `rate_limits.health_requests_per_minute` | int | `1000` | Health endpoint limit |
| `rate_limits.burst_size` | int | `50` | Token bucket burst size |
| `rate_limits.cleanup_interval` | duration | `5m` | Rate limiter cleanup |
| `rate_limits.trust_proxy_headers` | bool | `false` | Trust X-Forwarded-For |
| `rate_limits.trusted_proxy_cidrs` | []string | `["127.0.0.0/8","10.0.0.0/8","172.16.0.0/12","192.168.0.0/16"]` | Trusted proxy CIDRs |

Example:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    health_requests_per_minute: 5000
    burst_size: 50
    cleanup_interval: 5m
    trust_proxy_headers: true
    trusted_proxy_cidrs:
      - "10.0.0.0/8"
      - "172.16.0.0/12"
```

### CORS {#cors}

Cross-Origin Resource Sharing settings. Only relevant when browser clients (OpenWebUI, custom dashboards) connect directly to Olla. Disabled by default; non-browser clients (curl, SDKs, coding agents) are unaffected regardless of this setting.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `cors.enabled` | bool | `false` | Enable CORS middleware |
| `cors.allowed_origins` | []string | `["*"]` | Permitted origins. Must be explicit URLs when `allow_credentials` is `true` |
| `cors.allowed_methods` | []string | `["GET","POST","OPTIONS"]` | Permitted HTTP methods |
| `cors.allowed_headers` | []string | `["*"]` | Permitted request headers |
| `cors.exposed_headers` | []string | `[]` | Response headers exposed to browser JS. Empty = auto-expose full `X-Olla-*` set |
| `cors.allow_credentials` | bool | `false` | Send `Access-Control-Allow-Credentials: true` |
| `cors.max_age` | int | `300` | Preflight cache duration in seconds |

!!! warning "Credentials + wildcard origin"
    Setting `allow_credentials: true` with `allowed_origins: ["*"]` is forbidden by the CORS spec. Olla rejects this combination at startup with a fatal error. List explicit origins when credentials are required.

Example:

```yaml
server:
  cors:
    enabled: true
    allowed_origins:
      - "http://localhost:3000"
    allow_credentials: true
    max_age: 600
```

## Proxy Configuration

Proxy engine and request handling settings.

### Basic Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `engine` | string | `"olla"` | Proxy engine (`olla` or `sherpa`; `sherpa` is in maintenance mode) |
| `profile` | string | `"auto"` | Proxy profile (`auto`, `streaming`, `standard`) |
| `load_balancer` | string | `"least-connections"` | Load balancer strategy (`round-robin`, `least-connections`, `priority`) |

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
| `connection_timeout` | duration | `60s` | Backend connection timeout |
| `connection_keep_alive` | duration | `30s` | TCP keep-alive interval for backend connections |
| `response_timeout` | duration | `15m` | Response timeout |
| `read_timeout` | duration | `10m` | Read timeout |
| `response_header_timeout` | duration | `30s` | Max wait for the backend's first response header. Raise it for backends that load models on demand (e.g. Lemonade), where the first request blocks until the model is resident and the 30s default would abort the cold start. |
| `tls_handshake_timeout` | duration | `10s` | Maximum time allowed for a TLS handshake with a backend |

Example:

```yaml
proxy:
  connection_timeout: 45s
  connection_keep_alive: 30s
  response_timeout: 0s    # Disable for streaming
  read_timeout: 0s
  response_header_timeout: 180s  # allow slow on-demand model loads
  tls_handshake_timeout: 10s
```

### Retry Behaviour

As of v0.0.16, the retry mechanism is automatic and built-in for connection failures. When a connection error occurs (e.g., connection refused, network unreachable, timeout), Olla will automatically:

1. Mark the failed endpoint as unhealthy
2. Try the next available healthy endpoint 
3. Continue until a successful connection is made or all endpoints have been tried
4. Use exponential backoff for unhealthy endpoints to prevent overwhelming them

**Note**: The fields `max_retries` and `retry_backoff` that may still appear in your `proxy:` section are deprecated struct stubs. The `retry:` block (with `enabled`, `on_connection_failure`, `max_attempts`) that appears in the shipped `config.yaml` as a comment template has no corresponding struct in the config schema and is silently ignored if set. Retry behaviour is automatic and built-in; none of these fields have any effect.

### Streaming Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `stream_buffer_size` | int | unset (engine default: Olla 64KiB, Sherpa 8KiB) | Stream buffer size (bytes) |

Example:

```yaml
proxy:
  # stream_buffer_size: 8192  # Unset: the engine default applies (Olla 64KiB, Sherpa 8KiB)
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

### Sticky Sessions {#sticky-sessions}

KV-cache affinity routing for multi-turn LLM conversations. See [Sticky Sessions](../concepts/sticky-sessions.md) for a full explanation.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `sticky_sessions.enabled` | bool | `false` | Enable affinity routing (opt-in) |
| `sticky_sessions.idle_ttl_seconds` | int | `600` | Sliding TTL in seconds; 0 = no TTL expiry |
| `sticky_sessions.max_sessions` | uint64 | `10000` | LRU capacity; oldest entry evicted when full |
| `sticky_sessions.key_sources` | []string | `["session_header","prefix_hash","auth_header"]` | Ordered key source cascade; first match wins |
| `sticky_sessions.prefix_hash_bytes` | int | `512` | Bytes of the messages field to hash for `prefix_hash` |

**Environment Variable**: `OLLA_PROXY_STICKY_SESSIONS_ENABLED` (only `enabled` is exposed as an env var)

Example:

```yaml
proxy:
  sticky_sessions:
    enabled: true               # opt-in
    idle_ttl_seconds: 600       # 10-min sliding window
    max_sessions: 10000         # LRU cap
    key_sources:
      - "session_header"        # X-Olla-Session-ID header
      - "prefix_hash"           # hash of messages prefix
      - "auth_header"           # hash of Authorization header
      # - "ip"                  # client IP (unreliable behind NAT)
    prefix_hash_bytes: 512
```

## Discovery Configuration

Endpoint discovery and health checking.

### Discovery Type

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"static"` | Discovery type (only `static` supported) |
| `refresh_interval` | duration | `30s` | Discovery refresh interval |

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
| `static.endpoints[].type` | string | Yes | Backend type (`ollama`, `lm-studio`, `llamacpp`, `vllm`, `vllm-mlx`, `sglang`, `lemonade`, `litellm`, `lmdeploy`, `docker-model-runner`, `omlx`, `openai-compatible`). `openai` is accepted as an alias for `openai-compatible`. |
| `static.endpoints[].priority` | int | No | Selection priority (higher=preferred, default: `100`) |
| `static.endpoints[].preserve_path` | bool | No | Preserve base path in URL when proxying (default: `false`) |
| `static.endpoints[].health_check_url` | string | No | Health check path (optional, uses profile default if not specified) |
| `static.endpoints[].model_url` | string | No | Model discovery path (optional, uses profile default if not specified) |
| `static.endpoints[].check_interval` | duration | No | Health check interval (default: `5s`) |
| `static.endpoints[].check_timeout` | duration | No | Health check timeout (default: `2s`) |
| `static.endpoints[].model_filter` | object | No | Model filtering for this endpoint |
| `static.endpoints[].auth` | object | No | Outbound authentication credentials (see below) |
| `static.endpoints[].headers` | map[string]string | No | Custom outbound headers applied on every forwarded request |

#### Endpoint Authentication (`auth:`)

Attaches credentials to requests forwarded from Olla to a backend. See [Endpoint Authentication](endpoint-auth.md) for the full guide.

| Field | Type | Description |
|-------|------|-------------|
| `auth.type` | string | `bearer`, `api_key`, or `basic` |
| `auth.token` | string | Bearer token value. Sends `Authorization: Bearer <token>`. Mutually exclusive with `token_file`. |
| `auth.token_file` | string | Path to a file containing the bearer token. |
| `auth.key` | string | API key value. Mutually exclusive with `key_file`. |
| `auth.key_file` | string | Path to a file containing the API key. |
| `auth.header` | string | Header name for `api_key` type (default: `X-Api-Key`). |
| `auth.username` | string | Username for `basic` type. Mutually exclusive with `username_file`. |
| `auth.username_file` | string | Path to a file containing the username. |
| `auth.password` | string | Password for `basic` type. Mutually exclusive with `password_file`. |
| `auth.password_file` | string | Path to a file containing the password. |

`${VAR}` interpolation works on every value field. `_file` fields read and trim the file contents at startup. Setting both the inline field and its `_file` sibling is a fatal error, as is an unresolved `${VAR}` with no default.

**Bearer example:**

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-server:8000"
        name: "vllm-gpu"
        type: "vllm"
        auth:
          type: bearer
          token: "${VLLM_API_KEY}"
```

**API key with custom header:**

```yaml
      - url: "http://custom-gw:9000"
        name: "custom-gw"
        type: "openai-compatible"
        auth:
          type: api_key
          key: "${CUSTOM_API_KEY}"
          header: "X-Api-Key"
        headers:
          X-Tenant-ID: "team-a"
```

#### Custom Outbound Headers (`headers:`)

`headers:` is a free-form map of header names to values. All entries are copied verbatim onto every request forwarded to that endpoint. `auth:` and `headers:` can coexist; the `auth:` block always wins for its own credential header. `${VAR}` interpolation applies to values.

```yaml
      - url: "http://custom-llm:9000"
        name: "custom"
        type: "openai-compatible"
        headers:
          X-Tenant-ID: "acme"
          X-Request-Source: "olla"
```

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
| `openai-compatible` (or `openai`) | `/v1/models` | `/v1/models` |
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

- APIs deployed behind path-based routers
- Services that require specific URL structures
- Multi-service endpoints using path differentiation

Docker Model Runner's shipped profile does not require `preserve_path`: use pathless endpoint URLs such as `http://localhost:12434`, and the profile forwards DMR routes such as `/engines/v1/chat/completions` and `/anthropic/v1/messages` unchanged.

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

      # Docker Model Runner - pathless base URL, profile routes handle /engines/...
      - url: "http://localhost:12434"
        name: "docker-model-runner"
        type: "docker-model-runner"

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
| `model_discovery.retry_backoff` | duration | `1s` | Retry backoff |

Example:

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: 10m
    timeout: 30s
    concurrent_workers: 10
    retry_attempts: 3
    retry_backoff: 1s
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

Unification itself is toggled by `model_registry.enable_unifier` (see [Model Registry Configuration](#model-registry-configuration) above), not by anything under `unification:`. The `unification` block only tunes stale-model cleanup, and only takes effect when the unifier is enabled.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `unification.stale_threshold` | duration | `24h` | How long to retain a model after it was last seen |
| `unification.cleanup_interval` | duration | `10m` | How often to evict stale models |

Example:

```yaml
model_registry:
  enable_unifier: true
  unification:
    stale_threshold: 12h
    cleanup_interval: 15m
```

## Model Aliases Configuration

Define virtual model names that map to platform-specific model names across different backends.

### Model Alias Mapping

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model_aliases` | map[string][]string | `nil` | Map of alias name → list of actual model names |
| `model_aliases_mode` | string | `disabled` | Controls alias visibility in model listings (`disabled`, `append`, `hidden`) |

Each key is the virtual model name clients will use. Each value is a list of actual model names that backends may serve the model under. When a request matches an alias, Olla resolves endpoints for all listed model names and rewrites the request body to the correct name for the selected backend.

Example:

```yaml
model_aliases:
  my-llama:
    - "llama3.1:8b"                          # Ollama
    - llama-3.1-8b-instruct                  # LM Studio
    - Meta-Llama-3.1-8B-Instruct.gguf        # llamacpp

  my-codegen:
    - "qwen2.5-coder:7b"                     # Ollama
    - qwen2.5-coder-7b-instruct              # LM Studio

model_aliases_mode: "hidden"
```

!!! note
    Alias names take priority over standard model routing. If no endpoints are found for the alias, Olla falls back to standard routing using the alias name as a regular model name, honouring the configured routing strategy. Under `strict` (the default) an alias whose targets are all unavailable is rejected (`404`/`503`), not proxied to a compatible-but-wrong backend. See [Model Aliases](../concepts/model-aliases.md) for details.

### Model Aliases Mode

The `model_aliases_mode` setting controls how configured aliases appear in model listing endpoints (e.g., `/olla/proxy/v1/models`, `/olla/ollama/v1/models`, `/olla/openai/v1/models`, etc.):

| Mode | Description |
|------|-------------|
| `disabled` | Aliases are not shown in model listings. Only actual models discovered from backends are returned. This is the default for backward compatibility. |
| `append` | Aliases are added to model listings alongside their target models. Both the alias name and the actual model names appear in the response. |
| `hidden` | Aliases are shown in model listings, but their target models are hidden. Clients see only the alias names (e.g., `my-llama`), not the backend-specific names (e.g., `llama3.1:8b`, `llama-3.1-8b-instruct`). Target models remain accessible by their direct names if requested explicitly. |

When using `hidden` or `append` mode, the alias entries in model listings include their target models in the `aliases` field, allowing clients to discover the underlying model names if needed.

## Routing Response Headers

Routing decisions are exposed via response headers:

| Header | Description |
|--------|-------------|
| `X-Olla-Routing-Strategy` | Strategy used (strict/optimistic/discovery) |
| `X-Olla-Routing-Decision` | Action taken (routed/fallback/rejected) |
| `X-Olla-Routing-Reason` | Human-readable reason for decision |

The routing strategy itself is configured under `model_registry.routing_strategy`. See [Model Registry Configuration](#model-registry-configuration) above.

## Translators Configuration

API translation settings. Translators enable clients designed for one API format to work with backends that use a different format.

> :memo: **Anthropic Translation** (v0.0.20+)
> Enabled by default. Still actively being improved -- please report any issues or feedback.

### Anthropic Translator

The Anthropic translator enables Claude-compatible clients (Claude Code, OpenCode, Crush CLI) to work with OpenAI-compatible backends.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Master switch for the Anthropic translator. When `false`, the `/olla/anthropic/v1/*` endpoints do not exist. |
| `passthrough_enabled` | bool | `true` | Optimisation mode (only applies when `enabled: true`). When `true`, requests are forwarded directly to backends with native Anthropic support for zero translation overhead. When `false`, all requests go through the Anthropic-to-OpenAI translation pipeline regardless of backend capabilities. |
| `max_message_size` | int | `10485760` | Maximum request body size in bytes (10MB default). |

#### Two-Level Control: `enabled` + `passthrough_enabled`

The Anthropic translator uses a two-level configuration model:

1. **`enabled`** is the master switch. When `false`, the translator is completely disabled and the `passthrough_enabled` setting has no effect. It is `true` by default.
2. **`passthrough_enabled`** is the optimisation flag. It only takes effect when `enabled: true`.

When both are active, passthrough mode also requires that the backend profile declares native Anthropic support via `api.anthropic_support.enabled: true`. Both conditions must be true for passthrough to activate:

- `translators.anthropic.passthrough_enabled: true` (global configuration)
- Backend profile has `api.anthropic_support.enabled: true` (per-backend profile)

If either condition is false, Olla falls back to translation mode automatically.

#### Examples

**Enable translator with passthrough (recommended for production)**:

```yaml
translators:
  anthropic:
    enabled: true
    passthrough_enabled: true       # Forward directly to backends with native Anthropic support
    max_message_size: 10485760      # 10MB
```

**Enable translator with translation only (useful for debugging/testing)**:

```yaml
translators:
  anthropic:
    enabled: true
    passthrough_enabled: false      # Always translate Anthropic ↔ OpenAI format
    max_message_size: 10485760
```

**Disable translator entirely**:

```yaml
translators:
  anthropic:
    enabled: false
    # passthrough_enabled has no effect when enabled=false
    passthrough_enabled: true
```

#### Performance Implications

| Mode | Overhead | When Used |
|------|----------|-----------|
| **Passthrough** | Near-zero (~0ms) | `passthrough_enabled: true` and backend has native Anthropic support |
| **Translation** | ~1-5ms per request | `passthrough_enabled: false`, or backend lacks native Anthropic support |
| **Disabled** | N/A | `enabled: false` -- endpoints return 404 |

#### Detecting the Active Mode

Check the `X-Olla-Mode` response header:

- `X-Olla-Mode: passthrough` -- passthrough mode was used
- Header absent -- translation mode was used

#### Inspector (Development Only)

> :no_entry: **Do not enable in production** -- logs full request/response bodies including potentially sensitive user data.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `inspector.enabled` | bool | `false` | Enable request/response logging |
| `inspector.output_dir` | string | `"logs/inspector/anthropic"` | Directory for log output |
| `inspector.session_header` | string | `"X-Session-ID"` | Header for session grouping |

See [Anthropic Inspector](../notes/anthropic-inspector.md) for details.

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

An invalid `level`, `format` or `output` value is a config mistake, not a reason to fail startup: Olla logs a warning and falls back to the default for that field.

!!! note "Interactive terminals always get pretty output"
    On an interactive terminal (TTY), Olla always renders coloured/aligned text logs, regardless of `logging.format` in the config file. This is so a developer running `olla` locally doesn't see JSON just because `config.yaml` sets `format: "json"` for production. Off a TTY (piped output, Docker, a service manager), the configured format applies as written.

    Setting the `OLLA_LOGGING_FORMAT` environment variable overrides this in both directions - it forces its value everywhere, TTY or not.

`logging.output: "file"` adds a rotated file handler (see `OLLA_LOG_DIR`, `OLLA_LOG_SIZE_MB`, `OLLA_LOG_MAX_BACKUPS`, `OLLA_LOG_MAX_AGE_DAYS` in [Environment Variables](#environment-variables)) alongside stdout; it does not replace stdout output. There is no `OLLA_LOGGING_OUTPUT` environment variable - `output` is YAML-only.

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

Only the variables listed below are recognised. They are each hand-mapped in `internal/config/config.go`; arbitrary `OLLA_*` names are **not** auto-derived from YAML paths. Anything you set that isn't in this list will be silently ignored.

When set, an environment variable overrides the value loaded from the YAML config.

### Bootstrap and Runtime (read in `main.go`)

These control startup behaviour and are read before the YAML config is loaded.

| Variable | Default | Description |
|---|---|---|
| `OLLA_CONFIG_FILE` | _(unset)_ | Path to a YAML config file. CLI flags `-c` / `--config` take precedence. |
| `OLLA_ENABLE_PROFILER` | `false` | When `true`, starts a pprof server on `localhost:19841` (`/debug/pprof/`). Same effect as `--profile`. |
| `OLLA_SHOW_VERSION` | `false` | When `true`, prints the full version banner and exits. Same effect as `--version`. |
| `OLLA_LOG_LEVEL` | `info` | Logger level (`debug`, `info`, `warn`, `error`) for the bootstrap logger only. Active before the YAML config is parsed. Use `OLLA_LOGGING_LEVEL` to set the level for the runtime logger after config load. |
| `OLLA_PRETTY_LOGS` | `true` | When `true`, renders structured logs with colour and aligned columns. Set `false` for plain JSON-friendly output in CI/containers. |
| `OLLA_FILE_OUTPUT` | `true` | When `true`, also writes logs to rotating files under `OLLA_LOG_DIR`. |
| `OLLA_LOG_DIR` | `./logs` | Directory for rotated log files. Created on demand. |
| `OLLA_LOG_SIZE_MB` | `1` | Maximum size of each log file before rotation, in megabytes. |
| `OLLA_LOG_MAX_BACKUPS` | `7` | Number of rotated log files to keep. |
| `OLLA_LOG_MAX_AGE_DAYS` | `14` | Maximum age in days for rotated log files before they're pruned. |
| `OLLA_THEME` | `default` | Console theme name. Affects coloured output of the styled logger. |

`--validate-config` has no environment variable equivalent - it's a CLI flag only. See [Validating configuration](#validating-configuration) below.

### Server

| Variable | Maps to | Notes |
|---|---|---|
| `OLLA_SERVER_HOST` | `server.host` | |
| `OLLA_SERVER_PORT` | `server.port` | |
| `OLLA_SERVER_READ_TIMEOUT` | `server.read_timeout` | Go duration string. |
| `OLLA_SERVER_READ_HEADER_TIMEOUT` | `server.read_header_timeout` | Go duration string. Guards against Slowloris; defaults to `10s`. |
| `OLLA_SERVER_WRITE_TIMEOUT` | `server.write_timeout` | Keep at `0s` for streaming. |
| `OLLA_SERVER_MAX_BODY_SIZE` | `server.request_limits.max_body_size` | Bytes. |
| `OLLA_SERVER_MAX_HEADER_SIZE` | `server.request_limits.max_header_size` | Bytes. |
| `OLLA_SERVER_GLOBAL_RATE_LIMIT` | `server.rate_limits.global_requests_per_minute` | |
| `OLLA_SERVER_PER_IP_RATE_LIMIT` | `server.rate_limits.per_ip_requests_per_minute` | |
| `OLLA_SERVER_RATE_BURST_SIZE` | `server.rate_limits.burst_size` | |
| `OLLA_SERVER_HEALTH_RATE_LIMIT` | `server.rate_limits.health_requests_per_minute` | |
| `OLLA_SERVER_RATE_CLEANUP_INTERVAL` | `server.rate_limits.cleanup_interval` | |
| `OLLA_SERVER_TRUST_PROXY_HEADERS` | `server.rate_limits.trust_proxy_headers` | `true`/`false`. |
| `OLLA_SERVER_TRUSTED_PROXY_CIDRS` | `server.rate_limits.trusted_proxy_cidrs` | Comma-separated CIDRs. |
| `OLLA_SERVER_CORS_ENABLED` | `server.cors.enabled` | `true`/`false`. |
| `OLLA_SERVER_CORS_ALLOWED_ORIGINS` | `server.cors.allowed_origins` | Comma-separated origin URLs. No surrounding spaces. |
| `OLLA_SERVER_CORS_ALLOWED_METHODS` | `server.cors.allowed_methods` | Comma-separated HTTP methods. |
| `OLLA_SERVER_CORS_ALLOWED_HEADERS` | `server.cors.allowed_headers` | Comma-separated header names. |
| `OLLA_SERVER_CORS_EXPOSED_HEADERS` | `server.cors.exposed_headers` | Comma-separated header names. Empty = auto-expose `X-Olla-*` set. |
| `OLLA_SERVER_CORS_ALLOW_CREDENTIALS` | `server.cors.allow_credentials` | `true`/`false`. Incompatible with `allowed_origins: ["*"]`. |
| `OLLA_SERVER_CORS_MAX_AGE` | `server.cors.max_age` | Preflight cache seconds (int). |

### Proxy

| Variable | Maps to | Notes |
|---|---|---|
| `OLLA_PROXY_ENGINE` | `proxy.engine` | `olla` (default) or `sherpa`. |
| `OLLA_PROXY_PROFILE` | `proxy.profile` | `auto`, `streaming`, `standard`. |
| `OLLA_PROXY_LOAD_BALANCER` | `proxy.load_balancer` | `least-connections` (default), `priority`, `round-robin`. |
| `OLLA_PROXY_RESPONSE_TIMEOUT` | `proxy.response_timeout` | Go duration string. |
| `OLLA_PROXY_READ_TIMEOUT` | `proxy.read_timeout` | Go duration string. |
| `OLLA_PROXY_RESPONSE_HEADER_TIMEOUT` | `proxy.response_header_timeout` | Go duration string. Max wait for a backend's first response header. |
| `OLLA_PROXY_TLS_HANDSHAKE_TIMEOUT` | `proxy.tls_handshake_timeout` | Go duration string. Max time for a TLS handshake with a backend. |
| `OLLA_PROXY_CONNECTION_KEEP_ALIVE` | `proxy.connection_keep_alive` | Go duration string. TCP keep-alive interval for backend connections. |
| `OLLA_PROXY_STICKY_SESSIONS_ENABLED` | `proxy.sticky_sessions.enabled` | Only the master switch is exposed. Other sticky-session fields are YAML-only. |

### Logging and Diagnostics

The variables below override the `logging:` config section and take effect **after** the YAML config is parsed. For the bootstrap logger (before config load), use `OLLA_LOG_LEVEL` and the `OLLA_LOG_*` file-rotation vars in the [Bootstrap and Runtime](#bootstrap-and-runtime-read-in-maingo) table above.

| Variable | Maps to | Notes |
|---|---|---|
| `OLLA_LOGGING_LEVEL` | `logging.level` | Runtime log level. Overrides `logging.level` in YAML. |
| `OLLA_LOGGING_FORMAT` | `logging.format` | `json` or `text`. Overrides `logging.format` in YAML. |
| `OLLA_SHOW_NERD_STATS` | `engineering.show_nerdstats` | `true`/`false`. Prints memory/GC/goroutine stats on shutdown. |

### Model Registry

| Variable | Maps to | Notes |
|---|---|---|
| `OLLA_MODEL_REGISTRY_TYPE` | `model_registry.type` | Currently only `memory`. |
| `OLLA_MODEL_UNIFIER_ENABLED` | `model_registry.enable_unifier` | `true`/`false`. |

### Translators

| Variable | Maps to | Notes |
|---|---|---|
| `OLLA_TRANSLATORS_ANTHROPIC_ENABLED` | `translators.anthropic.enabled` | Master switch for the Anthropic Messages API translator. |
| `OLLA_TRANSLATORS_ANTHROPIC_PASSTHROUGH_ENABLED` | `translators.anthropic.passthrough_enabled` | Set `false` to force every request through the translation pipeline regardless of backend capability. |
| `OLLA_TRANSLATORS_ANTHROPIC_MAX_MESSAGE_SIZE` | `translators.anthropic.max_message_size` | Bytes. |

### Example

```bash
# Run on port 8080 with the Olla engine, profiling enabled, plain text logs for CI.
OLLA_SERVER_PORT=8080 \
OLLA_PROXY_ENGINE=olla \
OLLA_ENABLE_PROFILER=true \
OLLA_PRETTY_LOGS=false \
./olla
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
  read_timeout: 30s
  read_header_timeout: 10s
  write_timeout: 0s
  # idle_timeout: 0s  # Optional (0 = use read_timeout)
  shutdown_timeout: 10s
  request_logging: true
  request_limits:
    max_body_size: 104857600   # 100MB
    max_header_size: 1048576   # 1MB
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    health_requests_per_minute: 1000
    burst_size: 50
    cleanup_interval: 5m
    trust_proxy_headers: false
    trusted_proxy_cidrs:
      - "127.0.0.0/8"
      - "10.0.0.0/8"
      - "172.16.0.0/12"
      - "192.168.0.0/16"

proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "least-connections"
  connection_timeout: 60s
  response_timeout: 15m
  read_timeout: 10m
  response_header_timeout: 30s
  connection_keep_alive: 30s
  tls_handshake_timeout: 10s
  # DEPRECATED as of v0.0.16 - retry is now automatic
  # max_retries: 3
  # retry_backoff: 1s
  # stream_buffer_size: 8192  # Unset: the engine default applies (Olla 64KiB, Sherpa 8KiB)

discovery:
  type: "static"
  refresh_interval: 30s
  model_discovery:
    enabled: true
    interval: 5m
    timeout: 30s
    concurrent_workers: 5
    retry_attempts: 3
    retry_backoff: 1s
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
    stale_threshold: 24h
    cleanup_interval: 10m

translators:
  anthropic:
    enabled: true
    passthrough_enabled: true
    max_message_size: 10485760   # 10MB
    inspector:
      enabled: false
      output_dir: "logs/inspector/anthropic"
      session_header: "X-Session-ID"

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

Additionally, Olla's `Validate()` method catches dangerous zero or empty configuration values that would cause panics or silent failures at runtime. It runs after all config sources (file, environment overrides) have been merged, so the final state is what gets checked. The following conditions produce clear error messages at startup:

- `proxy.engine` is empty
- `proxy.load_balancer` is empty
- `discovery.type` is empty
- `server.port` is zero or negative
- When `model_discovery.enabled` is `true`: `interval`, `concurrent_workers`, or `timeout` is zero

### Validating configuration

Run `olla --validate-config` (optionally with `-c <path>`) to check `config.yaml`, `config/models.yaml` and every provider profile without starting the server:

```text
Olla Configuration Validation
==============================
[OK]   config.yaml  loaded from config/config.local.yaml
[OK]   models.yaml  loaded from config/models.yaml
[OK]   profiles     11 profile(s) loaded

Result: PASS - all configuration files are valid
```

Each line is `[OK]`, `[WARN]` (usable but with issues, e.g. a `models.yaml` candidate that failed to parse before a later candidate succeeded), or `[FAIL]` (unusable). Exit code is `0` only when everything is clean; `1` if any file that exists fails to parse or is unusable. This is stricter than the running server, which warns and falls back to defaults for the same failures rather than refusing to start. Use it in CI or as a pre-restart check before deploying a config change.

## Next Steps

- [Configuration Examples](examples.md) - Common configurations
- [Best Practices](practices/overview.md) - Production recommendations
- [Environment Variables](#environment-variables) - Override configuration
