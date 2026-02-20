---
title: Model Unification - Unified Catalogues per Provider Type
description: Learn how Olla unifies model catalogues across multiple endpoints of the same type. Deduplication, load balancing, and unified access for Ollama, LM Studio, and vLLM.
keywords: model unification, model catalogue, ollama models, lm studio models, model deduplication, unified model access
---

# Model Unification - Single Catalogue per Provider Type

> :memo: **Default Configuration**
> ```yaml
> discovery:
>   model_discovery:
>     enabled: true
>     interval: 5m
>     concurrent_workers: 5
> ```
> **Supported Settings**:
> 
> - `enabled` _(default: true)_ - Enable automatic model discovery
> - `interval` _(default: 5m)_ - How often to refresh model lists
> - `concurrent_workers` _(default: 5)_ - Parallel discovery workers
> 
> **Environment Variables**: 
> - `OLLA_DISCOVERY_MODEL_DISCOVERY_ENABLED`
> - `OLLA_DISCOVERY_MODEL_DISCOVERY_INTERVAL`

Model unification creates a consolidated view of models available across multiple endpoints **of the same type**. When you have multiple Ollama instances or multiple LM Studio servers, Olla deduplicates and merges the model lists to show you what's available and where for each type.

## Key Concept: Per-Provider Unification

**Important**: Model unification happens **within each provider type**, not across different providers:

- Multiple **Ollama** instances → Unified Ollama model catalogue
- Multiple **LM Studio** instances → Unified LM Studio model catalogue  
- Multiple **vLLM** servers → Unified vLLM model catalogue

Models are **NOT** unified across different provider types. This means `llama3.2` on Ollama remains separate from `meta/llama3.2` or `llama3.2` on LM Studio, as they may have different formats, quantizations, or capabilities.

## Why Per-Provider Unification?

Different providers handle models differently:

- **Format differences**: Ollama uses its own format, LM Studio uses GGUF, vLLM uses HuggingFace format
- **API differences**: Each provider has unique API endpoints and capabilities
- **Metadata differences**: Model information varies significantly between providers
- **Performance characteristics**: Same model may perform differently on different platforms

## How It Works

### 1. Model Discovery

Each endpoint of the same type reports its models:

```yaml
# Two Ollama instances
ollama-server-1:
  - llama3.2:latest
  - mistral:7b
  - codellama:13b

ollama-server-2:
  - llama3.2:latest  # Duplicate
  - mixtral:8x7b
  - phi3:mini
```

### 2. Deduplication

Models with the same name or digest are identified:

```yaml
# After deduplication
unified-ollama-models:
  - llama3.2:latest (available on: ollama-server-1, ollama-server-2)
  - mistral:7b (available on: ollama-server-1)
  - codellama:13b (available on: ollama-server-1)
  - mixtral:8x7b (available on: ollama-server-2)
  - phi3:mini (available on: ollama-server-2)
```

### 3. Unified Access

Request any model, and Olla routes to an available endpoint:

```bash
# Request llama3.2 via Ollama endpoints - Olla picks from server-1 or server-2
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3.2:latest", "messages": [{"role": "user", "content": "Hello"}]}'

# Or use the native Ollama API
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3.2:latest", "messages": [{"role": "user", "content": "Hello"}]}'
```

## Configuration

Enable model unification in your configuration:

```yaml
model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    stale_threshold: 24h   # Remove models not seen for 24 hours
    cleanup_interval: 10m  # Check for stale models every 10 minutes
```

## Example: Multiple Ollama Instances

### Configuration

```yaml
discovery:
  static:
    endpoints:
      # Production Ollama cluster
      - url: "http://ollama-prod-1:11434"
        name: "prod-1"
        type: "ollama"
        priority: 100
        
      - url: "http://ollama-prod-2:11434"
        name: "prod-2"
        type: "ollama"
        priority: 100
        
      - url: "http://ollama-prod-3:11434"
        name: "prod-3"
        type: "ollama"
        priority: 100
```

### Result

With unification, you get a single model list:

```json
{
  "models": [
    {
      "id": "llama3.2:latest",
      "available_on": ["prod-1", "prod-2", "prod-3"],
      "load_balanced": true
    },
    {
      "id": "mistral:7b",
      "available_on": ["prod-1", "prod-3"],
      "load_balanced": true
    },
    {
      "id": "specialized-model:latest",
      "available_on": ["prod-2"],
      "load_balanced": false
    }
  ]
}
```

## Mixed Provider Types

When you have different provider types, each maintains its own unified catalogue:

```yaml
discovery:
  static:
    endpoints:
      # Ollama instances (unified together)
      - url: "http://ollama-1:11434"
        type: "ollama"
      - url: "http://ollama-2:11434"
        type: "ollama"
        
      # LM Studio instances (unified together, separate from Ollama)
      - url: "http://lmstudio-1:1234"
        type: "lm-studio"
      - url: "http://lmstudio-2:1234"
        type: "lm-studio"
```

Result: Two separate unified catalogues:
- Unified Ollama models from ollama-1 and ollama-2
- Unified LM Studio models from lmstudio-1 and lmstudio-2

## Deduplication Strategies

### Digest Matching (Most Reliable)

For providers that expose model digests (like Ollama):

```yaml
# Same model file = same digest = unified
server-1: llama3.2 (sha256:abc123...)
server-2: llama3.2 (sha256:abc123...)
Result: Single model entry with 2 endpoints
```

### Name Matching (Fallback)

When digests aren't available:

```yaml
# Same name = potentially same model
server-1: mistral-7b-instruct
server-2: mistral-7b-instruct
Result: Unified if other parameters match
```

## Load Balancing Unified Models

When a model is available on multiple endpoints of the same type:

1. **Priority-based**: Route to highest priority endpoint with the model
2. **Round-robin**: Distribute requests across all endpoints with the model
3. **Least-connections**: Route to least busy endpoint with the model

```yaml
proxy:
  load_balancer: "round-robin"  # Distribute across all endpoints with the model
```

## Monitoring

### Model Listing Endpoints

Olla provides multiple ways to retrieve model information:

#### Unified Models Endpoint

The `/olla/models` endpoint returns all models across all providers with format support:

```bash
# Default unified format - comprehensive model information
curl http://localhost:40114/olla/models

# OpenAI-compatible format
curl http://localhost:40114/olla/models?format=openai

# Ollama native format
curl http://localhost:40114/olla/models?format=ollama

# LM Studio format
curl http://localhost:40114/olla/models?format=lmstudio

# vLLM format
curl http://localhost:40114/olla/models?format=vllm
```

#### Provider-Specific Endpoints

Each provider has its own model listing endpoints:

```bash
# Ollama models
curl http://localhost:40114/olla/ollama/api/tags         # Native Ollama format
curl http://localhost:40114/olla/ollama/v1/models        # OpenAI-compatible format

# LM Studio models
curl http://localhost:40114/olla/lmstudio/v1/models      # OpenAI format
curl http://localhost:40114/olla/lmstudio/api/v0/models  # Enhanced LM Studio format

# OpenAI models
curl http://localhost:40114/olla/openai/v1/models        # Standard OpenAI format

# vLLM models
curl http://localhost:40114/olla/vllm/v1/models          # OpenAI-compatible format
```

#### Internal Status Endpoints

For monitoring and debugging:

```bash
# View all models and their endpoints
curl http://localhost:40114/internal/status/models

# View detailed model information
curl http://localhost:40114/internal/status/models?detailed=true
```

### Response Format Examples

**Unified Format** (default):
```json
{
  "models": [
    {
      "id": "llama3.2:latest",
      "name": "llama3.2",
      "provider": "ollama",
      "endpoints": ["ollama-1", "ollama-2"],
      "capabilities": {
        "chat": true,
        "completion": true,
        "embeddings": false,
        "vision": false
      },
      "context_length": 8192,
      "created": "2024-01-15T10:00:00Z"
    }
  ]
}
```

**OpenAI Format** (`?format=openai`):
```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3.2:latest",
      "object": "model",
      "created": 1705316400,
      "owned_by": "olla"
    }
  ]
}
```

**Ollama Format** (`?format=ollama`):
```json
{
  "models": [
    {
      "name": "llama3.2:latest",
      "model": "llama3.2:latest",
      "modified_at": "2024-01-15T10:00:00Z",
      "size": 4000000000,
      "digest": "sha256:abc123...",
      "details": {
        "family": "llama",
        "parameter_size": "7B",
        "quantization_level": "Q4_K_M"
      }
    }
  ]
}
```

## Benefits

### Resource Efficiency
- **Deduplication**: No redundant model entries in your catalogue
- **Smart routing**: Requests go to available endpoints automatically
- **Failover**: If one endpoint fails, requests route to others with the same model

### Operational Simplicity
- **Single catalogue**: One model list per provider type instead of many
- **Transparent access**: Users don't need to know which endpoint has which model
- **Dynamic updates**: Model list updates as endpoints come and go

### Scalability
- **Horizontal scaling**: Add more endpoints without changing client configuration
- **Model distribution**: Spread different models across endpoints
- **Load distribution**: Balance requests across endpoints with the same model

## Common Patterns

### High Availability

Deploy the same models on multiple endpoints:

```yaml
# All servers have core models for redundancy
ollama-1: [llama3.2, mistral, codellama]
ollama-2: [llama3.2, mistral, codellama]
ollama-3: [llama3.2, mistral, codellama]
```

### Specialised Endpoints

Different endpoints serve different models:

```yaml
# Specialized model distribution
ollama-gpu-1: [llama3.2:70b, mixtral:8x7b]  # Large models
ollama-gpu-2: [codellama, starcoder]        # Code models
ollama-cpu-1: [phi3:mini, tinyllama]        # Small models
```

### Development vs Production

Separate model sets by environment:

```yaml
# Development has experimental models
ollama-dev: [llama3.2, experimental-model, test-model]

# Production has stable models only
ollama-prod: [llama3.2, mistral, codellama]
```

## Limitations

- **No cross-provider unification**: Ollama models stay separate from LM Studio models
- **Name conflicts**: Models with same name but different actual files may be incorrectly unified
- **Metadata sync**: Model metadata updates may take time to propagate

## Next Steps

- Configure [Load Balancing](load-balancing.md) for unified models
- Set up [Health Checking](health-checking.md) for endpoint monitoring
- Review [Configuration Examples](../configuration/examples.md) for common setups