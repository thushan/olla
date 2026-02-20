---
title: Docker Model Runner Integration - Docker's Built-in LLM Inference with Olla
description: Configure Docker Model Runner with Olla proxy for local LLM inference. Multi-engine support (llama.cpp/vLLM), OCI model distribution, native Anthropic Messages API, and OpenAI compatibility.
keywords: [Docker Model Runner, DMR, Olla proxy, Docker Desktop, LLM inference, llama.cpp, vLLM, OCI models, Docker Hub, local AI, OpenAI compatible]
---

# Docker Model Runner Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://docs.docker.com/ai/model-runner/">docs.docker.com/ai/model-runner/</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.23</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>docker-model-runner</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>dmr.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/dmr.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Health Check (via model listing)</li>
                <li>Model Unification</li>
                <li>Model Detection &amp; Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>Native Anthropic Messages API</li>
                <li>Anthropic Token Counting</li>
                <li>Multi-Engine Support (llama.cpp + vLLM)</li>
                <li>OCI Model Distribution</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Dedicated Health Endpoint (uses <code>/engines/v1/models</code>)</li>
                <li>Prometheus Metrics Endpoint</li>
                <li>Model Management via API (use <code>docker model</code> CLI)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>OpenAI Compatible</li>
                <li>Lazy Model Loading</li>
                <li>OCI Artifact Distribution</li>
                <li>Hardware Accelerated (Metal / CUDA)</li>
                <li>Multi-Engine (llama.cpp &amp; vLLM)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/dmr</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Endpoints</th>
        <td>
            See <a href="#endpoints-supported">below</a>
        </td>
    </tr>
</table>

!!! note "Lazy Model Loading"
    Docker Model Runner loads models into memory on the first inference request, not at startup. The `/engines/v1/models` endpoint returns `200` with an empty `data` array when no models have been pulled yet — this is normal and does not indicate an unhealthy endpoint.

## Configuration

### Basic Setup

Add Docker Model Runner to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:12434"
        name: "local-dmr"
        type: "docker-model-runner"
        priority: 95
        model_url: "/engines/v1/models"
        health_check_url: "/engines/v1/models"
        check_interval: 10s
        check_timeout: 5s
```

### From Within a Container

When Olla itself runs inside a Docker container, use the internal hostname:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://model-runner.docker.internal"
        name: "dmr-internal"
        type: "docker-model-runner"
        priority: 95
        model_url: "/engines/v1/models"
        health_check_url: "/engines/v1/models"
        check_interval: 10s
        check_timeout: 5s
```

### Production Setup

Configure multiple DMR instances across machines (Linux with NVIDIA):

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-host-1:12434"
        name: "dmr-gpu-1"
        type: "docker-model-runner"
        priority: 95
        model_url: "/engines/v1/models"
        health_check_url: "/engines/v1/models"
        check_interval: 10s
        check_timeout: 5s

      - url: "http://gpu-host-2:12434"
        name: "dmr-gpu-2"
        type: "docker-model-runner"
        priority: 95
        model_url: "/engines/v1/models"
        health_check_url: "/engines/v1/models"
        check_interval: 10s
        check_timeout: 5s

proxy:
  engine: "olla"
  load_balancer: "least-connections"
```

## Anthropic Messages API Support

Docker Model Runner natively supports the Anthropic Messages API at `/anthropic/v1/messages`, enabling Olla to forward Anthropic-format requests directly without translation overhead (passthrough mode).

When Olla detects a DMR endpoint with native Anthropic support (configured via the `anthropic_support` section in `config/profiles/dmr.yaml`), it bypasses the Anthropic-to-OpenAI translation pipeline and forwards requests directly to the backend.

**Profile configuration** (from `config/profiles/dmr.yaml`):

```yaml
api:
  anthropic_support:
    enabled: true
    messages_path: /anthropic/v1/messages
    token_count: true
```

**Key details**:

- Token counting (`/anthropic/v1/messages/count_tokens`): Supported
- Passthrough mode is automatic — no client-side configuration needed
- Responses include `X-Olla-Mode: passthrough` header when passthrough is active
- Falls back to translation mode if passthrough conditions are not met

For more information, see [API Translation](../../concepts/api-translation.md#passthrough-mode) and [Anthropic API Reference](../../api-reference/anthropic.md).

## Endpoints Supported

The following endpoints are supported by the DMR integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/engines/v1/models</code></td>
    <td>List Models &amp; Health Check (engine-routed)</td>
  </tr>
  <tr>
    <td><code>/engines/v1/chat/completions</code></td>
    <td>Chat Completions (engine-routed, OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/engines/v1/completions</code></td>
    <td>Text Completions (engine-routed, OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/engines/v1/embeddings</code></td>
    <td>Embeddings (engine-routed, OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/engines/llama.cpp/v1/models</code></td>
    <td>List Models (explicit llama.cpp engine)</td>
  </tr>
  <tr>
    <td><code>/engines/llama.cpp/v1/chat/completions</code></td>
    <td>Chat Completions (explicit llama.cpp engine)</td>
  </tr>
  <tr>
    <td><code>/engines/llama.cpp/v1/completions</code></td>
    <td>Text Completions (explicit llama.cpp engine)</td>
  </tr>
  <tr>
    <td><code>/engines/llama.cpp/v1/embeddings</code></td>
    <td>Embeddings (explicit llama.cpp engine)</td>
  </tr>
  <tr>
    <td><code>/engines/vllm/v1/models</code></td>
    <td>List Models (explicit vLLM engine)</td>
  </tr>
  <tr>
    <td><code>/engines/vllm/v1/chat/completions</code></td>
    <td>Chat Completions (explicit vLLM engine)</td>
  </tr>
  <tr>
    <td><code>/engines/vllm/v1/completions</code></td>
    <td>Text Completions (explicit vLLM engine)</td>
  </tr>
  <tr>
    <td><code>/engines/vllm/v1/embeddings</code></td>
    <td>Embeddings (explicit vLLM engine)</td>
  </tr>
</table>

The engine-routed paths (`/engines/v1/...`) let DMR automatically select the appropriate backend engine based on the model format. Use the explicit engine paths when you need to target a specific engine directly.

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain what Docker Model Runner is"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {"role": "user", "content": "Write a short story about containers"}
    ],
    "stream": true,
    "temperature": 0.8
  }'
```

### List Models

```bash
curl http://localhost:40114/olla/dmr/engines/v1/models
```

Example response:

```json
{
  "object": "list",
  "data": [
    {
      "id": "ai/smollm2",
      "object": "model",
      "created": 1734000000,
      "owned_by": "ai"
    },
    {
      "id": "ai/llama3.2",
      "object": "model",
      "created": 1734000001,
      "owned_by": "ai"
    }
  ]
}
```

### Explicit Engine Routing

Target the llama.cpp engine directly (useful for GGUF-specific parameters):

```bash
curl -X POST http://localhost:40114/olla/dmr/engines/llama.cpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {"role": "user", "content": "Hello"}
    ],
    "temperature": 0.7,
    "max_tokens": 200
  }'
```

## Docker Model Runner Specifics

### Multi-Engine Architecture

DMR automatically routes inference requests to the appropriate backend engine based on the model format:

| Engine | Model Format | Use Case |
|--------|-------------|----------|
| **llama.cpp** | GGUF (quantised) | Default; all platforms; CPU and GPU |
| **vLLM** | safetensors | Linux + NVIDIA; high throughput |

The `/engines/v1/...` paths use automatic engine selection. The explicit `/engines/llama.cpp/v1/...` and `/engines/vllm/v1/...` paths target a specific engine regardless of model format.

### Model Naming Convention

DMR uses a `namespace/name` format for model identifiers, matching Docker Hub image conventions:

```
ai/smollm2
ai/llama3.2
ai/phi4-mini
huggingface/meta-llama-3.1-8b
```

Use this exact format in API requests:

```json
{
  "model": "ai/smollm2",
  "messages": [...]
}
```

### Lazy Model Loading

DMR does not load models into memory until the first inference request. This means:

- The `/engines/v1/models` endpoint returns an empty `data` array until at least one model has been used
- The first request after startup (or after a model is idle) may have higher latency
- Olla treats an empty model list as a healthy state for DMR endpoints — this does not trigger circuit breaker logic

### Supported Platforms

| Platform | Acceleration | Notes |
|----------|-------------|-------|
| macOS Apple Silicon | Metal (GPU) | Default; fully supported |
| Linux x86_64 + NVIDIA | CUDA | Requires NVIDIA driver 575.57.08+ |
| Windows WSL2 + NVIDIA | CUDA | Requires NVIDIA driver 576.57+ |
| Windows ARM64 + Qualcomm | Adreno GPU | 6xx series+ |
| Linux (CPU / AMD / Intel) | Vulkan | Supported since October 2025 |

### Base URL Reference

| Access Method | Base URL |
|--------------|----------|
| Host (macOS/Linux/Windows) | `http://localhost:12434` |
| Docker container (Docker Desktop) | `http://model-runner.docker.internal` |
| Docker container (Linux Engine) | `http://172.17.0.1:12434` |

## Enabling Docker Model Runner

DMR ships with Docker Desktop 4.40+ but must be enabled before use.

### Enable via CLI

```bash
# Enable DMR with TCP access on port 12434
docker desktop enable model-runner --tcp 12434
```

### Pull Models

Models are distributed as OCI artifacts from Docker Hub:

```bash
# Pull a small model for testing
docker model pull ai/smollm2

# Pull Llama 3.2
docker model pull ai/llama3.2

# List locally available models
docker model ls
```

### Verify the API is Reachable

```bash
# List models via the API
curl http://localhost:12434/engines/v1/models

# Test a chat completion
curl -X POST http://localhost:12434/engines/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 50
  }'
```

## Profile Customisation

To customise DMR behaviour, create `config/profiles/dmr-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: docker-model-runner
version: "1.0"

# Add additional routing prefixes
routing:
  prefixes:
    - dmr
    - docker   # Add shorter alias

# Increase timeout for large models
characteristics:
  timeout: 10m

# Adjust concurrency for your hardware
resources:
  concurrency_limits:
    - min_memory_gb: 30
      max_concurrent: 1   # Reduce for constrained hardware
    - min_memory_gb: 8
      max_concurrent: 4
    - min_memory_gb: 0
      max_concurrent: 6
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Troubleshooting

### DMR Not Responding

**Issue**: Requests to `http://localhost:12434` fail with connection refused.

**Solution**:

1. Verify Docker Desktop 4.40+ is installed and running
2. Enable DMR with TCP access:
   ```bash
   docker desktop enable model-runner --tcp 12434
   ```
3. Confirm the port is listening:
   ```bash
   curl http://localhost:12434/engines/v1/models
   ```

### Empty Model List

**Issue**: `/engines/v1/models` returns `{"object":"list","data":[]}`.

**Solution**: This is expected when no models have been used yet. Pull a model first:

```bash
docker model pull ai/smollm2
```

After pulling, the model appears in the list only after it has been loaded (i.e., after the first inference request). This is normal lazy-loading behaviour.

### Slow First Response

**Issue**: The first request takes significantly longer than subsequent ones.

**Solution**: This is expected due to lazy loading. The model is loaded into memory on the first request. Subsequent requests use the already-loaded model. Consider sending a warm-up request at startup if latency on the first user request is a concern.

### Platform Not Supported

**Issue**: DMR fails to start or GPU acceleration is unavailable.

**Solution**: Check platform requirements:

- **macOS**: Requires Apple Silicon (M1 or later)
- **Linux**: Requires NVIDIA GPU with driver 575.57.08+ for GPU acceleration; CPU-only is also supported via Vulkan
- **Windows**: Requires WSL2 with NVIDIA driver 576.57+ for NVIDIA GPU; Qualcomm Adreno 6xx+ for ARM

### Model Name Not Found

**Issue**: Inference requests fail with model not found errors.

**Solution**: Use the full `namespace/name` format:

```bash
# Incorrect
"model": "smollm2"

# Correct
"model": "ai/smollm2"
```

Verify available model names with:
```bash
curl http://localhost:12434/engines/v1/models
```

## Best Practices

### 1. Use the Correct Endpoint for Health Checks

DMR has no dedicated `/health` endpoint. Olla uses `/engines/v1/models` for both model discovery and health checking. This endpoint always returns `200` (even with an empty list), so Olla correctly treats DMR as healthy at startup before any models are loaded.

### 2. Prefer Engine-Routed Paths

Use `/engines/v1/...` rather than explicit engine paths unless you have a specific reason to target a particular engine. DMR selects the optimal engine automatically based on model format.

### 3. Account for Lazy Loading in Timeouts

The DMR profile sets a 5-minute timeout by default to accommodate first-request model loading. If you are running large models (34B+), consider increasing the timeout:

```yaml
characteristics:
  timeout: 10m

resources:
  timeout_scaling:
    base_timeout_seconds: 300
    load_time_buffer: true
```

### 4. Use Priority Routing

If you have both DMR and other backends (e.g., Ollama), set priority to route requests appropriately:

```yaml
endpoints:
  - url: "http://localhost:12434"
    name: "dmr-local"
    type: "docker-model-runner"
    priority: 95   # High priority — local, built-in

  - url: "http://localhost:11434"
    name: "ollama-local"
    type: "ollama"
    priority: 80   # Lower priority — fallback
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/dmr/engines/v1",
    api_key="not-needed"
)

response = client.chat.completions.create(
    model="ai/smollm2",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)
```

### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:40114/olla/dmr/engines/v1",
    api_key="not-needed",
    model="ai/smollm2",
    temperature=0.7
)

response = llm.invoke("Hello!")
print(response.content)
```

### Anthropic SDK (via Passthrough)

When the target backend is DMR, Olla passes Anthropic-format requests through directly:

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:40114/olla/anthropic",
    api_key="not-needed"
)

message = client.messages.create(
    model="ai/smollm2",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(message.content[0].text)
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise DMR behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management across backends
- [Load Balancing](../../concepts/load-balancing.md) - Route across multiple DMR instances
- [API Translation](../../concepts/api-translation.md) - Use Anthropic-compatible clients with DMR
