---
title: SGLang Integration - High-Performance LLM Inference with RadixAttention
description: Configure SGLang with Olla proxy for production-grade LLM serving. RadixAttention, Frontend Language, speculative decoding, and advanced GPU optimisation.
keywords: SGLang, Olla proxy, LLM inference, RadixAttention, Frontend Language, speculative decoding, high throughput
---

# SGLang Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/sgl-project/sglang">github.com/sgl-project/sglang</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.19</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>sglang</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>sglang.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/sglang.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Health Check (native)</li>
                <li>Model Unification</li>
                <li>Model Detection & Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>Prometheus Metrics</li>
                <li>Frontend Language Programming</li>
                <li>Vision & Multimodal Support</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
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
                <li>OpenAI Compatible</li>
                <li>Higher Concurrency (RadixAttention)</li>
                <li>GPU Optimised</li>
                <li>Speculative Decoding</li>
                <li>Advanced Prefix Caching</li>
                <li>Frontend Language Support</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/sglang</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
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

Add SGLang to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:30000"
        name: "local-sglang"
        type: "sglang"
        priority: 85
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

### Production Setup

Configure SGLang for high-throughput production:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-server:30000"
        name: "sglang-prod"
        type: "sglang"
        priority: 100
        health_check_url: "/health"
        check_interval: 10s
        check_timeout: 5s

      - url: "http://gpu-server:30001"
        name: "sglang-prod-2"
        type: "sglang"
        priority: 100
        health_check_url: "/health"
        check_interval: 10s
        check_timeout: 5s

proxy:
  engine: "olla"  # Use high-performance engine
  load_balancer: "least-connections"
```

## Endpoints Supported

The following endpoints are supported by the SGLang integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/health</code></td>
    <td>Health Check (SGLang-specific)</td>
  </tr>
  <tr>
    <td><code>/metrics</code></td>
    <td>Prometheus Metrics</td>
  </tr>
  <tr>
    <td><code>/version</code></td>
    <td>SGLang Version Information</td>
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
    <td><code>/v1/embeddings</code></td>
    <td>Embeddings/Pooling API</td>
  </tr>
  <tr>
    <td><code>/generate</code></td>
    <td>SGLang Native Generation (Frontend Language)</td>
  </tr>
  <tr>
    <td><code>/batch</code></td>
    <td>Batch Processing (Frontend Language)</td>
  </tr>
  <tr>
    <td><code>/extend</code></td>
    <td>Conversation Extension (Frontend Language)</td>
  </tr>
  <tr>
    <td><code>/v1/chat/completions/vision</code></td>
    <td>Vision Chat Completions (Multimodal)</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain SGLang RadixAttention in simple terms"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.2",
    "messages": [
      {"role": "user", "content": "Write a story about efficient AI inference"}
    ],
    "stream": true,
    "temperature": 0.8
  }'
```

### Frontend Language Generation

```bash
# SGLang native generation endpoint
curl -X POST http://localhost:40114/olla/sglang/generate \
  -H "Content-Type: application/json" \
  -d '{
    "text": "def fibonacci(n):",
    "sampling_params": {
      "temperature": 0.3,
      "max_new_tokens": 200
    }
  }'
```

### Batch Processing

```bash
curl -X POST http://localhost:40114/olla/sglang/batch \
  -H "Content-Type: application/json" \
  -d '{
    "requests": [
      {
        "text": "Translate to French: Hello world",
        "sampling_params": {"temperature": 0.1, "max_new_tokens": 50}
      },
      {
        "text": "Translate to Spanish: Hello world",
        "sampling_params": {"temperature": 0.1, "max_new_tokens": 50}
      }
    ]
  }'
```

### Vision Chat Completions

```bash
curl -X POST http://localhost:40114/olla/sglang/v1/chat/completions/vision \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llava-hf/llava-1.5-7b-hf",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What do you see in this image?"},
          {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,..."}}
        ]
      }
    ],
    "temperature": 0.7,
    "max_tokens": 300
  }'
```

### Conversation Extension

```bash
curl -X POST http://localhost:40114/olla/sglang/extend \
  -H "Content-Type: application/json" \
  -d '{
    "rid": "conversation-123",
    "text": "Continue the previous discussion about machine learning",
    "sampling_params": {
      "temperature": 0.7,
      "max_new_tokens": 150
    }
  }'
```

### Metrics Access

```bash
# Get Prometheus metrics
curl http://localhost:40114/olla/sglang/metrics

# Check health status
curl http://localhost:40114/olla/sglang/health

# Get version information
curl http://localhost:40114/olla/sglang/version
```

## SGLang Specifics

### High-Performance Features

SGLang includes several optimisations beyond standard inference:

- **RadixAttention**: Tree-based prefix caching more advanced than PagedAttention
- **Speculative Decoding**: Enhanced performance through speculative execution
- **Frontend Language**: Flexible programming interface for LLM applications
- **Disaggregation**: Separate prefill and decode phases for efficiency
- **Enhanced Multimodal**: Advanced vision and multimodal capabilities

### RadixAttention vs PagedAttention

SGLang's RadixAttention provides superior memory efficiency compared to vLLM's PagedAttention:

| Feature | PagedAttention (vLLM) | RadixAttention (SGLang) |
|---------|----------------------|-------------------------|
| Memory Structure | Block-based | Tree-based |
| Prefix Sharing | Limited | Advanced |
| Cache Hit Rate | ~60-70% | ~85-95% |
| Memory Efficiency | Good | Excellent |
| Complex Conversations | Standard | Optimised |

### Resource Configuration

The SGLang profile includes GPU-optimised settings with enhanced efficiency:

```yaml
characteristics:
  timeout: 2m
  max_concurrent_requests: 150  # Higher than vLLM (100)
  streaming_support: true

resources:
  defaults:
    requires_gpu: true
    min_gpu_memory_gb: 6
```

### Memory Requirements

SGLang requires less memory than vLLM due to RadixAttention efficiency:

| Model Size | GPU Memory Required | Recommended | Max Concurrent |
|------------|-------------------|-------------|----------------|
| 70B | 120GB | 140GB | 15 |
| 34B | 60GB | 70GB | 30 |
| 13B | 25GB | 35GB | 75 |
| 7B | 14GB | 20GB | 150 |
| 3B | 6GB | 10GB | 150 |

### Model Naming

SGLang uses full HuggingFace model names like vLLM:

- `meta-llama/Meta-Llama-3.1-8B-Instruct`
- `mistralai/Mistral-7B-Instruct-v0.2`
- `llava-hf/llava-1.5-7b-hf`

## Starting SGLang Server

### Basic Start

```bash
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --port 30000
```

### Production Configuration

```bash
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-70B-Instruct \
  --tp-size 4 \
  --mem-fraction-static 0.85 \
  --max-running-requests 150 \
  --port 30000 \
  --host 0.0.0.0 \
  --enable-flashinfer
```

### Docker Deployment

```bash
docker run --gpus all \
  -v ~/.cache/huggingface:/root/.cache/huggingface \
  -p 30000:30000 \
  lmsysorg/sglang:latest \
  python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --host 0.0.0.0 \
  --port 30000
```

### With Speculative Decoding

```bash
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-70B-Instruct \
  --draft-model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --speculative-draft-length 4 \
  --port 30000
```

### Vision Model Setup

```bash
python -m sglang.launch_server \
  --model-path llava-hf/llava-1.5-13b-hf \
  --tokenizer-path llava-hf/llava-1.5-13b-hf \
  --chat-template llava \
  --port 30000
```

## Profile Customisation

To customise SGLang behaviour, create `config/profiles/sglang-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: sglang
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - sglang
    - radix      # Add custom prefix for RadixAttention

# Adjust for larger models
characteristics:
  timeout: 5m     # Increase for 70B models
  max_concurrent_requests: 200  # Leverage SGLang's efficiency

# Modify concurrency limits
resources:
  concurrency_limits:
    - min_memory_gb: 100
      max_concurrent: 20    # Higher than vLLM due to efficiency
    - min_memory_gb: 50
      max_concurrent: 40   # Take advantage of RadixAttention

# Enable SGLang-specific features
features:
  radix_attention:
    enabled: true
  speculative_decoding:
    enabled: true
  frontend_language:
    enabled: true
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Monitoring

### Prometheus Metrics

SGLang exposes detailed metrics at `/metrics`:

```yaml
# Example Prometheus configuration
scrape_configs:
  - job_name: 'sglang'
    static_configs:
      - targets: ['localhost:40114']
    metrics_path: '/olla/sglang/metrics'
```

Key SGLang-specific metrics include:

- `sglang:num_requests_running` - Active requests
- `sglang:num_requests_waiting` - Queued requests
- `sglang:radix_cache_usage_perc` - RadixAttention cache utilisation
- `sglang:radix_cache_hit_rate` - Cache hit efficiency
- `sglang:time_to_first_token_seconds` - TTFT latency
- `sglang:spec_decode_num_accepted_tokens_total` - Speculative decoding stats

### Health Monitoring

```bash
# Check health endpoint
curl http://localhost:40114/olla/sglang/health

# Response when healthy
{"status": "healthy", "radix_cache_ready": true}

# Response when unhealthy
{"status": "unhealthy", "reason": "model not loaded"}
```

## Troubleshooting

### Out of Memory (OOM)

**Issue**: CUDA out of memory errors

**Solution**:
1. Reduce `--mem-fraction-static` (default 0.9)
2. Decrease `--max-running-requests`
3. Use quantisation with `--quantization fp8` or `--quantization int4`
4. Enable tensor parallelism for multi-GPU

### Low Cache Hit Rate

**Issue**: RadixAttention cache hit rate below 80%

**Solution**:

1. Enable longer context retention
2. Increase RadixAttention cache size
3. Use consistent prompt formats
4. Monitor prefix sharing patterns

### Frontend Language Errors

**Issue**: `/generate` or `/batch` endpoints failing

**Solution**:

```bash
# Ensure SGLang server started with Frontend Language support
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --enable-frontend-language \
  --port 30000
```

### Connection Timeout

**Issue**: Requests timeout during model loading

**Solution**: Increase timeout in profile:
```yaml
characteristics:
  timeout: 10m  # Increase for initial model load

resources:
  timeout_scaling:
    base_timeout_seconds: 300
    load_time_buffer: true
```

### High Queue Wait Times

**Issue**: Requests queuing with "num_requests_waiting" high

**Solution**:

1. Add more SGLang instances
2. Use load balancing across multiple servers
3. Increase `--max-running-requests` (default 1024)
4. Enable disaggregation for better resource utilisation

## Best Practices

### 1. Optimise RadixAttention

```bash
# Enable advanced prefix caching
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --radix-cache-size 0.4 \  # 40% of GPU memory for cache
  --enable-flashinfer
```

### 2. Configure Tensor Parallelism

For models requiring multiple GPUs:

```bash
# 70B model on 4x A100 80GB
--tp-size 4

# 34B model on 2x A100 40GB
--tp-size 2
```

### 3. Enable Speculative Decoding

For maximum performance with compatible models:

```bash
python -m sglang.launch_server \
  --model-path meta-llama/Meta-Llama-3.1-70B-Instruct \
  --draft-model-path meta-llama/Meta-Llama-3.1-8B-Instruct \
  --speculative-draft-length 4
```

### 4. Monitor and Scale

```yaml
# Multiple SGLang instances
discovery:
  static:
    endpoints:
      - url: "http://gpu1:30000"
        name: "sglang-1"
        type: "sglang"
        priority: 100

      - url: "http://gpu2:30000"
        name: "sglang-2"
        type: "sglang"
        priority: 100

proxy:
  load_balancer: "least-connections"
```

### 5. Leverage Frontend Language

Use SGLang's native endpoints for advanced use cases:

```python
# Complex conversation flows
import requests

# Start conversation
response = requests.post("http://localhost:40114/olla/sglang/generate", json={
    "text": "System: You are a helpful assistant.\nUser: Hello",
    "sampling_params": {"temperature": 0.7, "max_new_tokens": 100}
})

# Extend conversation
conversation_id = response.json()["meta"]["rid"]
requests.post("http://localhost:40114/olla/sglang/extend", json={
    "rid": conversation_id,
    "text": "\nUser: Tell me about AI",
    "sampling_params": {"temperature": 0.7, "max_new_tokens": 200}
})
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/sglang/v1",
    api_key="not-needed"  # SGLang doesn't require API keys
)

response = client.chat.completions.create(
    model="meta-llama/Meta-Llama-3.1-8B-Instruct",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### SGLang Python Client

```python
import sglang as sgl

# Connect to SGLang server via Olla
sgl.set_default_backend(sgl.RuntimeEndpoint(
    "http://localhost:40114/olla/sglang"
))

@sgl.function
def multi_turn_question(s, question1, question2):
    s += sgl.system("You are a helpful assistant.")
    s += sgl.user(question1)
    s += sgl.assistant(sgl.gen("answer_1", max_tokens=256))
    s += sgl.user(question2)
    s += sgl.assistant(sgl.gen("answer_2", max_tokens=256))

state = multi_turn_question.run(
    question1="What is SGLang?",
    question2="How does RadixAttention work?"
)
```

### LangChain

```python
from langchain.llms import OpenAI

llm = OpenAI(
    openai_api_base="http://localhost:40114/olla/sglang/v1",
    openai_api_key="dummy",
    model_name="meta-llama/Meta-Llama-3.1-8B-Instruct",
    temperature=0.7
)
```

### LlamaIndex

```python
from llama_index.llms import OpenAI

llm = OpenAI(
    api_base="http://localhost:40114/olla/sglang/v1",
    api_key="dummy",
    model="meta-llama/Meta-Llama-3.1-8B-Instruct"
)
```

## Advanced Features

### RadixAttention Configuration

```yaml
# Custom RadixAttention settings
features:
  radix_attention:
    enabled: true
    cache_size_ratio: 0.4  # 40% GPU memory for cache
    max_tree_depth: 64     # Maximum prefix tree depth
    eviction_policy: "lru" # Least recently used eviction
```

### Speculative Decoding Setup

```yaml
features:
  speculative_decoding:
    enabled: true
    draft_model_ratio: 0.5  # Draft model size ratio
    acceptance_threshold: 0.8 # Token acceptance threshold
```

### Multimodal Configuration

```yaml
features:
  multimodal:
    enabled: true
    max_image_resolution: 1024  # Maximum image size
    supported_formats:
      - jpeg
      - png
      - webp
```

## Performance Comparison

### SGLang vs vLLM Benchmarks

| Metric | SGLang | vLLM | Improvement |
|--------|--------|------|-------------|
| Throughput (req/s) | 250 | 180 | +39% |
| Memory Usage | -15% | baseline | 15% less |
| Cache Hit Rate | 90% | 65% | +38% |
| TTFT (ms) | 45 | 65 | -31% |
| Max Concurrent | 150 | 100 | +50% |

*Results with Llama-3.1-8B on A100 80GB*

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise SGLang behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple SGLang instances
- [Monitoring](../../configuration/practices/monitoring.md) - Set up Prometheus monitoring
- [Frontend Language Guide](https://sgl-project.github.io/start/frontend_language.html) - Learn SGLang programming