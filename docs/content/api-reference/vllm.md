# vLLM API

Proxy endpoints for vLLM inference servers. Available through the `/olla/vllm/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/vllm/health` | Health check |
| GET | `/olla/vllm/v1/models` | List available models |
| POST | `/olla/vllm/v1/chat/completions` | Chat completion |
| POST | `/olla/vllm/v1/completions` | Text completion |
| POST | `/olla/vllm/v1/embeddings` | Generate embeddings |
| GET | `/olla/vllm/metrics` | Prometheus metrics |

---

## GET /olla/vllm/health

Check vLLM server health status.

### Request

```bash
curl -X GET http://localhost:40114/olla/vllm/health
```

### Response

```json
{
  "status": "healthy",
  "model_loaded": true,
  "gpu_memory_usage": 0.65,
  "num_requests_running": 2,
  "num_requests_waiting": 0
}
```

---

## GET /olla/vllm/v1/models

List models available on the vLLM server.

### Request

```bash
curl -X GET http://localhost:40114/olla/vllm/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "meta-llama/Meta-Llama-3-8B-Instruct",
      "object": "model",
      "created": 1705334400,
      "owned_by": "vllm",
      "root": "meta-llama/Meta-Llama-3-8B-Instruct",
      "parent": null,
      "max_model_len": 8192,
      "permission": []
    },
    {
      "id": "mistralai/Mistral-7B-Instruct-v0.2",
      "object": "model",
      "created": 1705334400,
      "owned_by": "vllm",
      "root": "mistralai/Mistral-7B-Instruct-v0.2",
      "parent": null,
      "max_model_len": 32768,
      "permission": []
    }
  ]
}
```

---

## POST /olla/vllm/v1/chat/completions

OpenAI-compatible chat completion with vLLM optimizations.

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Meta-Llama-3-8B-Instruct",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful AI assistant."
      },
      {
        "role": "user",
        "content": "Explain the benefits of using vLLM for inference"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300,
    "stream": false,
    "guided_decoding_backend": "outlines",
    "guided_json": {
      "type": "object",
      "properties": {
        "benefits": {
          "type": "array",
          "items": {"type": "string"}
        },
        "summary": {"type": "string"}
      }
    }
  }'
```

### Response

```json
{
  "id": "chatcmpl-vllm-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "meta-llama/Meta-Llama-3-8B-Instruct",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "{\n  \"benefits\": [\n    \"High throughput with continuous batching\",\n    \"PagedAttention for efficient memory management\",\n    \"Tensor parallelism for multi-GPU serving\",\n    \"Optimized CUDA kernels for faster inference\",\n    \"Support for quantization methods like AWQ and GPTQ\"\n  ],\n  \"summary\": \"vLLM provides state-of-the-art serving throughput with efficient memory management and GPU utilization, making it ideal for production LLM deployment.\"\n}"
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 35,
    "completion_tokens": 98,
    "total_tokens": 133
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-vllm-abc123","object":"chat.completion.chunk","created":1705334400,"model":"meta-llama/Meta-Llama-3-8B-Instruct","choices":[{"index":0,"delta":{"role":"assistant"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-vllm-abc123","object":"chat.completion.chunk","created":1705334400,"model":"meta-llama/Meta-Llama-3-8B-Instruct","choices":[{"index":0,"delta":{"content":"vLLM"},"logprobs":null,"finish_reason":null}]}

...

data: {"id":"chatcmpl-vllm-abc123","object":"chat.completion.chunk","created":1705334401,"model":"meta-llama/Meta-Llama-3-8B-Instruct","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

data: [DONE]
```

---

## POST /olla/vllm/v1/completions

Text completion with vLLM-specific optimizations.

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.2",
    "prompt": "The advantages of PagedAttention in vLLM are:",
    "max_tokens": 200,
    "temperature": 0.8,
    "top_p": 0.95,
    "top_k": 50,
    "repetition_penalty": 1.1,
    "best_of": 1,
    "use_beam_search": false,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-vllm-xyz789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "mistralai/Mistral-7B-Instruct-v0.2",
  "choices": [
    {
      "text": "\n\n1. **Memory Efficiency**: PagedAttention manages attention key-value (KV) cache memory in non-contiguous blocks, eliminating memory fragmentation and allowing for higher batch sizes.\n\n2. **Dynamic Memory Allocation**: It allocates memory on-demand as sequences grow, rather than pre-allocating maximum sequence length, significantly reducing memory waste.\n\n3. **Memory Sharing**: Enables efficient memory sharing across parallel sampling requests and beam search, reducing redundant memory usage.\n\n4. **Higher Throughput**: By optimizing memory usage, PagedAttention allows vLLM to handle more concurrent requests, increasing overall serving throughput.",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 118,
    "total_tokens": 130
  }
}
```

---

## POST /olla/vllm/v1/embeddings

Generate embeddings using vLLM (if model supports embeddings).

### Request

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "BAAI/bge-large-en-v1.5",
    "input": "vLLM is a high-throughput inference engine",
    "encoding_format": "float"
  }'
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0234, -0.0567, 0.0891, ...]
    }
  ],
  "model": "BAAI/bge-large-en-v1.5",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}
```

---

## GET /olla/vllm/metrics

Prometheus-compatible metrics endpoint for monitoring.

### Request

```bash
curl -X GET http://localhost:40114/olla/vllm/metrics
```

### Response (Prometheus format)

```
# HELP vllm_request_duration_seconds Request duration in seconds
# TYPE vllm_request_duration_seconds histogram
vllm_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3-8B-Instruct",le="0.1"} 45
vllm_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3-8B-Instruct",le="0.5"} 120
vllm_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3-8B-Instruct",le="1"} 180
vllm_request_duration_seconds_sum{model="meta-llama/Meta-Llama-3-8B-Instruct"} 125.5
vllm_request_duration_seconds_count{model="meta-llama/Meta-Llama-3-8B-Instruct"} 200

# HELP vllm_num_requests_running Number of requests currently running
# TYPE vllm_num_requests_running gauge
vllm_num_requests_running{model="meta-llama/Meta-Llama-3-8B-Instruct"} 3

# HELP vllm_num_requests_waiting Number of requests waiting in queue
# TYPE vllm_num_requests_waiting gauge
vllm_num_requests_waiting{model="meta-llama/Meta-Llama-3-8B-Instruct"} 0

# HELP vllm_gpu_memory_usage_bytes GPU memory usage in bytes
# TYPE vllm_gpu_memory_usage_bytes gauge
vllm_gpu_memory_usage_bytes{gpu="0"} 12884901888
```

## vLLM-Specific Parameters

### Sampling Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `best_of` | integer | 1 | Number of sequences to generate and return best |
| `use_beam_search` | boolean | false | Use beam search instead of sampling |
| `top_k` | integer | -1 | Top-k sampling (-1 = disabled) |
| `min_p` | float | 0.0 | Min-p sampling threshold |
| `repetition_penalty` | float | 1.0 | Repetition penalty |
| `length_penalty` | float | 1.0 | Length penalty for beam search |
| `early_stopping` | boolean | false | Stop beam search early |
| `ignore_eos` | boolean | false | Continue generation after EOS |
| `min_tokens` | integer | 0 | Minimum tokens to generate |
| `skip_special_tokens` | boolean | true | Skip special tokens in output |

### Guided Generation

| Parameter | Type | Description |
|-----------|------|-------------|
| `guided_json` | object | JSON schema for structured output |
| `guided_regex` | string | Regular expression for guided generation |
| `guided_choice` | array | List of allowed choices |
| `guided_grammar` | string | Context-free grammar |
| `guided_decoding_backend` | string | Backend for guided generation (outlines/lm-format-enforcer) |

### Advanced Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `logprobs` | integer | Number of log probabilities to return |
| `prompt_logprobs` | integer | Log probabilities for prompt tokens |
| `detokenize` | boolean | Return detokenized text |
| `echo` | boolean | Include prompt in response |
| `add_generation_prompt` | boolean | Add generation prompt for chat models |
| `add_special_tokens` | boolean | Add special tokens to prompt |
| `include_stop_str_in_output` | boolean | Include stop string in output |

## Performance Features

vLLM provides several performance optimizations:

1. **Continuous Batching**: Dynamic batching of requests for higher throughput
2. **PagedAttention**: Efficient KV cache memory management
3. **Tensor Parallelism**: Multi-GPU serving support
4. **Quantization**: Support for AWQ, GPTQ, and SqueezeLLM
5. **Speculative Decoding**: Faster inference with draft models
6. **Prefix Caching**: Automatic caching of common prefixes

## Configuration Example

```yaml
endpoints:
  - url: "http://192.168.0.100:8000"
    name: "vllm-server"
    type: "vllm"
    priority: 90
    model_url: "/v1/models"
    health_check_url: "/health"
    check_interval: 5s
    check_timeout: 2s
    headers:
      X-API-Key: "${VLLM_API_KEY}"
```

## Request Headers

All requests are forwarded with:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- Custom headers from endpoint configuration

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., "vllm-server")
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always "vllm" for these endpoints
- `X-Olla-Response-Time` - Total processing time