# Text Embeddings Inference API

Proxy endpoints for Hugging Face Text Embeddings Inference (TEI) servers. Available through the `/olla/tei/` prefix (or the longer `/olla/text-embeddings-inference/`). All native endpoints are forwarded verbatim to the backend; Olla additionally serves an OpenAI-format model listing at `/v1/models`.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/tei/info` | Server info, carries the served model ID |
| GET | `/olla/tei/health` | Health check |
| GET | `/olla/tei/metrics` | Prometheus metrics |
| GET | `/olla/tei/v1/models` | List models (OpenAI format, served by Olla) |
| POST | `/olla/tei/v1/embeddings` | Embeddings (OpenAI format) |
| POST | `/olla/tei/embed` | Native embeddings (primary) |
| POST | `/olla/tei/embed_all` | Per-token embeddings without pooling |
| POST | `/olla/tei/embed_sparse` | Sparse (SPLADE) embeddings |
| POST | `/olla/tei/tokenize` | Encode text to token IDs |
| POST | `/olla/tei/decode` | Decode token IDs to text |
| POST | `/olla/tei/predict` | Sequence classification predictions |
| POST | `/olla/tei/rerank` | Cross-encoder rerank scores |
| POST | `/olla/tei/similarity` | Sentence similarity scores |

---

## GET /olla/tei/info

Server metadata. This is TEI's discovery path: it carries the model the instance serves (`served_model_name`, falling back to `model_id`) rather than a list of models.

### Request

```bash
curl -X GET http://localhost:40114/olla/tei/info
```

### Response

```json
{
  "model_id": "/data/snowflake-arctic-embed-l-v2.0",
  "served_model_name": "Snowflake/snowflake-arctic-embed-l-v2.0",
  "model_dtype": "float16",
  "max_input_length": 8192,
  "model_type": {
    "embedding": {
      "pooling": "cls"
    }
  }
}
```

---

## GET /olla/tei/health

Check TEI server health status. Returns `200 OK` when healthy.

### Request

```bash
curl -X GET http://localhost:40114/olla/tei/health
```

---

## GET /olla/tei/metrics

Prometheus metrics in the standard text exposition format.

### Request

```bash
curl -X GET http://localhost:40114/olla/tei/metrics
```

---

## GET /olla/tei/v1/models

List models available on the TEI instance, in OpenAI format. Served by Olla from the model discovered via `/info` (TEI itself has no `/v1/models`).

### Request

```bash
curl -X GET http://localhost:40114/olla/tei/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "Snowflake/snowflake-arctic-embed-l-v2.0",
      "object": "model",
      "created": 1705334400,
      "owned_by": "text-embeddings-inference"
    }
  ]
}
```

---

## POST /olla/tei/v1/embeddings

OpenAI-compatible embeddings. Accepts a single string or an array of strings as `input`.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Snowflake/snowflake-arctic-embed-l-v2.0",
    "input": ["Hello, world!", "How are you?"]
  }'
```

### Response

```json
{
  "object": "list",
  "model": "Snowflake/snowflake-arctic-embed-l-v2.0",
  "data": [
    {"object": "embedding", "embedding": [0.01, 0.02, 0.03], "index": 0},
    {"object": "embedding", "embedding": [0.04, 0.05, 0.06], "index": 1}
  ],
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}
```

---

## POST /olla/tei/embed

Native embeddings endpoint (primary). Returns a bare array of embedding vectors.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/embed \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["Hello, world!", "How are you?"]
  }'
```

### Response

```json
[
  [0.01, 0.02, 0.03],
  [0.04, 0.05, 0.06]
]
```

---

## POST /olla/tei/embed_all

Per-token embeddings without pooling. Returns one vector per token for each input.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/embed_all \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["Hello, world!"]
  }'
```

### Response

```json
[
  [[0.10, 0.20], [0.30, 0.40], [0.50, 0.60]]
]
```

---

## POST /olla/tei/embed_sparse

Sparse (SPLADE) embeddings. Returns index/value pairs per input.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/embed_sparse \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["Hello, world!"]
  }'
```

### Response

```json
[
  {"indices": [15, 88], "values": [0.42, 0.17]}
]
```

---

## POST /olla/tei/tokenize

Encode text to token IDs.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/tokenize \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["Hello, world!"]
  }'
```

### Response

```json
{
  "tokens": [[15496, 99, 1625, 299]],
  "attention_mask": [[1, 1, 1, 1]]
}
```

---

## POST /olla/tei/decode

Decode token IDs back to text.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/decode \
  -H "Content-Type: application/json" \
  -d '{
    "ids": [[15496, 99, 1625, 299]]
  }'
```

### Response

```json
["hello , world !"]
```

---

## POST /olla/tei/predict

Sequence classification predictions. Returns labels and scores per input.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/predict \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": ["I love this product"]
  }'
```

### Response

```json
[
  {"labels": ["positive", "negative"], "scores": [0.97, 0.03]}
]
```

---

## POST /olla/tei/rerank

Cross-encoder rerank scores for a query against a set of texts.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cross-encoder/ms-marco-MiniLM-L-6-v2",
    "query": "What is machine learning?",
    "texts": [
      "Machine learning is a subset of artificial intelligence",
      "The weather today is sunny"
    ]
  }'
```

### Response

```json
[
  {"index": 0, "relevance_score": 0.91},
  {"index": 1, "relevance_score": 0.04}
]
```

---

## POST /olla/tei/similarity

Sentence similarity scores between a source sentence and a set of sentences.

### Request

```bash
curl -X POST http://localhost:40114/olla/tei/similarity \
  -H "Content-Type: application/json" \
  -d '{
    "inputs": {
      "source_sentence": "What is machine learning?",
      "sentences": [
        "Machine learning is a subset of artificial intelligence",
        "The weather today is sunny"
      ]
    }
  }'
```

### Response

```json
[0.87, 0.06]
```
