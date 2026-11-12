---
title: Rapid-MLX Integration - Apple Silicon Inference with Olla
description: Configure Rapid-MLX with Olla for local MLX inference, OpenAI-compatible endpoints and native Anthropic Messages passthrough.
keywords: Rapid-MLX, Olla proxy, Apple Silicon, MLX, Anthropic, local LLM
---

# Rapid-MLX Integration

| Attribute | Value |
|-----------|-------|
| Home | [rapidmlx.com](https://rapidmlx.com) ([source](https://github.com/raullenchai/Rapid-MLX)) |
| Type | `rapid-mlx` |
| Profile | [`rapid-mlx.yaml`](https://github.com/thushan/olla/blob/main/config/profiles/rapid-mlx.yaml) |
| Prefix | `/olla/rapid-mlx` |
| Features | Proxy forwarding, model discovery and unification, health checks via model listing, OpenAI compatibility, native Anthropic Messages passthrough |
| Token counting | Olla's local estimator |

Rapid-MLX is an open-source (Apache-2.0) local LLM inference server for Apple Silicon, built on MLX. It supports continuous batching, prefix KV caching, tool-call parsing and reasoning parsing. A [native macOS desktop app](https://rapidmlx.com/desktop) is also available.

## Starting Rapid-MLX

```bash
brew install rapid-mlx
rapid-mlx serve qwen3.5-4b-4bit
```

Alternatively, use `pip install rapid-mlx` with Python 3.12+. Models are downloaded from Hugging Face into the standard Hugging Face cache.

The OpenAI base URL is `http://localhost:8000/v1`. With no `--port`, Rapid-MLX chooses the first free port in 8000-8009. If another server already uses 8000, update Olla's endpoint URL to match the selected port, or start Rapid-MLX with an explicit `--port` (which never falls back).

## Configuration

### Basic Setup

Add the server to Olla's configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "local-rapid-mlx"
        type: "rapid-mlx"
        priority: 75
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 5s
        check_timeout: 2s
```

Use the server root in `url`; Olla adds `/v1/models` for discovery and the request path when forwarding. Direct OpenAI clients use `http://localhost:8000/v1`; clients connecting through Olla use `http://localhost:40114/olla/rapid-mlx/v1`.

The profile uses the OpenAI parser and preserves the served model IDs and positive `context_window` metadata returned by `/v1/models`. Use the ID reported there, such as `mlx-community/Qwen3.5-4B-MLX-4bit`, in requests.

### Authentication

No API key is required by default. SDKs that require a key can use `not-needed`. If optional Rapid-MLX authentication is enabled with `--api-key` or `RAPID_MLX_API_KEY`, configure Olla's endpoint with a bearer token from the environment:

```yaml
        auth:
          type: bearer
          token: "${RAPID_MLX_API_KEY}"
```

See [Endpoint Authentication](../../configuration/endpoint-auth.md).

### Multiple Instances

Add one endpoint per server, each with `type: "rapid-mlx"`, a unique name and its server root URL. Use Olla's [load balancing](../../concepts/load-balancing.md) to route between them:

```yaml
proxy:
  engine: "olla"
  load_balancer: "priority"
```

The profile's timeout is an Olla default, not a Rapid-MLX server limit. See [Profile Configuration](../../concepts/profile-system.md) for customisation.

## Anthropic Messages API Support

The shipped profile enables native passthrough:

```yaml
api:
  anthropic_support:
    enabled: true
    messages_path: /v1/messages
    token_count: false
```

Requests to `/olla/anthropic/v1/messages` use passthrough when its conditions are met. Check for `X-Olla-Mode: passthrough`. Olla uses its local estimator for token counting.

```bash
curl http://localhost:40114/olla/anthropic/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{"model":"mlx-community/Qwen3.5-4B-MLX-4bit","max_tokens":64,"messages":[{"role":"user","content":"Hello!"}]}'
```

## Endpoints Supported

| Path | Description |
|------|-------------|
| `/v1/models` | Model discovery and health checks |
| `/v1/chat/completions` | OpenAI-compatible chat completions |
| `/v1/responses` | OpenAI Responses API |
| `/v1/embeddings` | Embeddings when an embedding model is served |
| `/v1/messages` | Native Anthropic Messages API |

See the [Rapid-MLX API reference](../../api-reference/rapid-mlx.md) for request examples.

## Usage Examples

### Chat Completion

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"mlx-community/Qwen3.5-4B-MLX-4bit","messages":[{"role":"user","content":"Hello!"}],"max_tokens":64}'
```

Add `"stream": true` to the request body for a streaming response and use `curl -N` to display it as it arrives.

### Models and Health

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/models
curl http://localhost:40114/internal/health
```

Olla probes `/v1/models` on the backend for health checks. The provider models route returns Olla's discovered, healthy models in OpenAI format.

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/rapid-mlx/v1",
    api_key="not-needed",
)
response = client.chat.completions.create(
    model="mlx-community/Qwen3.5-4B-MLX-4bit",
    messages=[{"role": "user", "content": "Hello!"}],
)
```

## Troubleshooting

- **Connection refused or wrong backend**: verify Rapid-MLX's selected port and Olla's endpoint URL. Port 8000 may already be in use by another local server.
- **Model not found**: list `/olla/rapid-mlx/v1/models` and use the served model ID.
- **Unauthorised**: if backend authentication is enabled, supply the bearer token through Olla's endpoint auth configuration.
- **Embeddings unavailable**: embeddings require an embedding model to be served.

## Next Steps

- [Rapid-MLX documentation](https://rapidmlx.com/docs/)
- [Profile Configuration](../../concepts/profile-system.md)
- [Model Unification](../../concepts/model-unification.md)
- [API Translation](../../concepts/api-translation.md)
