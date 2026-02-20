# vLLM-MLX API

Proxy endpoints for vLLM-MLX inference servers running on Apple Silicon. Available through the `/olla/vllm-mlx/` prefix.

vLLM-MLX serves a single model per instance using MLX-format weights from HuggingFace (e.g. `mlx-community/Llama-3.2-3B-Instruct-4bit`). It exposes a standard OpenAI-compatible API without guided generation or advanced vLLM features.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/vllm-mlx/health` | Health check |
| GET | `/olla/vllm-mlx/v1/models` | List available models |
| POST | `/olla/vllm-mlx/v1/chat/completions` | Chat completion |
| POST | `/olla/vllm-mlx/v1/completions` | Text completion |
| POST | `/olla/vllm-mlx/v1/embeddings` | Generate embeddings |

---

## GET /olla/vllm-mlx/health

Check vLLM-MLX server health status.

### Request

```bash
curl -X GET http://localhost:40114/olla/vllm-mlx/health
```

### Response

```json
{
  "status": "healthy"
}
```

---

## GET /olla/vllm-mlx/v1/models

List the model available on the vLLM-MLX server. Each instance serves a single model.

### Request

```bash
curl -X GET http://localhost:40114/olla/vllm-mlx/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "mlx-community/Llama-3.2-3B-Instruct-4bit",
      "object": "model",
      "created": 1705334400,
      "owned_by": "vllm-mlx",
      "root": "mlx-community/Llama-3.2-3B-Instruct-4bit",
      "parent": null,
      "permission": []
    }
  ]
}
```

---

## POST /olla/vllm-mlx/v1/chat/completions

OpenAI-compatible chat completion.

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm-mlx/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful AI assistant."
      },
      {
        "role": "user",
        "content": "What is MLX?"
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
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "MLX is an array framework for machine learning on Apple Silicon, developed by Apple's machine learning research team. It provides efficient GPU-accelerated computation using the unified memory architecture of Apple's M-series chips."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 42,
    "total_tokens": 67
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334400,"model":"mlx-community/Llama-3.2-3B-Instruct-4bit","choices":[{"index":0,"delta":{"role":"assistant"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334400,"model":"mlx-community/Llama-3.2-3B-Instruct-4bit","choices":[{"index":0,"delta":{"content":"MLX"},"logprobs":null,"finish_reason":null}]}

...

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334401,"model":"mlx-community/Llama-3.2-3B-Instruct-4bit","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

data: [DONE]
```

---

## POST /olla/vllm-mlx/v1/completions

Text completion.

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm-mlx/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "prompt": "Apple Silicon is designed for",
    "max_tokens": 200,
    "temperature": 0.8,
    "top_p": 0.95,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-xyz789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
  "choices": [
    {
      "text": " high-performance computing with exceptional energy efficiency. The unified memory architecture allows the CPU, GPU, and Neural Engine to share the same memory pool, eliminating the overhead of copying data between processors.",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 6,
    "completion_tokens": 38,
    "total_tokens": 44
  }
}
```

---

## POST /olla/vllm-mlx/v1/embeddings

Generate embeddings (if the loaded model supports embeddings).

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm-mlx/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "input": "MLX is optimised for Apple Silicon",
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
      "embedding": [0.0234, -0.0567, 0.0891, ...]
    }
  ],
  "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}
```

## Sampling Parameters

Standard OpenAI-compatible sampling parameters are supported.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `temperature` | float | 1.0 | Sampling temperature |
| `top_p` | float | 1.0 | Nucleus sampling threshold |
| `max_tokens` | integer | - | Maximum tokens to generate |
| `stop` | string/array | - | Stop sequences |
| `stream` | boolean | false | Enable streaming response |
| `frequency_penalty` | float | 0.0 | Frequency penalty |
| `presence_penalty` | float | 0.0 | Presence penalty |

## Configuration Example

```yaml
endpoints:
  - url: "http://192.168.0.100:8000"
    name: "vllm-mlx-server"
    type: "vllm-mlx"
    priority: 80
    model_url: "/v1/models"
    health_check_url: "/health"
    check_interval: 5s
    check_timeout: 2s
```

## Request Headers

All requests are forwarded with:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- Custom headers from endpoint configuration

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., "vllm-mlx-server")
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always "vllm-mlx" for these endpoints
- `X-Olla-Response-Time` - Total processing time
