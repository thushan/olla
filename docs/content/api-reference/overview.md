---
title: "Olla API Reference - Complete REST API Documentation"
description: "Comprehensive API reference for Olla LLM proxy. System endpoints, unified models API, and proxy endpoints for Ollama, LM Studio, vLLM and OpenAI-compatible services."
keywords: ["olla api", "llm proxy api", "rest api", "ollama api", "lm studio api", "vllm api", "openai api", "system endpoints"]
---

# API Reference

Olla exposes several API endpoints for proxy operations, health monitoring, and system status. All endpoints follow RESTful conventions and return JSON responses unless otherwise specified.

## Base URL

```
http://localhost:40114
```

If you ever need to remember the port, think - what's the port, 4 OLLA?!

## API Sections

### [System Endpoints](system.md)
Internal endpoints for health monitoring, system status, and statistics.

- `/internal/health` - Health check endpoint
- `/internal/status` - System status and statistics
- `/internal/metrics` - Prometheus metrics (same data as `/internal/status` and `/internal/stats/models`)
- `/internal/status/endpoints` - Endpoint status details
- `/internal/status/models` - Model registry status
- `/internal/stats/models` - Model usage statistics
- `/internal/stats/translators` - Translator usage and performance statistics
- `/internal/stats/sticky` - Sticky session statistics
- `/internal/process` - Process information
- `/version` - Olla version information

### Universal Proxy
The universal entry point that routes to any backend.

- `/olla/proxy/*` - Universal proxy entry, routes to any backend
- `/olla/proxy/v1/models` - OpenAI-compatible aggregated models listing

### [Unified Models API](models.md)
Cross-provider model discovery and information.

- `/olla/models` - List all available models across providers

### [Ollama API](ollama.md)
Proxy endpoints for Ollama instances.

- `/olla/ollama/*` - All Ollama API endpoints
- OpenAI-compatible endpoints included

### [LM Studio API](lmstudio.md)
Proxy endpoints for LM Studio servers.

- `/olla/lmstudio/*` - All LM Studio API endpoints
- `/olla/lm-studio/*` - Alternative prefix
- `/olla/lm_studio/*` - Alternative prefix

### [OpenAI API](openai.md)
Proxy endpoints for OpenAI-compatible services.

- `/olla/openai/*` - OpenAI API endpoints

### [LMDeploy API](lmdeploy.md)
Proxy endpoints for LMDeploy inference servers.

- `/olla/lmdeploy/*` - LMDeploy API endpoints
- OpenAI-compatible endpoints plus passthrough for LMDeploy-specific paths (token encoding, reward pooling, generate)

### [vLLM API](vllm.md)
Proxy endpoints for vLLM servers.

- `/olla/vllm/*` - vLLM API endpoints

### [vLLM-MLX API](vllm-mlx.md)
Proxy endpoints for vLLM-MLX servers (Apple Silicon).

- `/olla/vllm-mlx/*` - vLLM-MLX API endpoints

### [Docker Model Runner API](docker-model-runner.md)
Proxy endpoints for Docker Model Runner.

- `/olla/dmr/*` - Docker Model Runner API endpoints

### [SGLang API](sglang.md)
Proxy endpoints for SGLang servers with RadixAttention and Frontend Language support.

- `/olla/sglang/*` - SGLang API endpoints
- Includes vision model support and speculative decoding

### [LiteLLM API](litellm.md)
Proxy endpoints for LiteLLM gateway (100+ providers).

- `/olla/litellm/*` - LiteLLM API endpoints

### [llama.cpp API](llamacpp.md)
Proxy endpoints for llama.cpp servers.

- `/olla/llamacpp/*` - llama.cpp API endpoints
- OpenAI-compatible endpoints plus native llamacpp features
- Includes slot monitoring, code infill, and tokenisation

### [Lemonade SDK API](lemonade.md)
Proxy endpoints for Lemonade SDK servers with AMD Ryzen AI support.

- `/olla/lemonade/*` - Lemonade SDK API endpoints
- Includes ONNX and GGUF model support with hardware acceleration

### [oMLX API](omlx.md)
Proxy endpoints for oMLX, the multi-model Apple Silicon (MLX) inference server.

- `/olla/omlx/*` - oMLX API endpoints
- OpenAI-compatible endpoints with native Anthropic Messages API passthrough

## Translated APIs

APIs that translate between different formats in real-time.

### [Anthropic Messages API](anthropic.md)

Anthropic-compatible API endpoints for Claude clients.

**Endpoints**:
- `POST /olla/anthropic/v1/messages` - Create a message (chat)
- `GET /olla/anthropic/v1/models` - List available models
- `POST /olla/anthropic/v1/messages/count_tokens` - Estimate token count

**Features**:
- Full Anthropic Messages API v1 support
- **Passthrough mode** for backends with native Anthropic support (vLLM, vLLM-MLX, llama.cpp, LM Studio, Ollama, oMLX, Docker Model Runner)
- Automatic fallback to translation mode when needed
- Streaming with Server-Sent Events
- Tool use (function calling)
- Vision support (multi-modal)
- Translator metrics for observability

**Use With**:
- Claude Code
- OpenCode
- Crush CLI
- Any Anthropic API client

See [API Translation](../concepts/api-translation.md) for how passthrough and translation modes work.

## Authentication

Currently, Olla does not implement authentication at the proxy level. Authentication should be handled by:

- Backend services (Ollama, LM Studio, etc.)
- Network-level security (firewalls, VPNs)
- Reverse proxy authentication (nginx, Traefik)

Authentication headers (e.g. `x-api-key`, `Authorization`) are forwarded to backends without modification.

## Rate Limiting

Global and per-IP rate limits are enforced:

| Limit Type | Default Value |
|------------|---------------|
| Global requests/minute | 1000 |
| Per-IP requests/minute | 100 |
| Health endpoint requests/minute | 1000 |
| Burst size | 50 |

## Request Headers

### Required Headers
- `Content-Type: application/json` for POST requests

### Optional Headers
- `X-Request-ID` - Custom request ID for tracing

## Response Headers

All responses include:

| Header | Description |
|--------|-------------|
| `X-Olla-Request-ID` | Unique request identifier |
| `X-Olla-Endpoint` | Backend endpoint name |
| `X-Olla-Model` | Model used (if applicable) |
| `X-Olla-Backend-Type` | Provider type, examples: <br> `ollama/lm-studio/llamacpp/litellm/openai/openai-compatible/vllm/vllm-mlx/sglang/lemonade/lmdeploy/omlx/docker-model-runner` |
| `X-Olla-Response-Time` | Total processing time |
| `X-Olla-Routing-Strategy` | Routing strategy used (when model routing is active) |
| `X-Olla-Routing-Decision` | Routing decision made (routed/fallback/rejected) |
| `X-Olla-Routing-Reason` | Human-readable reason for routing decision |
| `X-Olla-Mode` | Translator mode (`passthrough` when native format used; absent for translation mode) |
| `X-Olla-Sticky-Session` | Sticky session status (hit/miss/repin/disabled) |
| `X-Olla-Sticky-Key-Source` | Key source used (session_header/prefix_hash/auth_header/ip/none) |
| `X-Olla-Session-ID` | Echoed session ID when client supplies one |

### Provider Metrics (Debug Logs)

When available, provider-specific performance metrics are extracted from responses and included in debug logs:

| Metric | Description | Providers |
|--------|-------------|-----------|
| `provider_total_ms` | Total processing time (ms) | Ollama, LM Studio |
| `provider_prompt_tokens` | Tokens in prompt (count) | All |
| `provider_completion_tokens` | Tokens generated (count) | All |
| `provider_tokens_per_second` | Generation speed (tokens/s) | Ollama, LM Studio |
| `provider_model` | Actual model used | All |

See [Provider Metrics](../concepts/provider-metrics.md) for detailed information.

## Error Responses

Standard HTTP status codes are used:

| Status Code | Description |
|-------------|-------------|
| 200 | Success |
| 400 | Bad Request |
| 404 | Not Found |
| 429 | Rate Limit Exceeded |
| 500 | Internal Server Error |
| 502 | Bad Gateway |
| 503 | Service Unavailable |

### Error Response Format

```json
{
  "error": {
    "message": "Error description",
    "type": "error_type",
    "code": "ERROR_CODE"
  }
}
```

## Streaming Responses

For streaming endpoints (chat completions, text generation), responses use:

- `Content-Type: text/event-stream` for SSE streams
- `Transfer-Encoding: chunked` for HTTP streaming
- Line-delimited JSON for data chunks

## CORS Support

CORS is **disabled by default**. Most clients (CLI tools, SDKs, coding agents, and server-side apps such as OpenWebUI's backend) send no `Origin` header, so CORS does not apply to them.

Enable it only when a browser connects directly to Olla, for example a custom web dashboard or a UI configured for browser-direct connections. Once enabled, Olla answers preflight requests automatically and, by default, exposes the full `X-Olla-*` response header set so browser JavaScript can read routing and model metadata.

The allowed origins, methods, headers, exposed headers, credentials, and preflight cache are all configurable. See [Security Best Practices - CORS](../configuration/practices/security.md#cors) and the [Configuration Reference](../configuration/reference.md#cors) for details.