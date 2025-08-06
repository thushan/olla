# Provider-Specific Routing

Olla uses provider-specific routing to namespace requests by backend type. All proxy requests now go through provider-specific endpoints (no more generic `/olla/` proxy).

## Architecture

Olla supports multiple backend providers, each with its own API and model management. 

To handle this, Olla uses a structured routing system:
- `/olla/ollama/*` - Routes only to Ollama backends
- `/olla/lmstudio/*` - Routes only to LM Studio backends  
- `/olla/openai/*` - Routes only to OpenAI-compatible backends
- `/olla/vllm/*` - Routes only to vLLM backends

This design:
- Makes provider selection explicit in the URL
- Allows provider-specific endpoint handling
- Maintains API compatibility with each provider
- Enables load balancing within each provider type

## Provider-Specific Model Endpoints

### Ollama Models
```
GET /olla/ollama/api/tags       # Ollama native format
GET /olla/ollama/v1/models      # OpenAI-compatible format
```

Returns all available models from healthy Ollama instances. Ollama supports both its native format and OpenAI-compatible format (experimental).

**Response Format Examples**:

Ollama format (`/api/tags`):
```json
{
  "models": [
    {
      "name": "llama3:latest",
      "model": "llama3:latest",
      "modified_at": "2024-01-27T10:30:00Z",
      "size": 4661224960,
      "digest": "sha256:abc123...",
      "details": {
        "family": "llama",
        "parameter_size": "8B",
        "quantization_level": "Q4_0"
      }
    }
  ]
}
```

OpenAI format (`/v1/models`):
```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3:latest",
      "object": "model",
      "created": 1706352600,
      "owned_by": "system"
    }
  ]
}
```

### LM Studio Models
```
GET /olla/lmstudio/v1/models        # OpenAI-compatible format  
GET /olla/lmstudio/api/v1/models    # OpenAI-compatible format (alt path)
GET /olla/lmstudio/api/v0/models    # Enhanced format with additional metadata
```

Returns all available models from healthy LM Studio instances in OpenAI-compatible format.

**Response Format**:

OpenAI format (`/v1/models`, `/api/v1/models`):
```json
{
  "object": "list",
  "data": [
    {
      "id": "TheBloke/Llama-2-7B-Chat-GGUF",
      "object": "model",
      "created": 1706352600,
      "owned_by": "system"
    }
  ]
}
```

LM Studio enhanced format (`/api/v0/models`):
```json
{
  "models": [
    {
      "id": "TheBloke/Llama-2-7B-Chat-GGUF",
      "type": "llm",
      "publisher": "TheBloke",
      "arch": "llama",
      "quantization": "Q4_K_M",
      "state": "loaded",
      "size": 3825819904,
      "max_context_length": 4096,
      "compatibility": {
        "openai_api": true,
        "completion": true,
        "chat_completion": true,
        "embeddings": false
      },
      "performance": {
        "tokens_per_second": 45.2,
        "memory_usage": 4096
      }
    }
  ]
}
```

## Provider-Specific Proxy Routes

All requests to provider-specific routes are automatically routed to healthy endpoints of that provider type only.

### Ollama Proxy
```
POST /olla/ollama/*
```

Routes requests to Ollama instances only. The path after `/olla/ollama/` is forwarded to the backend.

**Examples**:
- `/olla/ollama/api/generate` → `/api/generate` (on Ollama backend)
- `/olla/ollama/api/chat` → `/api/chat` (on Ollama backend)

### LM Studio Proxy
```
POST /olla/lmstudio/*
```

Routes requests to LM Studio instances only.

**Examples**:
- `/olla/lmstudio/v1/chat/completions` → `/v1/chat/completions` (on LM Studio backend)

### OpenAI Proxy
```
POST /olla/openai/*
```

Routes requests to OpenAI-compatible instances only.

### vLLM Proxy
```
POST /olla/vllm/*
```

Routes requests to vLLM instances only.

## Load Balancing

Within each provider type, the configured load balancing strategy (round-robin, least connections, priority) is applied. This means:

1. If you have 3 Ollama instances configured, requests to `/olla/ollama/*` will be load balanced across all healthy Ollama instances
2. The same model-based routing rules apply - if a specific model is requested, only endpoints with that model will be considered
3. Health checking ensures only available endpoints receive traffic

## Error Handling

- **404 Not Found**: Returned when no endpoints of the requested provider type are available
- **502 Bad Gateway**: Returned when all endpoints of the provider type are unhealthy
- **503 Service Unavailable**: Returned when the proxy cannot reach any backend

## Configuration

Provider types are configured in the endpoint configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"        # Provider type
        priority: 100
      
      - url: "http://192.168.1.100:1234"
        name: "lmstudio-1"
        type: "lmstudio"      # Provider type
        priority: 90
```

## Intercepted vs Proxied Endpoints

Olla uses a pattern where certain endpoints are intercepted and handled directly, while others are proxied to the appropriate backend. This allows Olla to provide unified model management while maintaining API compatibility.

### Intercepted Endpoints (Handled by Olla)

These endpoints are processed by Olla to provide unified functionality across multiple instances:

#### Ollama
- **GET** `/olla/ollama/api/tags` - Returns unified model list in Ollama native format
- **GET** `/olla/ollama/v1/models` - Returns unified model list in OpenAI format
- **GET** `/olla/ollama/api/list` - Returns 501 (running models across instances not supported)
- **POST** `/olla/ollama/api/show` - Returns 501 (model details across instances not supported)

#### LM Studio
- **GET** `/olla/lmstudio/v1/models` - Returns models in OpenAI format
- **GET** `/olla/lmstudio/api/v1/models` - Returns models in OpenAI format (alt path)
- **GET** `/olla/lmstudio/api/v0/models` - Returns models in LM Studio enhanced format with additional metadata

#### OpenAI-Compatible
- **GET** `/olla/openai/v1/models` - Returns unified model list in OpenAI format

#### vLLM
- **GET** `/olla/vllm/v1/models` - Returns unified model list in OpenAI format

### Model Management Operations (Return 501)

These endpoints return HTTP 501 Not Implemented as distributed model management is not supported:

#### Ollama
- **POST** `/olla/ollama/api/pull` - Model download
- **POST** `/olla/ollama/api/push` - Model upload
- **POST** `/olla/ollama/api/create` - Model creation
- **POST** `/olla/ollama/api/copy` - Model copying
- **DELETE** `/olla/ollama/api/delete` - Model deletion

### Proxied Endpoints

All other endpoints are transparently proxied to the appropriate backend with load balancing:

#### Inference Endpoints
- **Ollama**: `/api/generate`, `/api/chat`, `/api/embeddings`
- **LM Studio**: `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`
- **OpenAI/vLLM**: `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`

#### Specialised Endpoints (vLLM)
- `/tokenize`, `/detokenize`, `/pooling`, `/score`, `/v1/rerank`

The proxy automatically:
1. Strips the provider prefix (e.g., `/olla/ollama/` → `/`)
2. Routes to healthy endpoints of the specified provider type
3. Applies configured load balancing strategy
4. Adds response headers for observability

## Use Cases

### Dedicated Ollama Interface
Point your Ollama clients to `/olla/ollama/` to create a unified interface for multiple Ollama instances:
```bash
# Instead of pointing to a single Ollama instance
curl http://localhost:11434/api/tags

# Point to Olla's Ollama endpoint for load-balanced access
curl http://localhost:40114/olla/ollama/api/tags
```

### LM Studio Aggregation
Aggregate multiple LM Studio instances into a single endpoint:
```python
# Configure OpenAI client to use Olla's LM Studio endpoint
client = OpenAI(
    base_url="http://localhost:40114/olla/lmstudio/v1",
    api_key="not-needed"
)
```

### Provider Isolation
Keep different provider types isolated while still benefiting from Olla's load balancing and health checking:
- Development team uses `/olla/ollama/` for local models
- Production uses `/olla/openai/` for cloud models
- Testing uses `/olla/lmstudio/` for specific test models