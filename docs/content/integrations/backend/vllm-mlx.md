---
title: vLLM-MLX Integration - Apple Silicon LLM Inference with Olla
description: Configure vLLM-MLX with Olla proxy for high-performance LLM serving on Apple Silicon. MLX framework acceleration, unified memory architecture, and OpenAI compatibility.
keywords: vLLM-MLX, Olla proxy, Apple Silicon, MLX, M1, M2, M3, M4, unified memory, LLM inference, macOS
---

# vLLM-MLX Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/waybarrios/vllm-mlx">github.com/waybarrios/vllm-mlx</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.23</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>vllm-mlx</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>vllm-mlx.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/vllm-mlx.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Health Check (native)</li>
                <li>Model Unification</li>
                <li>Model Detection & Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>Native Anthropic Messages API</li>
                <li>Token Counting API</li>
                <li>Embeddings API</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Model Management (single model per instance, loaded at startup)</li>
                <li>Prometheus Metrics</li>
                <li>Tokenisation API</li>
                <li>Reranking API</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>Apple Silicon Only (M1/M2/M3/M4)</li>
                <li>MLX Framework Acceleration</li>
                <li>Unified Memory Architecture</li>
                <li>Single Model Server</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/vllm-mlx</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
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

## Configuration

### Basic Setup

Add vLLM-MLX to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "mlx-server"
        type: "vllm-mlx"
        priority: 85
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

### Apple Silicon Network Setup

Configure vLLM-MLX across multiple Mac instances behind Olla:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://mac-mini:8000"
        name: "mlx-llama"
        type: "vllm-mlx"
        priority: 85
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s

      - url: "http://mac-studio:8000"
        name: "mlx-qwen"
        type: "vllm-mlx"
        priority: 90
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s

proxy:
  engine: "olla"  # Use high-performance engine
  load_balancer: "priority"
```

## Anthropic Messages API Support

vLLM-MLX natively supports the Anthropic Messages API, enabling Olla to forward Anthropic-format requests directly without translation overhead (passthrough mode).

When Olla detects that a vLLM-MLX endpoint supports native Anthropic format (via the `anthropic_support` section in `config/profiles/vllm-mlx.yaml`), it will bypass the Anthropic-to-OpenAI translation pipeline and forward requests directly to `/v1/messages` on the backend.

**Profile configuration** (from `config/profiles/vllm-mlx.yaml`):

```yaml
api:
  anthropic_support:
    enabled: true
    messages_path: /v1/messages
    token_count: true
```

**Key details**:

- Token counting (`/v1/messages/count_tokens`): **Supported** (unlike standard vLLM)
- Passthrough mode is automatic -- no client-side configuration needed
- Responses include `X-Olla-Mode: passthrough` header when passthrough is active
- Falls back to translation mode if passthrough conditions are not met

For more information, see [API Translation](../../concepts/api-translation.md#passthrough-mode) and [Anthropic API Reference](../../api-reference/anthropic.md).

## Endpoints Supported

The following endpoints are supported by the vLLM-MLX integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/health</code></td>
    <td>Health Check</td>
  </tr>
  <tr>
    <td><code>/v1/models</code></td>
    <td>List Models (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/v1/chat/completions</code></td>
    <td>Chat Completions (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/v1/completions</code></td>
    <td>Text Completions (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/v1/embeddings</code></td>
    <td>Embeddings API</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/vllm-mlx/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain quantum computing in simple terms"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/vllm-mlx/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "messages": [
      {"role": "user", "content": "Write a story about a robot"}
    ],
    "stream": true,
    "temperature": 0.8
  }'
```

### Anthropic Messages API (Passthrough)

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "mlx-community/Llama-3.2-3B-Instruct-4bit",
    "max_tokens": 500,
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### Models and Health

```bash
# List available models
curl http://localhost:40114/olla/vllm-mlx/v1/models

# Check health status
curl http://localhost:40114/olla/vllm-mlx/health
```

## vLLM-MLX Specifics

### Apple Silicon Features

vLLM-MLX leverages the MLX framework for optimised inference on Apple Silicon:

- **Unified Memory Architecture**: Model weights and KV cache share the same memory pool -- no CPU-to-GPU transfer overhead
- **MLX Framework Acceleration**: Purpose-built for Apple's M-series neural engine and GPU cores
- **Continuous Batching**: Dynamic request batching for improved multi-user throughput (2-3.4x speedup)
- **Quantisation Support**: 2bit, 3bit, 4bit (most common), 6bit, 8bit, and bf16

### Resource Configuration

The vLLM-MLX profile includes Apple Silicon-optimised settings:

```yaml
characteristics:
  timeout: 2m
  max_concurrent_requests: 50
  streaming_support: true

resources:
  defaults:
    requires_gpu: false  # Uses unified memory, not discrete GPU
```

### Memory Requirements

vLLM-MLX uses unified memory shared between the system and model inference. There is no discrete GPU -- all memory is drawn from the Mac's unified pool:

| Model Size | Min Memory | Recommended | Max Concurrent |
|------------|------------|-------------|----------------|
| 70B+ (4bit) | 40GB | 48GB | 2 |
| 30B+ (4bit) | 20GB | 24GB | 5 |
| 13B (4bit) | 10GB | 16GB | 10 |
| 7-8B (4bit) | 6GB | 8GB | 20 |
| 3B (4bit) | 3GB | 4GB | 20 |
| 1B (4bit) | 2GB | 3GB | 20 |

### Performance (M4 Max 128GB)

| Model | Throughput |
|-------|-----------|
| Llama-3.2-3B-4bit | ~200 tok/s |
| Llama-3.2-1B-4bit | ~464 tok/s |
| With continuous batching | 2-3.4x speedup |

### Model Naming

vLLM-MLX uses HuggingFace model names from the `mlx-community` organisation:

- `mlx-community/Llama-3.2-3B-Instruct-4bit`
- `mlx-community/Llama-3.2-1B-Instruct-4bit`
- `mlx-community/Qwen2.5-7B-Instruct-4bit`
- `mlx-community/Mistral-7B-Instruct-v0.3-4bit`

## Starting vLLM-MLX Server

### Installation

```bash
# Using pip
pip install vllm-mlx

# Using uv (recommended)
uv tool install vllm-mlx
```

### Basic Start

```bash
vllm-mlx serve mlx-community/Llama-3.2-3B-Instruct-4bit --port 8000
```

### With Options

```bash
vllm-mlx serve mlx-community/Llama-3.2-3B-Instruct-4bit \
  --port 8000 \
  --host 0.0.0.0 \
  --continuous-batching \
  --max-tokens 4096
```

### With Reasoning Support

```bash
vllm-mlx serve mlx-community/QwQ-32B-4bit \
  --port 8000 \
  --host 0.0.0.0 \
  --continuous-batching \
  --reasoning-parser deepseek_r1
```

### With Embeddings

```bash
vllm-mlx serve mlx-community/Llama-3.2-3B-Instruct-4bit \
  --port 8000 \
  --embedding-model mlx-community/bge-small-en-v1.5
```

### Key CLI Flags

| Flag | Description |
|------|-------------|
| `--port` | Server port (default: 8000) |
| `--host` | Bind address (default: 127.0.0.1) |
| `--continuous-batching` | Enable dynamic request batching |
| `--cache-memory-percent` | Percentage of memory for KV cache |
| `--max-tokens` | Maximum token generation length |
| `--reasoning-parser` | Enable reasoning model support |
| `--embedding-model` | Load a separate embedding model |

## Profile Customisation

To customise vLLM-MLX behaviour, create `config/profiles/vllm-mlx-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: vllm-mlx
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - vllm-mlx
    - mlx      # Add custom prefix

# Adjust for larger models
characteristics:
  timeout: 5m     # Increase for 70B+ models

# Modify concurrency limits
resources:
  concurrency_limits:
    - min_memory_gb: 40
      max_concurrent: 2    # Reduce for very large models
    - min_memory_gb: 16
      max_concurrent: 10   # Adjust based on unified memory
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Troubleshooting

### Apple Silicon Only

**Issue**: vLLM-MLX fails to start or install

**Solution**: vLLM-MLX only runs on Apple Silicon Macs (M1/M2/M3/M4). It does not support Intel Macs, Linux, or Windows. Verify your hardware:

```bash
sysctl -n machdep.cpu.brand_string
# Should show "Apple M1", "Apple M2", etc.
```

### Single Model Server

**Issue**: Cannot switch models without restarting

**Solution**: vLLM-MLX loads a single model at startup. To serve multiple models, run separate instances on different ports and configure them as distinct endpoints in Olla:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "mlx-llama"
        type: "vllm-mlx"
        priority: 85

      - url: "http://localhost:8001"
        name: "mlx-qwen"
        type: "vllm-mlx"
        priority: 85
```

### Memory Pressure

**Issue**: macOS becomes sluggish or model inference slows down

**Solution**: Unified memory is shared between macOS and model inference. If the model consumes too much memory, the system will swap to disk, degrading performance:

1. Choose a smaller quantisation (e.g. 4bit instead of 8bit)
2. Use a smaller model that fits comfortably within available memory
3. Close memory-intensive applications
4. Monitor memory usage with `Activity Monitor` or `vm_stat`

### Connection Timeout

**Issue**: Requests timeout during model loading

**Solution**: Increase timeout in profile:
```yaml
characteristics:
  timeout: 10m  # Increase for initial model load

resources:
  timeout_scaling:
    base_timeout_seconds: 300
    load_time_buffer: true
```

## Best Practices

### 1. Use 4bit Quantisation

4bit quantisation provides the best balance of quality and performance on Apple Silicon:

```bash
# Recommended: 4bit models from mlx-community
vllm-mlx serve mlx-community/Llama-3.2-3B-Instruct-4bit --port 8000
```

### 2. Match Model Size to Memory

Choose models that fit comfortably within your Mac's unified memory, leaving headroom for macOS and other applications:

| Mac | Unified Memory | Recommended Max Model |
|-----|---------------|----------------------|
| Mac Mini (base) | 16GB | 7-8B (4bit) |
| Mac Mini (max) | 64GB | 30B+ (4bit) |
| Mac Studio | 64-192GB | 70B+ (4bit) |
| MacBook Pro | 18-128GB | Varies by config |

### 3. Enable Continuous Batching

For multi-user scenarios, continuous batching significantly improves throughput:

```bash
vllm-mlx serve mlx-community/Llama-3.2-3B-Instruct-4bit \
  --port 8000 \
  --continuous-batching
```

### 4. Deploy Multiple Instances

Run different models on separate vLLM-MLX instances and let Olla handle routing:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://mac-mini-1:8000"
        name: "mlx-coding"
        type: "vllm-mlx"
        priority: 90

      - url: "http://mac-mini-2:8000"
        name: "mlx-general"
        type: "vllm-mlx"
        priority: 85

proxy:
  load_balancer: "priority"
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/vllm-mlx/v1",
    api_key="not-needed"  # vLLM-MLX doesn't require API keys
)

response = client.chat.completions.create(
    model="mlx-community/Llama-3.2-3B-Instruct-4bit",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### Claude Code

```bash
# Set Olla as the Anthropic endpoint for Claude Code
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"

# Claude Code will use passthrough mode automatically
claude
```

### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:40114/olla/vllm-mlx/v1",
    api_key="not-needed",
    model="mlx-community/Llama-3.2-3B-Instruct-4bit",
    temperature=0.7
)
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise vLLM-MLX behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple vLLM-MLX instances
- [API Translation](../../concepts/api-translation.md) - Anthropic passthrough and translation modes
