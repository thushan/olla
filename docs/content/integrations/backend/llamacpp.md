---
title: llama.cpp Integration - Lightweight C++ Inference with Olla
description: Configure llama.cpp with Olla proxy for efficient GGUF model serving. Slot-based concurrency, CPU-first design, code infill support, and extensive quantization options.
keywords: [llamacpp, llama.cpp, Olla proxy, GGUF models, CPU inference, slot management, code infill, quantization, edge deployment, lightweight inference, gguf quantization]
---

# llama.cpp Integration

<table>
    <tr>
        <th>Home</th>
        <td>
          <a href="https://github.com/ggml-org/llama.cpp">github.com/ggml-org/llama.cpp</a> <br/>
          <a href="ttps://github.com/ikawrakow/ik_llama.cpp">github.com/gikawrakow/ik_llama.cpp</a> <br/>
        </td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.20</code> (previously since v0.0.1)</td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>llamacpp</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>llamacpp.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/llamacpp.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Model Unification</li>
                <li>Model Detection & Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>Tokenisation API</li>
                <li>Code Infill (FIM Support)</li>
                <li>GGUF Exclusive Format</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Runtime Model Switching</li>
                <li>Multi-Model Per Instance</li>
                <li>Model Management (loading/unloading)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>OpenAI Compatible</li>
                <li>Single Model Architecture</li>
                <li>Slot-Based Concurrency</li>
                <li>CPU Inference Ready</li>
                <li>GGUF Exclusive</li>
                <li>Edge Device Friendly</li>
                <li>Lightweight C++</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/llamacpp</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
                <li><code>/llama-cpp</code></li>
                <li><code>/llama_cpp</code></li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Priority</th>
        <td>95 (high priority, between Ollama and LM Studio)</td>
    </tr>
    <tr>
        <th>Endpoints</th>
        <td>
            See <a href="#endpoints-supported">below</a>
        </td>
    </tr>
</table>

!!! note "Compatibility with mainline LlamaCpp"
    Primary development was done for compatibility with the original [llamacpp](https://github.com/ggml-org/llama.cpp) and tested on forks like [ik_llama](https://github.com/ikawrakow/ik_llama.cpp) but we may not support the wider forks yet.



## Configuration

### Basic Setup

Add llama.cpp to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8080"
        name: "local-llamacpp"
        type: "llamacpp"
        priority: 95
        # Profile handles health checks and model discovery
```

### Production Setup

Configure llama.cpp for production with proper timeouts:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://inference-server:8080"
        name: "llamacpp-prod"
        type: "llamacpp"
        priority: 95
        # Profile handles health checks and model discovery

proxy:
  engine: "olla"  # Use high-performance engine
  load_balancer: "round-robin"
```

### Multiple Instances with Different Quantisations

Deploy multiple llama.cpp instances with different quantisation levels:

```yaml
discovery:
  static:
    endpoints:
      # High quality Q8 instance
      - url: "http://gpu-server:8080"
        name: "llamacpp-q8-quality"
        type: "llamacpp"
        priority: 100

      # Balanced Q4 instance
      - url: "http://cpu-server:8081"
        name: "llamacpp-q4-balanced"
        type: "llamacpp"
        priority: 80

      # Fast Q2 instance for edge
      - url: "http://edge-device:8082"
        name: "llamacpp-q2-fast"
        type: "llamacpp"
        priority: 60
```

## Endpoints Supported

The following 9 inference endpoints are proxied by the llama.cpp integration profile:

> **Note:** Monitoring endpoints (`/health`, `/props`, `/slots`, `/metrics`) are not exposed through the Olla proxy as per architectural design. The `/olla/*` endpoints are strictly for inference requests. Health checks are handled internally by Olla, and monitoring should use Olla's `/internal/*` endpoints or direct backend access.

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/v1/models</code></td>
    <td>List Models (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/completion</code></td>
    <td>Native Completion Endpoint</td>
  </tr>
  <tr>
    <td><code>/v1/completions</code></td>
    <td>Text Completions (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/v1/chat/completions</code></td>
    <td>Chat Completions (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/embedding</code></td>
    <td>Native Embedding Endpoint</td>
  </tr>
  <tr>
    <td><code>/v1/embeddings</code></td>
    <td>Embeddings (OpenAI format)</td>
  </tr>
  <tr>
    <td><code>/tokenize</code></td>
    <td>Encode Text to Tokens (llama.cpp-specific)</td>
  </tr>
  <tr>
    <td><code>/detokenize</code></td>
    <td>Decode Tokens to Text (llama.cpp-specific)</td>
  </tr>
  <tr>
    <td><code>/infill</code></td>
    <td>Code Infill/Completion (llama.cpp-specific, FIM)</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3.2-3b-instruct-q4_k_m.gguf",
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
curl -X POST http://localhost:40114/olla/llamacpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistral-7b-instruct-q4_k_m.gguf",
    "messages": [
      {"role": "user", "content": "Write a story about a robot"}
    ],
    "stream": true,
    "temperature": 0.8
  }'
```

### Code Infill (FIM Support)

Code completion using Fill-In-the-Middle (llama.cpp-specific):

```bash
curl -X POST http://localhost:40114/olla/llamacpp/infill \
  -H "Content-Type: application/json" \
  -d '{
    "input_prefix": "def fibonacci(n):\n    if n <= 1:\n        return n\n    ",
    "input_suffix": "\n    return result",
    "temperature": 0.2,
    "max_tokens": 100
  }'

# Useful for IDE integrations like Continue.dev, Aider
```

### Tokenisation

Encode and decode tokens using the model's tokeniser (llama.cpp-specific):

```bash
# Encode text to tokens
curl -X POST http://localhost:40114/olla/llamacpp/tokenize \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Hello, world!"
  }'

# Response: {"tokens": [15496, 11, 1917, 0]}

# Decode tokens to text
curl -X POST http://localhost:40114/olla/llamacpp/detokenize \
  -H "Content-Type: application/json" \
  -d '{
    "tokens": [15496, 11, 1917, 0]
  }'

# Response: {"content": "Hello, world!"}
```

### Embeddings

Generate embeddings for semantic search:

```bash
curl -X POST http://localhost:40114/olla/llamacpp/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text-v1.5-q4_k_m.gguf",
    "input": "The quick brown fox jumps over the lazy dog"
  }'
```

### List Models

```bash
# OpenAI format
curl http://localhost:40114/olla/llamacpp/v1/models

# Response typically shows single model (single-model architecture)
# {
#   "object": "list",
#   "data": [
#     {
#       "id": "llama-3.2-3b-instruct-q4_k_m.gguf",
#       "object": "model",
#       "created": 1704067200,
#       "owned_by": "meta-llama"
#     }
#   ]
# }
```

## llama.cpp Specifics

### Single Model Architecture

llama.cpp serves **one model per instance**, loaded at startup:

- **No Runtime Switching**: Cannot change models without restart
- **Model Discovery**: Returns single model in `/v1/models` response
- **Efficient Memory**: All resources dedicated to one model
- **Predictable Performance**: No model switching overhead

This differs from Ollama (multi-model) and requires running multiple llama.cpp instances for multiple models.

### Slot Management

llama.cpp uses **slot-based concurrency** for fine-grained control:

- **Default Slots**: 4 concurrent processing slots (configurable with `--parallel`)
- **Explicit Control**: Each slot handles one request at a time
- **Queue Management**: Additional requests queue when slots full
- **Monitoring**: Available via direct backend access at `http://backend:8080/slots`
- **Capacity Planning**: Adjust slots based on hardware and model size

Slot configuration example:
```bash
# Start with 8 slots for higher concurrency
llama-server -m model.gguf --parallel 8 --port 8080
```

> **Note:** Slot monitoring is not available through Olla proxy paths. Access slot status directly from the llama.cpp backend or use Olla's internal monitoring endpoints.

### CPU Inference Capabilities

llama.cpp is optimised for **CPU-first deployment**:

- **No GPU Required**: Full functionality on CPU-only systems
- **ARM Support**: Runs on Apple Silicon (M1/M2/M3), Raspberry Pi
- **Edge Deployment**: Suitable for IoT and embedded systems
- **Portable**: Pure C++ with minimal dependencies

CPU performance tips:
- Use Q4 quantisation for best CPU performance/quality trade-off
- Allocate sufficient threads (`--threads`)
- Consider smaller models (3B-7B) for CPU deployment

### Quantisation Options

llama.cpp provides extensive GGUF quantisation levels:

<table>
  <tr>
    <th>Quantisation</th>
    <th>BPW (Bits Per Weight)</th>
    <th>Memory vs F16</th>
    <th>Quality</th>
    <th>Use Case</th>
  </tr>
  <tr>
    <td><code>Q2_K</code></td>
    <td>2.63</td>
    <td>35%</td>
    <td>Low</td>
    <td>Extreme compression, edge devices</td>
  </tr>
  <tr>
    <td><code>Q3_K_M</code></td>
    <td>3.91</td>
    <td>45%</td>
    <td>Moderate</td>
    <td>Balanced compression</td>
  </tr>
  <tr>
    <td><code>Q4_K_M</code></td>
    <td>4.85</td>
    <td>50%</td>
    <td>Good</td>
    <td><strong>Recommended for most use cases</strong></td>
  </tr>
  <tr>
    <td><code>Q5_K_M</code></td>
    <td>5.69</td>
    <td>62.5%</td>
    <td>High</td>
    <td>Quality-focused deployments</td>
  </tr>
  <tr>
    <td><code>Q6_K</code></td>
    <td>6.59</td>
    <td>75%</td>
    <td>Very High</td>
    <td>Near-original quality</td>
  </tr>
  <tr>
    <td><code>Q8_0</code></td>
    <td>8.50</td>
    <td>87.5%</td>
    <td>Excellent</td>
    <td>Production quality requirements</td>
  </tr>
  <tr>
    <td><code>F16</code></td>
    <td>16</td>
    <td>100%</td>
    <td>Original</td>
    <td>Baseline reference</td>
  </tr>
  <tr>
    <td><code>F32</code></td>
    <td>32</td>
    <td>200%</td>
    <td>Perfect</td>
    <td>Research, original weights</td>
  </tr>
</table>

### Memory Requirements

Approximate memory requirements for different model sizes with Q4_K_M quantisation:

<table>
  <tr>
    <th>Model Size</th>
    <th>Q4_K_M Memory</th>
    <th>Q8_0 Memory</th>
    <th>Recommended RAM</th>
    <th>Max Slots (Typical)</th>
  </tr>
  <tr>
    <td>1-3B</td>
    <td>2-3GB</td>
    <td>3-5GB</td>
    <td>4GB</td>
    <td>8</td>
  </tr>
  <tr>
    <td>7-8B</td>
    <td>4-6GB</td>
    <td>8-12GB</td>
    <td>8GB</td>
    <td>4</td>
  </tr>
  <tr>
    <td>13-14B</td>
    <td>8-10GB</td>
    <td>16-20GB</td>
    <td>16GB</td>
    <td>2</td>
  </tr>
  <tr>
    <td>30-34B</td>
    <td>20GB</td>
    <td>40GB</td>
    <td>24GB</td>
    <td>2</td>
  </tr>
  <tr>
    <td>70-72B</td>
    <td>40GB</td>
    <td>80GB</td>
    <td>48GB</td>
    <td>1</td>
  </tr>
</table>

Note: Memory requirements increase with context length. Add ~1GB per 8K context tokens.

## Starting llama.cpp Server

### Basic Start

```bash
# Download llama.cpp
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp
make

# Start server with model
./llama-server \
  -m models/llama-3.2-3b-instruct-q4_k_m.gguf \
  --port 8080
```

### With Slots Configuration

```bash
# Configure 8 concurrent slots
./llama-server \
  -m models/mistral-7b-instruct-q4_k_m.gguf \
  --port 8080 \
  --parallel 8 \
  --threads 8
```

### CPU-Only Example

Optimised for CPU inference:

```bash
./llama-server \
  -m models/llama-3.2-3b-instruct-q4_k_m.gguf \
  --port 8080 \
  --threads 8 \
  --ctx-size 8192 \
  --parallel 4 \
  --host 0.0.0.0
```

### GPU Acceleration (CUDA)

For NVIDIA GPUs:

```bash
# Build with CUDA support
make GGML_CUDA=1

# Start with GPU offloading
./llama-server \
  -m models/llama-3.2-3b-instruct-q4_k_m.gguf \
  --port 8080 \
  --n-gpu-layers 35 \
  --parallel 8
```

### GPU Acceleration (Metal)

For Apple Silicon (M1/M2/M3):

```bash
# Build with Metal support (default on macOS)
make

# Start with GPU acceleration
./llama-server \
  -m models/llama-3.2-3b-instruct-q4_k_m.gguf \
  --port 8080 \
  --n-gpu-layers 35 \
  --parallel 4
```

### Docker Deployment

```bash
# Pull llama.cpp server image
docker pull ghcr.io/ggerganov/llama.cpp:server

# Run with model volume
docker run -d \
  --name llamacpp \
  -p 8080:8080 \
  -v /path/to/models:/models \
  ghcr.io/ggerganov/llama.cpp:server \
  -m /models/llama-3.2-3b-instruct-q4_k_m.gguf \
  --host 0.0.0.0 \
  --port 8080 \
  --parallel 4

# With GPU support (NVIDIA)
docker run -d \
  --gpus all \
  --name llamacpp-gpu \
  -p 8080:8080 \
  -v /path/to/models:/models \
  ghcr.io/ggerganov/llama.cpp:server-cuda \
  -m /models/mistral-7b-instruct-q4_k_m.gguf \
  --host 0.0.0.0 \
  --port 8080 \
  --n-gpu-layers 35 \
  --parallel 8
```

## Profile Customisation

To customise llama.cpp behaviour, create `config/profiles/llamacpp-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: llamacpp
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - llamacpp
    - cpp       # Add custom prefix
    - local     # Add custom prefix

# Adjust for larger models and CPU inference
characteristics:
  timeout: 10m                  # Increase for large models
  max_concurrent_requests: 2    # Reduce for limited hardware

# Model capability detection
models:
  capability_patterns:
    code:
      - "*deepseek-coder*"
      - "*codellama*"
      - "*starcoder*"
      - "*phind-codellama*"
    embeddings:
      - "*embed*"
      - "*nomic*"
      - "*bge*"

# Custom context patterns
  context_patterns:
    - pattern: "*-128k*"
      context: 131072
    - pattern: "*qwen2.5*"
      context: 32768

# Slot configuration
resources:
  slot_configuration:
    default_slots: 8          # Increase for more concurrency
    max_slots: 16
    slot_monitoring: true

  # Adjust concurrency for hardware
  concurrency_limits:
    - min_memory_gb: 20       # Large models
      max_concurrent: 1
    - min_memory_gb: 8        # Medium models
      max_concurrent: 4
    - min_memory_gb: 0        # Small models
      max_concurrent: 8
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Troubleshooting

### Slot Exhaustion (504 Errors)

**Issue**: "all slots are busy" or 504 timeout errors

**Solution**:

1. Increase parallel slots:
   ```bash
   ./llama-server -m model.gguf --parallel 8
   ```

2. Add more llama.cpp instances:
   ```yaml
   endpoints:
     - url: "http://server1:8080"
       name: "llamacpp-1"
     - url: "http://server2:8080"
       name: "llamacpp-2"
   ```

### Model Loading Failures

**Issue**: Model fails to load or crashes at startup

**Solution**:

1. Verify GGUF file integrity:
   ```bash
   # Check file size and format
   file model.gguf
   ```

2. Check memory requirements:
   ```bash
   # Estimate memory needed (Q4_K_M ≈ 0.5 bytes per parameter)
   # 7B model = 7 billion × 0.5 bytes = 3.5GB
   ```

3. Reduce context size:
   ```bash
   ./llama-server -m model.gguf --ctx-size 4096  # Reduce from default
   ```

4. Use smaller quantisation (Q4 instead of Q8):
   ```bash
   # Download Q4 variant instead of Q8
   ```

### Memory Issues

**Issue**: Out of memory errors or system freezing

**Solution**:

1. Use more aggressive quantisation:
   - Q8 → Q5_K_M (37.5% memory saving)
   - Q5_K_M → Q4_K_M (50% memory saving)
   - Q4_K_M → Q3_K_M (55% memory saving)

2. Reduce parallel slots:
   ```bash
   ./llama-server -m model.gguf --parallel 1  # Single concurrent request
   ```

3. Limit context window:
   ```bash
   ./llama-server -m model.gguf --ctx-size 2048  # Smaller context
   ```

4. Monitor memory usage:
   ```bash
   # Linux
   watch -n 1 'free -h'

   # macOS
   vm_stat
   ```

### Connection Timeouts

**Issue**: Requests timeout before completion

**Solution**:

1. Increase Olla timeout:
   ```yaml
   proxy:
     response_timeout: 600s  # 10 minutes
   ```

2. Increase llama.cpp timeout in profile:
   ```yaml
   characteristics:
     timeout: 10m
   ```

3. Monitor performance via Olla internal endpoints:
   ```bash
   # Check Olla's internal status
   curl http://localhost:40114/internal/status
   ```

### GGUF Format Incompatibility

**Issue**: "invalid model file" or version errors

**Solution**:

1. Update llama.cpp to latest version:
   ```bash
   cd llama.cpp
   git pull
   make clean && make
   ```

2. Re-download model with compatible GGUF version:
   ```bash
   # Check model compatibility with llama.cpp version
   ```

3. Verify GGUF metadata:
   ```bash
   # Use llama.cpp tools to inspect GGUF file
   ./llama-cli --model model.gguf --verbose
   ```

## Best Practices

### 1. Slot Configuration for Workload

Match slots to your workload pattern:

```yaml
# High concurrency (many short requests)
resources:
  slot_configuration:
    default_slots: 8
    max_slots: 16

# Low concurrency (few long requests)
resources:
  slot_configuration:
    default_slots: 2
    max_slots: 4
```

### 2. Quantisation Selection Guide

Choose quantisation based on requirements:

| Priority | Recommended Quantisation | Use Case |
|----------|------------------------|----------|
| Quality First | Q8_0, Q6_K | Production, quality-critical |
| Balanced | **Q4_K_M** | General purpose, recommended |
| Speed/Memory | Q3_K_M, Q2_K | Edge devices, limited resources |
| Research | F16, F32 | Benchmarking, development |

### 3. CPU vs GPU Deployment Decisions

**Use CPU when:**
- GPU not available (edge devices, workstations)
- Small models (1-7B with Q4 quantisation)
- Low concurrency requirements
- Cost-sensitive deployments

**Use GPU when:**
- Available GPU memory (8GB+)
- Large models (13B+)
- High throughput requirements
- Low latency critical

### 4. Multiple Instance Patterns

Deploy multiple llama.cpp instances strategically:

```yaml
# Pattern 1: Same model, different servers (load balancing)
endpoints:
  - url: "http://server1:8080"
    name: "llamacpp-1"
    priority: 100
  - url: "http://server2:8080"
    name: "llamacpp-2"
    priority: 100

# Pattern 2: Different models, different instances
endpoints:
  - url: "http://server1:8080"  # llama-3.2-3b-q4
    name: "llamacpp-small"
    priority: 90
  - url: "http://server2:8081"  # mistral-7b-q4
    name: "llamacpp-medium"
    priority: 95

# Pattern 3: Different quantisations, quality tiers
endpoints:
  - url: "http://server1:8080"  # Q8 high quality
    name: "llamacpp-quality"
    priority: 100
  - url: "http://server2:8080"  # Q4 balanced
    name: "llamacpp-balanced"
    priority: 80
```

### 5. Memory Management

Plan memory allocation carefully:

```bash
# Reserve memory headroom (20% buffer recommended)
# For 7B Q4 model requiring 5GB:
# System RAM needed = 5GB × 1.2 = 6GB minimum

# Monitor actual usage
./llama-server -m model.gguf --verbose

# Adjust slots based on memory:
# Total RAM / (Model Memory + Context Memory) = Max Slots
# 16GB / (5GB model + 1GB context) ≈ 2-3 safe slots
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/llamacpp/v1",
    api_key="not-needed"  # llama.cpp doesn't require API keys
)

response = client.chat.completions.create(
    model="llama-3.2-3b-instruct-q4_k_m.gguf",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)

print(response.choices[0].message.content)
```

### LangChain

```python
from langchain.llms import OpenAI

llm = OpenAI(
    openai_api_base="http://localhost:40114/olla/llamacpp/v1",
    openai_api_key="not-needed",
    model_name="mistral-7b-instruct-q4_k_m.gguf"
)

response = llm("Explain machine learning")
print(response)
```

### Continue.dev (Code Completion)

Configure Continue for IDE code completion:

```json
{
  "models": [{
    "title": "llama.cpp via Olla",
    "provider": "openai",
    "model": "deepseek-coder-6.7b-instruct-q5_k_m.gguf",
    "apiBase": "http://localhost:40114/olla/llamacpp/v1",
    "useLegacyCompletionsEndpoint": false
  }],
  "tabAutocompleteModel": {
    "title": "llama.cpp Autocomplete",
    "provider": "openai",
    "model": "deepseek-coder-1.3b-base-q4_k_m.gguf",
    "apiBase": "http://localhost:40114/olla/llamacpp/v1"
  }
}
```

### Aider (Pair Programming)

```bash
# Use llama.cpp with Aider for code assistance
aider \
  --openai-api-base http://localhost:40114/olla/llamacpp/v1 \
  --model deepseek-coder-6.7b-instruct-q5_k_m.gguf \
  --no-auto-commits

# For code infill (FIM) with compatible models
aider \
  --openai-api-base http://localhost:40114/olla/llamacpp/v1 \
  --model codellama-13b-instruct-q4_k_m.gguf \
  --edit-format whole
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise llama.cpp behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management across instances
- [Load Balancing](../../concepts/load-balancing.md) - Scale with multiple llama.cpp instances
- [Monitoring](../../configuration/practices/monitoring.md) - Set up slot and performance monitoring
