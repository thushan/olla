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

## API Sections

### [System Endpoints](system.md)
Internal endpoints for health monitoring and system status.

- `/internal/health` - Health check endpoint
- `/internal/status` - System status and statistics
- `/internal/process` - Process information

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

### [vLLM API](vllm.md)
Proxy endpoints for vLLM servers.

- `/olla/vllm/*` - vLLM API endpoints

## Authentication

Currently, Olla does not implement authentication at the proxy level. Authentication should be handled by:
- Backend services (Ollama, LM Studio, etc.)
- Network-level security (firewalls, VPNs)
- Reverse proxy authentication (nginx, Traefik)

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
| `X-Olla-Backend-Type` | Provider type (ollama/lmstudio/openai/vllm) |
| `X-Olla-Response-Time` | Total processing time |
| `X-Olla-Routing-Strategy` | Routing strategy used (when model routing is active) |
| `X-Olla-Routing-Decision` | Routing decision made (routed/fallback/rejected) |
| `X-Olla-Routing-Reason` | Human-readable reason for routing decision |

### Provider Metrics (Debug Logs)

When available, provider-specific performance metrics are extracted from responses and included in debug logs:

| Metric | Description | Providers |
|--------|-------------|-----------|
| `provider_total_ms` | Total processing time | Ollama, LM Studio |
| `provider_prompt_tokens` | Tokens in prompt | All |
| `provider_completion_tokens` | Tokens generated | All |
| `provider_tokens_per_second` | Generation speed | Ollama, LM Studio |
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

CORS headers are included for browser-based clients:

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization`