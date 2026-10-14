# ollamock

A multi-protocol mock LLM backend for end-to-end validation of Olla's proxy and routing logic without requiring real inference infrastructure.

It speaks Ollama, LM Studio, OpenAI-compatible, Lemonade, Anthropic, and Text Embedding Inference (TEI) wire formats and supports controllable fault injection via an HTTP control plane.

## Running

```bash
go run ./test/cmd/ollamock
```

With all flags:

```bash
go run ./test/cmd/ollamock \
  --addr 127.0.0.1:19431 \
  --name mock-a \
  --models llama3.2,phi4 \
  --ttft-ms 50 \
  --tps 20 \
  --stream-chunks 5
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `127.0.0.1:19431` | Listen address |
| `--name` | `mock-a` | Instance marker embedded in every response body and `X-Ollamock-Instance` header |
| `--models` | `test-model` | Comma-separated model names served in all list endpoints |
| `--ttft-ms` | `0` | Delay before first streamed byte (milliseconds) |
| `--tps` | `0` | Token-per-second pacing for stream chunks (0 = instant) |
| `--stream-chunks` | `5` | Number of content chunks per streamed response |

## Protocol Endpoints

| Route | Protocol |
|-------|----------|
| `GET /health` | Health check |
| `GET /` | Ollama root liveness (`"Ollama is running"`) |
| `GET /api/tags` | Ollama model listing |
| `GET /api/version` | Ollama version |
| `POST /api/chat` | Ollama chat (streaming default) |
| `POST /api/generate` | Ollama generate (streaming default) |
| `GET /api/v0/models` | LM Studio model listing |
| `GET /v1/models` | OpenAI-compatible model listing |
| `POST /v1/chat/completions` | OpenAI chat completions |
| `POST /v1/completions` | OpenAI text completions (legacy) |
| `GET /api/v1/models` | Lemonade model listing |
| `POST /api/v1/chat/completions` | Lemonade chat (OpenAI shape) |
| `POST /v1/messages` | Anthropic Messages API |
| `GET /info` | TEI server info (carries the served model name) |
| `POST /v1/embeddings` | TEI embeddings (OpenAI shape, single model per instance) |
| `POST /embed` | TEI native embeddings (bare vector arrays) |
| `POST /embed_all` | TEI per-token embeddings without pooling |
| `POST /embed_sparse` | TEI sparse (SPLADE) embeddings |
| `POST /tokenize` | TEI tokenise inputs to token IDs |
| `POST /decode` | TEI decode token IDs to text |
| `POST /predict` | TEI sequence classification predictions |
| `POST /rerank` | TEI cross-encoder rerank scores |
| `POST /similarity` | TEI sentence similarity scores |
| `GET /metrics` | TEI Prometheus metrics (mock counters) |

## Behaviour Modes

| Mode | Description |
|------|-------------|
| `ok` | Normal operation (default) |
| `error` | All requests return `error_status` with a mock error body |
| `flaky` | Returns errors randomly at `error_rate` frequency |
| `hang` | Blocks all requests indefinitely (until client disconnects) |
| `slow` | Adds `latency_ms` delay to every request |

## Behaviour Fields

| Field | Type | Description |
|-------|------|-------------|
| `mode` | string | One of the modes above |
| `error_status` | int | HTTP status for error/flaky modes (default 500) |
| `error_rate` | float | Probability of error in flaky mode, 0.0–1.0 (default 0.5) |
| `latency_ms` | int | Additional latency in slow mode (milliseconds) |
| `fail_health` | bool | Return 503 on `/health` and `/` only |
| `drop_mid_stream` | bool | Close connection after half the stream chunks |
| `malformed_json` | bool | Return truncated JSON (`{"broken":`) with 200 status |

## Control Plane

All `/_mock/*` routes are immune to behaviour - they always respond normally.

### Get current behaviour

```bash
curl http://127.0.0.1:19431/_mock/behaviour
```

### Set behaviour (partial merge)

```bash
# Switch to error mode
curl -X POST http://127.0.0.1:19431/_mock/behaviour \
  -H 'Content-Type: application/json' \
  -d '{"mode":"error","error_status":503}'

# Make it flaky at 30% error rate
curl -X POST http://127.0.0.1:19431/_mock/behaviour \
  -d '{"mode":"flaky","error_rate":0.3}'

# Add 200ms latency
curl -X POST http://127.0.0.1:19431/_mock/behaviour \
  -d '{"mode":"slow","latency_ms":200}'

# Fail only health checks (circuit-breaker testing)
curl -X POST http://127.0.0.1:19431/_mock/behaviour \
  -d '{"fail_health":true}'
```

### Reset to defaults

```bash
curl -X POST http://127.0.0.1:19431/_mock/reset
```

### Get request stats

```bash
curl http://127.0.0.1:19431/_mock/stats
# {"total":42,"by_path":{"/v1/chat/completions":30,"/api/tags":12}}
```

## Example Requests

```bash
# Ollama model list
curl http://127.0.0.1:19431/api/tags

# OpenAI model list
curl http://127.0.0.1:19431/v1/models

# OpenAI non-streaming chat
curl -X POST http://127.0.0.1:19431/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"test-model","stream":false,"messages":[{"role":"user","content":"hello"}]}'

# OpenAI streaming chat
curl -X POST http://127.0.0.1:19431/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"test-model","stream":true,"messages":[{"role":"user","content":"hello"}]}'

# Ollama streaming chat (stream: absent defaults to true)
curl -X POST http://127.0.0.1:19431/api/chat \
  -d '{"model":"test-model","messages":[{"role":"user","content":"hello"}]}'

# Anthropic non-streaming
curl -X POST http://127.0.0.1:19431/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"model":"test-model","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}'

# Anthropic streaming
curl -X POST http://127.0.0.1:19431/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"model":"test-model","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"hello"}]}'
```

## Running Multiple Instances

For load-balancer and routing tests, run several instances on different ports:

```bash
go run ./test/cmd/ollamock --addr 127.0.0.1:19431 --name mock-a --models llama3.2 &
go run ./test/cmd/ollamock --addr 127.0.0.1:19432 --name mock-b --models llama3.2,phi4 &
go run ./test/cmd/ollamock --addr 127.0.0.1:19433 --name mock-c --models phi4 &
```

Each instance embeds its name in every response body (`BACKEND:mock-a`) and in the `X-Ollamock-Instance` header, so the validation harness can confirm which backend served each request.
