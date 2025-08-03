# Model Discovery and Unification

Olla provides intelligent model discovery and per-provider unification across your LLM infrastructure. This guide explains how Olla discovers, unifies, and routes to models across different providers.

## How Model Discovery Works

Olla continuously discovers models from all configured endpoints and maintains separate catalogues for each provider type. This allows for intelligent routing while respecting the unique characteristics of each platform.

### Discovery Process

1. **Endpoint Polling**: Olla regularly polls each endpoint's model API
2. **Model Cataloguing**: Models are categorised by provider type (Ollama, LM Studio, etc.)
3. **Per-Provider Unification**: Similar models within the same provider are unified
4. **Route Mapping**: Request routing maps models to compatible endpoints

## Per-Provider Unification

Olla unifies models **within each provider type**, not across different providers. This is a critical distinction that ensures compatibility and prevents routing errors.

### Ollama Model Unification

Within Ollama endpoints, Olla unifies models with identical names and tags:

**Example: Multiple Ollama Endpoints**
```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://192.168.0.1:11434"
        name: "neo-ollama" 
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://192.168.0.113:11434"
        name: "beehive-ollama"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

In this real setup, the local Ollama endpoint has models like `llama3.2:latest` and `phi4:latest`. When these same models exist on multiple Ollama endpoints, Olla will:
- Unify them as single model entries
- Route requests to any available Ollama endpoint with the model
- Apply load balancing strategy across healthy endpoints

**Real Unified Model Response:**
```json
{
  "id": "llama3.2:latest",
  "object": "model", 
  "owned_by": "olla",
  "olla": {
    "family": "llama",
    "parameter_size": "3.2b",
    "quantization": "q4km",
    "aliases": ["llama3.2:latest"],
    "availability": [
      {
        "endpoint": "local-ollama",
        "state": "available"
      }
    ],
    "capabilities": ["text-generation"]
  }
}
```

### LM Studio Model Unification

Within LM Studio endpoints, Olla unifies models based on their metadata and file names:

**Example: Multiple LM Studio Endpoints**
```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11234"
        name: "local-lm-studio"
        type: "lm-studio"
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://192.168.0.1:11234" 
        name: "neo-lm-studio"
        type: "lm-studio"
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

In this real setup, LM Studio endpoints have models like `mistralai/devstral-small-2505` and `microsoft/phi-4-mini-reasoning`. When the same models exist on multiple LM Studio endpoints, Olla will:
- Recognise them as identical models based on their IDs
- Unify under the canonical name
- Route requests to available LM Studio endpoints only

**Real LM Studio Model:**
```json
{
  "id": "microsoft/phi-4-mini-reasoning",
  "object": "model",
  "owned_by": "olla", 
  "olla": {
    "max_context_length": 131072,
    "family": "phi3",
    "quantization": "q4km",
    "aliases": ["microsoft/phi-4-mini-reasoning"],
    "availability": [
      {
        "endpoint": "local-lm-studio",
        "state": "not-loaded"
      }
    ],
    "capabilities": ["long-context", "text-generation", "reasoning", "logic"]
  }
}
```

### OpenAI-Compatible Model Unification

For OpenAI-compatible endpoints (vLLM, OpenRouter, etc.), Olla unifies based on model IDs:

**Example: Multiple vLLM Endpoints**
```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://vllm-1.local:8000"
        name: "vllm-cluster-1"
        type: "openai-compatible"
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://vllm-2.local:8000"
        name: "vllm-cluster-2"
        type: "openai-compatible"
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 2s
        check_timeout: 1s
```

Models with identical IDs like `meta-llama/Llama-3.2-8B-Instruct` are unified within the OpenAI-compatible provider group.

## Cross-Provider Model Isolation

**Important**: Olla does **NOT** unify models across different provider types. Each provider maintains its own model catalogue.

### Example: Why Cross-Provider Unification Doesn't Happen

Consider this real setup from the running Olla instance:

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        # Has: phi4:latest (14.7b, q4km)
        
      - url: "http://localhost:11234"
        name: "local-lm-studio"
        type: "lm-studio"
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        # Has: microsoft/phi-4-mini-reasoning (phi3 family, q4km)
```

Even though both are Phi-4 models from Microsoft:
- **Ollama version**: `phi4:latest` (14.7b parameters, q4km quantization)
- **LM Studio version**: `microsoft/phi-4-mini-reasoning` (phi3 family, 131k context)

They remain completely separate in Olla's catalogue because:
- Different naming conventions (`phi4:latest` vs `microsoft/phi-4-mini-reasoning`)
- Different provider APIs (Ollama native vs OpenAI-compatible)
- Different capabilities (standard vs reasoning-optimized)

**Routing behavior:**
- Requests to `/olla/ollama/v1/chat/completions` with `model: "phi4:latest"` → Only goes to Ollama endpoints
- Requests to `/olla/lmstudio/v1/chat/completions` with `model: "microsoft/phi-4-mini-reasoning"` → Only goes to LM Studio endpoints
- No cross-provider routing even for similar models

This design ensures API compatibility and prevents routing errors between incompatible endpoints.

## Model Routing Examples

### Provider-Specific Routing

```bash
# This request only goes to Ollama endpoints
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# This request only goes to LM Studio endpoints  
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "microsoft/phi-4-mini-reasoning",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Native Ollama API also works
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "phi4:latest",
    "messages": [{"role": "user", "content": "Write code in Python"}]
  }'
```

### Unified Model Discovery

List all models across all providers:

```bash
# All models from all providers (21 total from this real setup)
curl http://localhost:40114/olla/models
# Returns: 15 Ollama + 6 LM Studio = 21 models total

# Provider-specific model lists
curl http://localhost:40114/olla/ollama/v1/models
# Returns: 15 Ollama models in OpenAI format

curl http://localhost:40114/olla/lmstudio/v1/models  
# Returns: 6 LM Studio models

# OpenAI-compatible unified view (both Ollama and LM Studio)
curl http://localhost:40114/olla/openai/v1/models
# Returns: All 21 models in OpenAI format for maximum compatibility
```

## The Magic of OpenAI-Compatible Unification

Here's the key insight: **Both Ollama and LM Studio support OpenAI-compatible APIs**, so Olla exposes a unified `/olla/openai/` endpoint that can route to either provider based on the requested model:

```bash
# This request can route to either Ollama OR LM Studio!
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
# ↳ Routes to Ollama endpoint (has llama3.2:latest)

curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "microsoft/phi-4-mini-reasoning", 
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
# ↳ Routes to LM Studio endpoint (has microsoft/phi-4-mini-reasoning)
```

**Real Response Headers Show the Magic:**
```bash
# Request to llama3.2:latest via /olla/openai/
X-Olla-Endpoint: local-ollama
X-Olla-Model: llama3.2:latest
X-Olla-Backend-Type: ollama

# Request to microsoft/phi-4-mini-reasoning via /olla/openai/
X-Olla-Endpoint: local-lm-studio  
X-Olla-Model: microsoft/phi-4-mini-reasoning
X-Olla-Backend-Type: lmstudio
```

This means you can:
- **Use one client** (`/olla/openai/`) to access models from multiple provider types
- **Let Olla route automatically** based on which endpoint actually has the model
- **Maintain full API compatibility** with OpenAI clients

### Three Ways to Access the Same Model

Depending on your needs, you can access models through different endpoints:

**1. Provider-Specific (Most Explicit)**
```bash
# Explicitly route to Ollama only
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -d '{"model": "llama3.2:latest", "messages": [...]}'
```

**2. OpenAI-Compatible (Most Flexible)**  
```bash
# Let Olla route based on model availability
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -d '{"model": "llama3.2:latest", "messages": [...]}'
```

**3. Native API (Provider-Specific Features)**
```bash  
# Use Ollama's native streaming API
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -d '{"model": "llama3.2:latest", "messages": [...], "stream": true}'
```

The real power is in approach #2: **one OpenAI client can seamlessly access models from different providers** without knowing or caring where they're hosted.

## Model Availability and Health

Olla tracks model availability per endpoint and factors this into routing decisions:

### Model Status Indicators

- **Available**: Model is loaded and ready to serve requests
- **Loading**: Model is being loaded into memory  
- **Unavailable**: Endpoint is down or model is not accessible
- **Error**: Model failed to load or endpoint returned an error

### Checking Model Status

```bash
# Get model status across all endpoints
curl http://localhost:40114/internal/status/models

# Real response from the running instance:
{
  "timestamp": "2025-08-03T10:34:54+10:00",
  "models": [
    {
      "name": "llama3.2:latest",
      "type": "ollama",
      "endpoints": [
        {
          "name": "local-ollama", 
          "status": "available",
          "size": "2.0GB"
        }
      ]
    },
    {
      "name": "microsoft/phi-4-mini-reasoning",
      "type": "lmstudio", 
      "endpoints": [
        {
          "name": "local-lm-studio",
          "status": "not-loaded",
          "context_length": 131072
        }
      ]
    }
  ]
}
```

## Configuration Options

### Model Discovery Settings

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: "5m"          # How often to refresh model lists
    timeout: "30s"          # Timeout for model discovery requests
    concurrent_workers: 3   # Parallel discovery workers
    retry_attempts: 2       # Retries for failed discovery
    retry_backoff: "1s"     # Backoff between retries
```

### Model Registry Configuration

```yaml
model_registry:
  type: "memory"           # Storage type (only memory supported)
  enable_unifier: true     # Enable model unification
  
  unification:
    enabled: true
    cache_ttl: "10m"       # How long to cache unified models
    
    # Custom unification rules per provider
    custom_rules:
      - type: "ollama"
        family_overrides:
          "phi4:*": "phi"  # Map phi4 models to phi family
        name_patterns:
          "llama-*": "llama3.2"  # Pattern-based name mapping
          
      - type: "lmstudio"
        name_patterns:
          "Meta-Llama-*": "llama"
```

## Best Practices

### 1. Use Provider-Specific Routes

Always use the appropriate provider prefix for your requests:

```python
# Good: Provider-specific routing
openai_client = openai.OpenAI(base_url="http://localhost:40114/olla/ollama/v1")
lmstudio_client = openai.OpenAI(base_url="http://localhost:40114/olla/lmstudio/v1")

# Avoid: Generic routing (not supported)
# generic_client = openai.OpenAI(base_url="http://localhost:40114/olla/v1")
```

### 2. Check Model Availability

Before making requests, verify models are available:

```python
import requests

def check_model_availability(base_url, model_name):
    response = requests.get(f"{base_url}/olla/models/{model_name}")
    if response.status_code == 200:
        model_info = response.json()
        available_endpoints = [
            e['name'] for e in model_info['endpoints'] 
            if e['status'] == 'available'
        ]
        return len(available_endpoints) > 0
    return False

# Usage
if check_model_availability("http://localhost:40114", "llama3.2"):
    # Proceed with request
    pass
```

### 3. Monitor Model Health

Set up monitoring for model availability:

```bash
#!/bin/bash
# Simple model health check script

check_models() {
    local status=$(curl -s http://localhost:40114/internal/status/models)
    local unavailable=$(echo "$status" | jq -r '.models[] | select(.endpoints[] | .status != "available") | .name')
    
    if [ -n "$unavailable" ]; then
        echo "Warning: Models unavailable: $unavailable"
        # Send alert or log warning
    fi
}

# Run every 5 minutes
while true; do
    check_models
    sleep 300
done
```

### 4. Plan for Model Distribution

Distribute popular models across multiple endpoints for better availability:

```yaml
# Good: Popular model on multiple endpoints
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://gpu-1:11434" 
        name: "ollama-primary"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        # Should have: llama3.2, mistral, phi
        
      - url: "http://gpu-2:11434"
        name: "ollama-secondary"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        # Should have: llama3.2, codellama, gemma
        
      - url: "http://cpu-server:11434"
        name: "ollama-backup"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        # Should have: llama3.2 (smaller quantization)
```

## Troubleshooting

### Model Not Found Errors

**Problem**: `model 'llama3.2' not found on any endpoint`

**Solutions**:
1. Check model is actually available: `curl http://localhost:40114/olla/models`
2. Verify endpoint health: `curl http://localhost:40114/internal/status/endpoints`
3. Check provider-specific route: Use `/olla/ollama/` instead of generic route
4. Wait for model discovery: Models may still be loading

### Cross-Provider Confusion

**Problem**: Expecting model unification across different providers

**Solution**: Remember that unification only happens within provider types:
- Ollama models only unify with other Ollama models
- LM Studio models only unify with other LM Studio models
- Use provider-specific routes: `/olla/ollama/`, `/olla/lmstudio/`, etc.

### Slow Model Discovery

**Problem**: Models taking too long to appear in catalogue

**Solutions**:
1. Reduce discovery interval: `interval: "1m"`
2. Increase concurrent workers: `concurrent_workers: 5`
3. Check endpoint response times: `curl -w "%{time_total}" http://endpoint/api/tags`

## See Also

- [API Endpoints Reference](./endpoints.md) - Complete API documentation
- [Load Balancing Guide](./load-balancers.md) - Load balancing strategies
- [Configuration Reference](./configuration.md) - All configuration options
- [Getting Started Guide](./getting-started.md) - Basic setup and deployment