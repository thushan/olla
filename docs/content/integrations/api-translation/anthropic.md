---
title: Anthropic API Translation
description: Configure Olla to translate Anthropic Messages API requests to OpenAI format, enabling Claude Code, OpenCode, and Crush CLI to work with local Ollama, LM Studio, vLLM, and other OpenAI-compatible backends.
keywords: Anthropic, Claude, API translation, Claude Code, OpenCode, Crush CLI, Ollama, LM Studio, vLLM, local LLM
---

# Anthropic API Translation

Olla's Anthropic API Translation enables Claude-compatible clients (Claude Code, OpenCode, Crush CLI) to work with any OpenAI-compatible backend through automatic format translation.

## Overview

<table>
    <tr>
        <th>Feature</th>
        <td>Anthropic Messages API Translation</td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>API Format Translation</td>
    </tr>
    <tr>
        <th>Endpoint</th>
        <td><code>/olla/anthropic/v1/*</code></td>
    </tr>
    <tr>
        <th>What Gets Translated</th>
        <td>
            <ul>
                <li>Request format (Anthropic → OpenAI)</li>
                <li>Response format (OpenAI → Anthropic)</li>
                <li>Streaming events (SSE format conversion)</li>
                <li>Tool definitions (function calling)</li>
                <li>Vision content (multi-modal images)</li>
                <li>Error responses (Anthropic error schema)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Supported Clients</th>
        <td>
            <ul>
                <li><a href="../../frontend/claude-code">Claude Code</a></li>
                <li><a href="../../frontend/opencode">OpenCode</a></li>
                <li><a href="../../frontend/crush-cli">Crush CLI</a></li>
                <li>Any Anthropic API client</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Compatible Backends</th>
        <td>
            All OpenAI-compatible backends:
            <ul>
                <li>Ollama</li>
                <li>LM Studio</li>
                <li>vLLM</li>
                <li>SGLang</li>
                <li>llama.cpp</li>
                <li>Lemonade SDK</li>
                <li>LiteLLM</li>
                <li>Any OpenAI-compatible endpoint</li>
            </ul>
        </td>
    </tr>
</table>

## Architecture

Olla supports two modes for handling Anthropic API requests, selected automatically based on backend capabilities:

### Passthrough Mode (When Available)

```
┌──────────────────┐       ┌────────── Olla ─────────────┐       ┌─────────────────┐
│  Claude Code     │       │ /olla/anthropic/v1/*        │       │ vLLM (v0.11.1+) │
│  OpenCode        │──────▶│                             │──────▶│ llama.cpp (b4847+)
│  Crush CLI       │       │ 1. Validate request         │       │ LM Studio (v0.4.1+)
│                  │       │ 2. Detect native support    │       │ Ollama (v0.14.0+)│
│  (Anthropic API) │       │ 3. Forward as-is            │       │                 │
│                  │◀──────│                             │◀──────│ (Native Anthropic)
└──────────────────┘       └─────────────────────────────┘       └─────────────────┘
```

**Passthrough Flow** (zero translation overhead):

1. Client sends Anthropic Messages API request
2. Olla detects backend has native Anthropic support (via profile config)
3. Request forwarded directly to backend without any format conversion
4. Backend processes request natively
5. Response returned to client as-is
6. Response includes `X-Olla-Mode: passthrough` header

### Translation Mode (Fallback)

```
┌──────────────────┐       ┌────────── Olla ─────────────┐       ┌─────────────────┐
│  Claude Code     │       │ /olla/anthropic/v1/*        │       │ SGLang          │
│  OpenCode        │──────▶│                             │──────▶│ LiteLLM         │
│  Crush CLI       │       │ 1. Validate request         │       │ OpenAI-compatible│
│                  │       │ 2. Translate to OpenAI      │       │                 │
│  (Anthropic API) │       │ 3. Route to backend         │       │ (OpenAI API)    │
│                  │◀──────│ 4. Translate response       │◀──────│                 │
│                  │       │ 5. Return Anthropic format  │       │                 │
└──────────────────┘       └─────────────────────────────┘       └─────────────────┘
```

**Translation Flow** (automatic fallback):

1. Client sends Anthropic Messages API request
2. Olla detects no backends with native Anthropic support
3. Olla translates to OpenAI Chat Completions format
4. Standard Olla routing (load balancing, health checks, failover)
5. Backend processes request (unaware of Anthropic origin)
6. Olla translates OpenAI response back to Anthropic format
7. Client receives Anthropic-formatted response

## What Gets Translated

### Request Translation (Anthropic → OpenAI)

**Anthropic Messages Request**:
```json
{
  "model": "llama3.2:latest",
  "max_tokens": 1024,
  "system": "You are a helpful assistant.",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "temperature": 0.7,
  "stream": true
}
```

**Translated to OpenAI**:
```json
{
  "model": "llama3.2:latest",
  "max_tokens": 1024,
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "temperature": 0.7,
  "stream": true
}
```

**Key Transformations**:
- `system` parameter → first message with `role: "system"`
- Content blocks → simple string content (or multi-part for vision)
- Tool definitions → OpenAI function definitions
- Stop sequences → OpenAI stop parameter

### Response Translation (OpenAI → Anthropic)

**OpenAI Response**:
```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "model": "llama3.2:latest",
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help you?"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 10,
    "total_tokens": 25
  }
}
```

**Translated to Anthropic**:
```json
{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Hello! How can I help you?"
    }
  ],
  "model": "llama3.2:latest",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 15,
    "output_tokens": 10
  }
}
```

**Key Transformations**:
- Message structure → Anthropic message format
- Content wrapping → content blocks array
- Finish reason mapping → stop_reason
- Token usage → input_tokens/output_tokens
- ID generation → Anthropic-compatible ID

### Streaming Translation

**OpenAI SSE** (from backend):
```
data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"!"}}]}

data: [DONE]
```

**Translated to Anthropic SSE** (to client):
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","role":"assistant"}}

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

## Configuration

### Translation Configuration

Anthropic translation is enabled by default. Edit your `config.yaml` to customise:

```yaml
# Anthropic Messages API Translation (v0.0.20+)
# Enabled by default. Still actively being improved -- please report any issues or feedback.
translators:
  anthropic:
    enabled: true                   # Enabled by default
    max_message_size: 10485760     # Max request size (10MB)
    passthrough_enabled: true       # Enable passthrough for backends with native Anthropic support

# Standard Olla configuration
discovery:
  type: static
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable Anthropic API translation (enabled by default) |
| `max_message_size` | integer | `10485760` | Maximum request size in bytes (10MB default) |
| `passthrough_enabled` | boolean | `true` | Passthrough optimisation mode. When `true` (default), requests are forwarded directly to backends that declare `anthropic_support` in their profile, with zero translation overhead. When `false`, all requests use translation regardless of backend capabilities. Only applies when `enabled: true`. |

### Multiple Backends

Configure multiple backends for load balancing and failover:

```yaml
translators:
  anthropic:
    enabled: true
    max_message_size: 10485760

discovery:
  type: static
  static:
    endpoints:
      # Local Ollama (highest priority)
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s

      # LM Studio (medium priority)
      - url: "http://localhost:1234"
        name: "local-lmstudio"
        type: "lm-studio"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s

      # Remote vLLM (low priority, fallback)
      - url: "http://192.168.1.100:8000"
        name: "remote-vllm"
        type: "vllm"
        priority: 50
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s

proxy:
  load_balancer: priority  # Use priority-based routing
  engine: sherpa          # or: olla (for high performance)
  profile: streaming      # Low-latency streaming
```

## Quick Start

### 1. Configure Endpoints

Anthropic translation is enabled by default. Create or edit `config.yaml` to configure your backends:

```yaml
discovery:
  type: static
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
```

### 2. Start Olla

```bash
olla --config config.yaml
```

### 3. Verify Endpoints

Check health:
```bash
curl http://localhost:40114/internal/health
```

List available models:
```bash
curl http://localhost:40114/olla/anthropic/v1/models | jq
```

### 4. Test Translation

Basic chat completion:
```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "Say hello in one sentence"
      }
    ]
  }' | jq
```

Streaming test:
```bash
curl -N -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 50,
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": "Count from 1 to 3"
      }
    ]
  }'
```

## Usage Examples

### Basic Chat Completion

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "Explain quantum computing in one sentence."
      }
    ]
  }'
```

### System Prompt

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 1024,
    "system": "You are a helpful coding assistant. Respond with concise, accurate code examples.",
    "messages": [
      {
        "role": "user",
        "content": "Write a Python function to calculate factorial"
      }
    ]
  }'
```

### Streaming Response

```bash
curl -N -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 100,
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": "Write a short poem about code"
      }
    ]
  }'
```

### Multi-Turn Conversation

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "What is the capital of France?"
      },
      {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      {
        "role": "user",
        "content": "What is its population?"
      }
    ]
  }'
```

### Vision (Multi-Modal)

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2-vision:latest",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What is in this image?"
          },
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/jpeg",
              "data": "/9j/4AAQSkZJRg..."
            }
          }
        ]
      }
    ]
  }'
```

**Note**: Vision support depends on backend capabilities. Not all local models support vision. Recommended vision models:
- `llama3.2-vision:latest` (Ollama)
- `llava:latest` (Ollama)
- Anthropic-compatible vision models via LiteLLM

## Supported Backends

### Passthrough-Compatible Backends (Native Anthropic Support)

These backends natively support the Anthropic Messages API and benefit from passthrough mode (zero translation overhead):

| Backend | Min Version | Token Counting | Config Section |
|---------|-------------|----------------|----------------|
| **vLLM** | v0.11.1+ | No | `api.anthropic_support` in `config/profiles/vllm.yaml` |
| **llama.cpp** | b4847+ | Yes | `api.anthropic_support` in `config/profiles/llamacpp.yaml` |
| **LM Studio** | v0.4.1+ | No | `api.anthropic_support` in `config/profiles/lmstudio.yaml` |
| **Ollama** | v0.14.0+ | No | `api.anthropic_support` in `config/profiles/ollama.yaml` |

When using these backends, Olla automatically detects native Anthropic support and forwards requests directly. You can verify passthrough mode is active by checking the `X-Olla-Mode: passthrough` response header.

### Translation-Compatible Backends (OpenAI Format)

All OpenAI-compatible backends work through translation:

### Local Backends

**Ollama**
```yaml
- url: "http://localhost:11434"
  name: "local-ollama"
  type: "ollama"
  model_url: "/api/tags"
  health_check_url: "/"
```

**LM Studio**
```yaml
- url: "http://localhost:1234"
  name: "local-lmstudio"
  type: "lm-studio"
  model_url: "/v1/models"
  health_check_url: "/"
```

**llama.cpp**
```yaml
- url: "http://localhost:8080"
  name: "local-llamacpp"
  type: "llamacpp"
  model_url: "/v1/models"
  health_check_url: "/health"
```

### High-Performance Backends

**vLLM**
```yaml
- url: "http://localhost:8000"
  name: "vllm-server"
  type: "vllm"
  model_url: "/v1/models"
  health_check_url: "/health"
```

**SGLang**
```yaml
- url: "http://localhost:30000"
  name: "sglang-server"
  type: "sglang"
  model_url: "/v1/models"
  health_check_url: "/health"
```

### Gateway/Proxy Backends

**LiteLLM**
```yaml
- url: "http://localhost:4000"
  name: "litellm-gateway"
  type: "litellm"
  model_url: "/v1/models"
  health_check_url: "/health"
```

## Client Integration

### Claude Code

Configure Claude Code to use Olla:

```bash
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"
export ANTHROPIC_API_KEY="not-required"  # Optional for local use
claude
```

See [Claude Code Integration](../frontend/claude-code.md) for complete setup.

### OpenCode

Configure OpenCode in `~/.opencode/config.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla-anthropic": {
      "npm": "@ai-sdk/anthropic",
      "options": {
        "baseURL": "http://localhost:40114/olla/anthropic/v1"
      }
    }
  }
}
```

See [OpenCode Integration](../frontend/opencode.md) for complete setup.

### Crush CLI

Configure Crush in `~/.crush/config.json`:

```json
{
  "providers": {
    "olla-anthropic": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "api_key": "not-required"
    }
  }
}
```

See [Crush CLI Integration](../frontend/crush-cli.md) for complete setup.

## Feature Support

### Fully Supported

- ✅ **Basic messages** - Standard chat completions
- ✅ **Streaming** - Server-Sent Events (SSE) format
- ✅ **System messages** - Translated to first message
- ✅ **Multi-turn conversations** - Full conversation history
- ✅ **Tool use** - Function calling translation
- ✅ **Temperature, top_p, top_k** - Sampling parameters
- ✅ **Stop sequences** - Custom stop conditions
- ✅ **Token usage** - Input/output token reporting

### Partially Supported

- ⚠️ **Vision (multi-modal)** - Depends on backend model capabilities
  - Works with: `llama3.2-vision`, `llava`, vision-capable models
  - Not all backends support vision
  - Image encoding must be base64

- ⚠️ **Token counting** - Estimated from backend usage
  - Pre-request token counting not available
  - Usage reported from backend's token counts
  - May differ from Anthropic's tokeniser

### Not Supported

- ❌ **Extended Thinking** - Advanced reasoning mode
- ❌ **Prompt Caching** - Response caching
- ❌ **Batches API** - Batch processing
- ❌ **Usage Tracking** - Account-level usage tracking
- ❌ **Anthropic-Version Header** - Accepted but not enforced

## Limitations

### Backend Dependency

Translation quality depends on backend capabilities:

- **Tool Use**: Not all local models support function calling
  - ✅ Works: Llama 3.2 3B+, Qwen 2.5 Coder, GPT-4 (via LiteLLM)
  - ❌ Limited: Smaller models, older architectures

- **Vision**: Multi-modal support varies
  - ✅ Works: llama3.2-vision, llava, GPT-4V (via LiteLLM)
  - ❌ Not supported: Most text-only models

- **Streaming**: Most backends support streaming
  - ✅ Works: Ollama, LM Studio, vLLM, llama.cpp
  - ⚠️ Buffered: Some older implementations

### Translation Limitations

- **Token Counting**: Cannot pre-count tokens before request
  - Anthropic's `count_tokens` endpoint not implemented
  - Use backend's reported token counts after generation

- **Error Messages**: Backend errors translated to Anthropic format
  - Original error context may be lost
  - Check `X-Olla-*` headers for backend details

- **Model Capabilities**: Translation doesn't add features
  - Backend must support the underlying capability
  - Translation only converts the API format

## Troubleshooting

### Translation Not Working

**Symptom**: 404 errors on `/olla/anthropic/v1/*` endpoints

**Solutions**:
1. Check translation is enabled (it is by default, but may have been disabled):
   ```bash
   grep -A 3 "translators:" config.yaml
   ```
   Should show `enabled: true` (or not be present, as it defaults to `true`)

2. Verify Olla is running:
   ```bash
   curl http://localhost:40114/internal/health
   ```

3. Check logs for startup errors:
   ```bash
   docker logs olla | grep -i anthropic
   ```

### Models Not Listed

**Symptom**: `/olla/anthropic/v1/models` returns empty or error

**Solutions**:
1. Check backend endpoints are healthy:
   ```bash
   curl http://localhost:40114/internal/status/endpoints | jq
   ```

2. Verify model discovery is working:
   ```bash
   curl http://localhost:40114/olla/models | jq
   ```

3. Check backend is accessible:
   ```bash
   # For Ollama
   curl http://localhost:11434/api/tags

   # For LM Studio
   curl http://localhost:1234/v1/models
   ```

### Request Fails with "Model Not Found"

**Symptom**:
```json
{
  "type": "error",
  "error": {
    "type": "not_found_error",
    "message": "model 'llama3.2:latest' not found"
  }
}
```

**Solutions**:
1. List available models:
   ```bash
   curl http://localhost:40114/olla/anthropic/v1/models | jq '.data[].id'
   ```

2. Pull model if using Ollama:
   ```bash
   ollama pull llama3.2:latest
   ```

3. Check model name matches exactly:
   - Use the `id` field from `/models` response
   - Model names are case-sensitive

### Streaming Not Working

**Symptom**: No streaming events received, or timeout

**Solutions**:
1. Ensure `stream: true` in request:
   ```json
   {
     "model": "llama3.2:latest",
     "max_tokens": 100,
     "stream": true,
     "messages": [...]
   }
   ```

2. Use `-N` flag with curl:
   ```bash
   curl -N http://localhost:40114/olla/anthropic/v1/messages ...
   ```

3. Check proxy settings (some proxies buffer SSE):
   ```bash
   # Direct connection test
   curl -N --no-buffer http://localhost:40114/...
   ```

4. Verify backend supports streaming:
   ```bash
   # Test directly with backend
   curl -N http://localhost:11434/v1/chat/completions \
     -d '{"model":"llama3.2:latest","stream":true,"messages":[...]}'
   ```

### Tool Use Not Working

**Symptom**: Model doesn't use tools or returns errors

**Solutions**:
1. Verify model supports function calling:
   - Not all local models support tool use
   - Try known tool-capable models: Llama 3.2 3B+, Qwen 2.5 Coder

2. Check tool definition format:
   ```json
   {
     "name": "get_weather",
     "description": "Get current weather",
     "input_schema": {
       "type": "object",
       "properties": {
         "location": {"type": "string"}
       },
       "required": ["location"]
     }
   }
   ```

3. Test with simpler tool:
   ```bash
   curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
     -H "Content-Type: application/json" \
     -d '{
       "model": "llama3.2:latest",
       "max_tokens": 1024,
       "tools": [{
         "name": "test_tool",
         "description": "A simple test tool",
         "input_schema": {
           "type": "object",
           "properties": {}
         }
       }],
       "messages": [
         {"role": "user", "content": "Use the test_tool"}
       ]
     }'
   ```

### Vision Not Working

**Symptom**: Image not processed or error response

**Solutions**:
1. Verify model supports vision:
   ```bash
   # Use vision-capable model
   curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
     -d '{"model":"llama3.2-vision:latest", ...}'
   ```

2. Check image encoding (must be base64):
   ```bash
   base64 -w 0 image.jpg > image.b64
   ```

3. Verify content block format:
   ```json
   {
     "type": "image",
     "source": {
       "type": "base64",
       "media_type": "image/jpeg",
       "data": "..."
     }
   }
   ```

### Passthrough Not Working

**Symptom**: Requests are being translated (no `X-Olla-Mode: passthrough` header) even though your backend supports native Anthropic format.

**Solutions**:

1. **Check `passthrough_enabled` is `true` in config**:
   ```bash
   grep -A 5 "translators:" config.yaml
   ```
   Should show:
   ```yaml
   translators:
     anthropic:
       enabled: true
       passthrough_enabled: true
   ```

2. **Check backend profile has `anthropic_support.enabled: true`**:
   Each backend type declares native Anthropic support in its profile YAML under `api.anthropic_support.enabled`. Verify the profile for your backend type (e.g., `config/profiles/vllm.yaml`, `config/profiles/ollama.yaml`) has:
   ```yaml
   api:
     anthropic_support:
       enabled: true
   ```

3. **Both conditions must be true** for passthrough to activate:
   - `translators.anthropic.passthrough_enabled: true` (global config)
   - Backend profile `api.anthropic_support.enabled: true` (per-backend)

   If either is `false`, Olla falls back to translation mode.

4. **Check all endpoints support passthrough**:
   Passthrough mode requires that all healthy endpoints' profiles declare `anthropic_support.enabled: true`. If any endpoint lacks native support, Olla falls back to translation for consistency.

5. **Enable debug logging** to see mode selection:
   ```yaml
   logging:
     level: "debug"
   ```
   Look for log entries indicating passthrough or translation mode selection.

### Forcing Translation Mode

**Scenario**: You want to force translation mode for debugging or testing, even though backends support native Anthropic format.

**Solution**: Set `passthrough_enabled: false`:
```yaml
translators:
  anthropic:
    enabled: true
    passthrough_enabled: false    # Force translation mode for all requests
```

Alternatively, to disable passthrough for a specific backend type only, set `anthropic_support.enabled: false` in that backend's profile:
```yaml
# config/profiles/vllm.yaml (custom override)
api:
  anthropic_support:
    enabled: false    # Force translation for vLLM backends only
```

### Detecting Which Mode Was Used

**Scenario**: You need to know whether a request used passthrough or translation mode.

**Solution**: Check the `X-Olla-Mode` response header:

```bash
curl -s -D - http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:latest","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}' \
  2>&1 | grep -i "x-olla-mode"
```

- **`X-Olla-Mode: passthrough`** -- request was forwarded directly to the backend
- **Header absent** -- request went through the Anthropic-to-OpenAI translation pipeline

### High Latency

**Symptom**: Slow response times compared to direct backend access

**Solutions**:
1. Switch to high-performance proxy engine:
   ```yaml
   proxy:
     engine: olla  # Not sherpa
     profile: streaming
   ```

2. Use local backends:
   ```yaml
   discovery:
     static:
       endpoints:
         - url: "http://localhost:11434"  # Local, not remote
           priority: 100
   ```

3. Check backend performance:
   ```bash
   # Test backend directly
   time curl http://localhost:11434/v1/chat/completions -d '...'
   ```

4. Review Olla logs for slow operations:
   ```bash
   docker logs olla | grep -i "response_time"
   ```

## Backend Profile Configuration (Passthrough)

Native Anthropic support is declared in each backend's profile YAML file under the `api.anthropic_support` section. This is how Olla knows which backends can receive Anthropic requests directly.

### Example Profile Configuration

```yaml
# config/profiles/vllm.yaml (excerpt)
api:
  anthropic_support:
    enabled: true              # Enable native Anthropic support
    messages_path: /v1/messages # Backend path for Messages API
    token_count: false          # Token counting not supported
    min_version: "0.11.1"      # Minimum vLLM version required
    limitations:
      - no_token_counting
```

### Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | boolean | Yes | Whether the backend supports native Anthropic format |
| `messages_path` | string | Yes | Backend path for the Anthropic Messages API |
| `token_count` | boolean | No | Whether `/v1/messages/count_tokens` is supported |
| `min_version` | string | No | Minimum backend version with Anthropic support |
| `limitations` | list | No | Known limitations (e.g., `no_token_counting`) |

### Disabling Passthrough

To force translation mode for a specific backend type, create a custom profile override:

```yaml
# config/profiles/vllm.yaml (custom override to disable passthrough)
name: vllm
api:
  anthropic_support:
    enabled: false
```

## Performance Notes

### Translation Overhead

Typical overhead per request:

- **Passthrough mode**: Near-zero overhead (no translation)
- **Request translation**: ~0.5-2ms
- **Response translation**: ~1-5ms
- **Streaming**: ~0.1-0.5ms per chunk

**Total overhead**: Usually <5ms for translation mode, negligible compared to model inference time. Passthrough mode eliminates translation overhead entirely.

### Memory Usage

- **Non-streaming**: ~1-5KB per request (minimal)
- **Streaming**: Constant memory (uses io.Pipe, no buffering)
- **Vision**: Proportional to image size (~100KB-5MB)

### Optimisation Tips

1. **Use Olla Proxy Engine** (lower overhead):
   ```yaml
   proxy:
     engine: olla
     profile: streaming
   ```

2. **Enable Connection Pooling** (reduces handshake overhead):
   ```yaml
   proxy:
     connection_timeout: 60s
     response_timeout: 900s
   ```

3. **Tune Message Size Limits**:
   ```yaml
   translators:
     anthropic:
       max_message_size: 52428800  # 50MB for large requests
   ```

4. **Use Priority Load Balancing** (prefer fast local endpoints):
   ```yaml
   proxy:
     load_balancer: priority

   discovery:
     static:
       endpoints:
         - url: "http://localhost:11434"
           priority: 100  # Highest = local Ollama
         - url: "http://remote-server:8000"
           priority: 50   # Lower = remote fallback
   ```

## Related Documentation

- **[Anthropic Messages API Reference](../../api-reference/anthropic.md)** - Complete API documentation
- **[API Translation Concept](../../concepts/api-translation.md)** - How translation and passthrough work
- **[Profile System](../../concepts/profile-system.md)** - Backend profile configuration (including `anthropic_support`)
- **[Monitoring](../../configuration/practices/monitoring.md)** - Translator metrics and observability
- **[Claude Code Integration](../frontend/claude-code.md)** - Set up Claude Code
- **[OpenCode Integration](../frontend/opencode.md)** - Set up OpenCode
- **[Crush CLI Integration](../frontend/crush-cli.md)** - Set up Crush CLI
- **[Model Routing](../../concepts/model-routing.md)** - How models are selected
- **[Load Balancing](../../concepts/load-balancing.md)** - Request distribution
- **[Health Checking](../../concepts/health-checking.md)** - Endpoint monitoring
