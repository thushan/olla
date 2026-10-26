# Ollama API

Proxy endpoints for Ollama instances. All Ollama API endpoints are available through the `/olla/ollama/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/ollama/` | Health check |
| POST | `/olla/ollama/api/generate` | Generate completion |
| POST | `/olla/ollama/api/chat` | Chat completion |
| POST | `/olla/ollama/api/embeddings` | Generate embeddings |
| POST | `/olla/ollama/api/embed` | Generate embeddings (newer form of `/api/embeddings`; `model` must be the full tag, such as `nomic-embed-text:latest`) |
| GET | `/olla/ollama/api/tags` | List local models |
| POST | `/olla/ollama/api/show` | _Not supported in multi-instance proxy_ (returns 501) |
| POST | `/olla/ollama/api/pull` | _Not supported in multi-instance proxy_ (returns 501) |
| POST | `/olla/ollama/api/push` | _Not supported in multi-instance proxy_ (returns 501) |
| POST | `/olla/ollama/api/copy` | _Not supported in multi-instance proxy_ (returns 501) |
| DELETE | `/olla/ollama/api/delete` | _Not supported in multi-instance proxy_ (returns 501) |
| POST | `/olla/ollama/api/create` | _Not supported in multi-instance proxy_ (returns 501) |
| GET | `/olla/ollama/api/list` | _Not supported in multi-instance proxy_ (returns 501; aggregating running models across instances requires per-instance polling and reconciliation that Olla deliberately does not perform) |

## OpenAI-Compatible Endpoints

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/ollama/v1/models` | List models (OpenAI format) |
| POST | `/olla/ollama/v1/chat/completions` | Chat completion (OpenAI format) |
| POST | `/olla/ollama/v1/completions` | Text completion (OpenAI format) |
| POST | `/olla/ollama/v1/embeddings` | Generate embeddings (OpenAI format) |

---

## POST /olla/ollama/api/generate

Generate text completion using Ollama's native API.

### Request

```bash
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "prompt": "Why is the sky blue?",
    "stream": false,
    "options": {
      "temperature": 0.7,
      "top_p": 0.9,
      "max_tokens": 200
    }
  }'
```

### Response

```json
{
  "model": "llama3.2:latest",
  "created_at": "2024-01-15T10:30:00Z",
  "response": "The sky appears blue because of a phenomenon called Rayleigh scattering. When sunlight enters Earth's atmosphere, it collides with gas molecules. Blue light waves are shorter and scatter more than other colors, making the sky appear blue to our eyes.",
  "done": true,
  "context": [1234, 5678, ...],
  "total_duration": 5250000000,
  "load_duration": 1200000000,
  "prompt_eval_count": 8,
  "prompt_eval_duration": 350000000,
  "eval_count": 45,
  "eval_duration": 3700000000
}
```

### Streaming Response

When `"stream": true`:

```
{"model":"llama3.2:latest","created_at":"2024-01-15T10:30:00Z","response":"The","done":false}
{"model":"llama3.2:latest","created_at":"2024-01-15T10:30:00Z","response":" sky","done":false}
{"model":"llama3.2:latest","created_at":"2024-01-15T10:30:00Z","response":" appears","done":false}
...
{"model":"llama3.2:latest","created_at":"2024-01-15T10:30:01Z","response":"","done":true,"total_duration":5250000000}
```

---

## POST /olla/ollama/api/chat

Chat completion with conversation history.

### Request

```bash
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [
      {
        "role": "user",
        "content": "What is the capital of France?"
      }
    ],
    "stream": false,
    "options": {
      "temperature": 0.7
    }
  }'
```

### Response

```json
{
  "model": "llama3.2:latest",
  "created_at": "2024-01-15T10:30:00Z",
  "message": {
    "role": "assistant",
    "content": "The capital of France is Paris."
  },
  "done": true,
  "total_duration": 2150000000,
  "load_duration": 1200000000,
  "prompt_eval_count": 10,
  "prompt_eval_duration": 150000000,
  "eval_count": 8,
  "eval_duration": 800000000
}
```

---

## POST /olla/ollama/v1/chat/completions

OpenAI-compatible chat completion endpoint.

### Request

```bash
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "What is 2+2?"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 100,
    "stream": false
  }'
```

### Response

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "llama3.2:latest",
  "system_fingerprint": "fp_abc123",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "2 + 2 equals 4."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 8,
    "total_tokens": 33
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334400,"model":"llama3.2:latest","choices":[{"index":0,"delta":{"role":"assistant","content":"2"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334400,"model":"llama3.2:latest","choices":[{"index":0,"delta":{"content":" +"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1705334400,"model":"llama3.2:latest","choices":[{"index":0,"delta":{"content":" 2"},"finish_reason":null}]}

data: [DONE]
```

---

## POST /olla/ollama/api/embeddings

Generate embeddings for text input.

### Request

```bash
curl -X POST http://localhost:40114/olla/ollama/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text",
    "prompt": "The quick brown fox jumps over the lazy dog"
  }'
```

### Response

```json
{
  "embedding": [0.123, -0.456, 0.789, ...],
  "model": "nomic-embed-text",
  "total_duration": 150000000,
  "load_duration": 50000000,
  "prompt_eval_count": 9
}
```

---

## GET /olla/ollama/api/tags

List all locally available models.

### Request

```bash
curl -X GET http://localhost:40114/olla/ollama/api/tags
```

### Response

```json
{
  "models": [
    {
      "name": "llama3.2:latest",
      "model": "llama3.2:latest",
      "modified_at": "2024-01-15T08:00:00Z",
      "size": 4089352192,
      "digest": "sha256:abc123def456",
      "details": {
        "parent_model": "",
        "format": "gguf",
        "family": "llama",
        "families": ["llama"],
        "parameter_size": "3.2B",
        "quantization_level": "Q4_0"
      }
    },
    {
      "name": "mistral:latest",
      "model": "mistral:latest",
      "modified_at": "2024-01-14T12:00:00Z",
      "size": 4113859584,
      "digest": "sha256:def789ghi012",
      "details": {
        "parent_model": "",
        "format": "gguf",
        "family": "mistral",
        "families": ["mistral"],
        "parameter_size": "7B",
        "quantization_level": "Q4_0"
      }
    }
  ]
}
```

---

## POST /olla/ollama/api/show

> :warning: **Not supported.** Olla returns HTTP 501. Aggregating model details across multiple Ollama instances is non-trivial because modelfiles, parameter values, and revisions can diverge between backends. To inspect a specific model, call the underlying Ollama instance directly.

## Model Management

These endpoints are recognised but deliberately **not supported** in Olla's multi-instance proxy. Each returns HTTP 501; perform model management directly against the relevant Ollama backend.

- **POST /olla/ollama/api/pull** - Download a model from the Ollama registry
- **POST /olla/ollama/api/push** - Upload a model to the Ollama registry
- **POST /olla/ollama/api/copy** - Create a copy of a model
- **DELETE /olla/ollama/api/delete** - Remove a model
- **POST /olla/ollama/api/create** - Create a model from a Modelfile
- **GET /olla/ollama/api/list** - List currently loaded models (Olla does not aggregate per-instance running state)

## Request Headers

All requests are forwarded with these additional headers:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- `X-Forwarded-Host` - Original host
- `X-Forwarded-Proto` - Original protocol

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., "local-ollama")
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always "ollama" for these endpoints
- `X-Olla-Response-Time` - Total processing time