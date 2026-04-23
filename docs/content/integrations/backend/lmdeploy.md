---
title: LMDeploy Integration - High-Performance LLM Inference with Olla
description: Configure LMDeploy with Olla proxy for efficient LLM serving. TurboMind engine, OpenAI-compatible API, VLM support, and GPU-optimised inference from InternLM.
keywords: LMDeploy, Olla proxy, TurboMind, InternLM, LLM inference, GPU optimisation, high throughput
---

# LMDeploy Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/InternLM/lmdeploy">github.com/InternLM/lmdeploy</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.21</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>lmdeploy</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>lmdeploy.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/lmdeploy.yaml">latest</a>)</td>
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
                <li>Token Encoding API</li>
                <li>Reward/Score Pooling</li>
                <li>VLM Inference (same <code>api_server</code>)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li><code>/v1/embeddings</code> (returns HTTP 400 — use <code>/pooling</code>)</li>
                <li><code>proxy_server</code> component (no <code>/health</code> endpoint)</li>
                <li>Model Management (loading/unloading)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>OpenAI Compatible</li>
                <li>GPU Optimised (TurboMind C++/CUDA engine)</li>
                <li>Continuous Batching</li>
                <li>VLM Support</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/lmdeploy</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
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

Register an LMDeploy `api_server` instance with Olla:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:23333"
        name: "local-lmdeploy"
        type: "lmdeploy"
        priority: 82
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

The default port for `lmdeploy serve api_server` is **23333**. Register individual `api_server` instances directly — do not point Olla at the `proxy_server` component, which lacks a `/health` endpoint and only forwards a subset of routes.

### Authentication

LMDeploy supports optional Bearer-token authentication via the `--api-keys` flag. Configure the token in Olla's endpoint headers so it is forwarded on every proxied request:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-server:23333"
        name: "lmdeploy-prod"
        type: "lmdeploy"
        priority: 82
        health_check_url: "/health"
        check_interval: 10s
        check_timeout: 5s
        headers:
          Authorization: "Bearer ${LMDEPLOY_API_KEY}"
```

The `/health` endpoint is auth-exempt on LMDeploy, so health checks will succeed even when a key is required for inference.

### Multiple Instances

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu1:23333"
        name: "lmdeploy-1"
        type: "lmdeploy"
        priority: 100

      - url: "http://gpu2:23333"
        name: "lmdeploy-2"
        type: "lmdeploy"
        priority: 100

proxy:
  engine: "olla"
  load_balancer: "least-connections"
```

## Endpoints Supported

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
    <td><code>/v1/encode</code></td>
    <td>Token Encoding (LMDeploy-specific)</td>
  </tr>
  <tr>
    <td><code>/generate</code></td>
    <td>Native Generation Endpoint</td>
  </tr>
  <tr>
    <td><code>/pooling</code></td>
    <td>Reward/Score Pooling (not <code>/v1/embeddings</code>)</td>
  </tr>
  <tr>
    <td><code>/is_sleeping</code></td>
    <td>Sleep State Probe</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is TurboMind?"}
    ],
    "temperature": 0.7,
    "max_tokens": 300
  }'
```

### Streaming

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "messages": [{"role": "user", "content": "Write a short story"}],
    "stream": true,
    "temperature": 0.8
  }'
```

### Token Encoding

```bash
curl -X POST http://localhost:40114/olla/lmdeploy/v1/encode \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "input": "Hello, world!"
  }'
```

### Pooling (Reward/Score)

```bash
# Use /pooling — not /v1/embeddings (which returns HTTP 400)
curl -X POST http://localhost:40114/olla/lmdeploy/pooling \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internlm/internlm2_5-7b-chat",
    "input": "The quick brown fox"
  }'
```

## Starting LMDeploy

### Basic Start

```bash
pip install lmdeploy

lmdeploy serve api_server internlm/internlm2_5-7b-chat \
  --server-port 23333
```

### TurboMind Backend (Default, GPU)

```bash
lmdeploy serve api_server internlm/internlm2_5-7b-chat \
  --backend turbomind \
  --server-port 23333 \
  --tp 1
```

### PyTorch Backend

Use `pytorch` when a model is not supported by TurboMind, or for CPU inference:

```bash
lmdeploy serve api_server internlm/internlm2_5-7b-chat \
  --backend pytorch \
  --server-port 23333
```

### With Authentication

```bash
lmdeploy serve api_server internlm/internlm2_5-7b-chat \
  --server-port 23333 \
  --api-keys my-secret-key
```

### VLM Inference

Vision-language models use the same `api_server` entrypoint — no separate binary:

```bash
lmdeploy serve api_server InternLM/internlm-xcomposer2-7b \
  --server-port 23333
```

### Docker

```bash
docker run --gpus all \
  -p 23333:23333 \
  openmmlab/lmdeploy:latest \
  lmdeploy serve api_server internlm/internlm2_5-7b-chat \
  --server-port 23333
```

## LMDeploy Specifics

### Sleep/Wake

LMDeploy supports a sleep mode to release GPU memory when idle:

```bash
# Suspend the engine (GPU memory freed)
curl -X POST http://localhost:23333/sleep

# Resume the engine
curl -X POST http://localhost:23333/wakeup

# Check state (proxied via Olla)
curl http://localhost:40114/olla/lmdeploy/is_sleeping
```

Olla treats a sleeping engine as transiently unavailable and will route around it if other healthy instances exist. Once the engine wakes, health checks recover it automatically.

### Embeddings vs Pooling

LMDeploy does not implement `/v1/embeddings`. The correct path for reward-model scoring and embedding-style pooling is `/pooling`. This is a deliberate upstream design decision — using TurboMind's native pooling path rather than the OpenAI embeddings spec.

### Model Naming

LMDeploy serves models by their HuggingFace identifiers:

- `internlm/internlm2_5-7b-chat`
- `meta-llama/Meta-Llama-3.1-8B-Instruct`
- `mistralai/Mistral-7B-Instruct-v0.2`
- `Qwen/Qwen2.5-7B-Instruct`

### Proxy Server vs API Server

LMDeploy ships two server components:

| Component | Port | Use with Olla? |
|-----------|------|----------------|
| `api_server` | 23333 | Yes — has `/health`, full route support |
| `proxy_server` | 8000 | No — no `/health`, limited routes |

Always register individual `api_server` instances. The `proxy_server` is LMDeploy's own load balancer and is redundant when Olla is in the stack.

## Profile Customisation

Create `config/profiles/lmdeploy-custom.yaml` to override defaults. See [Profile Configuration](../../concepts/profile-system.md) for the full schema.

```yaml
name: lmdeploy
version: "1.0"

# Add a shorter routing prefix
routing:
  prefixes:
    - lmdeploy
    - turbomind

# Increase timeout for large 70B models
characteristics:
  timeout: 5m
```

## OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/lmdeploy/v1",
    api_key="not-needed"  # omit if no --api-keys set on lmdeploy
)

response = client.chat.completions.create(
    model="internlm/internlm2_5-7b-chat",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

## Next Steps

- [LMDeploy API Reference](../../api-reference/lmdeploy.md) - Endpoint details and response formats
- [Profile Configuration](../../concepts/profile-system.md) - Customise LMDeploy behaviour
- [Load Balancing](../../concepts/load-balancing.md) - Scale across multiple LMDeploy instances
- [Health Checking](../../concepts/health-checking.md) - Circuit breakers and failover
