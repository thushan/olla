# SGLang API

Proxy endpoints for SGLang inference servers. Available through the `/olla/sglang/` prefix.

## Endpoints Overview

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/sglang/health` | Health check |
| GET | `/olla/sglang/v1/models` | List available models |
| POST | `/olla/sglang/v1/chat/completions` | Chat completion |
| POST | `/olla/sglang/v1/completions` | Text completion |
| POST | `/olla/sglang/v1/embeddings` | Generate embeddings |
| POST | `/olla/sglang/generate` | SGLang native generation |
| POST | `/olla/sglang/batch` | Batch processing |
| POST | `/olla/sglang/extend` | Conversation extension |
| POST | `/olla/sglang/v1/chat/completions/vision` | Vision chat completion |
| GET | `/olla/sglang/metrics` | Prometheus metrics |

---

## GET /olla/sglang/health

Check SGLang server health status including RadixAttention cache.

### Request

```bash
curl -X GET http://localhost:40114/olla/sglang/health
```

### Response

```json
{
  "status": "healthy",
  "model_loaded": true,
  "radix_cache_ready": true,
  "radix_cache_usage_perc": 0.35,
  "num_requests_running": 3,
  "num_requests_waiting": 0,
  "speculative_decoding_enabled": true
}
```

---

## GET /olla/sglang/v1/models

List models available on the SGLang server.

### Request

```bash
curl -X GET http://localhost:40114/olla/sglang/v1/models
```

### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "meta-llama/Meta-Llama-3.1-8B-Instruct",
      "object": "model",
      "created": 1705334400,
      "owned_by": "sglang",
      "root": "meta-llama/Meta-Llama-3.1-8B-Instruct",
      "parent": null,
      "max_model_len": 131072,
      "permission": [],
      "radix_cache_size": 17179869184,
      "speculative_decoding": true
    }
  ]
}
```

---

## POST /olla/sglang/v1/chat/completions

OpenAI-compatible chat completion with SGLang optimisations.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful AI assistant specialising in efficient inference."
      },
      {
        "role": "user",
        "content": "Explain the benefits of RadixAttention in SGLang"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300,
    "stream": false
  }'
```

### Response

```json
{
  "id": "chatcmpl-sglang-abc123",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "RadixAttention in SGLang provides several key benefits over traditional attention mechanisms:\n\n1. **Advanced Prefix Caching**: Uses a tree-based structure to cache common prefixes more efficiently than block-based approaches, achieving 85-95% cache hit rates.\n\n2. **Memory Efficiency**: Reduces memory usage by 15-20% compared to PagedAttention through better sharing of computation across requests.\n\n3. **Higher Throughput**: Enables handling 150+ concurrent requests efficiently due to the intelligent caching strategy.\n\n4. **Faster Response Times**: Dramatically reduces time-to-first-token for requests with cached prefixes.\n\nThis makes SGLang particularly effective for applications with repetitive patterns like chatbots, code completion, and conversational AI."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 42,
    "completion_tokens": 128,
    "total_tokens": 170
  },
  "metrics": {
    "time_to_first_token_ms": 35,
    "generation_time_ms": 850,
    "radix_cache_hit_rate": 0.92
  }
}
```

### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-sglang-abc123","object":"chat.completion.chunk","created":1705334400,"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","choices":[{"index":0,"delta":{"role":"assistant"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-sglang-abc123","object":"chat.completion.chunk","created":1705334400,"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","choices":[{"index":0,"delta":{"content":"RadixAttention"},"logprobs":null,"finish_reason":null}]}

...

data: {"id":"chatcmpl-sglang-abc123","object":"chat.completion.chunk","created":1705334401,"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

data: [DONE]
```

---

## POST /olla/sglang/v1/completions

Text completion with SGLang-specific optimisations.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
    "prompt": "The advantages of SGLang over vLLM include:",
    "max_tokens": 200,
    "temperature": 0.8,
    "top_p": 0.95,
    "stream": false
  }'
```

### Response

```json
{
  "id": "cmpl-sglang-xyz789",
  "object": "text_completion",
  "created": 1705334400,
  "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
  "choices": [
    {
      "text": "\n\n1. **RadixAttention Efficiency**: More advanced prefix caching with tree-based attention patterns, achieving 85-95% cache hit rates compared to vLLM's 60-70%.\n\n2. **Higher Concurrency**: Supports 150+ concurrent requests versus vLLM's typical 100 limit due to better memory management.\n\n3. **Frontend Language**: Provides a flexible programming interface for complex LLM applications beyond simple API calls.\n\n4. **Speculative Decoding**: Enhanced performance through speculative execution, reducing latency by up to 30%.\n\n5. **Memory Efficiency**: Uses 15-20% less GPU memory while maintaining the same model quality.",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 142,
    "total_tokens": 157
  },
  "metrics": {
    "time_to_first_token_ms": 28,
    "generation_time_ms": 920,
    "radix_cache_hit_rate": 0.88
  }
}
```

---

## POST /olla/sglang/generate

SGLang's native generation endpoint using Frontend Language programming interface.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/generate \
  -H "Content-Type: application/json" \
  -d '{
    "text": "def fibonacci(n):\n    if n <= 1:\n        return n",
    "sampling_params": {
      "temperature": 0.2,
      "max_new_tokens": 150,
      "top_p": 0.9
    }
  }'
```

### Response

```json
{
  "text": "def fibonacci(n):\n    if n <= 1:\n        return n\n    else:\n        return fibonacci(n-1) + fibonacci(n-2)\n\n# Example usage:\nprint(fibonacci(10))  # Output: 55",
  "meta": {
    "id": "gen-sglang-def456",
    "rid": "conversation-789",
    "finish_reason": "stop",
    "input_tokens": 12,
    "output_tokens": 28,
    "completion_time_ms": 450,
    "radix_cache_hit": true
  }
}
```

---

## POST /olla/sglang/batch

Process multiple generation requests in a single batch for efficiency.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/batch \
  -H "Content-Type: application/json" \
  -d '{
    "requests": [
      {
        "text": "Translate to French: Hello world",
        "sampling_params": {
          "temperature": 0.1,
          "max_new_tokens": 50
        }
      },
      {
        "text": "Translate to Spanish: Hello world",
        "sampling_params": {
          "temperature": 0.1,
          "max_new_tokens": 50
        }
      },
      {
        "text": "Translate to German: Hello world",
        "sampling_params": {
          "temperature": 0.1,
          "max_new_tokens": 50
        }
      }
    ]
  }'
```

### Response

```json
{
  "responses": [
    {
      "text": "Translate to French: Hello world\n\nBonjour le monde",
      "meta": {
        "id": "batch-1-sglang-ghi789",
        "finish_reason": "stop",
        "input_tokens": 7,
        "output_tokens": 6,
        "completion_time_ms": 320
      }
    },
    {
      "text": "Translate to Spanish: Hello world\n\nHola mundo",
      "meta": {
        "id": "batch-2-sglang-jkl012",
        "finish_reason": "stop",
        "input_tokens": 7,
        "output_tokens": 5,
        "completion_time_ms": 315
      }
    },
    {
      "text": "Translate to German: Hello world\n\nHallo Welt",
      "meta": {
        "id": "batch-3-sglang-mno345",
        "finish_reason": "stop",
        "input_tokens": 7,
        "output_tokens": 5,
        "completion_time_ms": 310
      }
    }
  ],
  "batch_meta": {
    "total_requests": 3,
    "successful": 3,
    "total_time_ms": 895,
    "avg_radix_cache_hit_rate": 0.67
  }
}
```

---

## POST /olla/sglang/extend

Extend an existing conversation using SGLang's conversation management.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/extend \
  -H "Content-Type: application/json" \
  -d '{
    "rid": "conversation-789",
    "text": "\n\n# Now create a memoized version:",
    "sampling_params": {
      "temperature": 0.3,
      "max_new_tokens": 200
    }
  }'
```

### Response

```json
{
  "text": "\n\n# Now create a memoized version:\n\ndef fibonacci_memo(n, memo={}):\n    if n in memo:\n        return memo[n]\n    if n <= 1:\n        memo[n] = n\n        return n\n    memo[n] = fibonacci_memo(n-1, memo) + fibonacci_memo(n-2, memo)\n    return memo[n]\n\n# This version is much more efficient for large n\nprint(fibonacci_memo(50))  # Much faster than the recursive version",
  "meta": {
    "id": "extend-sglang-pqr678",
    "rid": "conversation-789",
    "finish_reason": "stop",
    "input_tokens": 8,
    "output_tokens": 82,
    "completion_time_ms": 650,
    "radix_cache_hit": true,
    "context_length": 94
  }
}
```

---

## POST /olla/sglang/v1/chat/completions/vision

Vision-enabled chat completion for multimodal models.

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/chat/completions/vision \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llava-hf/llava-1.5-13b-hf",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What architectural pattern is shown in this diagram?"
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD..."
            }
          }
        ]
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300
  }'
```

### Response

```json
{
  "id": "chatcmpl-vision-sglang-stu901",
  "object": "chat.completion",
  "created": 1705334400,
  "model": "llava-hf/llava-1.5-13b-hf",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "This diagram shows a microservices architecture pattern with the following key components:\n\n1. **API Gateway**: Acts as the entry point routing requests to appropriate services\n2. **Load Balancer**: Distributes traffic across multiple service instances\n3. **Service Mesh**: Handles inter-service communication with features like service discovery, load balancing, and monitoring\n4. **Container Orchestration**: Manages deployment and scaling of containerised services\n5. **Shared Data Storage**: Each microservice has its own database following the database-per-service pattern\n\nThis architecture promotes scalability, fault isolation, and independent deployment of services."
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 1024,
    "completion_tokens": 118,
    "total_tokens": 1142
  },
  "vision_metrics": {
    "image_processing_time_ms": 145,
    "vision_encoding_tokens": 1012
  }
}
```

---

## POST /olla/sglang/v1/embeddings

Generate embeddings using SGLang (if model supports embeddings).

### Request

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "BAAI/bge-large-en-v1.5",
    "input": "SGLang provides efficient inference with RadixAttention",
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
    "prompt_tokens": 12,
    "total_tokens": 12
  }
}
```

---

## GET /olla/sglang/metrics

Prometheus-compatible metrics endpoint for monitoring.

### Request

```bash
curl -X GET http://localhost:40114/olla/sglang/metrics
```

### Response (Prometheus format)

```
# HELP sglang_request_duration_seconds Request duration in seconds
# TYPE sglang_request_duration_seconds histogram
sglang_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3.1-8B-Instruct",le="0.1"} 65
sglang_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3.1-8B-Instruct",le="0.5"} 150
sglang_request_duration_seconds_bucket{model="meta-llama/Meta-Llama-3.1-8B-Instruct",le="1"} 210
sglang_request_duration_seconds_sum{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 95.2
sglang_request_duration_seconds_count{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 250

# HELP sglang_num_requests_running Number of requests currently running
# TYPE sglang_num_requests_running gauge
sglang_num_requests_running{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 5

# HELP sglang_num_requests_waiting Number of requests waiting in queue
# TYPE sglang_num_requests_waiting gauge
sglang_num_requests_waiting{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 2

# HELP sglang_radix_cache_usage_perc RadixAttention cache usage percentage
# TYPE sglang_radix_cache_usage_perc gauge
sglang_radix_cache_usage_perc{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 0.42

# HELP sglang_radix_cache_hit_rate RadixAttention cache hit rate
# TYPE sglang_radix_cache_hit_rate gauge
sglang_radix_cache_hit_rate{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 0.89

# HELP sglang_time_to_first_token_seconds Time to first token in seconds
# TYPE sglang_time_to_first_token_seconds histogram
sglang_time_to_first_token_seconds_bucket{model="meta-llama/Meta-Llama-3.1-8B-Instruct",le="0.05"} 120
sglang_time_to_first_token_seconds_bucket{model="meta-llama/Meta-Llama-3.1-8B-Instruct",le="0.1"} 200
sglang_time_to_first_token_seconds_sum{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 15.8
sglang_time_to_first_token_seconds_count{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 250

# HELP sglang_spec_decode_num_accepted_tokens_total Total speculative tokens accepted
# TYPE sglang_spec_decode_num_accepted_tokens_total counter
sglang_spec_decode_num_accepted_tokens_total{model="meta-llama/Meta-Llama-3.1-8B-Instruct"} 12458
```

## SGLang-Specific Parameters

### Frontend Language Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `rid` | string | auto | Request/conversation ID for tracking |
| `return_logprob` | boolean | false | Return log probabilities |
| `logprob_start_len` | integer | 0 | Start position for logprob calculation |
| `top_logprobs_num` | integer | 0 | Number of top logprobs to return |

### RadixAttention Configuration

| Parameter | Type | Description |
|-----------|------|-------------|
| `radix_cache_enable` | boolean | Enable RadixAttention caching |
| `radix_cache_size` | float | Cache size as fraction of GPU memory |
| `max_tree_depth` | integer | Maximum prefix tree depth |

### Speculative Decoding Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `speculative_draft_length` | integer | Number of draft tokens to generate |
| `spec_decode_acceptance_threshold` | float | Token acceptance threshold |

### Vision Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `max_image_resolution` | integer | Maximum image resolution |
| `image_aspect_ratio` | string | Image aspect ratio handling |
| `vision_feature_select_strategy` | string | Vision feature selection strategy |

## Configuration Example

```yaml
endpoints:
  - url: "http://192.168.0.100:30000"
    name: "sglang-server"
    type: "sglang"
    priority: 90
    model_url: "/v1/models"
    health_check_url: "/health"
    check_interval: 5s
    check_timeout: 2s
    headers:
      X-API-Key: "${SGLANG_API_KEY}"
```

## Request Headers

All requests are forwarded with:

- `X-Olla-Request-ID` - Unique request identifier
- `X-Forwarded-For` - Client IP address
- Custom headers from endpoint configuration

## Response Headers

All responses include:

- `X-Olla-Endpoint` - Backend endpoint name (e.g., "sglang-server")
- `X-Olla-Model` - Model used for the request
- `X-Olla-Backend-Type` - Always "sglang" for these endpoints
- `X-Olla-Response-Time` - Total processing time
- `X-SGLang-Cache-Hit` - Whether RadixAttention cache was hit
- `X-SGLang-Spec-Decode` - Whether speculative decoding was used

## Performance Features

SGLang provides several performance optimisations:

1. **RadixAttention**: Advanced tree-based prefix caching with 85-95% hit rates
2. **Speculative Decoding**: Faster inference with draft models
3. **Frontend Language**: Flexible programming interface for complex workflows
4. **Disaggregation**: Separate prefill and decode phases
5. **Enhanced Multimodal**: Optimised vision and image processing
6. **Higher Concurrency**: Support for 150+ concurrent requests