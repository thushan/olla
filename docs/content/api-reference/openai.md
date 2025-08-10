# OpenAI API

Proxy endpoints for OpenAI and OpenAI-compatible services. Available through the `/olla/openai/` and `/olla/openai-compatible/` prefixes.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/openai/v1/models` | List available models |
| POST | `/olla/openai/v1/chat/completions` | Chat completion |
| POST | `/olla/openai/v1/completions` | Text completion |
| POST | `/olla/openai/v1/embeddings` | Generate embeddings |
| POST | `/olla/openai/v1/images/generations` | Generate images |
| POST | `/olla/openai/v1/audio/transcriptions` | Transcribe audio |
| POST | `/olla/openai/v1/audio/translations` | Translate audio |
| POST | `/olla/openai/v1/moderations` | Content moderation |

---

## GET /olla/openai/v1/models

List all available models from OpenAI or compatible endpoints.

### Request

```bash
curl -X GET http://localhost:40114/olla/openai/v1/models \
  -H "Authorization: Bearer YOUR_API_KEY"
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4-turbo-preview",
      "object": "model",
      "created": 1705334400,
      "owned_by": "openai",
      "permission": [],
      "root": "gpt-4-turbo-preview",
      "parent": null
    },
    {
      "id": "gpt-3.5-turbo",
      "object": "model",
      "created": 1677649963,
      "owned_by": "openai",
      "permission": [],
      "root": "gpt-3.5-turbo",
      "parent": null
    }
  ]
}
```

---

## POST /olla/openai/v1/chat/completions

Create a chat completion with GPT models.

### Request

```bash
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "Explain quantum computing in simple terms"
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
  "id": "chatcmpl-8q9ABC123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "gpt-3.5-turbo-0125",
  "system_fingerprint": "fp_abc123",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Quantum computing is a revolutionary type of computing that uses quantum bits or 'qubits' instead of traditional bits. While classical bits can only be 0 or 1, qubits can exist in multiple states simultaneously through a property called superposition. This allows quantum computers to process many calculations at once, potentially solving certain complex problems much faster than traditional computers. They're particularly promising for tasks like drug discovery, cryptography, and optimizing complex systems."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 27,
    "completion_tokens": 85,
    "total_tokens": 112
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-8q9ABC123","object":"chat.completion.chunk","created":1705334400,"model":"gpt-3.5-turbo-0125","system_fingerprint":"fp_abc123","choices":[{"index":0,"delta":{"role":"assistant","content":"Quantum"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-8q9ABC123","object":"chat.completion.chunk","created":1705334400,"model":"gpt-3.5-turbo-0125","system_fingerprint":"fp_abc123","choices":[{"index":0,"delta":{"content":" computing"},"logprobs":null,"finish_reason":null}]}

...

data: {"id":"chatcmpl-8q9ABC123","object":"chat.completion.chunk","created":1705334400,"model":"gpt-3.5-turbo-0125","system_fingerprint":"fp_abc123","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

data: [DONE]
```

### Advanced Features

#### Function Calling

```bash
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in London?"
      }
    ],
    "functions": [
      {
        "name": "get_weather",
        "description": "Get the current weather",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city name"
            }
          },
          "required": ["location"]
        }
      }
    ],
    "function_call": "auto"
  }'
```

#### JSON Mode

```bash
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "List 3 programming languages with their year of creation"
      }
    ],
    "response_format": { "type": "json_object" }
  }'
```

---

## POST /olla/openai/v1/completions

Legacy text completion endpoint.

### Request

```bash
curl -X POST http://localhost:40114/olla/openai/v1/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo-instruct",
    "prompt": "Write a haiku about programming:",
    "max_tokens": 50,
    "temperature": 0.9
  }'
```

### Response

```json
{
  "id": "cmpl-8q9XYZ789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "gpt-3.5-turbo-instruct",
  "choices": [
    {
      "text": "\n\nCode flows like water\nLogic builds the foundation\nBugs hide in shadows",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 7,
    "completion_tokens": 17,
    "total_tokens": 24
  }
}
```

---

## POST /olla/openai/v1/embeddings

Generate embeddings for text input.

### Request

```bash
curl -X POST http://localhost:40114/olla/openai/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "text-embedding-3-small",
    "input": "The quick brown fox jumps over the lazy dog",
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
      "embedding": [0.0234, -0.0156, 0.0891, ...]
    }
  ],
  "model": "text-embedding-3-small",
  "usage": {
    "prompt_tokens": 9,
    "total_tokens": 9
  }
}
```

---

## POST /olla/openai/v1/images/generations

Generate images using DALL-E (if configured).

### Request

```bash
curl -X POST http://localhost:40114/olla/openai/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "dall-e-3",
    "prompt": "A serene landscape with mountains and a lake at sunset",
    "n": 1,
    "size": "1024x1024",
    "quality": "standard"
  }'
```

### Response

```json
{
  "created": 1705334400,
  "data": [
    {
      "url": "https://...",
      "revised_prompt": "A tranquil scene featuring majestic mountains reflected in a calm lake during a vibrant sunset, with warm orange and pink hues painting the sky"
    }
  ]
}
```

## Request Parameters

### Common Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `model` | string | required | Model ID to use |
| `temperature` | float | 1.0 | Sampling temperature (0.0-2.0) |
| `top_p` | float | 1.0 | Nucleus sampling |
| `n` | integer | 1 | Number of completions |
| `stream` | boolean | false | Stream response |
| `stop` | string/array | null | Stop sequences |
| `max_tokens` | integer | inf | Maximum tokens to generate |
| `presence_penalty` | float | 0 | Penalize new tokens (-2.0 to 2.0) |
| `frequency_penalty` | float | 0 | Penalize repeated tokens (-2.0 to 2.0) |
| `logit_bias` | object | null | Token bias adjustments |
| `user` | string | null | End-user identifier |

### Chat-Specific Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `messages` | array | Conversation messages |
| `functions` | array | Available functions for calling |
| `function_call` | string/object | Function calling behavior |
| `response_format` | object | Output format (text/json_object) |
| `seed` | integer | Reproducibility seed |
| `tools` | array | Available tools (GPT-4) |
| `tool_choice` | string/object | Tool selection behavior |

## Authentication

The Authorization header is forwarded to the backend. Configure API keys in your endpoints:

```yaml
endpoints:
  - url: "https://api.openai.com"
    name: "openai-production"
    type: "openai"
    headers:
      Authorization: "Bearer ${OPENAI_API_KEY}"
```

## Rate Limits

OpenAI rate limits are enforced by the backend service. Olla adds its own configurable limits:

- Default: 200 requests per minute per endpoint
- Configurable per-endpoint limits
- Burst handling for traffic spikes

## Error Handling

OpenAI errors are forwarded with additional context:

```json
{
  "error": {
    "message": "Invalid API key provided",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_api_key"
  },
  "olla_context": {
    "endpoint": "openai-production",
    "request_id": "req_abc123",
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

## Response Headers

All responses include standard Olla headers:

- `X-Olla-Endpoint` - Backend endpoint name
- `X-Olla-Model` - Model used
- `X-Olla-Backend-Type` - Always "openai" for these endpoints
- `X-Olla-Response-Time` - Total processing time
- `X-Olla-Request-ID` - Request tracking ID