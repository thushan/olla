---
title: Configuration Overview - Complete Olla Configuration Guide
description: Master Olla configuration with this comprehensive guide. Learn server settings, proxy engines, load balancing, health checks, rate limiting, and model registry configuration.
keywords: olla configuration, proxy configuration, load balancer config, health check settings, rate limiting, model registry
---

# Configuration Overview

> :memo: **Default Configuration**
> ```yaml
> server:
>   host: "localhost"
>   port: 40114
> proxy:
>   engine: "sherpa"
>   profile: "auto"
> discovery:
>   static:
>     endpoints: []
> ```
> **Quick Start**: Copy this minimal config to `config.yaml` and add your endpoints
> 
> **Environment Variables**: Most settings support `OLLA_SECTION_KEY` format

Olla uses a YAML configuration file to control all aspects of its behaviour. This page provides an overview of the configuration structure with links to detailed documentation for each section.

## Configuration File Location

Olla searches for configuration in the following order:

1. Path specified with `--config` or `-c` flag
2. `OLLA_CONFIG_FILE` environment variable
3. `./config.yaml` in the current directory
4. `./config/config.yaml` relative to the executable

## Configuration Structure

The configuration file is organised into six main sections:

```yaml
server:    # HTTP server and security settings
proxy:     # Proxy engine and behaviour
discovery: # Endpoint discovery and health checking
model_registry: # Model management and unification
logging:   # Logging configuration
engineering: # Advanced debugging features
```

## Server Configuration

The `server` section controls the HTTP server, security and rate limiting.

```yaml
server:
  host: "localhost"  # Bind address (use 0.0.0.0 for all interfaces)
  port: 40114        # Listen port (4-OLLA)
  read_timeout: 20s
  write_timeout: 0s  # Keep at 0s for streaming
  shutdown_timeout: 10s
  # idle_timeout: 120s  # Optional: connection idle timeout (default: no timeout)
  request_logging: false # Enable HTTP request logging
```

Key settings:

- **host**: Network interface to bind to
- **port**: TCP port (default 40114 spells 4-OLLA)
- **write_timeout**: Must be 0s to support LLM streaming
- **shutdown_timeout**: Time to wait for active connections during shutdown
- **request_logging**: Enable detailed HTTP request/response logging

### Request Limits

Control the maximum size of incoming requests:

```yaml
server:
  request_limits:
    max_body_size: 52428800  # 50MB
    max_header_size: 524288  # 512KB
```

### Rate Limiting

Protect your endpoints from abuse:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    health_requests_per_minute: 1000
    burst_size: 50
    cleanup_interval: 5m  # Rate limiter cleanup frequency
    trust_proxy_headers: false  # Trust X-Forwarded-For headers
    trusted_proxy_cidrs:  # Trusted proxy IP ranges
      - "127.0.0.0/8"     # Localhost
      - "10.0.0.0/8"      # Private network
      - "172.16.0.0/12"   # Private network
      - "192.168.0.0/16"  # Private network
```

!!! info "Rate Limiting Behaviour"
    - Global limits apply across all clients
    - Per-IP limits use token bucket algorithm with burst capacity
    - Health endpoints have separate limits to prevent monitoring disruption
    - When behind a proxy, configure `trust_proxy_headers` and `trusted_proxy_cidrs` for accurate client IP detection

See [Rate Limiting Reference](reference.md#rate-limiting) for complete details.

## Proxy Configuration

The `proxy` section controls request routing and proxy behaviour.

```yaml
proxy:
  engine: "sherpa"            # sherpa or olla
  profile: "auto"             # auto, streaming or standard
  load_balancer: "priority"   # priority, round-robin or least-connections
  connection_timeout: 30s     # Timeout for establishing connections
  response_timeout: 600s      # Timeout for complete response (10 minutes)
  read_timeout: 120s         # Timeout for reading response chunks
  stream_buffer_size: 8192   # Buffer size for streaming responses (8KB)
```

### Proxy Settings

Key timeout and retry settings:

| Setting | Description | Default |
|---------|-------------|---------|
| **connection_timeout** | Time to establish TCP connection | `30s` |
| **response_timeout** | Maximum time for complete response | `600s` |
| **read_timeout** | Time to wait for response chunks | `120s` |
| **stream_buffer_size** | Buffer size for SSE streaming | `8192` |

> **ℹ️ Automatic Retry Behaviour**
> 
> As of v0.0.16, Olla automatically retries requests on connection failures. When a connection 
> error occurs (e.g., refused connection, network unreachable), the proxy will automatically 
> try the next available healthy endpoint. This continues until either a successful connection 
> is made or all endpoints have been tried. The retry logic uses intelligent exponential backoff 
> for marking failed endpoints as unhealthy.
>
> The deprecated fields `proxy.max_retries` and `proxy.retry_backoff` are no longer used and 
> can be removed from your configuration.

### Proxy Engines

Olla provides two proxy implementations:

| Engine | Description | Use Case |
|--------|-------------|----------|
| **sherpa** | Simple, maintainable implementation | Development, moderate load |
| **olla** | High-performance with advanced features | Production, high throughput |

See [Proxy Engines](../concepts/proxy-engines.md) for detailed comparison.

### Proxy Profiles

Profiles control how the proxy handles HTTP streaming:

| Profile | Description | Use Case |
|---------|-------------|----------|
| **auto** | Dynamically selects based on request | Recommended default |
| **streaming** | Immediate token streaming, no buffering | Chat applications |
| **standard** | Normal HTTP delivery | REST APIs, embeddings |

See **[Proxy Profiles](../concepts/proxy-profiles.md)** for complete documentation on auto, streaming, and standard profiles.

### Load Balancing

Three strategies are available:

| Strategy | Description | Best For |
|----------|-------------|----------|
| **priority** | Routes to highest priority endpoint | Preferring local/cheaper endpoints |
| **round-robin** | Cycles through endpoints equally | Distributing load evenly |
| **least-connections** | Routes to least busy endpoint | Optimising response times |

See **[Load Balancing](../concepts/load-balancing.md)** for detailed strategies, health-aware routing, and best practices.

## Discovery Configuration

The `discovery` section defines how Olla finds and monitors endpoints.

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
```

### Endpoint Configuration

Each endpoint requires:

| Field | Description | Example |
|-------|-------------|---------|
| **url** | Base URL of the endpoint | `http://localhost:11434` |
| **name** | Unique identifier | `local-ollama` |
| **type** | Platform type | `ollama`, `lm-studio`, `vllm`, `openai` |
| **priority** | Selection priority (higher = preferred) | `100` |

Optional fields:

| Field | Description | Default |
|-------|-------------|---------|
| **model_url** | Path for model discovery | Platform-specific |
| **health_check_url** | Path for health checks | Platform-specific |
| **check_interval** | Time between health checks | `5s` |
| **check_timeout** | Health check timeout | `2s` |

### Model Discovery

Configure automatic model discovery:

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: 5m         # How often to refresh models
    timeout: 30s         # Discovery request timeout
    concurrent_workers: 5 # Parallel discovery workers
```

!!! note "Discovery Behaviour"
    Model discovery only runs on healthy endpoints. Failed discoveries disable the endpoint temporarily.

## Model Registry Configuration

The `model_registry` section controls model management and unification.

```yaml
model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    stale_threshold: 24h   # Model retention time
    cleanup_interval: 10m  # Cleanup frequency
```

### Model Unification

Unification creates a single model catalogue across endpoints of the same type:

- Models with identical names are merged
- Endpoint availability is tracked per model
- Stale models are removed after the threshold

See **[Model Unification](../concepts/model-unification.md)** for deduplication strategies, routing, and monitoring.

## Logging Configuration

Control Olla's logging output:

```yaml
logging:
  level: "info"    # debug, info, warn, error
  format: "json"   # json or text
  output: "stdout" # stdout or file
```

### Log Levels

| Level | Description |
|-------|-------------|
| **debug** | Detailed debugging information |
| **info** | Normal operational messages |
| **warn** | Warning conditions |
| **error** | Error conditions only |

## Engineering Configuration

Advanced features for debugging and monitoring:

```yaml
engineering:
  show_nerdstats: false  # Show detailed memory stats on shutdown
```

When enabled, displays:

- Memory allocation statistics
- Garbage collection metrics
- Goroutine counts
- Runtime information

## Environment Variables

All configuration values can be overridden with environment variables:

```bash
OLLA_SERVER_PORT=8080
OLLA_PROXY_ENGINE=olla
OLLA_LOG_LEVEL=debug
```

Pattern: `OLLA_<SECTION>_<KEY>` in uppercase with underscores.

## Validation

Olla validates configuration on startup:

- Required fields are checked
- URLs are validated
- Timeouts are verified as valid durations
- Conflicts are reported

## Related Documentation

### Core Concepts
- **[Proxy Engines](../concepts/proxy-engines.md)** - Sherpa vs Olla engine comparison
- **[Load Balancing](../concepts/load-balancing.md)** - Request distribution strategies
- **[Model Unification](../concepts/model-unification.md)** - Model aggregation across endpoints
- **[Health Checking](../concepts/health-checking.md)** - Endpoint monitoring
- **[Proxy Profiles](../concepts/proxy-profiles.md)** - Streaming behaviour control

### Next Steps
- [Configuration Reference](reference.md) - Complete configuration options
- [Configuration Examples](examples.md) - Common configuration scenarios
- [Best Practices](practices/overview.md) - Production recommendations
- [Profile System](../concepts/profile-system.md) - Customise backend behaviour