# LMDeploy API

Proxy endpoints for LMDeploy inference servers. Available through the `/olla/lmdeploy/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/lmdeploy/health` | Health check |
| GET | `/olla/lmdeploy/v1/models` | List available models |
| POST | `/olla/lmdeploy/v1/chat/completions` | Chat completion |
| POST | `/olla/lmdeploy/v1/completions` | Text completion |
| POST | `/olla/lmdeploy/v1/encode` | Token encoding |
| POST | `/olla/lmdeploy/generate` | Native generation |
| POST | `/olla/lmdeploy/pooling` | Reward/score pooling |
| GET | `/olla/lmdeploy/is_sleeping` | Sleep state probe |

!!! warning "/v1/embeddings is not supported"
    LMDeploy returns HTTP 400 on `/v1/embeddings` unconditionally across all backends. Use `/pooling` for reward/score tasks instead.

---

## GET /olla/lmdeploy/health

Check LMDeploy server health.

### Request

```bash
curl http://localhost:40114/olla/lmdeploy/health
```

### Response

```json
{}
```

LMDeploy returns an empty body with HTTP 200 on a healthy server.

---

## GET /olla/lmdeploy/v1/models

List models available on the LMDeploy server.

### Request

```bash
curl http://localhost:40114/olla/lmdeploy/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "internlm/internlm2_5-7b-chat",
      "object": "model",
      "created": 1705334400,
      "owned_by": "lmdeploy",
      "root": "internlm/internlm2_5-7b-chat",
      "permission": []
    }
  ]
}
```

---

## POST /olla/lmdeploy/v1/chat/completions

OpenAI-compatible chat completion.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "messages": [
      {"role": "user", "content": "Explain the TurboMind engine in one paragraph"}
    ],
    "temperature": 0.7,
    "max_tokens": 200,
    "stream": false
  }'
```

### Response

```json
{
  "id": "chatcmpl-lmdeploy-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "internlm/internlm2_5-7b-chat",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "TurboMind is LMDeploy's C++/CUDA inference engine..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 18,
    "completion_tokens": 64,
    "total_tokens": 82
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-lmdeploy-abc123","object":"chat.completion.chunk","created":1705334400,"model":"internlm/internlm2_5-7b-chat","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-lmdeploy-abc123","object":"chat.completion.chunk","created":1705334400,"model":"internlm/internlm2_5-7b-chat","choices":[{"index":0,"delta":{"content":"TurboMind"},"finish_reason":null}]}

...

data: [DONE]
```

---

## POST /olla/lmdeploy/v1/completions

Text completion (OpenAI-compatible).

### Request

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "prompt": "LMDeploy uses TurboMind because",
    "max_tokens": 100,
    "temperature": 0.8,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-lmdeploy-xyz789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "internlm/internlm2_5-7b-chat",
  "choices": [
    {
      "text": " it provides efficient GPU utilisation through continuous batching...",
      "index": 0,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 55,
    "total_tokens": 64
  }
}
```

---

## POST /olla/lmdeploy/v1/encode

Token encoding (LMDeploy-specific). Encodes input text to token IDs without running inference.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/encode \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "input": "Hello, world!"
  }'
```

### Response

```json
{
  "input_ids": [1, 22172, 29892, 3186, 29991],
  "length": 5
}
```

---

## POST /olla/lmdeploy/generate

LMDeploy's native generation endpoint.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/generate \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": "def fibonacci(n):\n    ",
    "parameters": {
      "temperature": 0.2,
      "max_new_tokens": 100
    }
  }'
```

### Response

```json
{
  "generated_text": "def fibonacci(n):\n    if n <= 1:\n        return n\n    return fibonacci(n-1) + fibonacci(n-2)",
  "finish_reason": "stop"
}
```

---

## POST /olla/lmdeploy/pooling

Reward or score pooling for embedding-style tasks. This is the correct path for pooling operations — `/v1/embeddings` is not supported.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/pooling \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "input": "The quick brown fox"
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
      "embedding": [0.0123, -0.0456, 0.0789, ...]
    }
  ],
  "model": "internlm/internlm2_5-7b-chat",
  "usage": {
    "prompt_tokens": 5,
    "total_tokens": 5
  }
}
```

---

## GET /olla/lmdeploy/is_sleeping

Probe whether the LMDeploy engine is in sleep mode. Sleeping instances return HTTP 503 on generation endpoints — Olla's health checker treats this as a transient failure rather than a hard outage.

### Request

```bash
curl http://localhost:40114/olla/lmdeploy/is_sleeping
```

### Response

```json
{"is_sleeping": false}
```

---

## Configuration Example

```yaml
discovery:
  static:
    endpoints:
      - url: "http://192.168.0.100:23333"
        name: "lmdeploy-server"
        type: "lmdeploy"
        priority: 82
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

The default port for `lmdeploy serve api_server` is **23333**. The `proxy_server` component runs on 8000 but does not expose `/health` and is not supported by Olla.

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., `lmdeploy-server`)
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always `lmdeploy` for these endpoints
- `X-Olla-Response-Time` - Total processing time
