# LM Studio API

Proxy endpoints for LM Studio servers. Available through multiple prefixes: `/olla/lmstudio/`, `/olla/lm-studio/`, and `/olla/lm_studio/`.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/lmstudio/v1/models` | List available models |
| POST | `/olla/lmstudio/v1/chat/completions` | Chat completion |
| POST | `/olla/lmstudio/v1/completions` | Text completion |
| POST | `/olla/lmstudio/v1/embeddings` | Generate embeddings |
| GET | `/olla/lmstudio/api/v0/models` | Legacy models endpoint |

## Alternative Prefixes

All endpoints are available through these equivalent prefixes:

- `/olla/lmstudio/*`
- `/olla/lm-studio/*`
- `/olla/lm_studio/*`

---

## GET /olla/lmstudio/v1/models

List all models available in LM Studio.

### Request

```bash
curl -X GET http://localhost:40114/olla/lmstudio/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "TheBloke/phi-3-mini-4k-instruct-GGUF/phi-3-mini-4k-instruct.Q4_K_M.gguf",
      "object": "model",
      "created": 1705334400,
      "owned_by": "TheBloke",
      "permission": [],
      "root": "phi-3-mini",
      "parent": null,
      "max_context_length": 4096
    },
    {
      "id": "TheBloke/gemma-2b-instruct-GGUF/gemma-2b-instruct.Q4_K_M.gguf",
      "object": "model",
      "created": 1705334400,
      "owned_by": "TheBloke",
      "permission": [],
      "root": "gemma-2b",
      "parent": null,
      "max_context_length": 8192
    }
  ]
}
```

---

## POST /olla/lmstudio/v1/chat/completions

OpenAI-compatible chat completion endpoint.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "phi-3-mini",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful coding assistant."
      },
      {
        "role": "user",
        "content": "Write a Python function to calculate fibonacci numbers"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 500,
    "stream": false
  }'
```

### Response

```json
{
  "id": "chatcmpl-lmstudio-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "phi-3-mini",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Here's a Python function to calculate Fibonacci numbers:\n\n```python\ndef fibonacci(n):\n    if n <= 0:\n        return []\n    elif n == 1:\n        return [0]\n    elif n == 2:\n        return [0, 1]\n    \n    fib_sequence = [0, 1]\n    for i in range(2, n):\n        fib_sequence.append(fib_sequence[-1] + fib_sequence[-2])\n    \n    return fib_sequence\n\n# Example usage\nprint(fibonacci(10))  # [0, 1, 1, 2, 3, 5, 8, 13, 21, 34]\n```\n\nThis function generates the first n Fibonacci numbers."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 28,
    "completion_tokens": 142,
    "total_tokens": 170
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-lmstudio-abc123","object":"chat.completion.chunk","created":1705334400,"model":"phi-3-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Here's"},"finish_reason":null}]}

data: {"id":"chatcmpl-lmstudio-abc123","object":"chat.completion.chunk","created":1705334400,"model":"phi-3-mini","choices":[{"index":0,"delta":{"content":" a"},"finish_reason":null}]}

data: {"id":"chatcmpl-lmstudio-abc123","object":"chat.completion.chunk","created":1705334400,"model":"phi-3-mini","choices":[{"index":0,"delta":{"content":" Python"},"finish_reason":null}]}

...

data: {"id":"chatcmpl-lmstudio-abc123","object":"chat.completion.chunk","created":1705334401,"model":"phi-3-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

---

## POST /olla/lmstudio/v1/completions

Text completion endpoint for non-chat models.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmstudio/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-2b",
    "prompt": "The meaning of life is",
    "max_tokens": 100,
    "temperature": 0.8,
    "top_p": 0.9,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-lmstudio-xyz789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "gemma-2b",
  "choices": [
    {
      "text": " a question that has puzzled philosophers, theologians, and thinkers throughout human history. While there is no single definitive answer, many perspectives suggest that meaning comes from personal growth, relationships, contribution to society, and the pursuit of happiness and fulfillment.",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 6,
    "completion_tokens": 48,
    "total_tokens": 54
  }
}
```

---

## POST /olla/lmstudio/v1/embeddings

Generate embeddings for text input.

### Request

```bash
curl -X POST http://localhost:40114/olla/lmstudio/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text",
    "input": "The quick brown fox jumps over the lazy dog"
  }'
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "embedding": [0.123, -0.456, 0.789, ...],
      "index": 0
    }
  ],
  "model": "nomic-embed-text",
  "usage": {
    "prompt_tokens": 9,
    "total_tokens": 9
  }
}
```

---

## GET /olla/lmstudio/api/v0/models

Legacy models endpoint for backward compatibility.

### Request

```bash
curl -X GET http://localhost:40114/olla/lmstudio/api/v0/models
```

### Response

```json
{
  "models": [
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
    },
    {
      "id": "gemma-2b",
      "object": "model",
      "owned_by": "google",
      "permission": [],
      "engines": {
        "completions": {
          "context_length": 8192,
          "max_tokens": 8192,
          "tokenizer": "gemma"
        }
      }
    }
  ]
}
```

## Model Loading

LM Studio typically preloads models, resulting in:
- Fast initial response times
- Single model active at a time
- No model loading delays

## Request Options

### Common Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `temperature` | float | 0.7 | Sampling temperature (0.0-2.0) |
| `top_p` | float | 0.95 | Nucleus sampling threshold |
| `top_k` | integer | 40 | Top-k sampling |
| `max_tokens` | integer | - | Maximum tokens to generate |
| `stop` | array | - | Stop sequences |
| `presence_penalty` | float | 0 | Penalize new tokens (-2.0 to 2.0) |
| `frequency_penalty` | float | 0 | Penalize repeated tokens (-2.0 to 2.0) |
| `repetition_penalty` | float | 1.1 | Repetition penalty (0.0-2.0) |
| `seed` | integer | - | Random seed for reproducibility |

### LM Studio-Specific Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `n_predict` | integer | Alternative to max_tokens |
| `mirostat` | integer | Mirostat sampling (0/1/2) |
| `mirostat_tau` | float | Mirostat target entropy |
| `mirostat_eta` | float | Mirostat learning rate |
| `grammar` | string | BNF grammar for constrained generation |

## Performance Characteristics

- **Concurrency**: Single request at a time (LM Studio limitation)
- **Context Window**: Model-dependent (typically 4K-32K)
- **Response Time**: Fast (models preloaded in memory)
- **Streaming**: Fully supported with low latency

## Request Headers

All requests are forwarded with:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- `X-Forwarded-Host` - Original host

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., "local-lm-studio")
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always "lm-studio" for these endpoints
- `X-Olla-Response-Time` - Total processing time