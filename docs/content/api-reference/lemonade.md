# Lemonade SDK API (via Proxy)

Olla proxies requests to Lemonade SDK backends under the `/olla/lemonade/` prefix. All requests are forwarded to the configured Lemonade backend with minimal modification.

**What this document covers:**

- How Olla routes requests to Lemonade SDK
- The ONE endpoint Olla handles specially (`/v1/models`)
- Standard Olla response headers
- Brief reference to Lemonade SDK endpoints

**What this document does NOT cover:**

- Detailed Lemonade SDK API specifications (see [Lemonade SDK Documentation](https://lemonade-server.ai/docs/))
- Backend-only features like model lifecycle management
- Hardware detection and system information

---

## Proxy Routing

Olla forwards all requests under `/olla/lemonade/` to the configured Lemonade SDK backend:

```
/olla/lemonade/** → http://backend:8000/**
```

**Examples:**

- `/olla/lemonade/api/v1/chat/completions` → `http://backend:8000/api/v1/chat/completions`
- `/olla/lemonade/api/v1/health` → `http://backend:8000/api/v1/health`
- `/olla/lemonade/api/v1/system-info` → `http://backend:8000/api/v1/system-info`

Requests are forwarded **transparently** with:

- Original request body preserved
- Original headers forwarded
- Olla headers added to response

---

## Model Discovery (Special Handling)

### GET /olla/lemonade/v1/models

**Special handling:** Olla intercepts this endpoint and returns models in OpenAI format from its unified catalogue.

```bash
curl http://localhost:40114/olla/lemonade/v1/models
```

**Response (OpenAI format):**
```json
{
  "object": "list",
  "data": [
    {
      "id": "Qwen2.5-0.5B-Instruct-CPU",
      "object": "model",
      "created": 1759361710,
      "owned_by": "lemonade",
      "checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
      "recipe": "oga-cpu"
    },
    {
      "id": "Phi-3.5-mini-instruct-NPU",
      "object": "model",
      "created": 1759361710,
      "owned_by": "lemonade",
      "checkpoint": "amd/Phi-3.5-mini-instruct-onnx-npu",
      "recipe": "oga-npu"
    }
  ]
}
```

**Extended Fields (Lemonade-specific):**

- `checkpoint`: HuggingFace model path or GGUF filename
- `recipe`: Inference engine (`oga-cpu`, `oga-npu`, `oga-igpu`, `llamacpp`, `flm`)

### GET /olla/lemonade/api/v1/models

**Special handling:** Same as `/v1/models` - returns OpenAI-format response.

```bash
curl http://localhost:40114/olla/lemonade/api/v1/models
```

Response format identical to `/v1/models` above.

---

## Lemonade SDK Endpoints (Proxied)

The following endpoints are **provided by Lemonade SDK**. Olla forwards requests transparently.

For detailed request/response schemas, see the [Lemonade SDK documentation](https://lemonade-server.ai/docs/).

### Health & System

| Method | Endpoint | Description | Handled By |
|--------|----------|-------------|------------|
| GET | `/olla/lemonade/api/v1/health` | Health check with loaded model info | Lemonade SDK |
| GET | `/olla/lemonade/api/v1/stats` | Runtime statistics | Lemonade SDK |
| GET | `/olla/lemonade/api/v1/system-info` | Hardware detection and capabilities | Lemonade SDK |

**Example:**

```bash
# Health check (proxied to backend)
curl http://localhost:40114/olla/lemonade/api/v1/health

# Response from Lemonade SDK
{
  "status": "ok",
  "checkpoint_loaded": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
  "model_loaded": "Qwen2.5-0.5B-Instruct-CPU"
}
```

### Inference (OpenAI-Compatible)

| Method | Endpoint | Description | Handled By |
|--------|----------|-------------|------------|
| POST | `/olla/lemonade/api/v1/chat/completions` | Chat completion | Lemonade SDK |
| POST | `/olla/lemonade/api/v1/completions` | Text completion | Lemonade SDK |
| POST | `/olla/lemonade/api/v1/embeddings` | Generate embeddings | Lemonade SDK |

**Example:**

```bash
# Chat completion (proxied to backend)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-0.5B-Instruct-CPU",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }'
```

Request format follows OpenAI's chat completion API. See [OpenAI API Reference](https://platform.openai.com/docs/api-reference/chat) for details.

### Model Lifecycle

| Method | Endpoint | Description | Handled By |
|--------|----------|-------------|------------|
| POST | `/olla/lemonade/api/v1/pull` | Download/install model from HuggingFace | Lemonade SDK |
| POST | `/olla/lemonade/api/v1/load` | Load model into memory | Lemonade SDK |
| POST | `/olla/lemonade/api/v1/unload` | Unload model from memory | Lemonade SDK |
| POST | `/olla/lemonade/api/v1/delete` | Delete model from disk | Lemonade SDK |

**Example:**
```bash
# Load model (proxied to backend)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/load \
  -H "Content-Type: application/json" \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'

# Response from Lemonade SDK
{
  "status": "success",
  "message": "Model loaded successfully",
  "model": "Phi-3.5-mini-instruct-NPU"
}
```

**Note:** Olla does NOT manage model lifecycle - these operations are handled entirely by the Lemonade SDK backend.

---

## Request Headers (Added by Olla)

Olla adds the following header to proxied requests:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Olla-Request-ID` | `req-{uuid}` | Request tracing |

**Example:**

```
POST /api/v1/chat/completions HTTP/1.1
Host: backend:8000
Content-Type: application/json
X-Olla-Request-ID: req-abc123
```

---

## Response Headers (Added by Olla)

Olla adds the following headers to all responses from Lemonade SDK:

| Header | Example | Description |
|--------|---------|-------------|
| `X-Olla-Endpoint` | `lemonade-npu` | Backend name that processed request |
| `X-Olla-Model` | `Qwen2.5-0.5B-Instruct-CPU` | Model identifier used |
| `X-Olla-Backend-Type` | `lemonade` | Provider type |
| `X-Olla-Response-Time` | `234ms` | Total processing time |
| `X-Olla-Request-ID` | `req-abc123` | Request identifier |

**Example Response:**

```
HTTP/1.1 200 OK
Content-Type: application/json
X-Olla-Endpoint: lemonade-npu
X-Olla-Model: Phi-3.5-mini-instruct-NPU
X-Olla-Backend-Type: lemonade
X-Olla-Response-Time: 156ms
X-Olla-Request-ID: req-abc123

{
  "id": "chatcmpl-xyz",
  "object": "chat.completion",
  "created": 1759361710,
  "model": "Phi-3.5-mini-instruct-NPU",
  "choices": [...]
}
```

**Use cases:**

- **Debugging**: Identify which backend processed the request
- **Monitoring**: Track response times across backends
- **Auditing**: Trace requests through logs
- **Load balancing verification**: Confirm routing behaviour

---

## Recipe and Checkpoint Metadata

Lemonade SDK uses **recipes** to identify inference engines. Olla preserves this metadata in model listings.

### Recipe Types

| Recipe | Engine | Format | Hardware |
|--------|--------|--------|----------|
| `oga-cpu` | ONNX Runtime | ONNX | CPU |
| `oga-npu` | ONNX Runtime | ONNX | AMD Ryzen AI NPU |
| `oga-igpu` | ONNX Runtime | ONNX | DirectML iGPU |
| `llamacpp` | llama.cpp | GGUF | CPU/GPU |
| `flm` | Fast Language Models | Various | CPU/GPU |

### Checkpoint Format

Checkpoints identify HuggingFace model paths:

**ONNX models:**

```
amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx
amd/Phi-3.5-mini-instruct-onnx-npu
```

**GGUF models:**

```
Llama-3.2-3B-Instruct-Q4_K_M.gguf
```

Olla extracts and preserves this metadata in the unified catalogue but does not use it for routing decisions.

---

## Configuration Example

Basic endpoint configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "local-lemonade"
        type: "lemonade"
        priority: 85
        model_url: "/api/v1/models"
        health_check_url: "/api/v1/health"
        check_interval: 5s
        check_timeout: 2s
```

**Fields:**

- `url`: Lemonade SDK server address
- `type`: Must be `"lemonade"` to use Lemonade profile
- `model_url`: Endpoint for model discovery (default: `/api/v1/models`)
- `health_check_url`: Health check endpoint (default: `/api/v1/health`)

See the [Lemonade Integration Guide](../integrations/backend/lemonade.md) for detailed configuration examples.

---

## Error Responses

### Backend Unavailable

```json
{
  "error": {
    "message": "No healthy endpoint available for provider: lemonade",
    "type": "endpoint_unavailable",
    "code": 503
  }
}
```

**Cause:** All Lemonade backends are unhealthy or unreachable.

**Solution:** Check backend health at `/internal/status/endpoints`.

### Circuit Breaker Open

```json
{
  "error": {
    "message": "Circuit breaker open for endpoint: lemonade-npu",
    "type": "circuit_breaker",
    "code": 503
  }
}
```

**Cause:** Backend failed too many health checks and was marked unhealthy.

**Solution:** Wait for automatic recovery (default: 30s) or fix backend issues.

### Model Not Loaded

```json
{
  "error": {
    "message": "Model not loaded",
    "type": "model_not_loaded",
    "code": 400
  }
}
```

**Cause:** Lemonade SDK backend has no model loaded.

**Solution:** Load a model using `/api/v1/load` endpoint.

---

## Monitoring

### Health Check

```bash
# Olla's health endpoint (includes all backends)
curl http://localhost:40114/internal/health
```

**Response:**
```json
{
  "status": "healthy",
  "endpoints": {
    "lemonade-npu": {
      "url": "http://npu-server:8000",
      "healthy": true,
      "last_check": "2025-01-15T10:30:00Z"
    }
  }
}
```

### Endpoint Status

```bash
# Detailed endpoint status
curl http://localhost:40114/internal/status/endpoints
```

Shows health, request counts, error rates, and circuit breaker state for each Lemonade backend.

### Model Catalogue

```bash
# Unified model catalogue
curl http://localhost:40114/internal/status/models
```

Lists models from all Lemonade backends alongside other providers.

---

## Next Steps

- **[Lemonade Integration Guide](../integrations/backend/lemonade.md)** - Complete setup and configuration
- **[Profile System](../concepts/profile-system.md)** - Customise Lemonade behaviour
- **[Model Unification](../concepts/model-unification.md)** - Unified model catalogue
- **[Load Balancing](../concepts/load-balancing.md)** - Multi-backend configuration
- **[Lemonade SDK Documentation](https://lemonade-server.ai/docs/)** - Official Lemonade SDK documentation
