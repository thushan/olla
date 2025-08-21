# LiteLLM API

Proxy endpoints for LiteLLM gateway. All LiteLLM API endpoints are available through the `/olla/litellm/` prefix.

## Endpoints Overview

### Core Endpoints (Always Available)

These endpoints are available in all LiteLLM deployments:

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/litellm/health` | Health check |
| GET | `/olla/litellm/v1/models` | List available models |
| POST | `/olla/litellm/v1/chat/completions` | Chat completion |
| POST | `/olla/litellm/v1/completions` | Text completion |
| POST | `/olla/litellm/v1/embeddings` | Generate embeddings |

### Optional Endpoints (Kubernetes/Advanced Deployments)

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/litellm/health/readiness` | Readiness probe (K8s) |
| GET | `/olla/litellm/health/liveness` | Liveness probe (K8s) |

### Database-Required Endpoints

**Note:** These endpoints only work when LiteLLM is configured with a PostgreSQL database backend:

| Method | URI | Description | Requirements |
|--------|-----|-------------|--------------|
| POST | `/olla/litellm/key/generate` | Generate API key | Database + Admin |
| GET | `/olla/litellm/key/info` | Get key info | Database |
| GET | `/olla/litellm/user/info` | User information | Database |
| GET | `/olla/litellm/team/info` | Team information | Database |
| GET | `/olla/litellm/spend/calculate` | Calculate spend | Database |

---

## POST /olla/litellm/v1/chat/completions

Chat completion using OpenAI-compatible format, routing to 100+ providers.

### Request

```bash
curl -X POST http://localhost:40114/olla/litellm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user", 
        "content": "Explain quantum computing in simple terms."
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
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1705320600,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Quantum computing is like having a super-powered calculator that can try many solutions at once..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 120,
    "total_tokens": 145
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1705320600,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Quantum"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1705320600,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" computing"},"finish_reason":null}]}

data: [DONE]
```

---

## GET /olla/litellm/v1/models

List all available models from configured providers.

### Request

```bash
curl http://localhost:40114/olla/litellm/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4",
      "object": "model",
      "created": 1698959748,
      "owned_by": "openai"
    },
    {
      "id": "claude-3-opus",
      "object": "model",
      "created": 1698959748,
      "owned_by": "anthropic"
    },
    {
      "id": "gemini-pro",
      "object": "model",
      "created": 1698959748,
      "owned_by": "google"
    },
    {
      "id": "llama-70b",
      "object": "model",
      "created": 1698959748,
      "owned_by": "together_ai"
    }
  ]
}
```

---

## POST /olla/litellm/v1/embeddings

Generate embeddings using available embedding models.

### Request

```bash
curl -X POST http://localhost:40114/olla/litellm/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-ada-002",
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
      "index": 0,
      "embedding": [
        -0.006929283,
        -0.005336422,
        -0.00040876168,
        ...
      ]
    }
  ],
  "model": "text-embedding-ada-002",
  "usage": {
    "prompt_tokens": 9,
    "total_tokens": 9
  }
}
```

---

## GET /olla/litellm/health

Check LiteLLM gateway health status.

### Request

```bash
curl http://localhost:40114/olla/litellm/health
```

### Response

```json
{
  "status": "healthy",
  "models": 25,
  "providers": [
    "openai",
    "anthropic",
    "bedrock",
    "gemini",
    "together_ai"
  ]
}
```


---

## Provider-Specific Models

LiteLLM supports provider-prefixed model names:

### OpenAI Models
- `gpt-4`, `gpt-4-turbo`, `gpt-3.5-turbo`
- `text-embedding-ada-002`, `text-embedding-3-small`

### Anthropic Models
- `claude-3-opus`, `claude-3-sonnet`, `claude-3-haiku`
- `claude-2.1`, `claude-2`, `claude-instant`

### Google Models
- `gemini-pro`, `gemini-pro-vision`
- `palm-2`, `chat-bison`

### AWS Bedrock Models
- `bedrock/claude-3-opus`, `bedrock/claude-3-sonnet`
- `bedrock/llama2-70b`, `bedrock/mistral-7b`

### Together AI Models
- `together_ai/llama-3-70b`, `together_ai/mixtral-8x7b`
- `together_ai/qwen-72b`, `together_ai/deepseek-coder`

---

## Response Headers

All LiteLLM requests through Olla include tracking headers:

```
X-Olla-Endpoint: litellm-gateway
X-Olla-Backend-Type: litellm
X-Olla-Model: gpt-4
X-Olla-Request-ID: req_abc123
X-Olla-Response-Time: 2.341s
```

---

## Error Handling

LiteLLM errors are passed through with additional context:

```json
{
  "error": {
    "message": "Rate limit exceeded for model gpt-4",
    "type": "rate_limit_error",
    "code": 429,
    "provider": "openai"
  }
}
```

---

## See Also

- [LiteLLM Integration Guide](../integrations/backend/litellm.md)
- [OpenAI API Reference](./openai.md)
- [Model Routing](../concepts/model-routing.md)
- [Load Balancing](../concepts/load-balancing.md)