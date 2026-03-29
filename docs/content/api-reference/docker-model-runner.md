# Docker Model Runner API

Proxy endpoints for Docker Model Runner inference servers. Available through the `/olla/dmr/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/dmr/engines/v1/models` | List available models |
| POST | `/olla/dmr/engines/v1/chat/completions` | Chat completion |
| POST | `/olla/dmr/engines/v1/completions` | Text completion |
| POST | `/olla/dmr/engines/v1/embeddings` | Generate embeddings |
| GET | `/olla/dmr/engines/llama.cpp/v1/models` | List models (llama.cpp engine) |
| POST | `/olla/dmr/engines/llama.cpp/v1/chat/completions` | Chat completion (llama.cpp engine) |
| GET | `/olla/dmr/engines/vllm/v1/models` | List models (vLLM engine) |
| POST | `/olla/dmr/engines/vllm/v1/chat/completions` | Chat completion (vLLM engine) |

The `/engines/v1/...` paths use automatic engine selection. The explicit `/engines/llama.cpp/v1/...` and `/engines/vllm/v1/...` paths target a specific engine directly.

---

## GET /olla/dmr/engines/v1/models

List models available on the Docker Model Runner instance.

Returns an empty `data` array when no models have been loaded yet — this is normal behaviour due to lazy model loading and does not indicate an unhealthy endpoint.

### Request

```bash
curl -X GET http://localhost:40114/olla/dmr/engines/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "ai/smollm2",
      "object": "model",
      "created": 1734000000,
      "owned_by": "ai"
    },
    {
      "id": "ai/llama3.2",
      "object": "model",
      "created": 1734000001,
      "owned_by": "ai"
    }
  ]
}
```

---

## POST /olla/dmr/engines/v1/chat/completions

OpenAI-compatible chat completion routed to the appropriate DMR engine.

### Request

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "What is Docker Model Runner?"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300,
    "stream": false
  }'
```

### Response

```json
{
  "id": "chatcmpl-dmr-abc123",
  "object": "chat.completion",
  "created": 1734000000,
  "model": "ai/smollm2",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Docker Model Runner is Docker's built-in LLM inference server that ships with Docker Desktop 4.40+. It enables you to pull, run, and serve large language models directly from Docker Hub as OCI artifacts, using llama.cpp for GGUF models and vLLM for safetensors models."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 22,
    "completion_tokens": 54,
    "total_tokens": 76
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-dmr-abc123","object":"chat.completion.chunk","created":1734000000,"model":"ai/smollm2","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-dmr-abc123","object":"chat.completion.chunk","created":1734000000,"model":"ai/smollm2","choices":[{"index":0,"delta":{"content":"Docker"},"finish_reason":null}]}

...

data: {"id":"chatcmpl-dmr-abc123","object":"chat.completion.chunk","created":1734000001,"model":"ai/smollm2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

---

## POST /olla/dmr/engines/v1/completions

Text completion (non-chat) via DMR.

### Request

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "prompt": "Docker Model Runner is",
    "max_tokens": 100,
    "temperature": 0.8,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-dmr-xyz789",
  "object": "text_completion",
  "created": 1734000000,
  "model": "ai/smollm2",
  "choices": [
    {
      "text": " Docker's built-in LLM inference server, enabling developers to run AI models locally using the same tools they use for containers.",
      "index": 0,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 28,
    "total_tokens": 33
  }
}
```

---

## POST /olla/dmr/engines/v1/embeddings

Generate embeddings using a DMR model that supports embeddings.

### Request

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/mxbai-embed-large",
    "input": "Docker Model Runner provides local LLM inference",
    "encoding_format": "float"
  }'
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0234, -0.0567, 0.0891, "..."]
    }
  ],
  "model": "ai/mxbai-embed-large",
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}
```

---

## Explicit Engine Endpoints

Use these paths to target a specific inference engine, bypassing automatic engine selection.

### llama.cpp Engine

```bash
# List models on llama.cpp engine
curl http://localhost:40114/olla/dmr/engines/llama.cpp/v1/models

# Chat completion via llama.cpp
curl -X POST http://localhost:40114/olla/dmr/engines/llama.cpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 100
  }'
```

### vLLM Engine

```bash
# List models on vLLM engine (Linux + NVIDIA only)
curl http://localhost:40114/olla/dmr/engines/vllm/v1/models

# Chat completion via vLLM
curl -X POST http://localhost:40114/olla/dmr/engines/vllm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/llama3.1-8b",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 100
  }'
```

---

## Model Naming

DMR uses a `namespace/name` format for all model identifiers:

| Example | Description |
|---------|-------------|
| `ai/smollm2` | SmolLM2 from the `ai` namespace |
| `ai/llama3.2` | Llama 3.2 from the `ai` namespace |
| `ai/phi4-mini` | Phi-4 Mini from the `ai` namespace |

Always use the full `namespace/name` format in the `model` field of API requests.

---

## Request Headers

All requests are forwarded with:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- Custom headers from endpoint configuration

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., `local-dmr`)
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always `docker-model-runner` for these endpoints
- `X-Olla-Response-Time` - Total processing time

## Configuration Example

```yaml
endpoints:
  - url: "http://localhost:12434"
    name: "local-dmr"
    type: "docker-model-runner"
    priority: 95
    model_url: "/engines/v1/models"
    health_check_url: "/engines/v1/models"
    check_interval: 10s
    check_timeout: 5s
```

See the [Docker Model Runner Integration Guide](../integrations/backend/docker-model-runner.md) for full configuration and setup instructions.
