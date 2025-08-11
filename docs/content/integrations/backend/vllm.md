---
title: vLLM Integration - High-Performance LLM Inference with Olla
description: Configure vLLM with Olla proxy for production-grade LLM serving. PagedAttention, continuous batching, tensor parallelism, and GPU optimization.
keywords: vLLM, Olla proxy, LLM inference, GPU optimization, PagedAttention, tensor parallelism, high throughput
---

# vLLM Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/vllm-project/vllm">github.com/vllm-project/vllm</a></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>vllm</code> (use in endpoint configuration)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>vllm.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/vllm.yaml">latest</a>)</td>
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
                <li>Tokenisation API</li>
                <li>Reranking API</li>
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
                <li>High Concurrency (PagedAttention)</li>
                <li>GPU Optimised</li>
                <li>Continuous Batching</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/vllm</code> (see <a href="../../concepts/profile-system.md#routing-prefixes">Routing Prefixes</a>)</li>
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

Add vLLM to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "local-vllm"
        type: "vllm"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

### Production Setup

Configure vLLM for high-throughput production:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-server:8000"
        name: "vllm-prod"
        type: "vllm"
        priority: 100
        health_check_url: "/health"
        check_interval: 10s
        check_timeout: 5s
        
      - url: "http://gpu-server:8001"
        name: "vllm-prod-2"
        type: "vllm"
        priority: 100
        health_check_url: "/health"
        check_interval: 10s
        check_timeout: 5s
        
proxy:
  engine: "olla"  # Use high-performance engine
  load_balancer: "least-connections"
```

## Endpoints Supported

The following endpoints are supported by the vLLM integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/health</code></td>
    <td>Health Check (vLLM-specific)</td>
  </tr>
  <tr>
    <td><code>/metrics</code></td>
    <td>Prometheus Metrics</td>
  </tr>
  <tr>
    <td><code>/version</code></td>
    <td>vLLM Version Information</td>
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
    <td><code>/tokenize</code></td>
    <td>Encode Text to Tokens</td>
  </tr>
  <tr>
    <td><code>/detokenize</code></td>
    <td>Decode Tokens to Text</td>
  </tr>
  <tr>
    <td><code>/v1/tokenize</code></td>
    <td>Versioned Tokenise Endpoint</td>
  </tr>
  <tr>
    <td><code>/v1/detokenize</code></td>
    <td>Versioned Detokenise Endpoint</td>
  </tr>
  <tr>
    <td><code>/rerank</code></td>
    <td>Reranking API</td>
  </tr>
  <tr>
    <td><code>/v1/rerank</code></td>
    <td>Versioned Reranking API</td>
  </tr>
  <tr>
    <td><code>/v2/rerank</code></td>
    <td>v2 Reranking API</td>
  </tr>
  <tr>
    <td><code>/get_tokenizer_info</code></td>
    <td>Tokeniser Configuration Info</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Explain quantum computing in simple terms"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistralai/Mistral-7B-Instruct-v0.2",
    "messages": [
      {"role": "user", "content": "Write a story about a robot"}
    ],
    "stream": true,
    "temperature": 0.8
  }'
```

### Tokenisation

```bash
# Encode text to tokens
curl -X POST http://localhost:40114/olla/vllm/tokenize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Hello, world!",
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct"
  }'

# Decode tokens to text
curl -X POST http://localhost:40114/olla/vllm/detokenize \
  -H "Content-Type: application/json" \
  -d '{
    "tokens": [15496, 11, 1917, 0],
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct"
  }'
```

### Reranking

```bash
curl -X POST http://localhost:40114/olla/vllm/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "model": "BAAI/bge-reranker-v2-m3",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence",
      "The weather today is sunny",
      "ML algorithms learn from data"
    ]
  }'
```

### Metrics Access

```bash
# Get Prometheus metrics
curl http://localhost:40114/olla/vllm/metrics

# Check health status
curl http://localhost:40114/olla/vllm/health

# Get version information
curl http://localhost:40114/olla/vllm/version
```

## vLLM Specifics

### High-Performance Features

vLLM includes several optimisations:

- **PagedAttention**: Memory-efficient attention mechanism
- **Continuous Batching**: Dynamic request batching
- **Tensor Parallelism**: Multi-GPU support
- **Quantisation Support**: INT4/INT8 for reduced memory

### Resource Configuration

The vLLM profile includes GPU-optimised settings:

```yaml
characteristics:
  timeout: 2m
  max_concurrent_requests: 100  # High concurrency support
  streaming_support: true

resources:
  defaults:
    requires_gpu: true
    min_gpu_memory_gb: 8
```

### Memory Requirements

vLLM requires more memory for KV cache:

| Model Size | GPU Memory Required | Recommended | Max Concurrent |
|------------|-------------------|-------------|----------------|
| 70B | 140GB | 160GB | 10 |
| 34B | 70GB | 80GB | 20 |
| 13B | 30GB | 40GB | 50 |
| 7B | 16GB | 24GB | 100 |
| 3B | 8GB | 12GB | 100 |

### Model Naming

vLLM uses full HuggingFace model names:

- `meta-llama/Meta-Llama-3.1-8B-Instruct`
- `mistralai/Mistral-7B-Instruct-v0.2`
- `codellama/CodeLlama-13b-Instruct-hf`

## Starting vLLM Server

### Basic Start

```bash
python -m vllm.entrypoints.openai.api_server \
  --model meta-llama/Meta-Llama-3.1-8B-Instruct \
  --port 8000
```

### Production Configuration

```bash
python -m vllm.entrypoints.openai.api_server \
  --model meta-llama/Meta-Llama-3.1-70B-Instruct \
  --tensor-parallel-size 4 \
  --gpu-memory-utilization 0.95 \
  --max-model-len 32768 \
  --port 8000 \
  --host 0.0.0.0
```

### Docker Deployment

```bash
docker run --gpus all \
  -v ~/.cache/huggingface:/root/.cache/huggingface \
  -p 8000:8000 \
  vllm/vllm-openai:latest \
  --model meta-llama/Meta-Llama-3.1-8B-Instruct
```

## Profile Customisation

To customise vLLM behaviour, create `config/profiles/vllm-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: vllm
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - vllm
    - gpu      # Add custom prefix

# Adjust for larger models
characteristics:
  timeout: 5m     # Increase for 70B models
  
# Modify concurrency limits
resources:
  concurrency_limits:
    - min_memory_gb: 100
      max_concurrent: 5    # Reduce for very large models
    - min_memory_gb: 50
      max_concurrent: 15   # Adjust based on GPU memory
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Monitoring

### Prometheus Metrics

vLLM exposes detailed metrics at `/metrics`:

```yaml
# Example Prometheus configuration
scrape_configs:
  - job_name: 'vllm'
    static_configs:
      - targets: ['localhost:40114']
    metrics_path: '/olla/vllm/metrics'
```

Key metrics include:

- `vllm:num_requests_running` - Active requests
- `vllm:num_requests_waiting` - Queued requests
- `vllm:gpu_cache_usage_perc` - GPU cache utilisation
- `vllm:time_to_first_token_seconds` - TTFT latency

### Health Monitoring

```bash
# Check health endpoint
curl http://localhost:40114/olla/vllm/health

# Response when healthy
{"status": "healthy"}

# Response when unhealthy
{"status": "unhealthy", "reason": "model not loaded"}
```

## Troubleshooting

### Out of Memory (OOM)

**Issue**: CUDA out of memory errors

**Solution**: 
1. Reduce `--gpu-memory-utilization` (default 0.9)
2. Decrease `--max-model-len`
3. Use quantisation (`--quantization awq` or `--quantization gptq`)
4. Enable tensor parallelism for multi-GPU

### Slow First Token

**Issue**: High time to first token (TTFT)

**Solution**:

1. Enable prefix caching: `--enable-prefix-caching`
2. Increase GPU memory utilisation
3. Use smaller model or quantisation

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

1. Add more vLLM instances
2. Use load balancing across multiple servers
3. Increase `--max-num-seqs` (default 256)

## Best Practices

### 1. Use Appropriate GPU Memory

```bash
# Conservative setting for stability
--gpu-memory-utilization 0.9

# Aggressive setting for throughput
--gpu-memory-utilization 0.95
```

### 2. Configure Tensor Parallelism

For models requiring multiple GPUs:

```bash
# 70B model on 4x A100 80GB
--tensor-parallel-size 4

# 34B model on 2x A100 40GB
--tensor-parallel-size 2
```

### 3. Enable Prefix Caching

For chat applications with system prompts:

```bash
--enable-prefix-caching
--block-size 16
```

### 4. Monitor and Scale

```yaml
# Multiple vLLM instances
discovery:
  static:
    endpoints:
      - url: "http://gpu1:8000"
        name: "vllm-1"
        type: "vllm"
        priority: 100
        
      - url: "http://gpu2:8000"
        name: "vllm-2"
        type: "vllm"
        priority: 100
        
proxy:
  load_balancer: "least-connections"
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/vllm/v1",
    api_key="not-needed"  # vLLM doesn't require API keys
)

response = client.chat.completions.create(
    model="meta-llama/Meta-Llama-3.1-8B-Instruct",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### LangChain

```python
from langchain.llms import VLLM

llm = VLLM(
    model="meta-llama/Meta-Llama-3.1-8B-Instruct",
    vllm_server="http://localhost:40114/olla/vllm",
    temperature=0.7
)
```

### LlamaIndex

```python
from llama_index.llms import OpenAI

llm = OpenAI(
    api_base="http://localhost:40114/olla/vllm/v1",
    api_key="dummy",
    model="meta-llama/Meta-Llama-3.1-8B-Instruct"
)
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise vLLM behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple vLLM instances
- [Monitoring](../../configuration/practices/monitoring.md) - Set up Prometheus monitoring