---
title: Lemonade SDK Proxy Support - AMD-Optimised Local LLM Inference
description: Configure Olla as a proxy for Lemonade SDK backends with load balancing, health checks, and model unification for AMD-optimised local inference.
keywords: Lemonade SDK, Olla proxy, AMD Ryzen AI, NPU inference, local LLM, ONNX, GGUF, DirectML, Windows AI
---

# Lemonade SDK Proxy Support

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/lemonade-sdk/lemonade">github.com/lemonade-sdk/lemonade</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.19</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>lemonade</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>lemonade.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/lemonade.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Transparent Proxy Forwarding</li>
                <li>Model Discovery & Unification</li>
                <li>Load Balancing Across Instances</li>
                <li>Health Monitoring</li>
                <li>OpenAI Format Conversion</li>
                <li>Recipe & Checkpoint Metadata Preservation</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Backend Features (Proxied)</th>
        <td>
            <ul>
                <li>AMD Hardware Acceleration (NPU, iGPU, CPU)</li>
                <li>Multi-Recipe Support (ONNX & GGUF)</li>
                <li>Model Lifecycle Management</li>
                <li>System Information & Statistics</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/lemonade</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Lemonade SDK Docs</th>
        <td>
            <a href="https://lemonade-server.ai/docs/">Official Documentation</a>
        </td>
    </tr>
</table>

## Overview

Olla provides **proxy support** for Lemonade SDK backends, enabling:

- **Transparent Request Forwarding**: All requests to `/olla/lemonade/*` are forwarded to configured Lemonade SDK backends
- **Load Balancing**: Distribute requests across multiple Lemonade instances
- **Health Monitoring**: Automatic health checks and circuit breaking for unhealthy backends
- **Model Unification**: Lemonade models appear in Olla's unified model catalogue alongside other providers
- **OpenAI Compatibility**: `/v1/models` endpoint returns OpenAI-format responses

**What Olla Provides:**

- Routing and load balancing to Lemonade backends
- Model discovery and catalogue integration
- Health checks and availability monitoring
- Format conversion for model listings

**What Lemonade SDK Provides:**

- Actual LLM inference with hardware optimisation
- Model lifecycle management (pull, load, unload, delete)
- AMD-specific acceleration (NPU, iGPU, CPU)
- System information and runtime statistics

Olla acts as a **transparent proxy** - it forwards requests to Lemonade SDK and adds operational features like load balancing and health monitoring.

## Configuration

### Basic Setup

Configure a single Lemonade SDK endpoint:

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

**Configuration Fields:**

- `url`: Lemonade SDK server address (default: `http://localhost:8000`)
- `type`: Must be `"lemonade"` to use the Lemonade profile
- `priority`: Routing priority (0-100, higher = preferred)
- `model_url`: Endpoint for model discovery (default: `/api/v1/models`)
- `health_check_url`: Health check endpoint (default: `/api/v1/health`)

### Production Setup: Multiple Instances

Load balance across multiple Lemonade SDK instances:

```yaml
discovery:
  static:
    endpoints:
      # Primary NPU-optimised instance
      - url: "http://npu-server:8000"
        name: "lemonade-npu"
        type: "lemonade"
        priority: 100
        tags:
          hardware: npu
          location: office

      # Secondary GPU-optimised instance
      - url: "http://gpu-server:8000"
        name: "lemonade-gpu"
        type: "lemonade"
        priority: 90
        tags:
          hardware: igpu
          location: office

      # Fallback CPU instance
      - url: "http://cpu-server:8000"
        name: "lemonade-cpu"
        type: "lemonade"
        priority: 75
        tags:
          hardware: cpu
          location: backup
```

Olla will:

- Route requests to the highest-priority healthy backend
- Automatically failover if a backend becomes unhealthy
- Periodically check health and restore failed backends
- Provide a unified model catalogue across all instances

## How It Works

### Request Flow

```
Client Request → Olla Proxy → Lemonade SDK Backend
                     ↓
            - Load Balancing
            - Health Checking
            - Header Injection
                     ↓
                 Response ← Lemonade SDK
```

**Step-by-Step:**

1. **Client sends request** to `/olla/lemonade/api/v1/chat/completions`
2. **Olla routes** based on:
   - Backend health status (circuit breaker)
   - Priority configuration
   - Load balancing strategy
3. **Request forwarded** to `http://backend:8000/api/v1/chat/completions`
4. **Lemonade processes** the inference request
5. **Olla adds headers** to response:
   - `X-Olla-Endpoint`: Backend name used
   - `X-Olla-Model`: Model identifier
   - `X-Olla-Backend-Type`: `lemonade`
   - `X-Olla-Response-Time`: Processing time
6. **Response returned** to client

### Model Discovery

Olla periodically queries `/api/v1/models` from each Lemonade backend:

```json
// Lemonade SDK Response
{
  "object": "list",
  "data": [
    {
      "id": "Qwen2.5-0.5B-Instruct-CPU",
      "checkpoint": "amd/Qwen2.5-0.5B-Instruct-quantized_int4-float16-cpu-onnx",
      "recipe": "oga-cpu",
      "created": 1759361710,
      "owned_by": "lemonade"
    }
  ]
}
```

**Olla extracts:**

- **Model ID**: Used for routing requests
- **Checkpoint**: HuggingFace model path (preserved in metadata)
- **Recipe**: Inference engine (`oga-cpu`, `oga-npu`, `llamacpp`, etc.)
- **Format**: Inferred from recipe (ONNX for `oga-*`, GGUF for `llamacpp`)

**Unified Catalogue:**

Models from all Lemonade backends appear in Olla's unified catalogue at `/internal/status/models` alongside models from Ollama, OpenAI, and other providers.

### What Olla Adds vs What Lemonade Provides

| Feature | Olla | Lemonade SDK |
|---------|------|--------------|
| LLM Inference | ❌ | ✅ Hardware-optimised |
| Model Loading | ❌ | ✅ Dynamic memory management |
| Hardware Detection | ❌ | ✅ NPU/iGPU/CPU detection |
| Load Balancing | ✅ | ❌ |
| Health Monitoring | ✅ | ❌ |
| Multi-Backend Routing | ✅ | ❌ |
| Unified Catalogue | ✅ | ❌ |
| Failover | ✅ | ❌ |

## Model Format Support

Lemonade SDK uses **recipes** to identify inference engines and model formats.

### Recipe System

| Recipe | Engine | Format | Hardware | Description |
|--------|--------|--------|----------|-------------|
| `oga-cpu` | ONNX Runtime | ONNX | CPU | CPU-optimised inference |
| `oga-npu` | ONNX Runtime | ONNX | NPU | AMD Ryzen AI NPU acceleration |
| `oga-igpu` | ONNX Runtime | ONNX | iGPU | DirectML iGPU acceleration |
| `llamacpp` | llama.cpp | GGUF | CPU/GPU | GGUF quantised models |
| `flm` | Fast Language Models | Various | CPU/GPU | Alternative inference engine |

### Checkpoint and Recipe Metadata

Olla **preserves** Lemonade-specific metadata:

```json
{
  "id": "phi-3.5-mini-instruct-npu",
  "provider": "lemonade",
  "metadata": {
    "checkpoint": "amd/Phi-3.5-mini-instruct-onnx-npu",
    "recipe": "oga-npu",
    "format": "onnx"
  }
}
```

This metadata is:

- **Discovered** during model polling
- **Stored** in the unified registry
- **Returned** in Lemonade-format responses

### Format Detection

Olla infers model format from recipe:

```yaml
# From lemonade.yaml profile
models:
  name_format: "{{.Name}}"  # Use friendly names
  capability_patterns:
    chat:
      - "*-Instruct-*"
      - "*-Chat-*"
    embeddings:
      - "*embed*"
    code:
      - "*code*"
      - "*Coder*"
```

**Automatic Detection:**

- `oga-*` recipes → ONNX format
- `llamacpp` recipe → GGUF format
- Capabilities inferred from model name patterns

## Usage Examples

### Chat Completion (OpenAI-Compatible)

```bash
curl -X POST http://localhost:40114/olla/lemonade/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-0.5B-Instruct-CPU",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is AMD Ryzen AI?"}
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }'
```

**Response includes Olla headers:**

```
X-Olla-Endpoint: lemonade-npu
X-Olla-Model: Qwen2.5-0.5B-Instruct-CPU
X-Olla-Backend-Type: lemonade
X-Olla-Response-Time: 234ms
```

### Model Listing (OpenAI Format)

```bash
# OpenAI-compatible format
curl http://localhost:40114/olla/lemonade/v1/models

# Or Lemonade's native path
curl http://localhost:40114/olla/lemonade/api/v1/models
```

**Response:**
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
    }
  ]
}
```

### Accessing Lemonade SDK Features

All Lemonade SDK endpoints are accessible through the `/olla/lemonade/` prefix:

```bash
# System information (proxied to backend)
curl http://localhost:40114/olla/lemonade/api/v1/system-info

# Runtime statistics (proxied to backend)
curl http://localhost:40114/olla/lemonade/api/v1/stats

# Model management (proxied to backend)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/load \
  -H "Content-Type: application/json" \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'
```

These requests are **forwarded transparently** to the Lemonade SDK backend. Olla does not process or modify them beyond routing.

## Lemonade SDK Features (Proxied)

The following features are **provided by Lemonade SDK** and accessed through Olla's proxy:

### Model Lifecycle Management

```bash
# Download/install model (handled by Lemonade)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/pull \
  -d '{"checkpoint": "amd/Phi-3.5-mini-instruct-onnx-npu"}'

# Load model into memory (handled by Lemonade)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/load \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'

# Unload from memory (handled by Lemonade)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/unload \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'

# Delete from disk (handled by Lemonade)
curl -X POST http://localhost:40114/olla/lemonade/api/v1/delete \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'
```

### Hardware Optimisation

Lemonade SDK automatically:

- **Detects** available hardware (NPU, iGPU, CPU)
- **Selects** appropriate recipe for each model
- **Optimises** inference for AMD Ryzen AI platforms

Olla does not handle hardware detection - it simply routes to the backend.

### System Information

```bash
curl http://localhost:40114/olla/lemonade/api/v1/system-info
```

Returns Lemonade SDK's hardware detection results, memory status, and available acceleration engines.

For detailed API documentation, see the [Lemonade SDK API Reference](../../api-reference/lemonade.md) or [Lemonade SDK Documentation](https://lemonade-server.ai/docs/).

## Profile Configuration

The Lemonade profile (`config/profiles/lemonade.yaml`) defines routing behaviour, model patterns, and capabilities.

### Key Profile Sections

```yaml
# Routing prefixes
routing:
  prefixes:
    - lemonade  # Accessible at /olla/lemonade/

# API compatibility
api:
  openai_compatible: true
  model_discovery_path: /api/v1/models
  health_check_path: /api/v1/health

# Model capability detection
models:
  name_format: "{{.Name}}"
  capability_patterns:
    chat:
      - "*-Instruct-*"
      - "*-Chat-*"
    embeddings:
      - "*embed*"
    code:
      - "*code*"
      - "*Coder*"

# Recipe to format mapping
features:
  backends:
    enabled: true
    supported_engines:
      - "oga-cpu"      # ONNX Runtime for CPU
      - "oga-npu"      # ONNX Runtime for NPU
      - "oga-igpu"     # ONNX Runtime for iGPU
      - "llamacpp"     # llama.cpp for GGUF
      - "flm"          # Fast Language Models
```

### Customising the Profile

Create `config/profiles/lemonade-custom.yaml`:

```yaml
name: lemonade
version: "1.0"

# Override default timeouts
characteristics:
  timeout: 5m  # Increase for large models
  max_concurrent_requests: 50

# Add custom routing prefixes
routing:
  prefixes:
    - lemonade
    - amd      # Custom prefix: /olla/amd/

# Custom capability patterns
models:
  capability_patterns:
    reasoning:
      - "*DeepSeek-R1*"
      - "*Cogito*"
    vision:
      - "*VL-*"
      - "*Scout*"
```

See [Profile System](../../concepts/profile-system.md) for complete customisation options.

## Monitoring

### Health Checks

Olla monitors Lemonade backend health:

```bash
# Olla's health status (includes Lemonade backends)
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
# View all endpoints
curl http://localhost:40114/internal/status/endpoints
```

Shows:
- Health status per backend
- Circuit breaker state
- Request counts
- Error rates

### Model Availability

```bash
# Unified model catalogue
curl http://localhost:40114/internal/status/models
```

Lists models from all Lemonade backends alongside other providers.

### Response Headers

Every proxied response includes Olla headers:

```
HTTP/1.1 200 OK
Content-Type: application/json
X-Olla-Endpoint: lemonade-npu
X-Olla-Model: Phi-3.5-mini-instruct-NPU
X-Olla-Backend-Type: lemonade
X-Olla-Response-Time: 156ms
X-Olla-Request-ID: req-abc123
```

Use these for:
- **Debugging**: Which backend processed the request
- **Monitoring**: Response time tracking
- **Auditing**: Request tracing

## Load Balancing Strategies

Configure how Olla selects Lemonade backends:

### Priority-Based (Default)

```yaml
balancer:
  strategy: priority

endpoints:
  - url: "http://npu-server:8000"
    priority: 100  # Always prefer NPU
  - url: "http://cpu-server:8000"
    priority: 75   # Fallback to CPU
```

Always uses highest-priority healthy backend.

### Round-Robin

```yaml
balancer:
  strategy: round_robin

endpoints:
  - url: "http://server1:8000"
    priority: 100
  - url: "http://server2:8000"
    priority: 100  # Equal priority
```

Distributes requests evenly across backends.

### Least Connections

```yaml
balancer:
  strategy: least_connections

endpoints:
  - url: "http://server1:8000"
  - url: "http://server2:8000"
```

Routes to backend with fewest active connections.

See [Load Balancing](../../concepts/load-balancing.md) for detailed configuration.

## Troubleshooting

### Backend Unreachable

**Issue**: "failed to connect to backend"

**Solution**:
1. Verify Lemonade SDK is running:
   ```bash
   curl http://localhost:8000/api/v1/health
   ```

2. Check endpoint configuration:
   ```yaml
   endpoints:
     - url: "http://localhost:8000"  # Correct port?
       type: "lemonade"
   ```

3. Review Olla logs:
   ```bash
   olla serve --log-level debug
   ```

### Model Not Found

**Issue**: Model appears in `/v1/models` but inference fails

**Solution**:
1. Check if model is loaded in Lemonade:
   ```bash
   curl http://localhost:8000/api/v1/health
   ```

2. Load model explicitly:
   ```bash
   curl -X POST http://localhost:40114/olla/lemonade/api/v1/load \
     -d '{"model": "Qwen2.5-0.5B-Instruct-CPU"}'
   ```

3. Verify model ID matches Lemonade's format (check `/api/v1/models` response)

### Circuit Breaker Triggered

**Issue**: "circuit breaker open for endpoint"

**Solution**:
1. Check backend health:
   ```bash
   curl http://localhost:40114/internal/status/endpoints
   ```

2. Wait for automatic recovery (default: 30s)

3. Manually reset by fixing backend and waiting for next health check

### Slow Responses

**Issue**: First request to a model is slow

**Explanation**: Lemonade SDK loads models on-demand. First request triggers loading.

**Solutions**:
1. Pre-load models via Lemonade API:
   ```bash
   curl -X POST http://localhost:8000/api/v1/load \
     -d '{"model": "Phi-3.5-mini-instruct-NPU"}'
   ```

2. Increase timeout in Olla config:
   ```yaml
   proxy:
     response_timeout: 300s  # 5 minutes
   ```

## Best Practices

### 1. Use Model Unification

Enable unified model catalogue:

```yaml
model_registry:
  enable_unifier: true
  unification:
    enabled: true
```

Benefits:

- Single model catalogue across all backends
- Automatic failover to equivalent models
- Simplified client code

### 2. Configure Appropriate Timeouts

Lemonade models load on first request:

```yaml
proxy:
  response_timeout: 300s  # Allow time for model loading
  connection_timeout: 30s

discovery:
  static:
    endpoints:
      - check_timeout: 5s   # Health check timeout
        check_interval: 10s # Check frequency
```

### 3. Tag Backends by Hardware

Use tags for hardware-specific routing:

```yaml
endpoints:
  - url: "http://npu-server:8000"
    name: "lemonade-npu"
    tags:
      hardware: npu
      recipe: oga-npu
  - url: "http://cpu-server:8000"
    name: "lemonade-cpu"
    tags:
      hardware: cpu
      recipe: oga-cpu
```

### 4. Monitor Health Actively

```bash
# Set up periodic health monitoring
watch -n 5 'curl -s http://localhost:40114/internal/health | jq'

# Check endpoint status
curl http://localhost:40114/internal/status/endpoints
```

### 5. Pre-load Critical Models

Avoid first-request latency:

```bash
#!/bin/bash
# preload-models.sh
curl -X POST http://localhost:8000/api/v1/load \
  -d '{"model": "Qwen2.5-0.5B-Instruct-CPU"}'

curl -X POST http://localhost:8000/api/v1/load \
  -d '{"model": "Phi-3.5-mini-instruct-NPU"}'
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/lemonade/api",
    api_key="not-needed"  # Lemonade doesn't require keys
)

response = client.chat.completions.create(
    model="Qwen2.5-0.5B-Instruct-CPU",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:40114/olla/lemonade/api/v1",
    model="Phi-3.5-mini-instruct-NPU",
    api_key="not-needed"
)

response = llm.invoke("What is AMD Ryzen AI?")
```

### cURL Testing

```bash
# Test chat completion
curl -X POST http://localhost:40114/olla/lemonade/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-0.5B-Instruct-CPU",
    "messages": [{"role": "user", "content": "Hi"}],
    "max_tokens": 50
  }'
```

## Next Steps

- **[Lemonade SDK API Reference](../../api-reference/lemonade.md)** - Detailed endpoint documentation
- **[Profile System](../../concepts/profile-system.md)** - Customise Lemonade behaviour
- **[Model Unification](../../concepts/model-unification.md)** - Unified model catalogue
- **[Load Balancing](../../concepts/load-balancing.md)** - Configure multi-backend setups
- **[Lemonade SDK Documentation](https://lemonade-server.ai/docs/)** - Official Lemonade documentation
