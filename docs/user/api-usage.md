# API Usage Guide

Olla acts as an intelligent proxy and load balancer for LLM inference endpoints. It doesn't implement its own inference API but provides smart routing, unified model discovery, and observability on top of existing services.

## Base URL

```
http://localhost:40114
```

All API endpoints are relative to this base URL.

## API Endpoints

### Health and Status Endpoints

#### Health Check

```http
GET /internal/health
```

Basic health check endpoint:

```json
{
  "status": "healthy"
}
```

#### Overall Status

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

#### Endpoint Status

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

#### Model Status

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

#### Model Statistics

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

#### Process Metrics

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

#### Version Information

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

### Unified Model Discovery

#### List Models

```http
GET /olla/models
```

Returns all models across all endpoints with unification:

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

##### Query Parameters

- `format`: Response format (`unified`, `ollama`, `openai`, `lmstudio`)
- `endpoint`: Filter by endpoint name
- `family`: Filter by model family (e.g., `llama`, `phi`, `gemma`)
- `type`: Filter by model type (`llm`, `vlm`, `embeddings`)
- `available`: Filter by availability (`true`/`false`)

##### Examples

```http
# Get only LLM models
GET /olla/models?type=llm

# Get models from specific endpoint
GET /olla/models?endpoint=local-ollama

# Get models in OpenAI format
GET /olla/models?format=openai
```

#### Get Model Details

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

### Proxy Endpoints

Olla acts as a transparent proxy, forwarding requests to backend services with intelligent routing:

#### Main Proxy Endpoint

```http
{METHOD} /olla/*path
```

Forwards requests to backend services based on:
- Load balancing strategy (priority, round-robin, least connections)
- Model availability (extracted from request body)
- Endpoint compatibility (platform profiles)

##### Example: Chat Completion via Proxy

```bash
# Request to Olla
POST http://localhost:40114/olla/v1/chat/completions

# Olla strips the /olla prefix and forwards to backend:
# POST http://backend:11434/v1/chat/completions
```

##### Request Body

```json
{
  "model": "llama3.2",
  "messages": [
    {
      "role": "user",
      "content": "What is the capital of France?"
    }
  ],
  "temperature": 0.7,
  "stream": false
}
```

##### How It Works

1. **Model Extraction**: Olla inspects the request body to extract the model name
2. **Endpoint Selection**: Finds endpoints that have the requested model loaded
3. **Load Balancing**: Applies the configured strategy to select an endpoint
4. **Request Forwarding**: Strips the `/olla/` prefix and forwards the request
5. **Response Streaming**: Streams the response back to the client

#### Alternative Proxy Endpoint

```http
{METHOD} /proxy/*path
```

Mirror endpoint for Sherpa compatibility. Works identically to `/olla/*`.

### Platform-Specific Examples

#### Ollama Backend

```bash
# Generate completion
POST http://localhost:40114/olla/api/generate
{
  "model": "llama3.2",
  "prompt": "Why is the sky blue?"
}

# List models (Ollama format)
GET http://localhost:40114/olla/api/tags

# Pull a model
POST http://localhost:40114/olla/api/pull
{
  "name": "llama3.2"
}
```

#### OpenAI-Compatible Backend (Ollama, LM Studio)

```bash
# Chat completion
POST http://localhost:40114/olla/v1/chat/completions
{
  "model": "llama3.2",
  "messages": [{"role": "user", "content": "Hello!"}]
}

# List models (OpenAI format)
GET http://localhost:40114/olla/v1/models

# Embeddings
POST http://localhost:40114/olla/v1/embeddings
{
  "model": "nomic-embed-text",
  "input": "The quick brown fox"
}
```

### Model-Aware Routing

Olla automatically routes requests to endpoints that have the requested model:

```bash
# If llama3.2 is only available on gpu-server, this request
# will be routed there regardless of load balancer settings
POST http://localhost:40114/olla/v1/chat/completions
{
  "model": "llama3.2",
  "messages": [{"role": "user", "content": "Hello!"}]
}

# If the model is available on multiple endpoints, the load
# balancer strategy determines which one to use
```

### Error Handling

Olla returns appropriate HTTP status codes and error messages:

#### No Available Endpoints

```json
{
  "error": {
    "message": "no healthy endpoints available",
    "type": "service_unavailable",
    "code": 503
  }
}
```

#### Model Not Found

```json
{
  "error": {
    "message": "model 'gpt-4' not found on any endpoint",
    "type": "not_found",
    "code": 404
  }
}
```

#### Rate Limit Exceeded

```json
{
  "error": {
    "message": "rate limit exceeded",
    "type": "rate_limit_error",
    "code": 429
  }
}
```

#### Request Too Large

```json
{
  "error": {
    "message": "request body too large",
    "type": "request_error",
    "code": 413
  }
}
```

## Request Headers

Olla forwards most headers to backend services, with some special handling:

### Content Type

```http
Content-Type: application/json
```

Required for POST requests with JSON bodies.

### Custom Headers

```http
# Force routing to specific endpoint
X-Olla-Endpoint: gpu-server

# Add request ID for tracing
X-Request-ID: 123e4567-e89b-12d3-a456-426614174000
```

### Authorization

Olla forwards authorization headers to backends:

```http
# If your backend requires authentication
Authorization: Bearer backend-api-key
```

## Rate Limiting

Olla implements per-IP and global rate limiting:

### Configuration

Rate limits are configured in `config.yaml`:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    burst_size: 50
```

### Rate Limit Behaviour

- Requests are limited per minute with burst allowance
- Health check endpoints have separate, higher limits
- Rate limiting is per-instance (not distributed)

When rate limited, Olla returns:

```http
HTTP/1.1 429 Too Many Requests

{
  "error": {
    "message": "rate limit exceeded",
    "type": "rate_limit_error",
    "code": 429
  }
}
```

## Client Examples

### Python with OpenAI Client

```python
from openai import OpenAI

# Point OpenAI client to Olla
client = OpenAI(
    base_url="http://localhost:40114/olla/v1",
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

# Streaming
stream = client.chat.completions.create(
    model="llama3.2",
    messages=[
        {"role": "user", "content": "Write a story about a robot"}
    ],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### Direct API with Python

```python
import requests
import json

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
    f"{base_url}/olla/api/chat",
    json=data
)

result = response.json()
print(result['message']['content'])
```

### JavaScript/Node.js

```javascript
import OpenAI from 'openai';

// Configure OpenAI client for Olla
const openai = new OpenAI({
  baseURL: 'http://localhost:40114/olla/v1',
  apiKey: 'not-needed',
});

// List models
async function listModels() {
  const models = await openai.models.list();
  return models.data;
}

// Chat completion
async function chat() {
  const completion = await openai.chat.completions.create({
    model: 'llama3.2',
    messages: [
      { role: 'user', content: 'Explain how HTTP works' }
    ],
  });
  
  console.log(completion.choices[0].message.content);
}

// Direct API calls
async function getStatus() {
  const response = await fetch('http://localhost:40114/internal/status');
  const status = await response.json();
  console.log('Endpoints:', status.endpoints);
  console.log('Stats:', status.stats);
}
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

# Chat via Ollama API
curl -X POST http://localhost:40114/olla/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Why is the sky blue?"}
    ]
  }' | jq

# Chat via OpenAI API
curl -X POST http://localhost:40114/olla/v1/chat/completions \
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

## Best Practices

### 1. Use the Unified Model API

The `/olla/models` endpoint provides a unified view across all backends:

```bash
# Good: Use unified model discovery
curl http://localhost:40114/olla/models

# Less optimal: Query each backend separately
curl http://backend1:11434/api/tags
curl http://backend2:11434/v1/models
```

### 2. Check Model Availability

Before making inference requests, verify the model is available:

```python
# Check if model exists
response = requests.get(f"http://localhost:40114/olla/models/llama3.2")
if response.status_code == 200:
    model_info = response.json()
    available_endpoints = [e['name'] for e in model_info['endpoints']]
    print(f"Model available on: {available_endpoints}")
```

### 3. Handle Streaming Properly

For long responses, use streaming to get results progressively:

```python
# Ollama streaming format
response = requests.post(
    "http://localhost:40114/olla/api/generate",
    json={"model": "llama3.2", "prompt": "Write a long story"},
    stream=True
)

for line in response.iter_lines():
    if line:
        data = json.loads(line)
        print(data.get('response', ''), end='')
        if data.get('done'):
            break
```

### 4. Monitor Endpoint Health

Regularly check endpoint status to handle failures gracefully:

```python
import time

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

### 5. Use Request IDs for Debugging

Add request IDs to trace requests through the system:

```python
import uuid

headers = {
    "X-Request-ID": str(uuid.uuid4()),
    "Content-Type": "application/json"
}

response = requests.post(
    "http://localhost:40114/olla/v1/chat/completions",
    headers=headers,
    json={"model": "llama3.2", "messages": messages}
)
```