# API Reference

Olla provides a comprehensive API for interacting with your AI infrastructure. All endpoints are designed to be compatible with existing AI tools while adding powerful routing and management capabilities.

## Core Endpoints

### Model Management
- **`/olla/models`** - [Unified model listing](query-formats.md) with advanced filtering
- **`/olla/models/{id}`** - Get specific model details by ID or alias

### Provider-Specific Routing
Olla uses provider-specific namespaces instead of a generic proxy endpoint. See **[Provider-Specific Routing](provider-routing.md)** for details.

#### Proxy Endpoints (by Provider)
- **`/olla/ollama/*`** - Proxy to Ollama backends only
- **`/olla/lmstudio/*`** - Proxy to LM Studio backends only
- **`/olla/openai/*`** - Proxy to OpenAI-compatible backends only
- **`/olla/vllm/*`** - Proxy to vLLM backends only

#### Model Listing Endpoints
- **`/olla/ollama/api/tags`** - Ollama models (native format)
- **`/olla/ollama/v1/models`** - Ollama models (OpenAI format)
- **`/olla/lmstudio/v1/models`** - LM Studio models (OpenAI format)
- **`/olla/lmstudio/api/v1/models`** - LM Studio models (OpenAI format, alt path)
- **`/olla/lmstudio/api/v0/models`** - LM Studio models (enhanced format)
- **`/olla/openai/v1/models`** - OpenAI-compatible models
- **`/olla/vllm/v1/models`** - vLLM models

## Internal Endpoints

### Health & Monitoring
- **`/internal/health`** - Health check endpoint
- **`/internal/status`** - System status overview
- **`/internal/status/endpoints`** - Detailed endpoint status
- **`/internal/status/models`** - Model availability status
- **`/internal/stats/models`** - Model usage statistics
- **`/internal/process`** - Process and performance metrics

### System Information
- **`/version`** - Olla version information

## Features

### Model Unification
Olla automatically unifies models across different endpoints, recognizing when the same model is available on multiple backends. This enables intelligent routing based on:
- Model availability
- Endpoint health
- Load balancing preferences
- Model capabilities (vision, embeddings, etc.)

### Query Formats
The `/olla/models` endpoint supports multiple output formats:
- `unified` (default) - Olla's comprehensive format
- `openai` - OpenAI-compatible format
- `ollama` - Ollama-compatible format
- `lmstudio` - LM Studio-compatible format

See [Query Formats](query-formats.md) for detailed examples.

### Response Headers
All proxied requests include informative headers:
- `X-Olla-Endpoint` - Which backend handled the request
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Type of backend (ollama/openai/lmstudio)
- `X-Olla-Request-ID` - Unique request identifier
- `X-Olla-Response-Time` - Total processing time

## Authentication

Olla itself does not require authentication. However, if your backend endpoints require API keys, include them in your requests and Olla will forward them appropriately.

## Rate Limiting

Olla includes configurable rate limiting to protect your infrastructure:
- Global rate limits
- Per-IP rate limits
- Per-endpoint rate limits
- Separate limits for health check endpoints

See the configuration documentation for details on setting up rate limits.

## Error Responses

Olla returns standard HTTP status codes:
- `200` - Success
- `400` - Bad request (invalid parameters)
- `404` - Not found (model or endpoint not available)
- `429` - Too many requests (rate limited)
- `502` - Bad gateway (backend error)
- `503` - Service unavailable (no healthy endpoints)

## Next Steps

- Explore [Provider-Specific Routing](provider-routing.md) for dedicated provider endpoints
- Learn about [Query Formats](query-formats.md) for model listing options
- Check the [User Guide](../user-guide.md) for configuration examples