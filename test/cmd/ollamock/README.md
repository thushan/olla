# ollamock

A multi-protocol mock LLM backend for end-to-end validation of Olla's proxy and routing logic without requiring real inference infrastructure.

It speaks Ollama, LM Studio, OpenAI-compatible, Lemonade, Anthropic, and Text Embeddings Inference (TEI) wire formats and supports controllable fault injection via an HTTP control plane.

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
| `--tei-kind` | `embedding` | Kind a TEI instance reports in `GET /info` `model_type` and serves: `embedding`, `reranker` or `classifier`. Routes for another kind answer 424 |
| `--loaded` | (none) | Comma-separated models `GET /api/ps` reports as resident; empty reports none. Lets a fleet of instances hold different loaded sets |
| `--tags-capabilities` | `false` | Emit per-model `capabilities` in `GET /api/tags`, as Ollama 0.30+ does: `["embedding"]` for names that look like embedders, otherwise `["completion"]` plus `"vision"` for names such as `qwen3-vl` or `llava` and `"tools"` for names such as `llama3.2` or `qwen3`. Off omits the field, which is what an older Ollama sends |

## Protocol Endpoints

| Route | Protocol |
|-------|----------|
| `GET /health` | Health check |
| `GET /` | Ollama root liveness (`"Ollama is running"`) |
| `GET /api/tags` | Ollama model listing (`capabilities` with `--tags-capabilities`) |
| `GET /api/ps` | Ollama loaded models (from `--loaded`) |
| `GET /api/version` | Ollama version |
| `POST /api/chat` | Ollama chat (streaming default) |
| `POST /api/generate` | Ollama generate (streaming default) |
| `POST /api/embed` | Ollama native embeddings (`input` string or array). Empty or absent input is a 200 with empty `embeddings`; a named model the instance does not serve is a 404; a body with no model is served |
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
| `POST /tokenize` | TEI tokenise inputs to token objects |
| `POST /decode` | TEI decode token IDs to text |
| `POST /predict` | TEI sequence classification predictions |
| `POST /rerank` | TEI cross-encoder rerank scores |
| `POST /similarity` | TEI sentence similarity scores |
| `GET /metrics` | TEI Prometheus metrics (mock counters) |

## TEI error contract

The TEI routes follow real TEI's error shapes, so routing and client tests see what production would:

| Condition | Status | Body |
|-----------|--------|------|
| Empty input: `inputs: []`, `inputs: ""`, empty `texts`, empty `sentences`, an empty string in any of them (a rerank or predict pair only when both halves are empty) | 400 | `Empty` |
| Wrong shape: missing field, wrong JSON type | 422 | axum's plain text, `Failed to deserialize the JSON body into the target type: ...` |
| Not JSON at all | 400 | axum's plain text, `Failed to parse the request body as JSON: ...` |
| Route needs another kind (`/embed` on a reranker, `/rerank` on an embedder, `/predict` on an embedder) | 424 | `Backend`, `Backend error: Model is not an embedding model` (re-ranker: lower-case `model is not a re-ranker model`, as upstream writes it) |

Handler errors return `{"error": "...", "error_type": "..."}`; `POST /v1/embeddings` returns the OpenAI-compatible `{"message": "...", "code": 400, "type": "..."}` (the key is `type` there, not `error_type`). The order matches upstream: shape, then an empty array (400), then the kind check (424), then the tokeniser, which is where an empty string is found. So `{"inputs":""}` sent to the wrong kind is 424, while `{"inputs":[]}` is 400 on every kind. Whitespace-only text and a blank rerank `query` are valid input. The 415 for a missing `Content-Type: application/json` is not modelled. `/decode` only checks shape: upstream reads `ids: []` as a single empty sequence, so it decodes rather than failing. `/tokenize`, `/decode`, `/info`, `/health` and `/metrics` work for every kind.

## TEI request and response shapes

The embedding routes (`/embed`, `/embed_all`, `/embed_sparse`, and `/v1/embeddings` through its `input` field) take a string, an array of strings, an array of token IDs (one sequence) or an array of token-ID arrays (a batch). Token IDs must be unsigned 32-bit integers, 0 to 4294967295. A negative, fractional or larger value, or a non-number in an ID array, answers 422. `/tokenize` takes strings only.

| Route | Mock behaviour |
|-------|----------------|
| `/tokenize` | One array per input: `[CLS]`, one token per whitespace-separated word, `[SEP]`. Each token is `{id, text, special, start, stop}`; `start` and `stop` are byte offsets, and null on the special tokens. IDs are deterministic |
| `/decode` | `ids` is one sequence or a batch. Returns placeholder strings (`decoded-0`, `decoded-1`), not real text |
| `/predict` | Served by classifier and reranker instances. A string, `["a"]` or a pair `["a","b"]` is one sequence and returns a flat `[{score, label}]`; only an array of arrays is a batch, returning one such array per sequence. Each is sorted by score, descending |
| `/rerank` | Reranker instances only. Returns `[{index, score}]` in input order with descending scores |
| `/similarity` | Embedding instances only. Returns one score per sentence |

Run one instance per kind to model a mixed TEI fleet:

```bash
go run ./test/cmd/ollamock --addr 127.0.0.1:19438 --name mock-tei-embed  --models BAAI/bge-small-en-v1.5 --tei-kind embedding
go run ./test/cmd/ollamock --addr 127.0.0.1:19439 --name mock-tei-rerank --models BAAI/bge-reranker-base  --tei-kind reranker
go run ./test/cmd/ollamock --addr 127.0.0.1:19440 --name mock-tei-class  --models SamLowe/roberta-go-emotions --tei-kind classifier
```

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
