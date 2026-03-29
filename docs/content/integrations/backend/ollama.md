---
title: Ollama Integration - Complete Setup and Configuration Guide
description: Comprehensive guide to integrating Ollama with Olla proxy. Learn configuration, endpoints, model management, troubleshooting, and best practices for production deployments.
keywords: ollama integration, ollama proxy, olla ollama, ollama configuration, ollama load balancing, gguf models
---

# Ollama Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/ollama/ollama">github.com/ollama/ollama</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.1</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>ollama</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>ollama.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/ollama.yaml">latest</a>)</td>
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
                <li>GGUF Model Support</li>
                <li>Native Anthropic Messages API (v0.14.0+)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Model Management (pull/push/delete)</li>
                <li>Instance Management</li>
                <li>Model Creation (FROM commands)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>OpenAI Compatible</li>
                <li>Dynamic Model Loading</li>
                <li>Multi-Modal Support (LLaVA)</li>
                <li>Quantisation Support</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/ollama</code> (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)</li>
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

Add Ollama to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"        # optional, profile default: /api/tags
        health_check_url: "/"          # optional, profile default: /
        check_interval: 2s             # optional, default: 5s
        check_timeout: 1s              # optional, default: 2s
```

### Multiple Ollama Instances

Configure multiple Ollama servers for load balancing:

```yaml
discovery:
  static:
    endpoints:
      # Primary GPU server
      - url: "http://gpu-server:11434"
        name: "ollama-gpu"
        type: "ollama"
        priority: 100
        
      # Secondary server
      - url: "http://backup-server:11434"
        name: "ollama-backup"
        type: "ollama"
        priority: 75
        
      # Development machine
      - url: "http://dev-machine:11434"
        name: "ollama-dev"
        type: "ollama"
        priority: 50
```

### Remote Ollama Configuration

For remote Ollama servers:

```yaml
discovery:
  static:
    endpoints:
      - url: "https://ollama.example.com"
        name: "ollama-cloud"
        type: "ollama"
        priority: 80
        check_interval: 10s
        check_timeout: 5s
```

!!! note "Authentication Not Supported"
    Olla does not currently support authentication headers for endpoints. If your Ollama server requires authentication, you'll need to use a reverse proxy or wait for this feature to be added.

## Anthropic Messages API Support

Ollama v0.14.0+ natively supports the Anthropic Messages API, enabling Olla to forward Anthropic-format requests directly without translation overhead (passthrough mode).

When Olla detects that an Ollama endpoint supports native Anthropic format (via the `anthropic_support` section in `config/profiles/ollama.yaml`), it will bypass the Anthropic-to-OpenAI translation pipeline and forward requests directly to `/v1/messages` on the backend.

**Profile configuration** (from `config/profiles/ollama.yaml`):

```yaml
api:
  anthropic_support:
    enabled: true
    messages_path: /v1/messages
    token_count: false
    min_version: "0.14.0"
    limitations:
      - token_counting_404
```

**Key details**:

- Minimum Ollama version: **v0.14.0**
- Token counting (`/v1/messages/count_tokens`): Not supported (returns 404)
- Passthrough mode is automatic -- no client-side configuration needed
- Responses include `X-Olla-Mode: passthrough` header when passthrough is active
- Falls back to translation mode if passthrough conditions are not met

!!! note "Ollama Anthropic Compatibility"
    For details on Ollama's Anthropic compatibility, see the [Ollama Anthropic compatibility documentation](https://docs.ollama.com/api/anthropic-compatibility).

For more information, see [API Translation](../../concepts/api-translation.md#passthrough-mode) and [Anthropic API Reference](../../api-reference/anthropic.md).

## Endpoints Supported

The following endpoints are supported by the Ollama integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/</code></td>
    <td>Health Check</td>
  </tr>
  <tr>
    <td><code>/api/generate</code></td>
    <td>Text Completion (Ollama format)</td>
  </tr>
  <tr>
    <td><code>/api/chat</code></td>
    <td>Chat Completion (Ollama format)</td>
  </tr>
  <tr>
    <td><code>/api/embeddings</code></td>
    <td>Generate Embeddings</td>
  </tr>
  <tr>
    <td><code>/api/tags</code></td>
    <td>List Local Models</td>
  </tr>
  <tr>
    <td><code>/api/show</code></td>
    <td>Show Model Information</td>
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
    <td>Embeddings (OpenAI format)</td>
  </tr>
</table>

## Usage Examples

### Chat Completion (Ollama Format)

```bash
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the meaning of life?"}
    ],
    "stream": false
  }'
```

### Text Generation (Ollama Format)

```bash
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistral:latest",
    "prompt": "Once upon a time",
    "options": {
      "temperature": 0.8,
      "num_predict": 100
    }
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/ollama/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [
      {"role": "user", "content": "Write a haiku about programming"}
    ],
    "stream": true
  }'
```

### OpenAI Compatibility

```bash
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }'
```

### Embeddings

```bash
curl -X POST http://localhost:40114/olla/ollama/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text:latest",
    "prompt": "The quick brown fox jumps over the lazy dog"
  }'
```

### List Available Models

```bash
# Ollama format
curl http://localhost:40114/olla/ollama/api/tags

# OpenAI format
curl http://localhost:40114/olla/ollama/v1/models
```

### Model Information

```bash
curl -X POST http://localhost:40114/olla/ollama/api/show \
  -H "Content-Type: application/json" \
  -d '{"name": "llama3.2:latest"}'
```

## Ollama Specifics

### Model Loading Behaviour

Ollama has unique model loading characteristics:

- **Dynamic Loading**: Models load on first request
- **Memory Management**: Unloads models after idle timeout
- **Loading Delay**: First request to a model can be slow
- **Concurrent Models**: Limited by available memory

### Model Naming Convention

Ollama uses a specific naming format:

```
model:tag
model:version
namespace/model:tag
```

Examples:
- `llama3.2:latest`
- `llama3.2:3b`
- `mistral:7b-instruct-q4_0`
- `library/codellama:13b`

### Quantisation Levels

Ollama supports various quantisation levels:

| Quantisation | Memory Usage | Performance | Quality |
|--------------|--------------|-------------|---------|
| Q4_0 | ~50% | Fast | Good |
| Q4_1 | ~55% | Fast | Better |
| Q5_0 | ~60% | Moderate | Better |
| Q5_1 | ~65% | Moderate | Better |
| Q8_0 | ~85% | Slower | Best |
| F16 | 100% | Slowest | Highest |

### Options Parameters

Ollama-specific generation options:

```json
{
  "options": {
    "temperature": 0.8,      // Randomness (0-1)
    "top_k": 40,            // Top K sampling
    "top_p": 0.9,           // Nucleus sampling
    "num_predict": 128,     // Max tokens to generate
    "stop": ["\\n", "User:"], // Stop sequences
    "seed": 42,             // Reproducible generation
    "num_ctx": 2048,        // Context window size
    "repeat_penalty": 1.1,  // Repetition penalty
    "mirostat": 2,          // Mirostat sampling
    "mirostat_tau": 5.0,    // Mirostat target entropy
    "mirostat_eta": 0.1     // Mirostat learning rate
  }
}
```

## Starting Ollama

### Local Installation

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Start Ollama service
ollama serve

# Pull a model
ollama pull llama3.2:latest

# Test directly
ollama run llama3.2:latest "Hello"
```

### Docker Deployment

```bash
# CPU only
docker run -d \
  --name ollama \
  -p 11434:11434 \
  -v ollama:/root/.ollama \
  ollama/ollama

# With GPU support
docker run -d \
  --gpus all \
  --name ollama \
  -p 11434:11434 \
  -v ollama:/root/.ollama \
  ollama/ollama
```

### Docker Compose

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: unless-stopped
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    environment:
      - OLLAMA_HOST=0.0.0.0
      - OLLAMA_KEEP_ALIVE=5m
      - OLLAMA_MAX_LOADED_MODELS=2
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]

volumes:
  ollama_data:
    driver: local
```

## Profile Customisation

To customise Ollama behaviour, create `config/profiles/ollama-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: ollama
version: "1.0"

# Add custom routing prefixes
routing:
  prefixes:
    - ollama
    - ai        # Add custom prefix

# Adjust for slow model loading
characteristics:
  timeout: 10m   # Increase from 5m for large models
  
# Model capability detection
models:
  capability_patterns:
    vision:
      - "*llava*"
      - "*bakllava*"
      - "vision*"
    embeddings:
      - "*embed*"
      - "nomic-embed-text*"
      - "mxbai-embed*"
    code:
      - "*code*"
      - "codellama*"
      - "deepseek-coder*"
      - "qwen*coder*"
      
# Context window detection
  context_patterns:
    - pattern: "*-32k*"
      context: 32768
    - pattern: "*-16k*"
      context: 16384
    - pattern: "llama3*"
      context: 8192
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Environment Variables

Ollama behaviour can be controlled via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `OLLAMA_HOST` | Bind address | `127.0.0.1:11434` |
| `OLLAMA_MODELS` | Model storage path | `~/.ollama/models` |
| `OLLAMA_KEEP_ALIVE` | Model idle timeout | `5m` |
| `OLLAMA_MAX_LOADED_MODELS` | Max concurrent models | Unlimited |
| `OLLAMA_NUM_PARALLEL` | Parallel request handling | `1` |
| `OLLAMA_MAX_QUEUE` | Max queued requests | `512` |
| `OLLAMA_DEBUG` | Enable debug logging | `false` |

## Multi-Modal Support

### Vision Models (LLaVA)

Ollama supports vision models for image analysis:

```bash
# Pull a vision model
ollama pull llava:latest

# Use with image
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llava:latest",
    "prompt": "What is in this image?",
    "images": ["base64_encoded_image_data"]
  }'
```

### Supported Vision Models

- `llava:latest` - General vision model
- `llava:13b` - Larger vision model
- `bakllava:latest` - Alternative vision model

## Troubleshooting

### Model Not Found

**Issue**: "model not found" error

**Solution**:
1. Ensure model is pulled:
   ```bash
   ollama list  # Check available models
   ollama pull llama3.2:latest  # Pull if missing
   ```

2. Verify model name format:
   ```bash
   # Correct
   "model": "llama3.2:latest"
   
   # Incorrect
   "model": "llama3.2"  # Missing tag
   ```

### Slow First Request

**Issue**: First request to a model is very slow

**Solution**:
1. Pre-load models:
   ```bash
   ollama run llama3.2:latest ""  # Load into memory
   ```

2. Increase keep-alive:
   ```bash
   OLLAMA_KEEP_ALIVE=24h ollama serve
   ```

3. Adjust timeout in Olla:
   ```yaml
   characteristics:
     timeout: 10m  # Allow for model loading
   ```

### Out of Memory

**Issue**: "out of memory" errors

**Solution**:
1. Limit concurrent models:
   ```bash
   OLLAMA_MAX_LOADED_MODELS=1 ollama serve
   ```

2. Use smaller quantisation:
   ```bash
   ollama pull llama3.2:3b-q4_0  # Smaller variant
   ```

3. Configure memory limits:
   ```yaml
   resources:
     model_sizes:
       - patterns: ["7b"]
         min_memory_gb: 8
         max_concurrent: 1
   ```

### Connection Refused

**Issue**: Cannot connect to Ollama

**Solution**:
1. Check Ollama is running:
   ```bash
   ps aux | grep ollama
   systemctl status ollama  # If using systemd
   ```

2. Verify bind address:
   ```bash
   OLLAMA_HOST=0.0.0.0:11434 ollama serve
   ```

3. Check firewall:
   ```bash
   sudo ufw allow 11434  # Ubuntu/Debian
   ```

## Best Practices

### 1. Use Model Unification

With multiple Ollama instances, enable unification:

```yaml
model_registry:
  enable_unifier: true
  unification:
    enabled: true
```

This provides a single model catalogue across all instances.

### 2. Configure Appropriate Timeouts

Account for model loading times:

```yaml
proxy:
  response_timeout: 600s  # 10 minutes for large models (default)
  connection_timeout: 30s  # Default connection timeout
  
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        check_timeout: 5s  # Allow time for health checks
```

### 3. Optimise for Your Hardware

#### For GPU Servers

```yaml
endpoints:
  - url: "http://gpu-server:11434"
    name: "ollama-gpu"
    priority: 100  # Prefer GPU
    
resources:
  concurrency_limits:
    - min_memory_gb: 0
      max_concurrent: 4  # GPU can handle multiple
```

#### For CPU Servers

```yaml
endpoints:
  - url: "http://cpu-server:11434"
    name: "ollama-cpu"
    priority: 50  # Lower priority
    
resources:
  concurrency_limits:
    - min_memory_gb: 0
      max_concurrent: 1  # CPU limited to one
```

### 4. Monitor Performance

Use Olla's status endpoints:

```bash
# Check health
curl http://localhost:40114/internal/health

# View endpoint status
curl http://localhost:40114/internal/status/endpoints

# Monitor model availability
curl http://localhost:40114/internal/status/models
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/ollama/v1",
    api_key="not-needed"  # Ollama doesn't require API keys
)

response = client.chat.completions.create(
    model="llama3.2:latest",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### LangChain

```python
from langchain_community.llms import Ollama

llm = Ollama(
    base_url="http://localhost:40114/olla/ollama",
    model="llama3.2:latest"
)

response = llm.invoke("Tell me a joke")
```

### Continue.dev

Configure Continue to use Olla with Ollama:

```json
{
  "models": [{
    "title": "Ollama via Olla",
    "provider": "ollama",
    "model": "llama3.2:latest",
    "apiBase": "http://localhost:40114/olla/ollama"
  }]
}
```

### Aider

```bash
# Use with Aider
aider --openai-api-base http://localhost:40114/olla/ollama/v1 \
      --model llama3.2:latest
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise Ollama behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Configure multi-instance setups
- [OpenWebUI Integration](../frontend/openwebui.md) - Set up web interface