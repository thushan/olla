---
title: "Anthropic Messages API - Translated API Reference"
description: "Complete API reference for Olla's Anthropic Messages API translator. Use Claude Code, OpenCode, and Crush CLI with local LLM backends through transparent API translation."
keywords: ["anthropic api", "claude api", "messages api", "api translation", "claude code", "opencode", "crush cli", "anthropic compatibility"]
---

# Anthropic Messages API

Olla provides a complete Anthropic Messages API translator that enables Claude-compatible clients to work with local LLM infrastructure. This translation layer converts requests between Anthropic's API format and OpenAI's Chat Completions API, allowing tools like Claude Code to leverage your existing Ollama, LM Studio, vLLM, SGLang or other OpenAI-compatible backends supported by Olla.

## Overview

The Anthropic translator accepts requests in Anthropic Messages API format at `/olla/anthropic/v1/*` endpoints, translates them to OpenAI format, routes to available backends, and translates responses back to Anthropic format.

**Key Features**:

- ✅ Full Anthropic Messages API compatibility
- ✅ Streaming via Server-Sent Events (SSE)
- ✅ Tool use (function calling)
- ✅ Works with all OpenAI-compatible backends
- ✅ Zero backend changes required
- ⚠️ **Vision Support**: Image content blocks accepted but not yet processed
- ⛔ **Async Support**: Asynchronous workflows are not supported

**Supported Clients**:

- **[Claude Code](../integrations/frontend/claude-code.md)** - Anthropic's official CLI coding assistant
- **OpenCode** - Open-source AI coding assistant
- **Crush CLI** - Charmbracelet's AI CLI tool
- Any client using Anthropic Messages API format

## How it Works

```mermaid
sequenceDiagram
    participant Client as Claude Code
    participant Olla as Olla Translator
    participant Backend as OpenAI Backend

    Client->>Olla: POST /olla/anthropic/v1/messages<br/>(Anthropic format)

    Note over Olla: 1. Validate request
    Note over Olla: 2. Translate to OpenAI format
    Note over Olla: 3. Route to backend

    Olla->>Backend: POST /v1/chat/completions<br/>(OpenAI format)
    Backend->>Olla: Response (OpenAI format)

    Note over Olla: 4. Translate response back

    Olla->>Client: Response (Anthropic format)
```

**Translation Process**:

1. Client sends Anthropic-formatted request
2. Olla translates request to OpenAI format
3. Request routed through standard Olla pipeline (load balancing, health checks)
4. Backend processes request (unaware of original format)
5. Olla translates OpenAI response back to Anthropic format
6. Client receives Anthropic-formatted response

For detailed explanation, see [API Translation Concept](../concepts/api-translation.md).

## Endpoints Overview

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/olla/anthropic/v1/models` | List available models in Anthropic format |
| POST | `/olla/anthropic/v1/messages` | Create a message (chat completion) |
| POST | `/olla/anthropic/v1/messages/count_tokens` | Estimate token count for a message |

## POST /olla/anthropic/v1/messages

Create a message with support for streaming, tool use, and vision.

### Request

**Headers**:
```
Content-Type: application/json
anthropic-version: 2023-06-01  # Optional, for compatibility
```

**Body Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Model name (must exist on backend) |
| `messages` | array | Yes | Array of message objects |
| `max_tokens` | integer | Yes | Maximum tokens to generate |
| `system` | string | No | System prompt (translated to first message) |
| `temperature` | number | No | Sampling temperature (0.0-1.0) |
| `top_p` | number | No | Nucleus sampling parameter |
| `top_k` | integer | No | Top-k sampling parameter |
| `stop_sequences` | array | No | Stop sequences |
| `stream` | boolean | No | Enable streaming (default: false) |
| `tools` | array | No | Tool definitions for function calling |

**Message Object**:

```json
{
  "role": "user",  // or "assistant"
  "content": "Hello!"  // string or content blocks array
}
```

**Content Blocks** (for multi-modal):

```json
{
  "role": "user",
  "content": [
    {
      "type": "text",
      "text": "What's in this image?"
    },
    {
      "type": "image",
      "source": {
        "type": "base64",
        "media_type": "image/jpeg",
        "data": "base64-encoded-image-data"
      }
    }
  ]
}
```

### Example: Basic Chat

**Request**:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "Explain quantum computing in one sentence."
      }
    ]
  }'
```

**Response**:

```json
{
  "id": "msg_01ABC123def456GHI789jkl",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Quantum computing uses quantum mechanical phenomena like superposition and entanglement to perform calculations that would be infeasible for classical computers."
    }
  ],
  "model": "llama4:latest",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 15,
    "output_tokens": 28
  }
}
```

### Example: Streaming

**Request**:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -N \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 100,
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": "Count from 1 to 5."
      }
    ]
  }'
```

**Response** (Server-Sent Events):

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"llama4:latest","usage":{"input_tokens":12,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"1"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":","}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 2"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":","}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 3"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":","}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 4"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":","}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 5"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":9}}

event: message_stop
data: {"type":"message_stop"}
```

### Example: System Messages

The `system` parameter is translated to the first message with `role: "system"`:

**Request**:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 1024,
    "system": "You are a helpful coding assistant specialised in Python.",
    "messages": [
      {
        "role": "user",
        "content": "Write a function to calculate fibonacci numbers."
      }
    ]
  }'
```

**How it's Translated** (internally to OpenAI format):

```json
{
  "model": "llama4:latest",
  "max_tokens": 1024,
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful coding assistant specialised in Python."
    },
    {
      "role": "user",
      "content": "Write a function to calculate fibonacci numbers."
    }
  ]
}
```

### Example: Tool Use (Function Calling)

**Request**:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 1024,
    "tools": [
      {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"],
              "description": "Temperature unit"
            }
          },
          "required": ["location"]
        }
      }
    ],
    "messages": [
      {
        "role": "user",
        "content": "What'\''s the weather in San Francisco?"
      }
    ]
  }'
```

**Response** (if model decides to use tool):

```json
{
  "id": "msg_tool123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "tool_use",
      "id": "toolu_01ABC",
      "name": "get_weather",
      "input": {
        "location": "San Francisco",
        "unit": "fahrenheit"
      }
    }
  ],
  "model": "llama4:latest",
  "stop_reason": "tool_use",
  "usage": {
    "input_tokens": 120,
    "output_tokens": 45
  }
}
```

**Note**: Tool use requires a model that supports function calling. Not all local models support this feature.

### Example: Vision (Image Input)

**Request**:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llava:latest",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/jpeg",
              "data": "'"$(base64 -w 0 image.jpg)"'"
            }
          },
          {
            "type": "text",
            "text": "What do you see in this image?"
          }
        ]
      }
    ]
  }'
```

**Response**:

```json
{
  "id": "msg_vision123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "I see a sunset over a mountain range with orange and purple hues in the sky."
    }
  ],
  "model": "llava:latest",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 1250,
    "output_tokens": 22
  }
}
```

**Note**: Image content blocks are accepted in the request structure but not yet processed by Olla. Vision support requires a multi-modal model like LLaVA or similar.

---

## GET /olla/anthropic/v1/models

List all available models across configured backends in Anthropic API format.

### Request

```bash
curl http://localhost:40114/olla/anthropic/v1/models
```

### Response

```json
{
  "data": [
    {
      "id": "llama4:latest",
      "name": "llama4:latest",
      "created": 1704067200,
      "description": "Chat model via Olla proxy",
      "type": "chat"
    },
    {
      "id": "qwen2.5-coder:32b",
      "name": "qwen2.5-coder:32b",
      "created": 1704067200,
      "description": "Chat model via Olla proxy",
      "type": "chat"
    },
    {
      "id": "mistral-nemo:latest",
      "name": "mistral-nemo:latest",
      "created": 1704067200,
      "description": "Chat model via Olla proxy",
      "type": "chat"
    }
  ]
}
```

---

## POST /olla/anthropic/v1/messages/count_tokens

Estimate token count for a message request using character-based calculation.

### How It Works

Olla estimates token count using the formula: `totalCharacters / 4`

This provides a rough approximation for:

- System prompt content
- All message text content
- Tool names and inputs
- Tool result content

### Request

**Headers**:
```
Content-Type: application/json
```

**Body**: Same format as `/messages` request (without requiring actual model execution)

### Example

**Request**:
```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages/count_tokens \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello world"}
    ]
  }'
```

**Response**:
```json
{
  "input_tokens": 2,
  "output_tokens": 0,
  "total_tokens": 2
}
```

**Note**: This is a character-based estimation (chars/4), not actual tokenisation. Actual token counts may vary by model.

---


## Response Format

### Non-Streaming Response

**Structure**:

```json
{
  "id": "msg_01ABC123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Response content here"
    }
  ],
  "model": "llama4:latest",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 15,
    "output_tokens": 28
  }
}
```

**Fields**:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique message identifier |
| `type` | string | Always "message" |
| `role` | string | Always "assistant" |
| `content` | array | Array of content blocks |
| `model` | string | Model that generated response |
| `stop_reason` | string | Why generation stopped |
| `stop_sequence` | string | Stop sequence that triggered stop (if any) |
| `usage` | object | Token usage statistics |

**Stop Reasons**:

- `end_turn` - Natural completion
- `max_tokens` - Reached max_tokens limit
- `stop_sequence` - Hit a stop sequence
- `tool_use` - Model wants to use a tool

### Streaming Response

Streaming uses Server-Sent Events (SSE) with typed events.

**SSE Event Types** (6 total):

| Event | Description | Example Data |
|-------|-------------|--------------|
| `message_start` | Initial message metadata | `{"type":"message_start","message":{...}}` |
| `content_block_start` | Start of text or tool_use block | `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` |
| `content_block_delta` | Text chunks (`text_delta`) or tool JSON chunks (`input_json_delta`) | `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"..."}}` |
| `content_block_stop` | End of content block | `{"type":"content_block_stop","index":0}` |
| `message_delta` | Stop reason and final usage statistics | `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}` |
| `message_stop` | End of stream | `{"type":"message_stop"}` |

**Full Event Sequence**:
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","role":"assistant","content":[],"model":"llama4:latest"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}
```


## Response Headers

All responses include standard Olla headers:

| Header | Description | Example |
|--------|-------------|---------|
| `X-Olla-Request-ID` | Unique request identifier | `req_abc123` |
| `X-Olla-Endpoint` | Backend endpoint name | `local-ollama` |
| `X-Olla-Model` | Actual model used | `llama4:latest` |
| `X-Olla-Backend-Type` | Backend type | `ollama` |
| `X-Olla-Response-Time` | Total processing time | `1.234s` |


## Authentication

**For Local Use**: No authentication required by default.

**For Production**: Olla does not implement authentication at the proxy level. Use:

- Reverse proxy authentication (nginx, Traefik, Caddy)
- API gateway (Kong, Tyk)
- Network-level security (VPN, firewall)

**API Key Pass-through**: If the backend requires an API key, include it:
```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Authorization: Bearer your-backend-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"...", ...}'
```


## Rate Limiting

Standard Olla rate limits apply:

| Limit Type | Default |
|------------|---------|
| Global requests/minute | 1000 |
| Per-IP requests/minute | 100 |
| Burst size | 50 |

Configure in `config.yaml`:

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_minute: 100
    burst: 50
```


## Error Responses

Errors follow Anthropic API format:

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "model is required"
  }
}
```

**Common Error Types**:

| HTTP Status | Error Type | Description |
|-------------|------------|-------------|
| 400 | `invalid_request_error` | Invalid request format or parameters |
| 404 | `not_found_error` | Model or endpoint not found |
| 429 | `rate_limit_error` | Rate limit exceeded |
| 500 | `api_error` | Internal server error |
| 502 | `overloaded_error` | Backend service unavailable |
| 503 | `overloaded_error` | Service temporarily unavailable |


## Differences from Official Anthropic API

### ✅ Fully Supported

- Basic message creation (text, multi-turn conversations)
- Streaming responses with all SSE event types
- System messages (string or content blocks)
- Tool use (definitions, tool_choice, tool_use, tool_result)
- Tool streaming with `input_json_delta` events
- Token counting via `/count_tokens` endpoint
- Stop sequences
- Temperature, top_p, top_k parameters
- Content blocks (text, tool_use, tool_result)

**Tool Choice Mapping**:

- `"auto"` → OpenAI `"auto"`
- `"any"` → OpenAI `"required"` (semantic equivalent)
- `"none"` → OpenAI `"none"`
- `{"type": "tool", "name": "X"}` → OpenAI `{"type": "function", "function": {"name": "X"}}`

### ⚠️ Partially Supported

- **Vision**: Image content blocks accepted in request but not processed
- **Token Counting**: Character-based estimation (chars/4), not actual model tokenisation
- **TopK**: Passed to backend but may be ignored by OpenAI-compatible backends

### ❌ Not Supported

- **Extended Thinking**: Field accepted but not processed
- **Prompt Caching**: Not implemented
- **Batches API**: Not implemented
- **Message Editing**: Not supported
- **Anthropic-specific headers**: `anthropic-version` header accepted but not enforced
- **Asynchronous Flows**: Not supported, so Claude Code can't dispatch concurrent/parallel agents to work on a task.


## Configuration

Enable Anthropic translation in `config.yaml`:

```yaml
translators:
  anthropic:
    enabled: true                   # Enable Anthropic API translator
    max_message_size: 10485760     # Max request size (10MB)

# Standard Olla configuration
discovery:
  type: static
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
```

**Configuration Options**:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable Anthropic translator |
| `max_message_size` | integer | `10485760` | Max request size in bytes (10MB) |


## Performance Considerations

**Translation Overhead**:

- Request translation: ~0.5-2ms
- Response translation: ~1-5ms
- Streaming: Minimal additional latency per chunk

**Memory Usage**:

- Minimal for basic messages (~1-5KB per request)
- Higher for vision (proportional to image size)
- Streaming uses constant memory

**Recommendations**:

- Use `proxy.engine: "olla"` for lowest latency
- Enable `profile: "streaming"` for chat applications
- Configure appropriate `max_message_size` for your use case
- Use connection pooling for high throughput


## Troubleshooting

### Model Not Found

**Error**:
```json
{
  "type": "error",
  "error": {
    "type": "not_found_error",
    "message": "model 'llama4:latest' not found"
  }
}
```

**Solutions**:

1. Check model is available: `curl http://localhost:40114/olla/models`
2. Verify backend is healthy: `curl http://localhost:40114/internal/status/endpoints`
3. Pull model if using Ollama: `ollama pull llama4:latest`

### Streaming Not Working

**Symptoms**: No streaming events received

**Solutions**:

1. Ensure `stream: true` in request
2. Use `-N` flag with curl: `curl -N ...`
3. Check proxy settings (some proxies buffer SSE)
4. Verify backend supports streaming

### Tool Use Not Working

**Symptoms**: Model doesn't use tools or returns errors

**Solutions**:

1. Verify model supports function calling (not all local models do)
2. Check tool definition format matches Anthropic spec
3. Try with a known tool-capable model (e.g., Llama 3.2 3B+)

### High Latency

**Symptoms**: Slow response times

**Solutions**:

1. Switch to `proxy.engine: "olla"` (high-performance engine)
2. Use `profile: "streaming"` for lower latency
3. Check backend performance
4. Enable connection pooling
5. Use local backends (Ollama, LM Studio) for lowest latency


## Next Steps

- **[API Translation Concept](../concepts/api-translation.md)** - Understand how translation works
- **[Claude Code Integration](../integrations/frontend/claude-code.md)** - Set up Claude Code
- **[OpenCode Integration](../integrations/frontend/opencode.md)** - Set up OpenCode
- **[Crush CLI Integration](../integrations/frontend/crush-cli.md)** - Set up Crush CLI
- **[Anthropic Translation Setup](../integrations/api-translation/anthropic.md)** - Complete configuration guide


## Related Documentation

- [OpenAI API Reference](openai.md) - OpenAI endpoint documentation
- [Model Routing](../concepts/model-routing.md) - How models are selected
- [Load Balancing](../concepts/load-balancing.md) - Request distribution
- [Health Checking](../concepts/health-checking.md) - Endpoint monitoring
