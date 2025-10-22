---
title: llama.cpp API Reference - Olla Proxy Endpoints
description: Complete API reference for llama.cpp endpoints through Olla proxy. Includes OpenAI-compatible and llamacpp-native endpoints for chat, completions, embeddings, tokenization, and code infill.
keywords: [llamacpp api, llama.cpp endpoints, olla proxy api, gguf api, code infill, tokenization api, openai compatible, llama.cpp inference]
---

# llama.cpp API

Proxy endpoints for llama.cpp inference servers. llama.cpp is a high-performance C++ inference engine for GGUF models, offering both OpenAI-compatible and native endpoints.

Available through the `/olla/llamacpp/` (default), `/olla/llama-cpp/` (disabled) and `/olla/llama_cpp/` (disabled) prefixes.

**Key Features:**

- **OpenAI Compatibility**: Full OpenAI API compatibility for drop-in replacements
- **GGUF Format**: Exclusive support for GGUF quantized models (Q2 to F32)
- **Single Model Architecture**: Dedicated resources for one model per instance
- **CPU Inference**: Full-featured inference without GPU requirements
- **Code Infill**: Fill-In-the-Middle (FIM) support for IDE integration
- **Tokenization API**: Direct access to model tokenizer

For integration guides and configuration examples, see the [llama.cpp Integration Guide](../integrations/backend/llamacpp.md).

!!! note "Compatibility with mainline LlamaCpp"
    Primary development was done for compatibility with the original [llamacpp](https://github.com/ggml-org/llama.cpp) and tested on forks like [ik_llama](https://github.com/ikawrakow/ik_llama.cpp) but we may not support the wider forks yet.


## Endpoints Overview

The following inference endpoints are available through the Olla proxy:

| Method | URI | Description |
|--------|-----|-------------|
| GET | `/olla/llamacpp/v1/models` | List available models (OpenAI) |
| POST | `/olla/llamacpp/completion` | Native completion (llamacpp format) |
| POST | `/olla/llamacpp/v1/completions` | Text completion (OpenAI) |
| POST | `/olla/llamacpp/v1/chat/completions` | Chat completion (OpenAI) |
| POST | `/olla/llamacpp/embedding` | Native embedding (llamacpp format) |
| POST | `/olla/llamacpp/v1/embeddings` | Generate embeddings (OpenAI) |
| POST | `/olla/llamacpp/tokenize` | Tokenize text (llamacpp-specific) |
| POST | `/olla/llamacpp/detokenize` | Detokenize tokens (llamacpp-specific) |
| POST | `/olla/llamacpp/infill` | Code infill/FIM (llamacpp-specific) |

## Base URL & Authentication

```
Base URL: http://localhost:40114/olla/llamacpp
Alternative: http://localhost:40114/olla/llama-cpp
Alternative: http://localhost:40114/olla/llama_cpp
Authentication: Not required (or API key if configured)
```

All three routing prefixes are functionally equivalent and route to the same llama.cpp endpoints.

---

## Model Management

### GET /olla/llamacpp/v1/models

List models available on the llama.cpp server (OpenAI-compatible).

**Note**: llama.cpp typically serves a single model per instance. The response will contain one model - the GGUF model loaded at server startup.

#### Request

```bash
curl -X GET http://localhost:40114/olla/llamacpp/v1/models
```

#### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "llama-3.1-8b-instruct-q4_k_m.gguf",
      "object": "model",
      "created": 1704067200,
      "owned_by": "meta-llama"
    }
  ]
}
```

---

## Text Generation

### POST /olla/llamacpp/completion

Native llama.cpp completion endpoint.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/completion \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "The future of AI is",
    "n_predict": 200,
    "temperature": 0.7,
    "top_k": 40,
    "top_p": 0.9,
    "repeat_penalty": 1.1,
    "stream": false
  }'
```

#### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `prompt` | string | required | Input text to complete |
| `n_predict` | integer | 512 | Maximum tokens to generate |
| `temperature` | float | 0.8 | Sampling temperature (0.0-2.0) |
| `top_k` | integer | 40 | Top-k sampling |
| `top_p` | float | 0.95 | Top-p (nucleus) sampling |
| `min_p` | float | 0.05 | Minimum probability threshold |
| `repeat_penalty` | float | 1.1 | Repetition penalty (1.0 = no penalty) |
| `repeat_last_n` | integer | 64 | Last n tokens to penalize |
| `penalize_nl` | boolean | true | Penalize newline tokens |
| `stop` | array | [] | Stop sequences |
| `stream` | boolean | false | Enable streaming response |
| `seed` | integer | -1 | Random seed (-1 = random) |
| `grammar` | string | "" | GBNF grammar for constrained output |

#### Response

```json
{
  "content": "The future of AI is incredibly promising and transformative. We're seeing rapid advances in natural language processing, computer vision, and autonomous systems. Machine learning models are becoming more capable, efficient, and accessible. Key trends include:\n\n1. Multimodal AI that can process text, images, audio, and video\n2. Smaller, more efficient models that run on edge devices\n3. Enhanced reasoning and problem-solving capabilities\n4. Better alignment with human values and safety\n5. Integration into everyday tools and workflows\n\nThese developments will revolutionise healthcare, education, scientific research, and countless other fields.",
  "stop": true,
  "generation_settings": {
    "n_ctx": 4096,
    "n_predict": 200,
    "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
    "seed": 1234567890,
    "temperature": 0.7,
    "top_k": 40,
    "top_p": 0.9,
    "repeat_penalty": 1.1
  },
  "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
  "prompt": "The future of AI is",
  "stopped_eos": true,
  "stopped_word": false,
  "stopped_limit": false,
  "stopping_word": "",
  "tokens_predicted": 142,
  "tokens_evaluated": 5,
  "truncated": false,
  "timings": {
    "prompt_n": 5,
    "prompt_ms": 45.2,
    "prompt_per_token_ms": 9.04,
    "prompt_per_second": 110.6,
    "predicted_n": 142,
    "predicted_ms": 1850.5,
    "predicted_per_token_ms": 13.03,
    "predicted_per_second": 76.7
  }
}
```

#### Streaming Response

When `"stream": true`, responses are sent as Server-Sent Events (SSE):

```
data: {"content":"The","stop":false}

data: {"content":" future","stop":false}

data: {"content":" of","stop":false}

...

data: {"content":"","stop":true,"stopped_eos":true,"timings":{...}}
```

---

### POST /olla/llamacpp/v1/completions

OpenAI-compatible text completion.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
    "prompt": "Explain GGUF quantization benefits:",
    "max_tokens": 200,
    "temperature": 0.7,
    "top_p": 0.9,
    "stream": false
  }'
```

#### Response

```json
{
  "id": "cmpl-llamacpp-abc123",
  "object": "text_completion",
  "created": 1704067200,
  "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
  "choices": [
    {
      "text": "\n\n1. **Reduced Memory Footprint**: GGUF quantization compresses model weights from 16-bit (F16) to 4-bit (Q4), reducing memory requirements by approximately 75%. This allows running larger models on consumer hardware.\n\n2. **Faster Inference**: Lower precision arithmetic operations are faster on CPUs and GPUs, improving inference speed by 2-3x compared to full precision.\n\n3. **Maintained Quality**: Q4_K_M quantization carefully preserves model accuracy, with minimal quality degradation compared to F16 models.\n\n4. **Format Standardisation**: GGUF provides a universal format for quantized models, ensuring compatibility across llama.cpp, Ollama, and other inference engines.\n\n5. **Flexible Quantization**: Multiple quantization levels (Q2 to Q8) allow trading memory/speed for quality based on deployment requirements.",
      "index": 0,
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 8,
    "completion_tokens": 145,
    "total_tokens": 153
  }
}
```

#### Streaming Response

When `"stream": true`:

```
data: {"id":"cmpl-llamacpp-abc123","object":"text_completion","created":1704067200,"choices":[{"text":"\n\n","index":0,"logprobs":null,"finish_reason":null}],"model":"llama-3.1-8b-instruct-q4_k_m.gguf"}

data: {"id":"cmpl-llamacpp-abc123","object":"text_completion","created":1704067200,"choices":[{"text":"1","index":0,"logprobs":null,"finish_reason":null}],"model":"llama-3.1-8b-instruct-q4_k_m.gguf"}

...

data: {"id":"cmpl-llamacpp-abc123","object":"text_completion","created":1704067201,"choices":[{"text":"","index":0,"logprobs":null,"finish_reason":"stop"}],"model":"llama-3.1-8b-instruct-q4_k_m.gguf","usage":{"prompt_tokens":8,"completion_tokens":145,"total_tokens":153}}

data: [DONE]
```

---

### POST /olla/llamacpp/v1/chat/completions

OpenAI-compatible chat completion with conversation history.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful AI assistant specialising in efficient LLM deployment."
      },
      {
        "role": "user",
        "content": "What are the best practices for deploying llama.cpp in production?"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300,
    "stream": false
  }'
```

#### Response

```json
{
  "id": "chatcmpl-llamacpp-xyz789",
  "object": "chat.completion",
  "created": 1704067200,
  "model": "llama-3.1-8b-instruct-q4_k_m.gguf",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Here are key best practices for deploying llama.cpp in production:\n\n**1. Model Selection & Quantization**\n- Use Q4_K_M or Q5_K_M quantization for optimal quality/performance balance\n- Match model size to available hardware (8B models for 8-16GB RAM)\n- Test quantization levels against your quality requirements\n\n**2. Slot Configuration**\n- Configure slots based on expected concurrency (typically 4-8 slots)\n- Monitor slot usage via /slots endpoint\n- Implement queue management for slot exhaustion scenarios\n\n**3. Resource Management**\n- Allocate sufficient context window size (n_ctx) for your use case\n- Enable GPU acceleration (CUDA/Metal) when available\n- Consider CPU-only deployment for edge/serverless environments\n\n**4. Monitoring & Observability**\n- Integrate Prometheus metrics endpoint for monitoring\n- Track TTFT, throughput, and slot utilisation\n- Set up alerts for slot exhaustion and high latency\n\n**5. High Availability**\n- Deploy multiple llama.cpp instances behind Olla proxy\n- Use health checks for automatic failover\n- Implement request retries with exponential backoff\n\n**6. Security**\n- Deploy behind reverse proxy with authentication\n- Bind to localhost for local-only access\n- Implement rate limiting to prevent abuse"
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 45,
    "completion_tokens": 245,
    "total_tokens": 290
  }
}
```

#### Streaming Response

When `"stream": true`:

```
data: {"id":"chatcmpl-llamacpp-xyz789","object":"chat.completion.chunk","created":1704067200,"model":"llama-3.1-8b-instruct-q4_k_m.gguf","choices":[{"index":0,"delta":{"role":"assistant"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-llamacpp-xyz789","object":"chat.completion.chunk","created":1704067200,"model":"llama-3.1-8b-instruct-q4_k_m.gguf","choices":[{"index":0,"delta":{"content":"Here"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chatcmpl-llamacpp-xyz789","object":"chat.completion.chunk","created":1704067200,"model":"llama-3.1-8b-instruct-q4_k_m.gguf","choices":[{"index":0,"delta":{"content":" are"},"logprobs":null,"finish_reason":null}]}

...

data: {"id":"chatcmpl-llamacpp-xyz789","object":"chat.completion.chunk","created":1704067201,"model":"llama-3.1-8b-instruct-q4_k_m.gguf","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

data: [DONE]
```

---

## Embeddings

### POST /olla/llamacpp/embedding

Native llama.cpp embedding endpoint.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/embedding \
  -H "Content-Type: application/json" \
  -d '{
    "content": "llama.cpp enables efficient LLM inference with GGUF quantization"
  }'
```

#### Response

```json
{
  "embedding": [0.0234, -0.0567, 0.0891, -0.1203, 0.0456, ...],
  "model": "nomic-embed-text-v1.5.Q4_K_M.gguf"
}
```

**Note**: Requires an embedding model (e.g., nomic-embed-text, bge-large).

---

### POST /olla/llamacpp/v1/embeddings

OpenAI-compatible embeddings endpoint.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text-v1.5.Q4_K_M.gguf",
    "input": "llama.cpp enables efficient LLM inference with GGUF quantization",
    "encoding_format": "float"
  }'
```

#### Response

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0234, -0.0567, 0.0891, -0.1203, 0.0456, ...]
    }
  ],
  "model": "nomic-embed-text-v1.5.Q4_K_M.gguf",
  "usage": {
    "prompt_tokens": 12,
    "total_tokens": 12
  }
}
```

#### Batch Embeddings

The input parameter supports arrays for batch processing:

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text-v1.5.Q4_K_M.gguf",
    "input": [
      "First document to embed",
      "Second document to embed",
      "Third document to embed"
    ]
  }'
```

---

## Tokenization (llamacpp-specific)

### POST /olla/llamacpp/tokenize

Encode text to token IDs using the model's tokenizer.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/tokenize \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Hello, world! This is a test."
  }'
```

#### Response

```json
{
  "tokens": [9906, 11, 1917, 0, 1115, 374, 264, 1296, 13]
}
```

**Use Cases:**
- Token counting for context management
- Custom prompt engineering and optimization
- Token-level analysis and debugging
- Billing calculations based on token usage

---

### POST /olla/llamacpp/detokenize

Decode token IDs back to text.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/detokenize \
  -H "Content-Type: application/json" \
  -d '{
    "tokens": [9906, 11, 1917, 0, 1115, 374, 264, 1296, 13]
  }'
```

#### Response

```json
{
  "content": "Hello, world! This is a test."
}
```

---

## Code Infill (llamacpp-specific)

### POST /olla/llamacpp/infill

Fill-In-the-Middle (FIM) code completion for IDE integration.

#### Request

```bash
curl -X POST http://localhost:40114/olla/llamacpp/infill \
  -H "Content-Type: application/json" \
  -d '{
    "input_prefix": "def fibonacci(n):\n    \"\"\"Calculate fibonacci number\"\"\"\n    if n <= 1:\n        return n\n    ",
    "input_suffix": "\n    return result",
    "n_predict": 100,
    "temperature": 0.2,
    "stop": ["\n\n"]
  }'
```

#### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `input_prefix` | string | required | Code before the cursor/insertion point |
| `input_suffix` | string | "" | Code after the cursor/insertion point |
| `n_predict` | integer | 512 | Maximum tokens to generate |
| `temperature` | float | 0.8 | Sampling temperature (lower for code) |
| `top_k` | integer | 40 | Top-k sampling |
| `top_p` | float | 0.95 | Top-p sampling |
| `stop` | array | [] | Stop sequences (e.g., ["\n\n"]) |

#### Response

```json
{
  "content": "else:\n        return fibonacci(n-1) + fibonacci(n-2)",
  "stop": true,
  "model": "deepseek-coder-6.7b-instruct.Q5_K_M.gguf",
  "stopped_word": true,
  "stopping_word": "\n\n",
  "tokens_predicted": 24,
  "tokens_evaluated": 45,
  "truncated": false,
  "timings": {
    "prompt_n": 45,
    "prompt_ms": 125.8,
    "predicted_n": 24,
    "predicted_ms": 310.5,
    "predicted_per_token_ms": 12.94,
    "predicted_per_second": 77.3
  }
}
```

**Supported Models:**

- CodeLlama (code-instruct variants)
- StarCoder / StarCoder2
- DeepSeek-Coder
- WizardCoder
- Phind-CodeLlama

**Use Cases:**

- IDE code completion
- In-line code generation
- Code refactoring suggestions
- Docstring generation
- Test case generation

---

## Response Headers

All responses from Olla include these headers:

- `X-Olla-Backend-Type: llamacpp` - Identifies the backend type
- `X-Olla-Endpoint: <name>` - Backend endpoint name (e.g., "llamacpp-server")
- `X-Olla-Model: <model>` - GGUF model used for the request
- `X-Olla-Request-Id: <id>` - Unique request identifier
- `X-Olla-Response-Time: <ms>` - Total processing time in milliseconds
- `Via: 1.1 olla/<version>` - Olla proxy version

---

## Configuration Example

```yaml
endpoints:
  - url: "http://192.168.0.100:8080"
    name: "llamacpp-llama-8b"
    type: "llamacpp"
    priority: 95
    # Profile handles health checks and model discovery
    headers:
      X-API-Key: "${LLAMACPP_API_KEY}"
```

### Multi-Instance Setup

llama.cpp serves one model per instance. For multiple models, run multiple instances:

```yaml
endpoints:
  # Instance 1: Chat model
  - url: "http://192.168.0.100:8080"
    name: "llamacpp-chat"
    type: "llamacpp"
    priority: 90

  # Instance 2: Code model
  - url: "http://192.168.0.101:8080"
    name: "llamacpp-code"
    type: "llamacpp"
    priority: 85

  # Instance 3: Embedding model
  - url: "http://192.168.0.102:8080"
    name: "llamacpp-embed"
    type: "llamacpp"
    priority: 80
```

Olla will automatically route requests to the appropriate instance based on the model name.

---

## Error Responses

### 503 Service Unavailable (All Slots Full)

When all processing slots are busy:

```json
{
  "error": {
    "message": "All slots are busy. Please try again later.",
    "type": "server_error",
    "code": "slots_exhausted"
  }
}
```

### 404 Model Not Found

When requesting a non-existent model:

```json
{
  "error": {
    "message": "Model not found: unknown-model.gguf",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

### 400 Bad Request (Invalid Parameters)

When request parameters are invalid:

```json
{
  "error": {
    "message": "Invalid parameter: temperature must be between 0.0 and 2.0",
    "type": "invalid_request_error",
    "code": "invalid_parameter"
  }
}
```

---

## Examples

### Python with OpenAI SDK

```python
from openai import OpenAI

# Configure OpenAI SDK to use Olla proxy
client = OpenAI(
    base_url="http://localhost:40114/olla/llamacpp/v1",
    api_key="not-needed"
)

# Chat completion
response = client.chat.completions.create(
    model="llama-3.1-8b-instruct-q4_k_m.gguf",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Explain GGUF quantization"}
    ],
    temperature=0.7,
    max_tokens=200
)

print(response.choices[0].message.content)

# Streaming chat completion
stream = client.chat.completions.create(
    model="llama-3.1-8b-instruct-q4_k_m.gguf",
    messages=[
        {"role": "user", "content": "Count to 10 slowly"}
    ],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)

# Embeddings
embeddings_response = client.embeddings.create(
    model="nomic-embed-text-v1.5.Q4_K_M.gguf",
    input="Text to embed"
)

print(embeddings_response.data[0].embedding)
```

### Code Infill for IDE Integration

```bash
# Python function completion
curl -X POST http://localhost:40114/olla/llamacpp/infill \
  -H "Content-Type: application/json" \
  -d '{
    "input_prefix": "def calculate_statistics(data: list) -> dict:\n    \"\"\"Calculate mean, median, and mode\"\"\"\n    ",
    "input_suffix": "\n    return stats",
    "n_predict": 150,
    "temperature": 0.2,
    "stop": ["\n\n", "def "]
  }'

# JavaScript function completion
curl -X POST http://localhost:40114/olla/llamacpp/infill \
  -H "Content-Type: application/json" \
  -d '{
    "input_prefix": "async function fetchUserData(userId) {\n    try {\n        ",
    "input_suffix": "\n    } catch (error) {\n        console.error(error);\n    }\n}",
    "n_predict": 100,
    "temperature": 0.2
  }'
```

### Token Counting for Context Management

```bash
# Count tokens in prompt
TOKEN_RESPONSE=$(curl -s -X POST http://localhost:40114/olla/llamacpp/tokenize \
  -H "Content-Type: application/json" \
  -d "{\"content\": \"$PROMPT_TEXT\"}")

TOKEN_COUNT=$(echo $TOKEN_RESPONSE | jq '.tokens | length')

# Check against context window
MAX_CONTEXT=4096
MAX_COMPLETION=512
AVAILABLE=$((MAX_CONTEXT - TOKEN_COUNT - MAX_COMPLETION))

if [ $TOKEN_COUNT -gt $AVAILABLE ]; then
    echo "Prompt too long: $TOKEN_COUNT tokens (max: $AVAILABLE)"
    exit 1
fi

echo "Prompt size: $TOKEN_COUNT tokens, available for completion: $AVAILABLE"
```

---

## llamacpp-Specific Features

### Slot Management

llama.cpp uses a slot-based architecture for concurrency control:

- **Processing Slots**: Fixed number of concurrent request handlers
- **State Tracking**: Real-time visibility into slot usage (via direct backend access)
- **Queue Management**: Requests wait when all slots are busy
- **Capacity Planning**: Monitor slot utilisation to scale infrastructure

> **Note:** Slot monitoring is not available through Olla proxy endpoints. Access slot status directly from the llama.cpp backend at `http://backend:8080/slots` or use Olla's internal monitoring endpoints.

### GGUF Quantization Levels

llama.cpp exclusively supports GGUF format with extensive quantization options:

| Quantization | BPW | Memory | Quality | Use Case |
|--------------|-----|--------|---------|----------|
| Q2_K | 2.63 | ~35% | Lower | Extreme compression |
| Q3_K_M | 3.91 | ~45% | Moderate | Balanced small models |
| **Q4_K_M** | 4.85 | ~50% | Good | **Recommended default** |
| Q5_K_M | 5.69 | ~62% | High | Quality-focused |
| Q6_K | 6.59 | ~75% | Very High | Near-original quality |
| Q8_0 | 8.50 | ~87% | Excellent | High-fidelity |
| F16 | 16.0 | 100% | Original | Baseline |

**Recommendation**: Q4_K_M provides the best quality/performance/memory balance for most use cases.

### Grammar Support

llama.cpp supports GBNF (GGML BNF) grammars for constrained generation:

```bash
curl -X POST http://localhost:40114/olla/llamacpp/completion \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Generate a JSON object with name and age:",
    "n_predict": 100,
    "temperature": 0.7,
    "grammar": "root ::= \"{\" ws \"\\\"name\\\":\" ws string \",\" ws \"\\\"age\\\":\" ws number ws \"}\" ws\nstring ::= \"\\\"\" [^\"]* \"\\\"\"\nnumber ::= [0-9]+\nws ::= [ \\t\\n]*"
  }'
```

This ensures output conforms to the specified grammar structure.

---

## Performance Characteristics

### Memory Requirements (Q4_K_M Quantization)

| Model Size | RAM (CPU) | VRAM (GPU) | Typical Slots |
|------------|-----------|------------|---------------|
| 1B-3B | 2-4 GB | 2-4 GB | 8 |
| 7B-8B | 6-8 GB | 6-8 GB | 4 |
| 13B-14B | 10-16 GB | 10-16 GB | 2-4 |
| 30B-34B | 20-24 GB | 20-24 GB | 1-2 |
| 70B+ | 40-48 GB | 40-48 GB | 1 |

### Hardware Backend Support

llama.cpp supports multiple hardware acceleration backends:

- **CPU**: Full functionality without GPU (AVX2, AVX512, NEON)
- **CUDA**: NVIDIA GPUs (compute capability 6.0+)
- **Metal**: Apple Silicon (M1/M2/M3/M4)
- **Vulkan**: Cross-platform GPU acceleration
- **SYCL**: Intel GPUs
- **ROCm**: AMD GPUs

### Throughput Benchmarks

Typical performance on consumer hardware (Q4_K_M, batch size 1):

**CPU-only (Ryzen 9 5950X):**

- 7B model: 15-25 tokens/sec
- 13B model: 8-12 tokens/sec

**GPU-accelerated (RTX 4090):**

- 7B model: 80-120 tokens/sec
- 13B model: 50-80 tokens/sec
- 70B model: 15-25 tokens/sec

**Apple Silicon (M3 Max):**

- 7B model: 60-90 tokens/sec
- 13B model: 35-55 tokens/sec

---

## Best Practices

### Production Deployment

1. **Use Q4_K_M or Q5_K_M quantization** for optimal quality/performance balance
2. **Configure slots based on expected concurrency** (typically 4-8 slots)
3. **Monitor performance** via Olla's internal endpoints or direct backend access for capacity planning
4. **Deploy multiple instances** for different model types (chat, code, embeddings)
5. **Enable GPU acceleration** when available for higher throughput
6. **Set appropriate context window** (n_ctx) based on use case requirements
7. **Implement health checks** and automatic failover via Olla proxy

### Slot Management

1. **Monitor slot exhaustion** and scale when consistently at capacity
2. **Implement client-side retries** with exponential backoff for 503 errors
3. **Use Olla load balancing** to distribute load across multiple instances
4. **Set reasonable timeout values** to prevent stuck slots

### Security

1. **Bind to localhost** (127.0.0.1) for local-only access
2. **Use reverse proxy** (nginx/caddy) with authentication for external access
3. **Implement rate limiting** at the reverse proxy level
4. **Monitor metrics** for unusual usage patterns
5. **Restrict file system access** if model loading is dynamic

### Performance Optimization

1. **Choose appropriate quantization level** based on quality requirements
2. **Enable Flash Attention** if supported by hardware
3. **Tune slot count** based on available memory and expected load
4. **Use batch processing** for embeddings when possible
5. **Monitor TTFT and throughput** metrics for optimization opportunities

---

## Troubleshooting

### All Slots Busy (503 Errors)

**Symptoms**: Requests return 503 "All slots are busy"

**Solutions**:

- Increase slot count in llama.cpp server configuration
- Deploy additional llama.cpp instances behind Olla
- Implement client-side retry logic with exponential backoff
- Monitor slot usage patterns and scale accordingly

### High Memory Usage

**Symptoms**: OOM errors, system slowdown

**Solutions**:

- Use lower quantization level (Q4 instead of Q8)
- Reduce context window size (n_ctx)
- Decrease slot count
- Upgrade to system with more RAM
- Enable GPU offloading to move workload to VRAM

### Slow Inference Speed

**Symptoms**: Low tokens/second, high latency

**Solutions**:

- Enable GPU acceleration (CUDA/Metal)
- Use lower quantization for faster inference (Q4 vs Q8)
- Reduce batch size if using continuous batching
- Check CPU/GPU utilisation and thermal throttling
- Upgrade hardware if consistently CPU/GPU-bound

### Model Loading Failures

**Symptoms**: Server fails to start or load model

**Solutions**:

- Verify GGUF file integrity and format
- Ensure sufficient memory for model size
- Check file permissions and paths
- Validate quantization level is supported
- Review llama.cpp server logs for detailed errors
