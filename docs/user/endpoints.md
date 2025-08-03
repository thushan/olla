# API Endpoints Reference

Olla provides multiple types of endpoints: provider-specific endpoints for routing to specific LLM backends, unified model discovery endpoints, and internal monitoring endpoints.

## Base URL

```
http://localhost:40114
```

All API endpoints are relative to this base URL, Olla by default uses port `40114` (4 OLLA) but some examples use `8080`.

## Provider Endpoints

Provider endpoints route to specific LLM backends using the `/olla/{provider}/` prefix pattern.

### Ollama Endpoints

Access Ollama endpoints through `/olla/ollama/`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/olla/ollama/api/tags` | GET | List available models (native format) |
| `/olla/ollama/v1/models` | GET | List models (OpenAI format) |
| `/olla/ollama/api/list` | GET | List currently running models |
| `/olla/ollama/api/show` | POST | Get detailed model information |
| `/olla/ollama/api/generate` | POST | Generate text (native Ollama API) |
| `/olla/ollama/v1/chat/completions` | POST | Chat completions (OpenAI format) |
| `/olla/ollama/api/chat` | POST | Chat (native Ollama format) |
| `/olla/ollama/api/*` | ANY | Proxy to Ollama backend |

**Model Management (Not Supported)**
- `/olla/ollama/api/pull` - Model downloading
- `/olla/ollama/api/push` - Model uploading  
- `/olla/ollama/api/create` - Model creation
- `/olla/ollama/api/copy` - Model copying
- `/olla/ollama/api/delete` - Model deletion

#### Example: Ollama Chat

```bash
# Native Ollama format
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "What is machine learning?"}
    ]
  }'

# OpenAI-compatible format
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "What is machine learning?"}
    ]
  }'
```

### LM Studio Endpoints

Access LM Studio endpoints through `/olla/lmstudio/` (or `/olla/lm-studio/`, `/olla/lms/`):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/olla/lmstudio/v1/models` | GET | List models (OpenAI format) |
| `/olla/lmstudio/api/v1/models` | GET | Alternative models endpoint |
| `/olla/lmstudio/api/v0/models` | GET | Enhanced models with extra metadata |
| `/olla/lmstudio/v1/chat/completions` | POST | Chat completions |
| `/olla/lmstudio/*` | ANY | Proxy to LM Studio backend |

#### Example: LM Studio Chat

```bash
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-model",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### OpenAI-Compatible Endpoints

Access OpenAI-compatible services through `/olla/openai/`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/olla/openai/v1/models` | GET | List available models |
| `/olla/openai/v1/chat/completions` | POST | Chat completions |
| `/olla/openai/v1/embeddings` | POST | Generate embeddings |
| `/olla/openai/*` | ANY | Proxy to OpenAI-compatible backend |

### vLLM Endpoints

Access vLLM endpoints through `/olla/vllm/`:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/olla/vllm/v1/models` | GET | List models (OpenAI format) |
| `/olla/vllm/v1/chat/completions` | POST | Chat completions |
| `/olla/vllm/*` | ANY | Proxy to vLLM backend |

## Model Discovery Endpoints

Model discovery endpoints aggregate information across all configured providers. Models are unified within each provider type but not across different providers.

For detailed information about model discovery and unification, see [Model Discovery Guide](./models.md).

### List All Models

```http
GET /olla/models
```

Returns all models across all endpoints with per-provider unification:

```json
{
  "models": [
    {
      "id": "llama3.2:latest",
      "name": "llama3.2:latest",
      "unified_name": "llama3.2",
      "aliases": ["llama3.2", "llama-3.2-7b"],
      "family": "llama",
      "type": "llm",
      "size": 7000000000,
      "digest": "sha256:abc123...",
      "modified_at": "2024-01-20T10:00:00Z",
      "endpoints": [
        {
          "name": "local-ollama",
          "url": "http://localhost:11434",
          "platform": "ollama"
        }
      ]
    }
  ]
}
```

#### Query Parameters

- `format`: Response format (`unified`, `ollama`, `openai`, `lmstudio`)
- `endpoint`: Filter by endpoint name
- `family`: Filter by model family (e.g., `llama`, `phi`, `gemma`)
- `type`: Filter by model type (`llm`, `vlm`, `embeddings`)
- `available`: Filter by availability (`true`/`false`)

#### Examples

```bash
# All models from all providers
curl http://localhost:40114/olla/models

# Only Ollama models (unified format)
curl http://localhost:40114/olla/models?format=ollama

# Only LLM models
curl http://localhost:40114/olla/models?type=llm

# Models from specific endpoint
curl http://localhost:40114/olla/models?endpoint=local-ollama

# Models in OpenAI format
curl http://localhost:40114/olla/models?format=openai
```

### Get Model Details

```http
GET /olla/models/{id}
```

Returns details for a specific model by ID or alias:

```json
{
  "id": "llama3.2:latest",
  "name": "llama3.2:latest",
  "unified_name": "llama3.2",
  "aliases": ["llama3.2", "llama-3.2-7b"],
  "family": "llama",
  "type": "llm",
  "size": 7000000000,
  "parameters": "7B",
  "quantization": "Q4_0",
  "context_window": 8192,
  "digest": "sha256:abc123...",
  "endpoints": [
    {
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "platform": "ollama",
      "status": "available"
    }
  ]
}
```

## Internal Endpoints

Internal endpoints provide monitoring, health checks, and operational information.

### Health Check

```http
GET /internal/health
```

Basic health check endpoint:

```json
{
  "status": "healthy"
}
```

### System Status

```http
GET /internal/status
```

Returns system status with endpoint health and traffic metrics:

```json
{
  "endpoints": [
    {
      "url": "http://localhost:11434",
      "status": "Healthy",
      "priority": 100,
      "active_connections": 2
    }
  ],
  "stats": {
    "total_requests": 1543,
    "successful_requests": 1531,
    "failed_requests": 12,
    "average_latency": 234
  }
}
```

### Endpoint Status

```http
GET /internal/status/endpoints
```

Detailed status for each endpoint:

```json
{
  "endpoints": [
    {
      "name": "local-ollama",
      "url": "http://localhost:11434",
      "status": "Healthy",
      "priority": 100,
      "active_connections": 2,
      "total_requests": 1000,
      "success_rate": 99.8,
      "average_latency": 189
    }
  ]
}
```

### Model Status

```http
GET /internal/status/models
```

Status of models across all endpoints:

```json
{
  "models": [
    {
      "name": "llama3.2",
      "endpoints": ["http://localhost:11434"],
      "available": true
    }
  ]
}
```

### Model Statistics

```http
GET /internal/stats/models
```

Performance statistics per model:

```json
{
  "llama3.2": {
    "total_requests": 543,
    "successful_requests": 540,
    "failed_requests": 3,
    "average_latency": 234,
    "p95_latency": 450,
    "p99_latency": 780,
    "error_rate": 0.55
  }
}
```

### Process Metrics

```http
GET /internal/process
```

Process-level metrics:

```json
{
  "cpu_percent": 12.5,
  "memory_mb": 256.3,
  "goroutines": 42,
  "uptime_seconds": 3600
}
```

### Version Information

```http
GET /version
```

Olla version information:

```json
{
  "version": "0.1.0",
  "commit": "abc123",
  "build_time": "2024-01-20T10:00:00Z"
}
```

## Legacy Endpoints

These endpoints exist for backward compatibility:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/olla/proxy/*` | ANY | Legacy proxy endpoint |
| `/olla/proxy/v1/models` | GET | Legacy models endpoint |

## Response Headers

All proxy requests include tracking headers:

```
X-Olla-Endpoint: local-ollama
X-Olla-Model: llama3.2
X-Olla-Backend-Type: ollama
X-Olla-Request-ID: req_abc123
X-Olla-Response-Time: 1.234s
```

## Error Handling

Olla provides helpful error messages when endpoints are unavailable:

### No Available Endpoints (503)

```json
{
  "error": {
    "message": "no healthy endpoints available",
    "type": "service_unavailable",
    "code": 503
  }
}
```

### Model Not Found (404)

```json
{
  "error": {
    "message": "model 'gpt-4' not found on any endpoint",
    "type": "not_found",
    "code": 404
  }
}
```

### Rate Limit Exceeded (429)

```json
{
  "error": {
    "message": "rate limit exceeded",
    "type": "rate_limit_error",
    "code": 429
  }
}
```

### Request Too Large (413)

```json
{
  "error": {
    "message": "request body too large",
    "type": "request_error",
    "code": 413
  }
}
```

## Usage Examples

### Python with OpenAI Client

```python
from openai import OpenAI

# Point OpenAI client to Olla
client = OpenAI(
    base_url="http://localhost:40114/olla/ollama/v1",
    api_key="not-needed"  # Olla doesn't require auth by default
)

# List available models
models = client.models.list()
for model in models:
    print(f"Model: {model.id}")

# Chat completion
response = client.chat.completions.create(
    model="llama3.2",
    messages=[
        {"role": "user", "content": "What is machine learning?"}
    ],
    temperature=0.7
)

print(response.choices[0].message.content)
```

### Direct API with Python

```python
import requests

# Base URL
base_url = "http://localhost:40114"

# List unified models
response = requests.get(f"{base_url}/olla/models")
models = response.json()
print(f"Available models: {[m['unified_name'] for m in models['models']]}")

# Get model details
model_id = "llama3.2"
response = requests.get(f"{base_url}/olla/models/{model_id}")
details = response.json()
print(f"Model {model_id} available on: {[e['name'] for e in details['endpoints']]}")

# Chat via Ollama API
data = {
    "model": "llama3.2",
    "messages": [
        {"role": "user", "content": "What is the speed of light?"}
    ]
}

response = requests.post(
    f"{base_url}/olla/ollama/api/chat",
    json=data
)

result = response.json()
print(result['message']['content'])
```

### cURL Examples

```bash
# Check health
curl http://localhost:40114/internal/health

# Get system status
curl http://localhost:40114/internal/status | jq

# List unified models
curl http://localhost:40114/olla/models | jq

# Filter models by type
curl "http://localhost:40114/olla/models?type=llm" | jq

# Chat via OpenAI API
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Write a haiku about coding"}
    ]
  }' | jq

# Get endpoint statistics
curl http://localhost:40114/internal/status/endpoints | jq

# Get model statistics
curl http://localhost:40114/internal/stats/models | jq
```

## Provider Detection and Routing

Olla automatically detects which provider to route requests to based on the URL prefix:

- `/olla/ollama/*` → Routes to Ollama endpoints only
- `/olla/lmstudio/*` → Routes to LM Studio endpoints only
- `/olla/openai/*` → Routes to OpenAI-compatible endpoints only
- `/olla/vllm/*` → Routes to vLLM endpoints only

### Model-Aware Routing

Within each provider type, Olla routes requests to endpoints that have the requested model available:

```bash
# If llama3.2 is available on multiple Ollama endpoints,
# load balancing strategy determines which one gets the request
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Authentication

Olla forwards authentication headers to backends. Configure authentication in your endpoint configuration:

```yaml
discovery:
  endpoints:
    - name: "authenticated-api"
      url: "https://api.provider.com"
      headers:
        Authorization: "Bearer ${API_KEY}"
```

## Rate Limiting

Rate limits are configured in `config.yaml`:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    burst_size: 50
```

All endpoints respect these limits, with internal endpoints having higher allowances.

## Best Practices

### 1. Use Unified Model Discovery

```bash
# Good: Use unified model discovery
curl http://localhost:40114/olla/models

# Less optimal: Query each backend separately
curl http://backend1:11434/api/tags
curl http://backend2:11434/v1/models
```

### 2. Check Model Availability

```python
# Check if model exists before using
response = requests.get(f"http://localhost:40114/olla/models/llama3.2")
if response.status_code == 200:
    model_info = response.json()
    available_endpoints = [e['name'] for e in model_info['endpoints']]
    print(f"Model available on: {available_endpoints}")
```

### 3. Monitor Endpoint Health

```python
def wait_for_healthy_endpoints(base_url, timeout=30):
    start = time.time()
    while time.time() - start < timeout:
        response = requests.get(f"{base_url}/internal/status")
        status = response.json()
        
        healthy = [e for e in status['endpoints'] if e['status'] == 'Healthy']
        if healthy:
            return True
        
        time.sleep(1)
    return False
```

### 4. Use Request IDs for Debugging

```python
import uuid

headers = {
    "X-Request-ID": str(uuid.uuid4()),
    "Content-Type": "application/json"
}

response = requests.post(
    "http://localhost:40114/olla/ollama/v1/chat/completions",
    headers=headers,
    json={"model": "llama3.2", "messages": messages}
)
```