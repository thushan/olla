---
title: OpenCode + Olla Integration
description: Configure OpenCode AI coding assistant to use local LLM models via Olla's API endpoints. Supports both OpenAI and Anthropic API formats for load-balancing, failover, and model unification across Ollama, LM Studio, vLLM, and other backends.
keywords: OpenCode, Olla, SST, AI SDK, local LLM, Ollama, vLLM, LM Studio, load balancing, API translation, coding assistant
---

# OpenCode Integration with Olla

OpenCode is an open-source AI coding assistant that can connect to Olla's API endpoints, enabling you to use local LLM infrastructure with flexible OpenAI or Anthropic API compatibility.

**Set in OpenCode config** (`~/.opencode/config.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:40114/olla/openai/v1"
      }
    }
  }
}
```

**What you get via Olla**

* One stable API base URL for all local backends
* Priority/least-connections load-balancing and health checks
* Streaming passthrough
* Unified `/v1/models` across providers
* Support for both OpenAI and Anthropic API formats

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://github.com/sst/opencode">OpenCode</a> (SST fork of archived AI coding assistant)</td>
    </tr>
    <tr>
        <th>Status</th>
        <td>Original project archived; actively maintained by <a href="https://sst.dev">SST</a></td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Frontend UI / Terminal Coding Assistant</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>AI SDK with OpenAI-compatible or Anthropic providers</td>
    </tr>
    <tr>
        <th>
          Features Supported <br/>
          <small>(via Olla)</small>
        </th>
        <td>
            <ul>
                <li>Chat Completions</li>
                <li>Code Generation & Editing</li>
                <li>Streaming Responses</li>
                <li>Model Selection</li>
                <li>Tool Use (Function Calling)</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Configuration</th>
        <td>
            Edit <code>~/.opencode/config.json</code> to add Olla provider <br/>
            <pre>baseURL: "http://localhost:40114/olla/openai/v1"</pre>
            or
            <pre>baseURL: "http://localhost:40114/olla/anthropic/v1"</pre>
        </td>
    </tr>
    <tr>
        <th>Example</th>
        <td>
            Complete working example available in <code>examples/opencode-lmstudio/</code>
        </td>
    </tr>
</table>

## What is OpenCode?

OpenCode is an open-source AI coding assistant built with Node.js and TypeScript that provides:

- **Intelligent Code Generation**: Context-aware code suggestions and completions
- **Multi-file Editing**: Understands and modifies entire codebases
- **Terminal Integration**: Works directly in your development environment
- **Flexible API Support**: Compatible with OpenAI and Anthropic APIs via AI SDK

**Repository**: [https://github.com/sst/opencode](https://github.com/sst/opencode)

**Project Status**: The original OpenCode project was archived by the creator. It is now actively maintained as a fork by the SST (Serverless Stack) team. The SST fork continues to receive updates and improvements.

By default, OpenCode can connect to OpenAI or Anthropic cloud APIs. With Olla's API compatibility, you can redirect it to local models whilst maintaining full functionality.

## Architecture

```
┌──────────────┐    OpenAI or          ┌──────────┐    OpenAI API    ┌─────────────────────┐
│  OpenCode    │    Anthropic API      │   Olla   │─────────────────▶│ Ollama :11434       │
│  (Node.js)   │─────────────────────▶│  :40114  │  /v1/*           └─────────────────────┘
│              │  /openai/v1/* or      │          │                   ┌─────────────────────┐
│              │  /anthropic/v1/*      │  • API   │─────────────────▶│ LM Studio :1234     │
│              │                       │    Translation              └─────────────────────┘
│              │                       │  • Load Balancing           ┌─────────────────────┐
│              │◀─────────────────────│  • Health Checks │─────────▶│ vLLM :8000          │
└──────────────┘    API format        └──────────┘                   └─────────────────────┘
                                           │
                                           ├─ Routes to healthy backend
                                           ├─ Translates formats if needed
                                           └─ Unified model registry
```

## Prerequisites

Before starting, ensure you have:

1. **OpenCode Installed**
   - SST fork: [https://github.com/sst/opencode](https://github.com/sst/opencode)
   - Node.js 18+ required
   - Install via: `npm install -g @sst/opencode` (check SST documentation for current method)

2. **Olla Running**
   - Installed and configured (see [Installation Guide](../../installation.md))
   - Both OpenAI and Anthropic endpoints available (default configuration)

3. **At Least One Backend**
   - Ollama, LM Studio, vLLM, llama.cpp, or any OpenAI-compatible endpoint
   - With at least one model loaded/available

4. **Docker & Docker Compose** (for examples)
   - Required only if following Docker-based quick start

## Quick Start (Docker Compose)

### 1. Create Project Directory

```bash
mkdir opencode-olla
cd opencode-olla
```

### 2. Create Configuration Files

Create **`compose.yaml`**:

```yaml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    container_name: olla
    restart: unless-stopped
    ports:
      - "40114:40114"
    volumes:
      - ./olla.yaml:/app/config.yaml:ro
      - ./logs:/app/logs
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:40114/internal/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

  lmstudio:
    image: lmstudio/lmstudio:latest  # Adjust based on LM Studio Docker availability
    container_name: lmstudio
    restart: unless-stopped
    ports:
      - "1234:1234"
    volumes:
      - lmstudio_models:/app/models
    environment:
      - LMSTUDIO_PORT=1234

volumes:
  lmstudio_models:
    driver: local
```

> **Note**: LM Studio may not have an official Docker image. See the [example directory](../../../../examples/opencode-lmstudio/) for alternative setup methods, or substitute with Ollama if preferred.

Create **`olla.yaml`**:

```yaml
server:
  host: 0.0.0.0
  port: 40114
  log_level: info

proxy:
  engine: sherpa           # or: olla (lower overhead)
  load_balancer: priority  # or: least-connections
  response_timeout: 1800s  # 30 min for long generations
  read_timeout: 600s

# Both translators enabled by default
translators:
  openai:
    enabled: true
  anthropic:
    enabled: true

# Service discovery for backends
discovery:
  type: static
  static:
    endpoints:
      - url: http://lmstudio:1234
        name: lmstudio-local
        type: lmstudio
        priority: 100
        health_check:
          enabled: true
          interval: 30s
          timeout: 5s

# Optional: Rate limiting
security:
  rate_limit:
    enabled: false  # Enable in production

# Optional: Streaming optimisation
# proxy:
#   profile: streaming
```

### 3. Start Services

```bash
docker compose up -d
```

Wait for services to be healthy:

```bash
docker compose ps
```

### 4. Load a Model in LM Studio

Start LM Studio and load a model through its UI, or use Ollama as an alternative:

```bash
# Alternative: Use Ollama instead
# Replace lmstudio service with ollama in compose.yaml, then:
docker exec ollama ollama pull llama3.2:latest

# Or a coding-focused model:
docker exec ollama ollama pull qwen2.5-coder:32b
```

### 5. Verify Olla Setup

```bash
# Health check
curl http://localhost:40114/internal/health

# List available models (OpenAI format)
curl http://localhost:40114/olla/openai/v1/models | jq

# Or Anthropic format
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Test message (OpenAI format)
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role":"user","content":"Hello from Olla"}],
    "max_tokens": 100
  }' | jq
```

### 6. Configure OpenCode

OpenCode uses a configuration file at `~/.opencode/config.json`.

**Option A: OpenAI-Compatible Endpoint (Recommended)**

Create or edit `~/.opencode/config.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla-openai": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:40114/olla/openai/v1",
        "apiKey": "not-required"
      }
    }
  }
}
```

**Option B: Anthropic API Endpoint**

If you prefer using Anthropic's API format:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla-anthropic": {
      "npm": "@ai-sdk/anthropic",
      "options": {
        "baseURL": "http://localhost:40114/olla/anthropic/v1",
        "apiKey": "not-required"
      }
    }
  }
}
```

**Configuration Notes**:
- The `npm` field specifies which AI SDK provider package to use
- `@ai-sdk/openai-compatible` works with any OpenAI-compatible API
- `@ai-sdk/anthropic` uses Anthropic's SDK (requires Anthropic API format)
- `apiKey` can be any value for local use; Olla doesn't enforce authentication by default

### 7. Start OpenCode

```bash
opencode
```

Try prompts like:
- "Write a Python function to calculate factorial"
- "Explain this code: [paste code]"
- "Help me refactor this function"

## Configuration Options

### OpenCode Configuration File

**Location**: `~/.opencode/config.json`

**Full Configuration Example**:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:40114/olla/openai/v1",
        "apiKey": "not-required"
      }
    }
  },
  "model": "qwen2.5-coder:32b",
  "temperature": 0.7,
  "maxTokens": 4096
}
```

**Configuration Options**:

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `provider.<name>.npm` | string | AI SDK package to use | Required |
| `provider.<name>.options.baseURL` | string | Olla API endpoint | Required |
| `provider.<name>.options.apiKey` | string | API key (not validated locally) | Optional |
| `model` | string | Default model to use | Provider default |
| `temperature` | number | Sampling temperature (0-2) | 0.7 |
| `maxTokens` | number | Maximum tokens in response | 4096 |

### Olla Configuration

Edit `olla.yaml` to customise:

**Load Balancing Strategy**:
```yaml
proxy:
  load_balancer: priority  # Options: priority, round-robin, least-connections
```

- **priority**: Uses highest priority backend first (recommended for local + fallback setup)
- **round-robin**: Distributes evenly across all backends
- **least-connections**: Routes to backend with fewest active requests

**Timeout Configuration**:
```yaml
proxy:
  response_timeout: 1800s  # Max time for response (30 minutes)
  read_timeout: 600s       # Max time for reading response body
  write_timeout: 30s       # Max time for writing request
```

**Streaming Optimisation**:
```yaml
proxy:
  profile: streaming  # Optimised for streaming responses
```

**Multiple Backends**:
```yaml
discovery:
  static:
    endpoints:
      - url: http://lmstudio:1234
        name: lmstudio-gpu
        type: lmstudio
        priority: 100

      - url: http://ollama:11434
        name: local-ollama
        type: ollama
        priority: 90

      - url: http://vllm:8000
        name: vllm-cluster
        type: vllm
        priority: 80
```

## Usage Examples

### Basic Code Generation

```bash
# In OpenCode terminal
> Write a Python function that calculates the Fibonacci sequence recursively
```

### Multi-file Code Editing

```bash
# OpenCode can read and modify multiple files
> Refactor the user authentication in auth.js to use async/await
```

### Code Explanation

```bash
# Paste code and ask for explanation
> Explain this code:
[paste complex code snippet]
```

### Debugging Assistance

```bash
> I'm getting this error: [paste error message and code]
> Help me fix it
```

### Using Specific Models

Configure default model in `~/.opencode/config.json`:

```json
{
  "model": "qwen2.5-coder:32b"
}
```

Or query available models:

```bash
# List models from Olla
curl http://localhost:40114/olla/openai/v1/models | jq '.data[].id'
```

## Docker Deployment (Production)

For production deployments, enhance security and reliability:

### Enhanced compose.yaml

```yaml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    container_name: olla
    restart: unless-stopped
    ports:
      - "40114:40114"
    volumes:
      - ./olla.yaml:/app/config.yaml:ro
      - ./logs:/app/logs
    environment:
      - OLLA_LOG_LEVEL=info
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:40114/internal/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    networks:
      - olla-network

  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: unless-stopped
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    volumes:
      - ollama_data:/root/.ollama
    networks:
      - olla-network
    healthcheck:
      test: ["CMD", "ollama", "list"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  ollama_data:
    driver: local

networks:
  olla-network:
    driver: bridge
```

### Production olla.yaml

```yaml
server:
  host: 0.0.0.0
  port: 40114
  log_level: info

proxy:
  engine: olla  # Use high-performance engine
  load_balancer: least-connections
  response_timeout: 1800s
  read_timeout: 600s
  profile: streaming

translators:
  openai:
    enabled: true
  anthropic:
    enabled: true

discovery:
  type: static
  static:
    endpoints:
      - url: http://ollama:11434
        name: local-ollama
        type: ollama
        priority: 100
        health_check:
          enabled: true
          interval: 30s
          timeout: 5s
          unhealthy_threshold: 3
          healthy_threshold: 2

security:
  rate_limit:
    enabled: true
    requests_per_minute: 100
    burst: 50

logging:
  level: info
  format: json
```

## Model Selection Tips

### Recommended Models for OpenCode

**Code-Focused Models**:
- `qwen2.5-coder:32b` - Excellent for code generation and understanding
- `deepseek-coder-v2:latest` - Strong multi-language support
- `codellama:34b` - Meta's specialised coding model
- `phi3.5:latest` - Efficient, good for quick tasks

**General Purpose (Code + Chat)**:
- `llama3.3:latest` - Well-balanced, fast
- `mistralai/magistral-small` - Good reasoning abilities
- `qwen3:32b` - Strong multi-task performance

**Performance vs Quality Trade-offs**:

| Model Size | Response Time | Quality | Memory Required |
|------------|---------------|---------|-----------------|
| 3-8B       | Fast (< 2s)   | Good    | 4-8 GB          |
| 13-20B     | Medium (2-5s) | Better  | 12-16 GB        |
| 30-70B     | Slow (5-15s)  | Best    | 24-64 GB        |

**Loading Models**:

```bash
# Ollama
docker exec ollama ollama pull qwen2.5-coder:32b

# Check loaded models
docker exec ollama ollama list

# Remove unused models to save space
docker exec ollama ollama rm <model-name>
```

## Troubleshooting

### OpenCode Can't Connect to Olla

**Check configuration file**:
```bash
cat ~/.opencode/config.json
# Verify baseURL points to correct Olla endpoint
```

**Test Olla directly**:
```bash
curl http://localhost:40114/internal/health

# If this fails, Olla isn't running
docker compose ps
```

**Check OpenCode logs**:
```bash
# OpenCode typically outputs logs to stderr/stdout
opencode --verbose  # Check if verbose mode is available
```

### No Models Available

**List models from Olla**:
```bash
# OpenAI format
curl http://localhost:40114/olla/openai/v1/models | jq

# Anthropic format
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Should show models from all backends
```

**Check backend health**:
```bash
curl http://localhost:40114/internal/status/endpoints | jq
```

**Verify backend directly**:
```bash
# Ollama
docker exec ollama ollama list

# Or via API
curl http://localhost:11434/api/tags

# LM Studio
curl http://localhost:1234/v1/models
```

**Pull a model if empty**:
```bash
docker exec ollama ollama pull llama3.2:latest
```

### Slow Responses

**Switch to high-performance proxy engine**:
```yaml
proxy:
  engine: olla  # Instead of sherpa
  profile: streaming
```

**Use smaller, faster models**:
```bash
docker exec ollama ollama pull phi3.5:latest
docker exec ollama ollama pull llama3.2:latest
```

**Increase timeout for large models**:
```yaml
proxy:
  response_timeout: 3600s  # 1 hour
  read_timeout: 1200s
```

**Check backend performance**:
```bash
# Ollama GPU usage
docker exec ollama nvidia-smi

# Container resources
docker stats ollama
```

### Connection Refused

**From OpenCode to Olla**:
```bash
# Test from host
curl http://localhost:40114/internal/health

# If this works but OpenCode fails, check firewall
```

**From Olla to Backend (Docker)**:
```bash
# Test from Olla container
docker exec olla wget -q -O- http://ollama:11434/api/tags

# If this fails, check Docker network
docker network inspect opencode-olla_default
```

### Streaming Issues

**Enable streaming profile**:
```yaml
proxy:
  profile: streaming
```

**Check OpenCode streaming support**:
- Ensure you're using a recent version
- Check SST fork documentation for streaming capabilities

**Test streaming directly**:
```bash
curl -N -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role":"user","content":"Count to 5"}],
    "stream": true
  }'
```

### API Key Issues

Olla doesn't enforce API keys by default. If OpenCode requires one:

```json
{
  "provider": {
    "olla": {
      "options": {
        "apiKey": "dummy-key-not-validated"
      }
    }
  }
}
```

Any placeholder value will work.

## Advanced Configuration

### Using Non-Docker Backends

If your backends run outside Docker:

**olla.yaml with host services**:
```yaml
discovery:
  static:
    endpoints:
      # Linux: Use host IP
      - url: http://192.168.1.100:11434
        name: ollama-workstation
        type: ollama
        priority: 100

      # macOS/Windows: Use host.docker.internal
      - url: http://host.docker.internal:1234
        name: lmstudio-local
        type: lmstudio
        priority: 100
```

### Load Balancing Across Multiple GPUs

**Setup multiple backend instances**:
```yaml
discovery:
  static:
    endpoints:
      - url: http://gpu1-ollama:11434
        name: gpu1
        type: ollama
        priority: 100

      - url: http://gpu2-ollama:11434
        name: gpu2
        type: ollama
        priority: 100

      - url: http://gpu3-vllm:8000
        name: gpu3-vllm
        type: vllm
        priority: 90

proxy:
  load_balancer: least-connections  # Distribute load evenly
```

### Switching Between OpenAI and Anthropic APIs

OpenCode supports both API formats. You can configure multiple providers:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla-openai": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:40114/olla/openai/v1"
      }
    },
    "olla-anthropic": {
      "npm": "@ai-sdk/anthropic",
      "options": {
        "baseURL": "http://localhost:40114/olla/anthropic/v1"
      }
    }
  }
}
```

Check OpenCode documentation for how to switch between providers.

### Integration with CI/CD

**Using OpenCode in CI pipelines** (if supported):

```yaml
# .github/workflows/code-review.yml
name: AI Code Review

on: [pull_request]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install OpenCode
        run: |
          npm install -g @sst/opencode

      - name: Configure for Olla
        run: |
          mkdir -p ~/.opencode
          cat > ~/.opencode/config.json << EOF
          {
            "provider": {
              "olla": {
                "npm": "@ai-sdk/openai-compatible",
                "options": {
                  "baseURL": "${{ secrets.OLLA_URL }}"
                }
              }
            }
          }
          EOF

      - name: Run AI Review
        run: |
          # Check OpenCode CLI documentation for review commands
          opencode review
```

### Monitoring and Observability

**Check Olla metrics**:
```bash
# Endpoint status
curl http://localhost:40114/internal/status/endpoints | jq

# Model statistics
curl http://localhost:40114/internal/status/models | jq

# Health
curl http://localhost:40114/internal/health
```

**View logs**:
```bash
# Olla logs
docker compose logs -f olla

# Backend logs
docker compose logs -f ollama
docker compose logs -f lmstudio

# Filter for errors
docker compose logs olla | grep -i error
```

**Custom logging**:
```yaml
logging:
  level: debug  # Options: debug, info, warn, error
  format: json  # Options: json, text
```

## Best Practices

### 1. Model Management

- **Start small**: Test with smaller models (3-8B) before using larger ones
- **Specialised models**: Use code-specific models (e.g., `qwen2.5-coder`) for better results
- **Clean up**: Remove unused models to save disk space
- **Version models**: Use specific tags (`:v1.2`) rather than `:latest` for consistency

### 2. Performance Optimisation

- **GPU acceleration**: Use CUDA-enabled backend images for GPU support
- **Resource limits**: Set Docker memory/CPU limits to prevent host resource exhaustion
- **Connection pooling**: Use `olla` proxy engine for better connection handling
- **Streaming profile**: Enable for real-time response feel

### 3. Development Workflow

- **Local-first**: Configure highest priority for local backends
- **Fallback remotes**: Add lower-priority remote endpoints for reliability
- **Model isolation**: Separate models for different tasks (code vs chat vs analysis)
- **Version control**: Keep `olla.yaml` and OpenCode config in your project repo

### 4. Security

- **Network isolation**: Use Docker networks to isolate services
- **Rate limiting**: Enable in production to prevent abuse
- **No public exposure**: Don't expose Olla directly to the internet without authentication
- **API gateway**: Use nginx/Traefik with auth for external access

### 5. Cost Efficiency

- **Local models**: Save on API costs whilst maintaining privacy
- **Batch operations**: Group similar tasks to reduce cold-start delays
- **Model caching**: Keep frequently used models loaded
- **Resource sharing**: One Olla instance can serve multiple developers

## OpenCode vs Claude Code vs Crush CLI

| Feature | OpenCode | Claude Code | Crush CLI |
|---------|----------|-------------|-----------|
| **License** | Open Source (archived) | Proprietary | Open Source |
| **Maintenance** | SST fork active | Anthropic official | Charmbracelet active |
| **API Support** | OpenAI or Anthropic | Anthropic only | Both |
| **Platform** | Node.js/TypeScript | Unknown | Go |
| **Configuration** | JSON config file | Environment variables | YAML config |
| **Best For** | Customisable workflows | Official Anthropic support | Modern terminal UI |

## Next Steps

### Related Documentation

- **[Anthropic Messages API Reference](../../api-reference/anthropic.md)** - Complete API documentation
- **[OpenAI API Reference](../../api-reference/openai.md)** - OpenAI endpoint documentation
- **[API Translation Concept](../../concepts/api-translation.md)** - How translation works
- **[Load Balancing](../../concepts/load-balancing.md)** - Understanding request distribution
- **[Model Routing](../../concepts/model-routing.md)** - How models are selected

### Integration Examples

- **[OpenCode + LM Studio Example](../../../../examples/opencode-lmstudio/)** - Complete setup (if available)
- **[Claude Code + Ollama Example](../../../../examples/claude-code-ollama/)** - Similar setup pattern
- **[Claude Code Integration](claude-code.md)** - Official Anthropic CLI
- **[Crush CLI Integration](crush-cli.md)** - Modern terminal assistant

### Backend Guides

- **[Ollama Integration](../backend/ollama.md)** - Ollama-specific configuration
- **[LM Studio Integration](../backend/lmstudio.md)** - LM Studio setup
- **[vLLM Integration](../backend/vllm.md)** - High-performance inference

### Advanced Topics

- **[Health Checking](../../concepts/health-checking.md)** - Endpoint monitoring
- **[Circuit Breaking](../../concepts/circuit-breaking.md)** - Failure handling
- **[Statistics Collection](../../concepts/statistics.md)** - Performance metrics

---

## Support

**Community**:
- GitHub Issues: [https://github.com/thushan/olla/issues](https://github.com/thushan/olla/issues)
- Discussions: [https://github.com/thushan/olla/discussions](https://github.com/thushan/olla/discussions)

**OpenCode Resources**:
- SST OpenCode Fork: [https://github.com/sst/opencode](https://github.com/sst/opencode)
- Original Project: [https://github.com/opencodecli/opencode](https://github.com/opencodecli/opencode) (archived)
- SST Documentation: [https://sst.dev](https://sst.dev)

**Common Resources**:
- [Olla Project Home](../../index.md)
- [OpenAI API Reference](https://platform.openai.com/docs/api-reference)
- [Anthropic API Reference](https://docs.anthropic.com/en/api)

**Quick Help**:
```bash
# Verify setup
curl http://localhost:40114/internal/health
curl http://localhost:40114/olla/openai/v1/models | jq

# Test message
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:latest","messages":[{"role":"user","content":"Hi"}]}' | jq

# Check logs
docker compose logs -f olla
```
