# Unified Models API

Cross-provider model discovery endpoint that aggregates models from all configured backends.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/models` | List all available models |
| GET | `/olla/models/{id}` | Get specific model by ID or alias |
| GET | `/olla/models?format=unified` | Unified format (default) |
| GET | `/olla/models?format=openai` | OpenAI format |
| GET | `/olla/models?format=ollama` | Ollama format |
| GET | `/olla/models?format=lmstudio` | LM Studio format |
| GET | `/olla/models?format=vllm` | vLLM format |

---

## GET /olla/models

Returns all available models across all configured and healthy endpoints.

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `format` | string | `unified` | Response format (unified/openai/ollama/lmstudio/vllm) |
| `provider` | string | all | Filter by provider type |
| `capability` | string | all | Filter by capability (chat/completion/embedding/vision) |

### Request Examples

#### Default Unified Format
```bash
curl -X GET http://localhost:40114/olla/models
```

#### OpenAI Format
```bash
curl -X GET http://localhost:40114/olla/models?format=openai
```

#### Filter by Provider
```bash
curl -X GET http://localhost:40114/olla/models?provider=ollama
```

#### Filter by Capability
```bash
curl -X GET http://localhost:40114/olla/models?capability=chat
```

### Response Formats

#### Unified Format (Default)

```json
{
  "models": [
    {
      "id": "llama3.2:latest",
      "name": "llama3.2:latest",
      "provider": "ollama",
      "endpoint": "local-ollama",
      "capabilities": ["chat", "completion"],
      "context_window": 8192,
      "max_tokens": 8192,
      "vision": false,
      "embedding": false,
      "available": true,
      "metadata": {
        "size": "3.8GB",
        "modified": "2024-01-15T08:00:00Z",
        "family": "llama",
        "parameter_size": "3B",
        "quantization": "Q4_0"
      }
    },
    {
      "id": "phi-3-mini",
      "name": "phi-3-mini",
      "provider": "lm-studio",
      "endpoint": "local-lm-studio",
      "capabilities": ["chat", "completion"],
      "context_window": 4096,
      "max_tokens": 4096,
      "vision": false,
      "embedding": false,
      "available": true,
      "metadata": {
        "loaded": true,
        "gpu_layers": 32
      }
    }
  ],
  "total": 2,
  "providers": {
    "ollama": 1,
    "lm-studio": 1
  }
}
```

#### OpenAI Format

```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3.2:latest",
      "object": "model",
      "created": 1705334400,
      "owned_by": "ollama",
      "permission": [],
      "root": "llama3.2:latest",
      "parent": null
    },
    {
      "id": "phi-3-mini",
      "object": "model",
      "created": 1705334400,
      "owned_by": "lm-studio",
      "permission": [],
      "root": "phi-3-mini",
      "parent": null
    }
  ]
}
```

#### Ollama Format

```json
{
  "models": [
    {
      "name": "llama3.2:latest",
      "model": "llama3.2:latest",
      "modified_at": "2024-01-15T08:00:00Z",
      "size": 4089352192,
      "digest": "sha256:abc123",
      "details": {
        "parent_model": "",
        "format": "gguf",
        "family": "llama",
        "families": ["llama"],
        "parameter_size": "3.2B",
        "quantization_level": "Q4_0"
      }
    }
  ]
}
```

#### LM Studio Format

```json
{
  "data": [
    {
      "id": "phi-3-mini",
      "object": "model",
      "owned_by": "microsoft",
      "permission": [],
      "engines": {
        "chat_completions": {
          "context_length": 4096,
          "max_tokens": 4096,
          "tokenizer": "phi-3"
        }
      }
    }
  ]
}
```

#### vLLM Format

```json
{
  "object": "list",
  "data": [
    {
      "id": "meta-llama/Meta-Llama-3-8B",
      "object": "model",
      "created": 1705334400,
      "owned_by": "vllm",
      "max_model_len": 8192,
      "permission": []
    }
  ]
}
```

### Response Fields (Unified Format)

| Field | Type | Description |
|-------|------|-------------|
| `models` | array | List of available models |
| `models[].id` | string | Unique model identifier |
| `models[].name` | string | Model display name |
| `models[].provider` | string | Provider type (ollama/lmstudio/openai/vllm) |
| `models[].endpoint` | string | Backend endpoint name |
| `models[].capabilities` | array | Supported capabilities |
| `models[].context_window` | integer | Maximum context size |
| `models[].max_tokens` | integer | Maximum output tokens |
| `models[].vision` | boolean | Vision support |
| `models[].embedding` | boolean | Embedding support |
| `models[].available` | boolean | Model availability |
| `models[].metadata` | object | Provider-specific metadata |
| `total` | integer | Total number of models |
| `providers` | object | Model count by provider |

---

## GET /olla/models/{id}

Get information about a specific model by its ID or alias.

### Request

```bash
curl -X GET http://localhost:40114/olla/models/llama3.2:latest
```

### Response

```json
{
  "id": "llama3.2:latest",
  "name": "llama3.2:latest",
  "provider": "ollama",
  "endpoint": "local-ollama",
  "capabilities": ["chat", "completion"],
  "context_window": 8192,
  "max_tokens": 8192,
  "vision": false,
  "embedding": false,
  "available": true,
  "metadata": {
    "size": "3.8GB",
    "modified": "2024-01-15T08:00:00Z",
    "family": "llama",
    "parameter_size": "3B",
    "quantization": "Q4_0"
  }
}
```

### Error Responses

#### Model Not Found
```json
{
  "error": {
    "message": "Model not found: unknown-model",
    "type": "not_found",
    "code": "MODEL_NOT_FOUND"
  }
}
```

#### No Healthy Endpoints
```json
{
  "error": {
    "message": "No healthy endpoints available",
    "type": "service_unavailable",
    "code": "NO_ENDPOINTS"
  }
}
```

#### Invalid Format Parameter
```json
{
  "error": {
    "message": "Invalid format: unsupported. Supported formats: unified, openai, ollama, lmstudio, vllm",
    "type": "bad_request",
    "code": "INVALID_FORMAT"
  }
}
```

---

## Model Discovery

Models are discovered automatically from healthy endpoints:

1. **Discovery Interval**: Every 5 minutes (configurable)
2. **Discovery Timeout**: 30 seconds per endpoint
3. **Concurrent Workers**: 5 workers for parallel discovery
4. **Retry Policy**: 3 attempts with exponential backoff

## Caching

Model information is cached to improve performance:

- **Cache Duration**: 5 minutes
- **Stale Threshold**: 24 hours
- **Cleanup Interval**: 10 minutes

The cache is invalidated when:
- An endpoint becomes unhealthy
- Manual refresh is triggered
- New endpoints are added