# Rapid-MLX API

Proxy endpoints for the [Rapid-MLX](../integrations/backend/rapid-mlx.md) local inference server on Apple Silicon. Available through the `/olla/rapid-mlx/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/rapid-mlx/v1/models` | List discovered, healthy models in OpenAI format |
| POST | `/olla/rapid-mlx/v1/chat/completions` | Chat completion |
| POST | `/olla/rapid-mlx/v1/responses` | OpenAI Responses API |
| POST | `/olla/rapid-mlx/v1/embeddings` | Embeddings when an embedding model is served |
| POST | `/olla/rapid-mlx/v1/messages` | Native Anthropic Messages API |

Anthropic clients can also use `/olla/anthropic/v1/messages` with native passthrough. See the [Anthropic API Reference](anthropic.md). Token counting through Olla uses its local estimator.

## GET /olla/rapid-mlx/v1/models

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/models
```

Use the served model ID from this listing in generation requests, for example `mlx-community/Qwen3.5-4B-MLX-4bit`. Olla preserves positive `context_window` values from backend discovery as model context metadata. The provider listing is generated from Olla's model registry rather than forwarding the backend JSON unchanged.

The same backend path, `/v1/models`, is used for health checks.

## POST /olla/rapid-mlx/v1/chat/completions

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "mlx-community/Qwen3.5-4B-MLX-4bit",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 64
  }'
```

For streaming, add `"stream": true` and use `curl -N`.

## POST /olla/rapid-mlx/v1/responses

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/responses \
  -H 'Content-Type: application/json' \
  -d '{"model":"mlx-community/Qwen3.5-4B-MLX-4bit","input":"Hello!","max_output_tokens":64}'
```

## POST /olla/rapid-mlx/v1/embeddings

Requires a served embedding model. Replace `SERVED_EMBEDDING_MODEL_ID` with its ID from `/v1/models`:

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/embeddings \
  -H 'Content-Type: application/json' \
  -d '{"model":"SERVED_EMBEDDING_MODEL_ID","input":"Hello!"}'
```

## POST /olla/rapid-mlx/v1/messages

```bash
curl http://localhost:40114/olla/rapid-mlx/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{"model":"mlx-community/Qwen3.5-4B-MLX-4bit","max_tokens":64,"messages":[{"role":"user","content":"Hello!"}]}'
```

## Configuration Example

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

The direct OpenAI base URL is `http://localhost:8000/v1`. Use the server root in Olla's endpoint URL. Rapid-MLX selects the first free port in 8000-8009 unless `--port` is explicit; match Olla's URL to the selected port.

## Request and Response Headers

Olla forwards configured endpoint headers and adds `X-Olla-Request-ID` and `X-Forwarded-For`. Backend authentication is optional; see [Endpoint Authentication](../configuration/endpoint-auth.md) for bearer-token configuration.

Proxy responses include:

- `X-Olla-Endpoint`: selected endpoint name
- `X-Olla-Model`: model used for the request
- `X-Olla-Backend-Type`: `rapid-mlx`
- `X-Olla-Response-Time`: processing time

The Anthropic translator route also sets `X-Olla-Mode: passthrough` when native passthrough is active.
