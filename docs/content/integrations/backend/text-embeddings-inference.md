---
title: Text Embeddings Inference Integration - High-Performance Embedding Serving with Olla
description: Configure Hugging Face Text Embeddings Inference (TEI) with the Olla proxy for embedding, classification and reranker workloads. Single-model instances discovered via /info.
keywords: TEI, Text Embeddings Inference, Olla proxy, embeddings, reranking, sentence-transformers, Hugging Face
---

# Text Embeddings Inference Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/huggingface/text-embeddings-inference">github.com/huggingface/text-embeddings-inference</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.30</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>text-embeddings-inference</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>text-embeddings-inference.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/text-embeddings-inference.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding (all native endpoints verbatim)</li>
                <li>Health Check (native)</li>
                <li>Model Discovery via /info</li>
                <li>OpenAI API Compatibility (/v1/embeddings)</li>
                <li>Embeddings, Reranking and Classification APIs</li>
                <li>Prometheus Metrics</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Chat / text generation (TEI serves embedding, classifier and reranker models only)</li>
                <li>Model Management (loading/unloading)</li>
                <li>Instance Management</li>
                <li>Model Download</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>Single model per instance</li>
                <li>CPU and GPU inference</li>
                <li>OpenAI-compatible embeddings</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/text-embeddings-inference</code></li>
                <li><code>/tei</code></li>
            </ul>
            (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)
        </td>
    </tr>
    <tr>
        <th>Endpoints</th>
        <td>
            See <a href="#endpoints-supported">below</a>
        </td>
    </tr>
</table>

## How TEI differs from LLM backends

TEI serves a single model per instance (an embedding, sequence-classification or reranker model) and does not expose an OpenAI-style `/v1/models` list. Instead, Olla discovers the served model from the `/info` endpoint (`served_model_name`, falling back to `model_id`) using a dedicated parser. Every native TEI endpoint is forwarded verbatim by the provider catch-all route; there are no per-endpoint handlers.

Chat and generation traffic does not reach a TEI endpoint for two reasons: the TEI profile declares no chat or completion paths, so those requests are never sent to TEI instances, and a request naming a chat model is routed by model name to the endpoints hosting that model. [Model kinds and routing](#model-kinds-and-routing) covers how embed, rerank and classify requests find the right instance.

## Configuration

### Basic Setup

Add a TEI instance to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:3000"
        name: "local-tei"
        type: "text-embeddings-inference"
        priority: 80
        model_url: "/info"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

!!! tip "Authenticated TEI"
    TEI deployments behind a gateway can require a bearer token (for example an `HF_TOKEN`). Configure the matching credential with `auth.type: bearer` on the endpoint. See [Endpoint Authentication](../../configuration/endpoint-auth.md) for details.

## Endpoints Supported

The following endpoints are forwarded verbatim to the TEI backend by the provider catch-all route:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/v1/embeddings</code></td>
    <td>Embeddings (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/embed</code></td>
    <td>Native embeddings (primary)</td>
  </tr>
  <tr>
    <td><code>/embed_all</code></td>
    <td>Per-token embeddings without pooling</td>
  </tr>
  <tr>
    <td><code>/embed_sparse</code></td>
    <td>Sparse (SPLADE) embeddings</td>
  </tr>
  <tr>
    <td><code>/tokenize</code></td>
    <td>Encode text to tokens</td>
  </tr>
  <tr>
    <td><code>/decode</code></td>
    <td>Decode token IDs to text</td>
  </tr>
  <tr>
    <td><code>/predict</code></td>
    <td>Sequence classification predictions</td>
  </tr>
  <tr>
    <td><code>/rerank</code></td>
    <td>Cross-encoder rerank scores</td>
  </tr>
  <tr>
    <td><code>/similarity</code></td>
    <td>Sentence similarity scores</td>
  </tr>
  <tr>
    <td><code>/info</code></td>
    <td>Server info, carries the served model ID (discovery path)</td>
  </tr>
  <tr>
    <td><code>/health</code></td>
    <td>Health check</td>
  </tr>
  <tr>
    <td><code>/metrics</code></td>
    <td>Prometheus metrics</td>
  </tr>
</table>

Olla additionally serves an OpenAI-format model listing at `/v1/models`. It lists the models discovered via `/info` on every healthy TEI endpoint, not just one instance.

## Usage Examples

### Embeddings (OpenAI format)

```bash
curl -X POST http://localhost:40114/olla/tei/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Snowflake/snowflake-arctic-embed-l-v2.0",
    "input": "Hello, world!"
  }'
```

### Native embeddings

```bash
curl -X POST http://localhost:40114/olla/tei/embed \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["Hello, world!", "How are you?"]
  }'
```

### Reranking

```bash
curl -X POST http://localhost:40114/olla/tei/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is machine learning?",
    "texts": [
      "Machine learning is a subset of artificial intelligence",
      "The weather today is sunny"
    ]
  }'
```

### Server info and health

```bash
# Served model metadata (discovery path)
curl http://localhost:40114/olla/tei/info

# Health status
curl http://localhost:40114/olla/tei/health

# Prometheus metrics
curl http://localhost:40114/olla/tei/metrics
```

## TEI Specifics

### Single model per instance

Each TEI instance serves exactly one model. When Olla discovers the instance it registers that single model (the `served_model_name`, or `model_id` as a fallback). TEI itself ignores the `model` field and answers with the one model it loaded, but Olla's routing does not: it picks the backend by the requested name. Under the default `strict` model routing (`model_registry.routing_strategy.type` in `config.yaml`) a name Olla has not discovered returns 404, so use the model the instance reports in `/info`, or one of its aliases. Model-less TEI requests (`/embed`, `/rerank`, `/predict` and the like carry no `model` field) are routed by kind instead.

### Model discovery via /info

Unlike LLM backends, TEI has no `/v1/models` listing. Olla's dedicated parser reads the served model from `/info`, so `model_url` should point at `/info` (the profile default).

### Model kinds and routing

Olla reads each instance's kind from `/info`'s `model_type` and gives the model a matching type and capabilities. None of them include `text-generation`.

| TEI kind | Olla model type | Capabilities |
|----------|-----------------|--------------|
| `embedding` | `embeddings` | `embeddings`, `similarity`, `vector-search` |
| `reranker` | `reranker` | `rerank`, `classification` |
| `classifier` | `classifier` | `classification` |

An instance whose `/info` has no `model_type` (older TEI) or a kind Olla does not know is treated as an embedding model.

TEI's native request bodies carry no `model` field, so for those requests Olla picks the instance from the request path:

| Request path | Routed to an instance serving |
|--------------|-------------------------------|
| `/embed`, `/embed_all`, `/embed_sparse`, `/similarity` | an embedding model |
| `/rerank` | a reranker model |
| `/predict` | a classifier or reranker model |

This applies to the unified route (`/olla/proxy/rerank`) and the provider route (`/olla/tei/rerank`) alike, so a fleet can mix embedding, reranker and classifier instances behind one Olla. A request that names a model (for example `/v1/embeddings` with `"model": "..."`) is routed by that model name instead, and capability is not consulted.

If no healthy endpoint hosts a model of the needed kind, Olla does not reject the request: it falls back to every compatible endpoint, as it does for other capability-based routing. TEI then answers `424` when the instance it reaches serves a different kind, for example `/rerank` sent to an embedding instance. The same `424` applies to a request you pin to a specific instance yourself.

The kind is also visible through Olla: `/olla/models?type=reranker` and `?type=classifier` filter by it, and each model's `capabilities` list shows `rerank` and/or `classification`.

## Starting a TEI Server

TEI is distributed as a container image and a standalone binary. The container exposes port `80`; the binary listens on port `3000` by default.

### Docker

```bash
model=sentence-transformers/all-MiniLM-L6-v2
volume=$PWD/data   # TEI caches weights under /data

docker run -p 3000:80 -v $volume:/data --pull always \
  ghcr.io/huggingface/text-embeddings-inference:cpu-1.9 --model-id $model
# GPU: use cuda-1.9 (or an arch tag such as 89-1.9 or hopper-1.9) with --gpus all
```

Point the Olla endpoint `url` at the published port (for example `http://localhost:3000`).

## Monitoring

### Prometheus Metrics

TEI exposes metrics at `/metrics`, reachable through Olla at `/olla/tei/metrics`:

```yaml
scrape_configs:
  - job_name: 'tei'
    static_configs:
      - targets: ['localhost:40114']
    metrics_path: '/olla/tei/metrics'
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise TEI behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple TEI instances
- [Monitoring](../../configuration/practices/monitoring.md) - Set up Prometheus monitoring
