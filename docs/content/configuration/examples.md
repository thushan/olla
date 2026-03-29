---
title: "Olla Configuration Examples - Production & Development Setups"
description: "Ready-to-use Olla configuration examples for single endpoints, home labs, production clusters, development environments and high-availability deployments."
keywords: ["olla configuration", "configuration examples", "production setup", "development config", "high availability", "docker deployment"]
---

# Configuration Examples

This page provides practical configuration examples for common Olla deployment scenarios.

## Basic Single Endpoint

Minimal configuration for a single Ollama instance:

```yaml
server:
  host: "localhost"
  port: 40114

proxy:
  engine: "sherpa"
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        # health_check_url and model_url are optional
        # Profile defaults: health_check_url="/", model_url="/api/tags"

logging:
  level: "info"
  format: "json"
```

## Home Lab Setup

Multi-GPU setup with local and network endpoints:

```yaml
server:
  host: "0.0.0.0"  # Accept connections from network
  port: 40114
  request_logging: true

proxy:
  engine: "sherpa"
  profile: "auto"
  load_balancer: "priority"
  connection_timeout: 30s

discovery:
  type: "static"
  static:
    endpoints:
      # Primary workstation GPU
      - url: "http://localhost:11434"
        name: "workstation-rtx4090"
        type: "ollama"
        priority: 100
        check_interval: 5s
        
      # Secondary machine on network
      - url: "http://192.168.1.50:11434"
        name: "server-rtx3090"
        type: "ollama"
        priority: 75
        check_interval: 10s
        
      # LM Studio instance
      - url: "http://localhost:1234"
        name: "lmstudio-local"
        type: "lm-studio"
        priority: 50
        check_interval: 10s

      # llama.cpp instance
      - url: "http://localhost:8080"
        name: "llamacpp-local"
        type: "llamacpp"
        priority: 95
        check_interval: 10s

model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "optimistic"  # Home lab: more flexible routing
    options:
      fallback_behavior: "all"  # Use any available endpoint if model not found
      discovery_timeout: 5s
      discovery_refresh_on_miss: false
  unification:
    enabled: true
    stale_threshold: 24h
    cleanup_interval: 10m

logging:
  level: "info"
  format: "text"
  output: "stdout"
```

## Production High-Performance

High-throughput production configuration with multiple backends:

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  read_timeout: 30s
  write_timeout: 0s  # Required for streaming
  shutdown_timeout: 30s
  request_logging: false  # Disable for performance
  
  request_limits:
    max_body_size: 104857600  # 100MB
    max_header_size: 1048576   # 1MB
  
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 500
    health_requests_per_minute: 5000
    burst_size: 100

proxy:
  engine: "olla"  # High-performance engine
  profile: "auto"
  load_balancer: "least-connections"
  connection_timeout: 45s
  # Note: Retry is automatic and built-in for connection failures

discovery:
  type: "static"
  model_discovery:
    enabled: true
    interval: 5m
    timeout: 30s
    concurrent_workers: 10
  
  static:
    endpoints:
      # Production cluster
      - url: "http://llm-prod-1.internal:11434"
        name: "prod-node-1"
        type: "ollama"
        priority: 100
        check_interval: 5s
        check_timeout: 2s
        
      - url: "http://llm-prod-2.internal:11434"
        name: "prod-node-2"
        type: "ollama"
        priority: 100
        check_interval: 5s
        check_timeout: 2s
        
      - url: "http://llm-prod-3.internal:11434"
        name: "prod-node-3"
        type: "ollama"
        priority: 100
        check_interval: 5s
        check_timeout: 2s
        
      # vLLM cluster for specific models
      - url: "http://vllm-cluster.internal:8000"
        name: "vllm-prod"
        type: "vllm"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 10s

      # llama.cpp instances (different quantisation levels)
      - url: "http://llamacpp-q4.internal:8080"
        name: "llamacpp-q4"
        type: "llamacpp"
        priority: 95
        check_interval: 5s

      - url: "http://llamacpp-q8.internal:8080"
        name: "llamacpp-q8"
        type: "llamacpp"
        priority: 90
        check_interval: 5s

model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "strict"  # Production: only route to endpoints with the model
    options:
      fallback_behavior: "none"  # Fail fast in production
      discovery_timeout: 2s
      discovery_refresh_on_miss: false
  unification:
    enabled: true
    stale_threshold: 1h  # More aggressive cleanup
    cleanup_interval: 5m

logging:
  level: "warn"  # Reduce logging overhead
  format: "json"
  output: "stdout"

engineering:
  show_nerdstats: false  # Disable in production
```

## Development Environment

Configuration for development and testing:

```yaml
server:
  host: "localhost"
  port: 40114
  request_logging: true  # Enable for debugging
  
  request_limits:
    max_body_size: 10485760  # 10MB for testing
    max_header_size: 262144   # 256KB

proxy:
  engine: "sherpa"  # Simpler for debugging
  profile: "auto"
  load_balancer: "round-robin"  # Test all endpoints equally
  connection_timeout: 10s
  # Note: Retry is automatic and built-in for connection failures

discovery:
  type: "static"
  model_discovery:
    enabled: true
    interval: 1m  # Frequent updates for testing
    timeout: 10s
    concurrent_workers: 2
  
  static:
    endpoints:
      # Local development
      - url: "http://localhost:11434"
        name: "dev-ollama"
        type: "ollama"
        priority: 100
        check_interval: 30s
        
      # Test instance
      - url: "http://localhost:11435"
        name: "test-ollama"
        type: "ollama"
        priority: 50
        check_interval: 30s

model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "discovery"  # Development: auto-discover models
    options:
      fallback_behavior: "compatible_only"  # Default: safe fallback
      discovery_timeout: 10s
      discovery_refresh_on_miss: true  # Auto-refresh in dev
  unification:
    enabled: true
    stale_threshold: 1h
    cleanup_interval: 5m

logging:
  level: "debug"  # Maximum verbosity
  format: "text"  # Easier to read
  output: "stdout"

engineering:
  show_nerdstats: true  # Show memory stats
```

## Mixed Backend Types

Using different LLM backend types together:

```yaml
server:
  host: "0.0.0.0"
  port: 40114

proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      # Ollama for general models
      - url: "http://localhost:11434"
        name: "ollama-general"
        type: "ollama"
        priority: 100
        
      # LM Studio for specific fine-tuned models
      - url: "http://localhost:1234"
        name: "lmstudio-finetuned"
        type: "lm-studio"
        priority: 90
        model_url: "/v1/models"
        health_check_url: "/"

      # llama.cpp for edge deployment
      - url: "http://localhost:8080"
        name: "llamacpp-edge"
        type: "llamacpp"
        priority: 95
        health_check_url: "/health"

      # vLLM for high-throughput serving
      - url: "http://vllm-server:8000"
        name: "vllm-production"
        type: "vllm"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/health"
        
      # Generic OpenAI-compatible endpoint
      - url: "https://api.llm-provider.com"
        name: "cloud-backup"
        type: "openai"
        priority: 10  # Low priority fallback
        health_check_url: "/v1/models"

model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: "strict"  # Mixed backends: strict routing
    options:
      fallback_behavior: "compatible_only"  # Important: prevent API incompatibilities
      discovery_timeout: 2s
      discovery_refresh_on_miss: false
  unification:
    enabled: true
    stale_threshold: 12h
    cleanup_interval: 15m
```

## Path Preservation Examples

Path Preservation allows Olla to maintain the existing paths when mapping proxied queries via the `preserve_path` configuration for endpoints. This is especially useful for inference platforms like [Docker Model Runner](https://docs.docker.com/ai/model-runner/).

### Docker Model Runner

Docker Model Runner endpoints often have base paths that need to be preserved:

```yaml
server:
  host: "0.0.0.0"
  port: 40114

proxy:
  engine: "olla"
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      # Docker Model Runner with base path
      - url: "http://docker-runner:8080/api/models/llama"
        name: "docker-llama"
        type: "openai"
        preserve_path: true  # Preserves /api/models/llama
        priority: 100
        health_check_url: "/api/models/llama/health"

      # Another model on the same server
      - url: "http://docker-runner:8080/api/models/mistral"
        name: "docker-mistral"
        type: "openai"
        preserve_path: true  # Preserves /api/models/mistral
        priority: 90
        health_check_url: "/api/models/mistral/health"
```

### Path-Based API Gateway

When endpoints are behind an API gateway using path-based routing:

```yaml
discovery:
  static:
    endpoints:
      # API Gateway with path routing
      - url: "https://api.company.com/llm/prod/v1"
        name: "gateway-prod"
        type: "openai"
        preserve_path: true  # Keep /llm/prod/v1 prefix
        priority: 100

      # Development endpoint
      - url: "https://api.company.com/llm/dev/v1"
        name: "gateway-dev"
        type: "openai"
        preserve_path: true  # Keep /llm/dev/v1 prefix
        priority: 50
```

### Microservices with Path Prefixes

Different microservices exposed through path prefixes:

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      # Chat service
      - url: "http://services:8080/chat/api"
        name: "chat-service"
        type: "openai"
        preserve_path: true
        priority: 100
        model_filter:
          include: ["*chat*", "*instruct*"]

      # Code generation service
      - url: "http://services:8080/code/api"
        name: "code-service"
        type: "openai"
        preserve_path: true
        priority: 100
        model_filter:
          include: ["*code*", "deepseek-coder*"]

      # Embedding service
      - url: "http://services:8080/embeddings/api"
        name: "embedding-service"
        type: "openai"
        preserve_path: true
        priority: 100
        model_filter:
          include: ["*embed*", "bge-*", "e5-*"]
```

## URL Configuration Examples

Showcase various URL configuration patterns using profile defaults, relative paths, and absolute URLs.

### Minimal Configuration with Profile Defaults

Simplest setup using automatic profile defaults:

```yaml
discovery:
  static:
    endpoints:
      # Ollama - uses default health_check_url="/" and model_url="/api/tags"
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100

      # llama.cpp - uses default health_check_url="/health" and model_url="/v1/models"
      - url: "http://localhost:8080"
        name: "llamacpp-server"
        type: "llamacpp"
        priority: 90

      # vLLM - uses default health_check_url="/health" and model_url="/v1/models"
      - url: "http://localhost:8000"
        name: "vllm-server"
        type: "vllm"
        priority: 85
```

### Endpoints with Base Paths

Relative URL paths are automatically joined with the endpoint base URL:

```yaml
discovery:
  static:
    endpoints:
      # Endpoint with base path - URLs are preserved
      - url: "http://localhost:8080/api/"
        name: "api-gateway"
        type: "vllm"
        # health_check_url="/health" becomes http://localhost:8080/api/health
        # model_url="/v1/models" becomes http://localhost:8080/api/v1/models

      # Custom relative paths
      - url: "http://backend:9000/inference/"
        name: "custom-backend"
        type: "openai"
        health_check_url: "/status"      # -> http://backend:9000/inference/status
        model_url: "/api/v1/models"      # -> http://backend:9000/inference/api/v1/models
```

### External Health Monitoring and Model Registry

Use absolute URLs to point health checks or model discovery to different hosts:

```yaml
discovery:
  static:
    endpoints:
      # Health check on external monitoring service
      - url: "http://localhost:11434"
        name: "monitored-ollama"
        type: "ollama"
        health_check_url: "http://monitoring.local:9090/health/ollama"
        # Model discovery still uses the endpoint URL with profile default
        # model_url="/api/tags" becomes http://localhost:11434/api/tags

      # Model registry on different host
      - url: "http://llamacpp-1:8080"
        name: "llamacpp-with-registry"
        type: "llamacpp"
        model_url: "http://model-registry.local/api/models/llamacpp-1"
        # Health check uses profile default on the endpoint host
        # health_check_url="/health" becomes http://llamacpp-1:8080/health

      # Both on external services
      - url: "http://ollama-node-1:11434"
        name: "fully-external"
        type: "ollama"
        health_check_url: "http://health-service.local/check/ollama-1"
        model_url: "http://registry.local/models/ollama-1"
```

### Mixed Configuration Patterns

Combine minimal defaults with custom overrides as needed:

```yaml
discovery:
  static:
    endpoints:
      # Minimal - all defaults
      - url: "http://localhost:11434"
        name: "ollama-default"
        type: "ollama"

      # Override health check only
      - url: "http://localhost:8080"
        name: "llamacpp-custom-health"
        type: "llamacpp"
        health_check_url: "/api/health"  # Custom, relative
        # model_url="/v1/models" (default)

      # Override model discovery only
      - url: "http://localhost:8000"
        name: "vllm-custom-models"
        type: "vllm"
        # health_check_url="/health" (default)
        model_url: "/api/v2/models"      # Custom, relative

      # Override both with absolute URLs
      - url: "http://localhost:1234"
        name: "lmstudio-external"
        type: "lm-studio"
        health_check_url: "http://health.local/lmstudio"
        model_url: "http://registry.local/lmstudio/models"
```

## Rate-Limited Public API

Configuration for exposing Olla as a public API:

```yaml
server:
  host: "0.0.0.0"
  port: 443  # Standard HTTPS port
  read_timeout: 30s
  write_timeout: 0s
  shutdown_timeout: 60s
  
  request_limits:
    max_body_size: 5242880   # 5MB limit
    max_header_size: 131072  # 128KB
  
  rate_limits:
    global_requests_per_minute: 5000
    per_ip_requests_per_minute: 60  # Strict per-client limit
    health_requests_per_minute: 1000
    burst_size: 10  # Small burst allowance

proxy:
  engine: "olla"
  profile: "standard"  # No streaming for public API
  load_balancer: "least-connections"
  connection_timeout: 20s
  # Note: Automatic retry with failover to other endpoints is built-in

discovery:
  type: "static"
  model_discovery:
    enabled: false  # Disable auto-discovery
  
  static:
    endpoints:
      - url: "http://internal-llm-1:11434"
        name: "api-backend-1"
        type: "ollama"
        priority: 100
        check_interval: 5s
        
      - url: "http://internal-llm-2:11434"
        name: "api-backend-2"
        type: "ollama"
        priority: 100
        check_interval: 5s

model_registry:
  type: "memory"
  enable_unifier: false  # Disable for security
  routing_strategy:
    type: "strict"  # Public API: strict model routing
    options:
      fallback_behavior: "none"  # Never fall back for public API
      discovery_timeout: 1s
      discovery_refresh_on_miss: false

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

## Minimal Docker Configuration

Simple configuration for Docker deployment:

```yaml
server:
  host: "0.0.0.0"  # Required for Docker
  port: 40114

proxy:
  engine: "sherpa"
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      # Use Docker service names or host.docker.internal
      - url: "http://host.docker.internal:11434"
        name: "host-ollama"
        type: "ollama"
        priority: 100

logging:
  level: "info"
  format: "json"
```

## High Availability Setup

Configuration for redundant, highly available deployment:

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  shutdown_timeout: 60s  # Graceful shutdown
  
  rate_limits:
    global_requests_per_minute: 20000
    per_ip_requests_per_minute: 1000
    burst_size: 200

proxy:
  engine: "olla"
  profile: "auto"
  load_balancer: "least-connections"
  connection_timeout: 30s
  # Note: Automatic retry tries all endpoints for maximum availability

discovery:
  type: "static"
  model_discovery:
    enabled: true
    interval: 2m
    timeout: 20s
    concurrent_workers: 5
  
  static:
    endpoints:
      # Primary region
      - url: "http://region1-llm-1:11434"
        name: "region1-primary"
        type: "ollama"
        priority: 100
        check_interval: 3s
        check_timeout: 1s
        
      - url: "http://region1-llm-2:11434"
        name: "region1-secondary"
        type: "ollama"
        priority: 100
        check_interval: 3s
        check_timeout: 1s
        
      # Failover region
      - url: "http://region2-llm-1:11434"
        name: "region2-primary"
        type: "ollama"
        priority: 50
        check_interval: 5s
        check_timeout: 2s
        
      - url: "http://region2-llm-2:11434"
        name: "region2-secondary"
        type: "ollama"
        priority: 50
        check_interval: 5s
        check_timeout: 2s

model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    stale_threshold: 30m  # Faster detection of failures
    cleanup_interval: 2m

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

## Resilient Configuration with Auto-Recovery

Configuration optimised for automatic failure handling and recovery:

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  shutdown_timeout: 30s

proxy:
  engine: "olla"  # Uses circuit breakers
  profile: "auto"
  load_balancer: "priority"
  connection_timeout: 30s
  response_timeout: 900s
  # Note: Automatic retry on failures tries all available endpoints

discovery:
  type: "static"
  health_check:
    initial_delay: 1s  # Quick startup checks
  
  model_discovery:
    enabled: true  # Auto-discover models
    interval: 5m
    timeout: 30s
    
  static:
    endpoints:
      # Primary endpoint - gets most traffic
      - url: "http://primary-gpu:11434"
        name: "primary"
        type: "ollama"
        priority: 100
        check_interval: 2s  # Frequent checks for quick recovery
        check_timeout: 1s
        
      # Secondary endpoint - failover
      - url: "http://secondary-gpu:11434"
        name: "secondary"
        type: "ollama"
        priority: 75
        check_interval: 5s
        check_timeout: 2s
        
      # Tertiary endpoint - last resort
      - url: "http://backup-gpu:11434"
        name: "backup"
        type: "ollama"
        priority: 50
        check_interval: 10s
        check_timeout: 3s

model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    stale_threshold: 1h  # Quick cleanup of stale models
    cleanup_interval: 5m

logging:
  level: "info"
  format: "json"

# Key resilience features in this config:
# 1. Automatic retry on connection failures
# 2. Circuit breakers prevent cascading failures (Olla engine)
# 3. Quick health checks for fast recovery detection
# 4. Automatic model discovery on endpoint recovery
# 5. Priority routing with automatic failover
# 6. Exponential backoff for failing endpoints
```

## Filtering Examples

Examples showing profile and model filtering capabilities. See [Filter Concepts](filters.md) for detailed pattern syntax.

### Specialized Embedding Service

Configure endpoints to serve only embedding models:

```yaml
server:
  port: 40114

proxy:
  engine: "sherpa"
  load_balancer: "priority"
  # Only load profiles that support embeddings
  profile_filter:
    include:
      - "ollama"
      - "openai*"
    exclude:
      - "lm-studio"  # Doesn't have good embedding support

discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "embedding-server"
        type: "ollama"
        priority: 100
        model_filter:
          include:
            - "*embed*"      # Embedding models
            - "bge-*"        # BGE models
            - "e5-*"         # E5 models
            - "nomic-*"      # Nomic models
          exclude:
            - "*test*"       # No test models
```

### Production Chat Service

Filter out experimental and inappropriate models:

```yaml
proxy:
  engine: "olla"
  load_balancer: "least-connections"
  # Exclude test/debug profiles
  profile_filter:
    exclude:
      - "*test*"
      - "*debug*"

discovery:
  static:
    endpoints:
      - url: "http://prod-gpu-1:11434"
        name: "prod-chat-1"
        type: "ollama"
        priority: 100
        model_filter:
          include:
            - "llama*"       # Llama family
            - "mistral*"     # Mistral family
            - "qwen*"        # Qwen family
          exclude:
            - "*uncensored*" # No uncensored models
            - "*test*"       # No test models
            - "*debug*"      # No debug models
            - "*embed*"      # No embedding models
            
      - url: "http://prod-gpu-2:11434"
        name: "prod-chat-2"
        type: "ollama"
        priority: 100
        model_filter:
          # Same filters for consistency
          include: ["llama*", "mistral*", "qwen*"]
          exclude: ["*uncensored*", "*test*", "*debug*", "*embed*"]
```

### Mixed Workload with Different Endpoints

Different model types on different endpoints:

```yaml
discovery:
  static:
    endpoints:
      # Code generation endpoint
      - url: "http://code-server:11434"
        name: "code-gen"
        type: "ollama"
        priority: 100
        model_filter:
          include:
            - "*code*"       # Code models
            - "deepseek-coder*"
            - "codellama*"
            - "starcoder*"
            
      # General chat endpoint
      - url: "http://chat-server:11434"
        name: "chat"
        type: "ollama"
        priority: 90
        model_filter:
          include:
            - "*chat*"       # Chat models
            - "*instruct*"   # Instruction models
          exclude:
            - "*code*"       # No code models
            - "*embed*"      # No embeddings
            
      # Vision endpoint
      - url: "http://vision-server:11434"
        name: "vision"
        type: "ollama"
        priority: 80
        model_filter:
          include:
            - "*vision*"     # Vision models
            - "llava*"       # LLaVA models
            - "*clip*"       # CLIP models
```

### Resource-Constrained Environment

Filter by model size:

```yaml
discovery:
  static:
    endpoints:
      # Small GPU - only small models
      - url: "http://small-gpu:11434"
        name: "small-models"
        type: "ollama"
        priority: 100
        model_filter:
          include:
            - "*-3b*"        # 3B models
            - "*-7b*"        # 7B models
            - "*-8b*"        # 8B models
          exclude:
            - "*-13b*"       # Nothing larger
            - "*-34b*"
            - "*-70b*"
            
      # Large GPU - only large models
      - url: "http://large-gpu:11434"
        name: "large-models"
        type: "ollama"
        priority: 50
        model_filter:
          include:
            - "*-34b*"       # 34B+ models
            - "*-70b*"
            - "*-72b*"
```

## Anthropic Translation with Passthrough

Configuration for Anthropic API translation with passthrough mode for backends with native Anthropic support:

```yaml
server:
  host: "localhost"
  port: 40114

proxy:
  engine: "olla"
  profile: "streaming"
  load_balancer: "priority"

# Enable Anthropic translator with passthrough optimisation
translators:
  anthropic:
    enabled: true
    # passthrough_enabled only applies when enabled=true
    # When true: Forwards requests directly to backends with native Anthropic support (optimal performance)
    # When false: Always translates Anthropic ↔ OpenAI format (useful for debugging/testing)
    passthrough_enabled: true
    max_message_size: 10485760   # 10MB

discovery:
  type: "static"
  static:
    endpoints:
      # Ollama v0.14.0+ supports native Anthropic API (passthrough eligible)
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100

      # vLLM v0.11.1+ supports native Anthropic API (passthrough eligible)
      - url: "http://vllm-server:8000"
        name: "vllm-prod"
        type: "vllm"
        priority: 80

logging:
  level: "info"
  format: "json"
```

### Anthropic Translation Only (No Passthrough)

Force all Anthropic requests through the translation pipeline, useful for debugging or when backends do not have native Anthropic support:

```yaml
translators:
  anthropic:
    enabled: true
    passthrough_enabled: false    # Force translation mode for all requests
    max_message_size: 10485760

discovery:
  type: "static"
  static:
    endpoints:
      # SGLang does not have native Anthropic support - uses translation
      - url: "http://localhost:30000"
        name: "sglang-server"
        type: "sglang"
        priority: 100

      # LiteLLM gateway - uses translation
      - url: "http://localhost:4000"
        name: "litellm-gateway"
        type: "litellm"
        priority: 50
```

## Environment Variables Override

Example showing environment variable overrides:

```bash
# Set via environment variables
export OLLA_SERVER_PORT=8080
export OLLA_PROXY_ENGINE=olla
export OLLA_LOG_LEVEL=debug

# Minimal config.yaml
```

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local"
        type: "ollama"
        priority: 100
```

## Next Steps

- [Configuration Reference](reference.md) - Complete configuration options
- [Best Practices](practices/overview.md) - Production recommendations
- [Profile System](../concepts/profile-system.md) - Customise backend behaviour