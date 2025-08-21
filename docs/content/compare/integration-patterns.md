---
title: LLM Infrastructure Integration Patterns with Olla
description: Learn how to integrate Olla with LiteLLM, GPUStack, LocalAI, and other tools. Production-ready patterns for robust AI infrastructure.
keywords: olla integration, llm infrastructure patterns, ai deployment architecture, litellm integration, gpustack setup, production llm
---

# Integration Patterns

This guide shows how to combine Olla with other tools to build robust LLM infrastructure for different use cases.

## Common Architectures

### 1. The Reliability Layer Pattern

Add Olla as a reliability layer on top of existing infrastructure:

```
Before:  Apps → LLM Endpoints (single point of failure)

After:   Apps → Olla → LLM Endpoints (automatic failover)
                   ├── Primary endpoint
                   ├── Secondary endpoint
                   └── Tertiary endpoint
```

**Benefits**:
- Zero changes to existing endpoints
- Instant failover capability
- Gradual migration path

**Example Config**:
```yaml
proxy:
  engine: sherpa  # Start simple
  load_balancer: priority

endpoints:
  - name: existing-setup
    url: http://current-llm:8080
    priority: 1
  - name: new-backup
    url: http://backup-llm:8080
    priority: 2
```

### 2. The Hybrid Cloud Pattern

Combine local and cloud resources intelligently:

```
         Olla
           ├── Local GPU (priority 1)
           ├── [LiteLLM](./litellm.md) → Cloud APIs (priority 10)
           └── [LocalAI](./localai.md)/[Ollama](https://github.com/ollama/ollama) (priority 2)
```

**Use Cases**:
- Prefer local for privacy/cost
- Overflow to cloud for capacity
- Different models on different platforms

**Example Config**:
```yaml
endpoints:
  - name: local-gpu
    url: http://localhost:11434
    priority: 1
    type: ollama
    
  - name: edge-server
    url: http://edge:11434
    priority: 2
    type: ollama
    
  - name: cloud-litellm
    url: http://litellm:8000
    priority: 10  # Only when local unavailable
    type: openai
```

### 3. The Multi-Tenant Pattern

Different teams/projects get different routing:

```
Team A Apps → Olla Config A → Team A Resources
Team B Apps → Olla Config B → Shared Resources + Team B
Production  → Olla Config C → Production Pool
```

**Implementation**:
```bash
# Run multiple Olla instances with different configs
olla -c team-a-config.yaml -p 8080
olla -c team-b-config.yaml -p 8081
olla -c production-config.yaml -p 8082
```

### 4. The Geographic Distribution Pattern

Route to nearest endpoints with fallback:

```
     Global Olla
         ├── Sydney [GPUStack](./gpustack.md) (for ANZ users)
         ├── Singapore [LocalAI](./localai.md) (for APAC users)
         └── US [vLLM](https://github.com/vllm-project/vllm) (for Americas users)
```

**Config with Regional Preferences**:
```yaml
endpoints:
  - name: syd-primary
    url: http://syd.internal:8080
    priority: 1  # Highest for local users
    
  - name: sing-secondary
    url: http://sing.internal:8080
    priority: 5  # Regional fallback
    
  - name: us-tertiary
    url: http://us.internal:8080
    priority: 10  # Last resort
```

## Tool-Specific Integrations

### Olla + [LiteLLM](./litellm.md) (Native Support!)

**Option 1: Native Integration with LiteLLM (Recommended)**
```yaml
# Olla has native LiteLLM support - use the dedicated profile
endpoints:
  - name: local-models
    url: http://ollama:11434
    type: ollama
    priority: 100
    
  - name: litellm-cloud
    url: http://litellm:4000
    type: litellm  # Native LiteLLM profile!
    priority: 75
    model_url: /v1/models
    health_check_url: /health
```

**Option 2: Redundant LiteLLM Instances with Native Support**
```yaml
# Multiple LiteLLM instances with health monitoring
endpoints:
  - name: litellm-primary
    url: http://litellm1:4000
    type: litellm
    priority: 90
    
  - name: litellm-backup
    url: http://litellm2:4000
    type: litellm
    priority: 90  # Same priority for round-robin
```

### Olla + [GPUStack](./gpustack.md)

**Production GPU Cluster**:
```yaml
endpoints:
  # GPUStack managed cluster
  - name: gpustack-pool-a
    url: http://gpustack-a:8080
    priority: 1
    
  - name: gpustack-pool-b
    url: http://gpustack-b:8080
    priority: 1
    
  # Manual fallback
  - name: static-ollama
    url: http://backup:11434
    priority: 10
```

### Olla + [Ollama](https://github.com/ollama/ollama)

**Multi-Instance Ollama**:
```yaml
endpoints:
  - name: ollama-3090
    url: http://desktop:11434
    priority: 1  # Fastest GPU
    
  - name: ollama-m1
    url: http://macbook:11434
    priority: 2  # Fallback
    
  - name: ollama-cpu
    url: http://server:11434
    priority: 10  # Emergency only
```

### Olla + [LocalAI](./localai.md)

**OpenAI Compatibility Layer**:
```yaml
endpoints:
  - name: localai-primary
    url: http://localai:8080
    priority: 1
    type: openai
    
  - name: localai-backup
    url: http://localai2:8080
    priority: 2
    type: openai
```

## Advanced Patterns

### Circuit Breaker Pattern

Olla automatically implements circuit breakers, but you can tune them:

```yaml
# Olla engine provides circuit breakers
proxy:
  engine: olla  # Required for circuit breakers
  
health:
  interval: 10s
  timeout: 5s
  
# Circuit breaker opens after 5 failures
# Attempts recovery after 30s
```

### Canary Deployment Pattern

Test new models/endpoints gradually:

```yaml
endpoints:
  - name: stable-model
    url: http://stable:8080
    priority: 1
    
  - name: canary-model
    url: http://canary:8080
    priority: 10  # Low priority = less traffic
```

Gradually increase canary priority as confidence grows.

### Model Specialisation Pattern

Route different model types to optimised endpoints:

```yaml
# Embeddings to CPU-optimised endpoint
# LLMs to GPU endpoint
# Using path-based routing with different Olla configs
```

### Chaos Engineering Pattern

Test resilience by randomly failing endpoints:

```yaml
endpoints:
  - name: primary
    url: http://primary:8080
    priority: 1
    
  - name: chaos-endpoint
    url: http://chaos:8080  # Fails 10% of requests
    priority: 1  # Equal priority to test failover
    
  - name: backup
    url: http://backup:8080
    priority: 2
```

## Production Best Practices

### 1. Start Simple, Evolve Gradually
```yaml
# Phase 1: Basic failover
proxy:
  engine: sherpa
  load_balancer: priority

# Phase 2: Add circuit breakers
proxy:
  engine: olla
  load_balancer: priority

# Phase 3: Sophisticated routing
proxy:
  engine: olla
  load_balancer: least_connections
```

### 2. Monitor Everything
- Use `/internal/status` for metrics
- Set up alerts on circuit breaker trips
- Monitor endpoint health scores
- Track failover events

### 3. Test Failure Scenarios
- Kill endpoints during load
- Simulate network issues
- Test circuit breaker recovery
- Verify model routing

### 4. Capacity Planning
```yaml
# Reserve capacity with priorities
endpoints:
  - name: primary-pool
    priority: 1  # 80% traffic
    
  - name: overflow-pool
    priority: 5  # 20% traffic
    
  - name: emergency-pool
    priority: 10  # Only when needed
```

## Docker Compose Examples

### Development Setup
```yaml
version: '3.8'
services:
  olla:
    image: thushan/olla:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/config.yaml
    
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    
  litellm:
    image: ghcr.io/berriai/litellm:latest
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    ports:
      - "8000:8000"
```

### Production Setup
```yaml
version: '3.8'
services:
  olla:
    image: thushan/olla:latest
    deploy:
      replicas: 2  # HA Olla
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/internal/health"]
      interval: 10s
    
  # Multiple backend services...
```

## Troubleshooting Integration Issues

### Issue: Endpoints Not Being Discovered
```yaml
# Ensure discovery is enabled
discovery:
  enabled: true
  interval: 30s
```

### Issue: Circuit Breaker Too Aggressive
```yaml
# Tune circuit breaker settings
health:
  interval: 10s  # Check more frequently
  timeout: 10s   # Allow more time
```

### Issue: Load Not Distributing
```yaml
# Check load balancer setting
proxy:
  load_balancer: round_robin  # For even distribution
```

## Conclusion

Olla's strength lies in its ability to integrate with existing tools rather than replace them. Use these patterns as starting points and adapt them to your specific needs. Remember: the best architecture is one that solves your actual problems, not theoretical ones.