---
title: LM Studio Integration - Olla Proxy
description: Configure and integrate LM Studio with Olla for local LLM inference. Support for OpenAI-compatible API, model management, and optimised performance.
keywords: LM Studio, Olla, LLM proxy, local inference, OpenAI compatible, model serving
---

# LM Studio Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://lmstudio.ai/">lmstudio.ai</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.7</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>lm-studio</code> (use in endpoint configuration)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>lmstudio.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/lmstudio.yaml">latest</a>)</td>
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
                <li>Single Model Concurrency</li>
                <li>Preloaded Models</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/lmstudio</code></li>
                <li><code>/lm-studio</code></li>
                <li><code>/lm_studio</code></li>
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

## Configuration

### Basic Setup

Add LM Studio to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:1234"
        name: "local-lm-studio"
        type: "lm-studio"
        priority: 90
        model_url: "/api/v0/models"
        health_check_url: "/v1/models"
        check_interval: 2s
        check_timeout: 1s
```

### Multiple LM Studio Instances

Run multiple LM Studio servers on different ports:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:1234"
        name: "lm-studio-1"
        type: "lm-studio"
        priority: 100
        
      - url: "http://localhost:1235"
        name: "lm-studio-2"
        type: "lm-studio"
        priority: 90
        
      - url: "http://192.168.1.10:1234"
        name: "lm-studio-remote"
        type: "lm-studio"
        priority: 50
```

## Endpoints Supported

The following endpoints are supported by the LM Studio integration profile:

<table>
  <tr>
    <th style="text-align: left;">Path</th>
    <th style="text-align: left;">Description</th>
  </tr>
  <tr>
    <td><code>/v1/models</code></td>
    <td>List Models & Health Check</td>
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
    <td>Generate Embeddings</td>
  </tr>
  <tr>
    <td><code>/api/v0/models</code></td>
    <td>Legacy Models Endpoint</td>
  </tr>
</table>

## Usage Examples

### Chat Completion

```bash
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3.2-3b-instruct",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ],
    "temperature": 0.7
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/lm-studio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mistral-7b-instruct",
    "messages": [
      {"role": "user", "content": "Write a short poem about coding"}
    ],
    "stream": true
  }'
```

### List Available Models

```bash
curl http://localhost:40114/olla/lm_studio/v1/models
```

## LM Studio Specifics

### Model Loading Behaviour

LM Studio differs from other backends:

- **Preloaded Models**: Models must be loaded in LM Studio before use
- **Single Concurrency**: Only one request processed at a time
- **Fast Response**: No model loading delay during requests

### Resource Configuration

The LM Studio profile includes optimised resource settings:

```yaml
characteristics:
  timeout: 3m
  max_concurrent_requests: 1  # LM Studio handles one at a time
  streaming_support: true
```

### Memory Requirements

LM Studio uses quantised models with reduced memory requirements:

| Model Size | Memory Required | Recommended |
|------------|----------------|-------------|
| 70B | 42GB | 52GB |
| 34B | 20GB | 25GB |
| 13B | 8GB | 10GB |
| 7B | 5GB | 6GB |
| 3B | 2GB | 3GB |

## Profile Customisation

To customise LM Studio behaviour, create `config/profiles/lmstudio-custom.yaml`. See [Profile Configuration](../../concepts/profile-system.md) for detailed explanations of each section.

### Example Customisation

```yaml
name: lm-studio
version: "1.0"

# Add custom prefixes
routing:
  prefixes:
    - lmstudio
    - lm-studio
    - lm_studio
    - studio      # Add custom prefix

# Adjust timeouts for slower hardware
characteristics:
  timeout: 5m     # Increase from 3m
  
# Modify resource limits
resources:
  concurrency_limits:
    - min_memory_gb: 0
      max_concurrent: 1  # Always single-threaded
```

See [Profile Configuration](../../concepts/profile-system.md) for complete customisation options.

## Troubleshooting

### Models Not Appearing

**Issue**: Models don't show in Olla's model list

**Solution**: 
1. Ensure models are loaded in LM Studio UI
2. Check LM Studio is running on the configured port
3. Verify with: `curl http://localhost:1234/v1/models`

### Request Timeout

**Issue**: Requests timeout on large models

**Solution**: Increase timeout in profile:
```yaml
characteristics:
  timeout: 10m  # Increase for large models
```

### Connection Refused

**Issue**: Cannot connect to LM Studio

**Solution**:

1. Verify LM Studio is running
2. Check "Enable CORS" in LM Studio settings
3. Ensure firewall allows the port
4. Test direct connection: `curl http://localhost:1234/v1/models`

### Single Request Limitation

**Issue**: Concurrent requests fail

**Solution**: LM Studio processes one request at a time. Use priority load balancing to route overflow to other endpoints:

```yaml
proxy:
  load_balancer: "priority"
  
discovery:
  static:
    endpoints:
      - url: "http://localhost:1234"
        name: "lm-studio"
        type: "lm-studio"
        priority: 100
        
      - url: "http://localhost:11434"
        name: "ollama-backup"
        type: "ollama"
        priority: 50  # Fallback for concurrent requests
```

## Best Practices

### 1. Use for Interactive Sessions

LM Studio excels at:

- Development and testing
- Interactive chat sessions
- Quick model switching via UI

### 2. Configure Appropriate Timeouts

```yaml
proxy:
  response_timeout: 600s  # 10 minutes for long generations
  read_timeout: 300s      # 5 minutes read timeout
```

### 3. Monitor Memory Usage

LM Studio shows real-time memory usage in its UI. Monitor this to:

- Prevent out-of-memory errors
- Choose appropriate model sizes
- Optimise quantisation levels

### 4. Combine with Other Backends

Use LM Studio for development and Ollama/vLLM for production:

```yaml
discovery:
  static:
    endpoints:
      # Development - high priority
      - url: "http://localhost:1234"
        name: "lm-studio-dev"
        type: "lm-studio"
        priority: 100
        
      # Production - lower priority
      - url: "http://localhost:11434"
        name: "ollama-prod"
        type: "ollama"
        priority: 50
```

## Integration with Tools

### OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/lmstudio/v1",
    api_key="not-needed"  # LM Studio doesn't require API keys
)

response = client.chat.completions.create(
    model="llama-3.2-3b-instruct",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

### LangChain

```python
from langchain.llms import OpenAI

llm = OpenAI(
    openai_api_base="http://localhost:40114/olla/lm-studio/v1",
    openai_api_key="not-needed",
    model_name="mistral-7b-instruct"
)
```

### Continue.dev

Configure Continue to use Olla with LM Studio:

```json
{
  "models": [{
    "title": "LM Studio via Olla",
    "provider": "openai",
    "model": "llama-3.2-3b-instruct",
    "apiBase": "http://localhost:40114/olla/lmstudio/v1"
  }]
}
```

## Next Steps

- [Profile Configuration](../../concepts/profile-system.md) - Customise LM Studio behaviour
- [Model Unification](../../concepts/model-unification.md) - Understand model management
- [Load Balancing](../../concepts/load-balancing.md) - Configure multi-backend setups