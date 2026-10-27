---
title: oMLX Integration - Multi-Model Apple Silicon Inference with Olla
description: Configure oMLX with Olla proxy for multi-model LLM serving on Apple Silicon. Concurrent models, tiered hot/cold KV cache, native Anthropic Messages API, and OpenAI compatibility.
keywords: oMLX, Olla proxy, Apple Silicon, MLX, multi-model, KV cache, M1, M2, M3, M4, macOS, Anthropic, Claude Code
---

# oMLX Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://omlx.ai/">omlx.ai</a> (source: <a href="https://github.com/jundot/omlx">github.com/jundot/omlx</a>)</td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.28</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>omlx</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>omlx.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/omlx.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Health Check (native)</li>
                <li>Model Unification</li>
                <li>Model Detection &amp; Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>Native Anthropic Messages API</li>
                <li>Embeddings API</li>
                <li>Reranking API</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Native Token Counting (Olla uses the local estimator; oMLX's <code>/v1/messages/count_tokens</code> is not yet forwarded)</li>
                <li>Model Load/Unload Control (oMLX manages residency itself; Olla does not proxy lifecycle endpoints)</li>
                <li>Prometheus Metrics</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>Apple Silicon Only (M1/M2/M3/M4, macOS 15.0+)</li>
                <li>MLX Framework Acceleration</li>
                <li>Unified Memory Architecture</li>
                <li>Multi-Model Server (concurrent, lazy-loaded)</li>
                <li>Tiered KV Cache (hot RAM + cold SSD)</li>
                <li>LRU/TTL Eviction &amp; Model Pinning</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/omlx</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
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

oMLX is a multi-model inference server for Apple Silicon, managed from the macOS menu bar. Unlike single-model MLX servers, a single oMLX instance serves many models concurrently, loading them on demand and evicting the least-recently-used ones when memory runs low. Because it is OpenAI-compatible on the wire, Olla reuses the standard OpenAI parser and forwards requests with no translation overhead.

## Configuration

### Basic Setup

Add oMLX to your Olla configuration. A single endpoint exposes every model the server has discovered:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "local-omlx"
        type: "omlx"
        priority: 75
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

!!! tip "Allow for cold starts"
    oMLX loads models lazily, so the first request for a model that is not resident triggers a load that can take several seconds. The profile defaults to a 3-minute timeout to absorb this. Pin frequently used models in the oMLX admin panel to avoid cold starts on hot paths.

### Apple Silicon Network Setup

Place multiple Macs behind Olla and balance across them. Because each oMLX instance is multi-model, you do not need one endpoint per model:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://mac-studio:8000"
        name: "omlx-studio"
        type: "omlx"
        priority: 90
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s

      - url: "http://mac-mini:8000"
        name: "omlx-mini"
        type: "omlx"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s

proxy:
  engine: "olla"        # High-performance engine
  load_balancer: "priority"
```

## Anthropic Messages API Support

oMLX natively implements the Anthropic Messages API, so Olla forwards Anthropic-format requests directly without the Anthropic-to-OpenAI-to-Anthropic translation round trip (passthrough mode).

When Olla detects native Anthropic support (via the `anthropic_support` section in `config/profiles/omlx.yaml`), it bypasses the translation pipeline and sends requests straight to `/v1/messages` on the backend.

**Profile configuration** (from `config/profiles/omlx.yaml`):

```yaml
api:
  anthropic_support:
    enabled: true
    messages_path: /v1/messages
    token_count: false
```

**Key details**:

- Passthrough mode is automatic -- no client-side configuration needed
- Responses include the `X-Olla-Mode: passthrough` header when passthrough is active
- Falls back to translation mode if passthrough conditions are not met
- Token counting (`/v1/messages/count_tokens`): oMLX implements this natively, but Olla currently answers token-count requests with its local estimator rather than forwarding them, so `token_count` is left `false`

oMLX also ships a Claude Code context-scaling mode that rescales reported token counts so auto-compact fires at the right time on smaller-context models. This pairs well with pointing Claude Code at Olla's Anthropic endpoint.

For more information, see [API Translation](../../concepts/api-translation.md#passthrough-mode) and [Anthropic API Reference](../../api-reference/anthropic.md).

## Endpoints Supported

The following endpoints are supported by the oMLX integration profile:

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
    <td>List Models (OpenAI format; returns aliases where configured)</td>
  </tr>
  <tr>
    <td><code>/v1/models/status</code></td>
    <td>Loaded-model state (oMLX-specific: residency, size, last access)</td>
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
  <tr>
    <td><code>/v1/rerank</code></td>
    <td>Reranking API (Cohere/Jina-compatible)</td>
  </tr>
  <tr>
    <td><code>/v1/messages</code></td>
    <td>Anthropic Messages API (native passthrough)</td>
  </tr>
  <tr>
    <td><code>/v1/messages/count_tokens</code></td>
    <td>Anthropic token count -- forwarded to oMLX via this prefix; the <code>/olla/anthropic</code> route uses Olla's local estimator (<code>token_count: false</code>)</td>
  </tr>
  <tr>
    <td><code>/v1/responses</code></td>
    <td>OpenAI Responses API</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/omlx/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-7B-Instruct-4bit",
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
curl -X POST http://localhost:40114/olla/omlx/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-7B-Instruct-4bit",
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
    "model": "Qwen2.5-7B-Instruct-4bit",
    "max_tokens": 500,
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### Reranking

```bash
curl -X POST http://localhost:40114/olla/omlx/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/bge-reranker-base",
    "query": "What is unified memory?",
    "documents": [
      "Apple Silicon shares memory between CPU and GPU.",
      "MLX is an array framework for Apple Silicon.",
      "Discrete GPUs use separate VRAM."
    ],
    "top_n": 2
  }'
```

### Loaded State, Models and Health

```bash
# List available models (aliases shown where configured)
curl http://localhost:40114/olla/omlx/v1/models

# Inspect which models are currently resident in memory
curl http://localhost:40114/olla/omlx/v1/models/status

# Check health status
curl http://localhost:40114/olla/omlx/health
```

## oMLX Specifics

### Multi-Model Serving

A single oMLX instance hosts many models at once and manages residency automatically. This is the key difference from single-model MLX servers and shapes how you configure Olla:

- **Lazy loading**: models load on first request. Expect a cold-start delay the first time a model is used after startup or eviction.
- **LRU eviction**: the least-recently-used model is unloaded automatically when memory runs low.
- **Model pinning**: pin frequently used models in the admin panel so they stay resident.
- **Per-model TTL**: set an idle timeout per model to auto-unload after inactivity.
- **Process memory enforcement**: a total memory ceiling (default: system RAM minus 8GB) prevents system-wide out-of-memory conditions.

Because the server already discovers and exposes every model through `/v1/models`, you typically need **one Olla endpoint per oMLX instance**, not one per model.

### Tiered KV Cache (Hot + Cold)

oMLX keeps KV cache blocks across two tiers: a hot tier in RAM and a cold tier on SSD (safetensors). When the hot cache fills, blocks spill to disk and are restored from a matching prefix on the next request instead of being recomputed -- even after a server restart. This makes multi-turn, tool-heavy workloads (such as coding agents) far cheaper to resume.

### Model Naming and Aliases

oMLX discovers models from subdirectories, so model IDs are directory names such as `Qwen2.5-7B-Instruct-4bit`, or a custom alias configured per model in the admin panel.

!!! note "Aliases vs directory names"
    `/v1/models` returns the **alias** when one is configured, and requests accept both the alias and the directory name. The oMLX-specific `/v1/models/status` endpoint reports residency keyed by **directory name**. Olla discovers models from `/v1/models`, so the names you see in [unified models](../../concepts/model-unification.md) are the aliases.

### Resource Configuration

The oMLX profile uses Apple Silicon-oriented defaults. Concurrency is deliberately conservative because several models may share unified memory simultaneously. These values are metadata only: Olla does not enforce them, and oMLX's own settings limit parallelism:

```yaml
characteristics:
  timeout: 3m                 # Absorbs cold-start model loads
  max_concurrent_requests: 4  # not enforced
  streaming_support: true

resources:
  defaults:
    requires_gpu: false       # Unified memory, no discrete GPU
```

### Memory Requirements

Unified memory is shared between macOS and every loaded model. Because oMLX holds several models at once, size your Mac for the *sum* of the models you intend to keep resident, plus headroom for the OS:

| Mac unified memory | Comfortable resident set | Notes |
|--------------------|--------------------------|-------|
| 16GB | One 7-8B (4bit) model | Pin one model; expect evictions |
| 32GB | A 7-8B model plus an embedding/rerank model | Good for a single-user coding setup |
| 64GB | Several 7-13B models, or one 30B (4bit) | Comfortable multi-model |
| 128GB+ | A 70B (4bit) model with room for helpers | Mac Studio territory |

## Starting oMLX Server

oMLX runs only on Apple Silicon Macs (M1/M2/M3/M4) with macOS 15.0+ (Sequoia) and Python 3.10+.

### macOS App

Download the `.dmg` from [Releases](https://github.com/jundot/omlx/releases), drag oMLX to Applications, and launch it. The welcome flow walks through choosing a model directory, starting the server, and downloading a first model. The server listens on `http://localhost:8000` by default.

### Homebrew

```bash
brew tap jundot/omlx https://github.com/jundot/omlx
brew install omlx

# Run as a managed background service (auto-restarts on crash)
omlx start
```

### From Source

```bash
git clone https://github.com/jundot/omlx.git
cd omlx
pip install -e .

# Foreground server attached to this terminal
omlx serve --model-dir ~/models
```

The server auto-discovers LLMs, VLMs, embedding models, and rerankers from subdirectories of the model directory. Any OpenAI-compatible client can then connect to `http://localhost:8000/v1`.

## Profile Customisation

To customise oMLX behaviour, create `config/profiles/omlx-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: omlx
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - omlx
    - mlx       # Add an alternate prefix

# Allow longer cold starts for very large models
characteristics:
  timeout: 5m

# Raise concurrency on a high-memory Mac (metadata only, not enforced)
resources:
  concurrency_limits:  # metadata only, not enforced
    - min_memory_gb: 0
      max_concurrent: 8
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Troubleshooting

### Apple Silicon and macOS Version

**Issue**: oMLX fails to start or install

**Solution**: oMLX requires an Apple Silicon Mac running macOS 15.0+ (Sequoia). It does not run on Intel Macs, Linux, or Windows. Verify your hardware:

```bash
sysctl -n machdep.cpu.brand_string   # Should show "Apple M1", "Apple M2", etc.
sw_vers -productVersion              # Should be 15.0 or later
```

### Cold-Start Latency

**Issue**: The first request to a model is slow or times out

**Solution**: oMLX loads models lazily, so the first request after startup or eviction pays a load cost. Either raise the timeout or keep the model resident:

```yaml
characteristics:
  timeout: 5m

resources:
  timeout_scaling:
    base_timeout_seconds: 300
    load_time_buffer: true
```

Pin the model in the oMLX admin panel so it stays loaded between requests.

### Eviction Thrashing

**Issue**: Models keep unloading and reloading, hurting latency

**Solution**: Too many models are competing for unified memory. Reduce the resident set:

1. Pin only the models you use most
2. Set a per-model TTL so idle models unload cleanly
3. Choose smaller quantisations (e.g. 4bit) to fit more models
4. Raise the process memory ceiling only if you have headroom for macOS

### Model Name Not Found

**Issue**: A request returns a model-not-found error even though the model exists

**Solution**: oMLX accepts both the alias and the directory name, but the name Olla advertises in `/olla/models` is whatever `/v1/models` returns (the alias when configured). List the models through Olla and use the name shown:

```bash
curl http://localhost:40114/olla/omlx/v1/models
```

## Best Practices

### 1. One Endpoint per Instance

Because oMLX is multi-model, configure a single Olla endpoint per server and let oMLX manage model residency. Avoid creating one endpoint per model.

### 2. Pin Your Hot Models

Pin the models on your critical paths (for example, the model behind your coding agent) so they never pay a cold-start cost.

### 3. Size for the Resident Set

Plan memory around the *sum* of models you keep loaded, not the largest single model, and leave several GB of headroom for macOS.

### 4. Use the Olla Engine for Multiple Instances

When balancing across several Macs, use the `olla` engine with the `priority` load balancer to prefer your fastest hardware.

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/omlx/v1",
    api_key="not-needed"  # oMLX does not require API keys by default
)

response = client.chat.completions.create(
    model="Qwen2.5-7B-Instruct-4bit",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### Claude Code

```bash
# Point Claude Code at Olla's Anthropic endpoint
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"

# Requests use passthrough mode to oMLX automatically
claude
```

### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:40114/olla/omlx/v1",
    api_key="not-needed",
    model="Qwen2.5-7B-Instruct-4bit",
    temperature=0.7
)
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise oMLX behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple oMLX instances
- [API Translation](../../concepts/api-translation.md) - Anthropic passthrough and translation modes

## Further Reading

- TensorFoundry's [Run MLX Behind Olla on Your Mac](https://tensorfoundry.io/blog/running-mlx-with-olla) - a full walkthrough of this exact setup
- TensorFoundry's [How MLX Runs LLMs on Apple Silicon](https://tensorfoundry.io/blog/mlx-apple-silicon) - background on the MLX framework
