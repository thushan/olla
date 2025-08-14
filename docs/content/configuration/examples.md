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

model_registry:
  type: "memory"
  enable_unifier: true
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
  retry:
    enabled: true
    on_connection_failure: true
    max_attempts: 0  # Try all available endpoints

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

model_registry:
  type: "memory"
  enable_unifier: true
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
  retry:
    enabled: true  # Enable to test retry logic
    on_connection_failure: true
    max_attempts: 2  # Limited retries for debugging

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
  unification:
    enabled: true
    stale_threshold: 12h
    cleanup_interval: 15m
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
  # Automatic retry with failover to other endpoints
  retry:
    enabled: true
    on_connection_failure: true
    max_attempts: 2  # Limit retries for public API

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
  retry:
    enabled: true
    on_connection_failure: true
    max_attempts: 0  # Try all endpoints for maximum availability

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
  retry:
    enabled: true  # Automatic retry on failures
    on_connection_failure: true
    max_attempts: 0  # Try all available endpoints

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