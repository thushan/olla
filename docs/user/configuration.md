# Configuration Reference

Olla supports configuration through YAML files and environment variables. This guide covers all available configuration options.

## Configuration File

Olla looks for configuration files in these locations (in order):
0. `config/config.local.yaml` (use for development only)
1. `config/config.yaml`
2. `config.yaml`
3. `default.yaml`
4. Path specified in `OLLA_CONFIG_FILE` environment variable.

You can also forgo all that and pass in the config file at the CLI:

```bash
./olla -c my.config.yaml

## Or...
./olla --config my.config.yaml
```

## Configuration Structure

```yaml
server:      # HTTP server settings
proxy:       # Proxy engine configuration
discovery:   # Endpoint discovery settings
logging:     # Logging configuration
model_registry: # Model registry and unification
engineering: # Development/debugging settings
```

## Server Configuration

Controls the HTTP server behaviour:

```yaml
server:
  host: "localhost"              # Listen address
  port: 40114                    # Listen port (default: 40114)
  read_timeout: "30s"            # Request read timeout
  write_timeout: "0s"            # Response write timeout (0 = no timeout)
  idle_timeout: "0s"             # Keep-alive timeout
  shutdown_timeout: "10s"        # Graceful shutdown timeout
  request_logging: true          # Enable request logging
  
  # Request limits
  request_limits:
    max_body_size: 104857600     # 100MB max body size
    max_header_size: 1048576     # 1MB max header size
  
  # Rate limiting
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    burst_size: 50
    health_requests_per_minute: 1000
    cleanup_interval: "5m"
    trust_proxy_headers: false
    trusted_proxy_cidrs:
      - "127.0.0.0/8"
      - "10.0.0.0/8"
      - "172.16.0.0/12"
      - "192.168.0.0/16"
```

### Environment Variables

```bash
OLLA_SERVER_HOST=0.0.0.0
OLLA_SERVER_PORT=8080
OLLA_SERVER_READ_TIMEOUT=5m
OLLA_SERVER_WRITE_TIMEOUT=10m
OLLA_SERVER_MAX_BODY_SIZE=104857600
OLLA_SERVER_MAX_HEADER_SIZE=1048576
OLLA_SERVER_GLOBAL_RATE_LIMIT=2000
OLLA_SERVER_PER_IP_RATE_LIMIT=200
```

## Proxy Configuration

Configure the proxy engine and behaviour:

```yaml
proxy:
  # Engine selection
  engine: "sherpa"             # "sherpa" or "olla"
  
  # Load balancing
  load_balancer: "priority"    # "priority", "round_robin", "least_connections"
  
  # Timeouts
  connection_timeout: "30s"    # Connection establishment timeout
  response_timeout: "10m"      # Overall response timeout
  read_timeout: "2m"           # Read timeout
  
  # Retry settings
  max_retries: 3               # Maximum retry attempts
  retry_backoff: "500ms"       # Backoff between retries
  
  # Streaming
  stream_buffer_size: 8192     # Buffer size for streaming (8KB default)
```

### Environment Variables

```bash
OLLA_PROXY_ENGINE=olla
OLLA_PROXY_LOAD_BALANCER=priority
OLLA_PROXY_CONNECTION_TIMEOUT=30s
OLLA_PROXY_RESPONSE_TIMEOUT=10m
OLLA_PROXY_MAX_RETRIES=3
```

## Discovery Configuration

Define your inference endpoints:

```yaml
discovery:
  type: "static"               # Only "static" is currently implemented
  refresh_interval: "30s"      # How often to refresh endpoints
  
  static:
    endpoints:
      - name: "local-ollama"
        url: "http://localhost:11434"
        type: "ollama"         # Platform type
        priority: 100          # Higher number = higher priority
        health_check_url: "/health"
        model_url: "/api/tags"
        check_interval: "5s"
        check_timeout: "2s"
        
      - name: "gpu-server"
        url: "http://10.0.1.10:11434"
        type: "ollama"
        priority: 50
        health_check_url: "/health"
        model_url: "/api/tags"
        check_interval: "10s"
        check_timeout: "5s"
  
  # Model discovery settings
  model_discovery:
    enabled: true
    interval: "5m"
    timeout: "30s"
    concurrent_workers: 3
    retry_attempts: 2
    retry_backoff: "1s"
```

## Logging Configuration

Control logging behaviour:

```yaml
logging:
  level: "info"    # debug, info, warn, error
  format: "json"   # json or text
  output: "stdout" # stdout, stderr, or file path
```

### Environment Variables

```bash
OLLA_LOG_LEVEL=debug
OLLA_LOG_FORMAT=text
```

## Model Registry Configuration

Configure model unification:

```yaml
model_registry:
  type: "memory"           # Only "memory" is implemented
  enable_unifier: true     # Enable model unification
  
  unification:
    enabled: true
    cache_ttl: "10m"       # How long to cache unified models
    
    # Custom unification rules
    custom_rules:
      - platform: "ollama"
        family_overrides:
          "phi4:*": "phi"  # Map phi4 models to phi family
        name_patterns:
          "llama-*": "llama3.2"
```

## Engineering Configuration

Development and debugging settings:

```yaml
engineering:
  show_nerdstats: false    # Show detailed performance stats
```

## Complete Example

### Development Configuration

```yaml
# config-dev.yaml
server:
  host: "localhost"
  port: 8080
  request_logging: true

proxy:
  engine: "sherpa"
  load_balancer: "round_robin"

discovery:
  type: "static"
  static:
    endpoints:
      - name: "local"
        url: "http://localhost:11434"
        type: "ollama"
        priority: 100

logging:
  level: "debug"
  format: "text"

engineering:
  show_nerdstats: true
```

### Production Configuration

```yaml
# config-prod.yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "5m"
  write_timeout: "0s"  # No timeout for streaming
  request_logging: false
  
  request_limits:
    max_body_size: 524288000  # 500MB for large contexts
    max_header_size: 2097152  # 2MB
  
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 1000
    burst_size: 100

proxy:
  engine: "olla"
  load_balancer: "priority"
  connection_timeout: "30s"
  response_timeout: "30m"  # Long timeout for slow models
  stream_buffer_size: 65536  # 64KB for better streaming

discovery:
  type: "static"
  static:
    endpoints:
      - name: "gpu-primary"
        url: "http://10.0.1.10:11434"
        type: "ollama"
        priority: 100
        check_interval: "10s"
        check_timeout: "5s"
        
      - name: "gpu-secondary"
        url: "http://10.0.1.11:11434"
        type: "ollama"
        priority: 50
        check_interval: "10s"
        check_timeout: "5s"

logging:
  level: "info"
  format: "json"

model_registry:
  enable_unifier: true
  unification:
    enabled: true
    cache_ttl: "30m"
```

## Minimal Configuration

The absolute minimum configuration needed:

```yaml
# Olla will use defaults for everything else
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
```

## Configuration Validation

Olla validates configuration on startup. Common validation errors:

1. **Invalid duration format**: Use Go duration format (e.g., "5s", "10m", "1h")
2. **Invalid byte size**: Use numeric bytes or units (e.g., "1048576", "1MB", "1MiB")
3. **Missing required fields**: URL is required for endpoints
4. **Invalid engine/balancer**: Must be one of the supported values

## Notes

1. **Environment variables** override configuration file values
2. **Duration values** use Go's duration format: "5s", "10m", "1h30m"
3. **Byte sizes** can use human-readable format: "100MB", "1GB"
4. **Default values** are sensible for most deployments
5. **Rate limiting** is per-instance (not distributed)