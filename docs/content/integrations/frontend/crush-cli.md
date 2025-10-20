---
title: Crush CLI + Olla Integration (Dual OpenAI/Anthropic API)
description: Configure Crush CLI to use local LLM models via Olla's dual API support. Load-balancing, failover, streaming, and model unification for Ollama, LM Studio, vLLM, and other backends with native support for both OpenAI and Anthropic formats.
keywords: Crush CLI, Olla, Charmbracelet, OpenAI API, Anthropic API, dual API, local LLM, Ollama, vLLM, LM Studio, load balancing
---

# Crush CLI Integration with Dual API Support

Crush CLI is a modern terminal AI assistant by Charmbracelet that natively supports both OpenAI and Anthropic APIs. Connect it to Olla to use local LLM infrastructure with seamless provider switching—no cloud API costs.

**Set in Crush CLI (`~/.crush/config.json`):**

```json
{
  "providers": {
    "olla-anthropic": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "api_key": "not-required"
    },
    "olla-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required"
    }
  }
}
```

**What you get via Olla**

* Dual API support in one proxy (both OpenAI and Anthropic endpoints)
* Switch between providers with `crush model switch`
* Priority/least-connections load-balancing and health checks
* Streaming passthrough for both formats
* Unified `/v1/models` across all backends
* Seamless format translation (Anthropic-to-OpenAI or direct OpenAI)

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://github.com/charmbracelet/crush">Crush CLI</a> (by Charmbracelet)</td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Terminal UI / CLI Assistant</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>Native dual API support (OpenAI + Anthropic)</td>
    </tr>
    <tr>
        <th>
          Features Supported <br/>
          <small>(via Olla)</small>
        </th>
        <td>
            <ul>
                <li>Chat Completions (both APIs)</li>
                <li>Code Generation & Editing</li>
                <li>Streaming Responses</li>
                <li>Model Selection & Switching</li>
                <li>Multi-provider Configuration</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Configuration</th>
        <td>
            Configure providers in <code>~/.crush/config.json</code> with both API endpoints
        </td>
    </tr>
    <tr>
        <th>Example</th>
        <td>
            Complete working example available in <code>examples/crush-vllm/</code>
        </td>
    </tr>
</table>

## What is Crush CLI?

Crush CLI is the successor to OpenCode, created by the original author and actively maintained by Charmbracelet. Key features:

- **Go-based**: Fast, lightweight, single binary
- **TUI Interface**: Beautiful terminal user interface using Bubble Tea
- **Dual API Support**: Natively supports both OpenAI and Anthropic formats
- **Provider Switching**: Change providers on the fly with `crush model switch`
- **Modern Design**: Charmbracelet's signature polished experience

**Official Repository**: [https://github.com/charmbracelet/crush](https://github.com/charmbracelet/crush)

By default, Crush CLI connects to cloud APIs. With Olla's dual endpoint support, you can redirect it to local models whilst maintaining full compatibility with both API formats.

## Architecture

```
┌──────────────┐                        ┌──────────┐                    ┌─────────────────────┐
│  Crush CLI   │  OpenAI API            │   Olla   │  OpenAI API        │ Ollama :11434       │
│   (TUI)      │─────────────────────▶  │  :40114  │─────────────────▶  └─────────────────────┘
│              │  /openai/v1/*          │          │  /v1/*             ┌─────────────────────┐
│              │                         │  • Dual  │─────────────────▶  │ LM Studio :1234     │
│              │  Anthropic API          │    API   │                    └─────────────────────┘
│              │─────────────────────▶  │  • Load  │                    ┌─────────────────────┐
│              │  /anthropic/v1/*       │    Balance│─────────────────▶ │ vLLM :8000          │
│              │◀─────────────────────  │  • Health│                    └─────────────────────┘
└──────────────┘  Both formats          └──────────┘
                                             │
                                             ├─ Direct OpenAI format passthrough
                                             ├─ Anthropic → OpenAI translation
                                             └─ Routes to healthy backend
```

## Prerequisites

Before starting, ensure you have:

1. **Crush CLI Installed**
   - Download from [GitHub Releases](https://github.com/charmbracelet/crush/releases)
   - Or build from source: `go install github.com/charmbracelet/crush@latest`
   - Verify: `crush --version`

2. **Olla Running**
   - Installed and configured (see [Installation Guide](../../installation.md))
   - Both OpenAI and Anthropic endpoints enabled (default)

3. **At Least One Backend**
   - Ollama, LM Studio, vLLM, llama.cpp, or any OpenAI-compatible endpoint
   - With at least one model loaded/available

4. **Docker & Docker Compose** (for examples)
   - Required only if following Docker-based quick start

## Quick Start (Docker Compose)

### 1. Create Project Directory

```bash
mkdir crush-olla
cd crush-olla
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

  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: unless-stopped
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    healthcheck:
      test: ["CMD", "ollama", "list"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  ollama_data:
    driver: local
```

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

# Both API translators enabled by default
translators:
  anthropic:
    enabled: true

# Service discovery for backends
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

### 4. Pull a Model (Ollama)

```bash
docker exec ollama ollama pull llama3.2:latest

# Or a coding-focused model:
docker exec ollama ollama pull qwen2.5-coder:32b
```

### 5. Verify Olla Setup

```bash
# Health check
curl http://localhost:40114/internal/health

# List models via OpenAI endpoint
curl http://localhost:40114/olla/openai/v1/models | jq

# List models via Anthropic endpoint
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Test OpenAI format
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role":"user","content":"Hello from Olla"}]
  }' | jq

# Test Anthropic format
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 100,
    "messages": [{"role":"user","content":"Hello from Olla"}]
  }' | jq
```

### 6. Configure Crush CLI

Create or edit **`~/.crush/config.json`**:

**macOS/Linux**: `~/.crush/config.json`
**Windows**: `%USERPROFILE%\.crush\config.json`

```json
{
  "providers": {
    "olla-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required",
      "default_model": "llama3.2:latest"
    },
    "olla-anthropic": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "api_key": "not-required"
    }
  },
  "default_provider": "olla-openai"
}
```

### 7. Start Crush CLI

```bash
crush
```

You can now:
- Start chatting with your local models
- Switch providers: `crush model switch` (or within TUI)
- Try prompts like:
  - "Write a Python function to calculate factorial"
  - "Explain this code: [paste code]"
  - "Help me debug this error: [paste error]"

## Configuration Options

### Crush CLI Configuration

Edit `~/.crush/config.json` to customise provider settings:

**Basic Configuration**:
```json
{
  "providers": {
    "olla-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required",
      "default_model": "llama3.2:latest"
    },
    "olla-anthropic": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "api_key": "not-required"
    }
  },
  "default_provider": "olla-openai"
}
```

**Multiple Providers** (with cloud fallback):
```json
{
  "providers": {
    "local-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required",
      "default_model": "qwen2.5-coder:32b"
    },
    "local-anthropic": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "api_key": "not-required"
    },
    "openai-cloud": {
      "type": "openai",
      "api_key": "sk-..."
    },
    "anthropic-cloud": {
      "type": "anthropic",
      "api_key": "sk-ant-..."
    }
  },
  "default_provider": "local-openai"
}
```

**Provider-Specific Models**:
```json
{
  "providers": {
    "coding": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required",
      "default_model": "qwen2.5-coder:32b"
    },
    "chat": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required",
      "default_model": "llama3.3:latest"
    }
  },
  "default_provider": "coding"
}
```

### Olla Configuration

Edit `olla.yaml` to customise backend behaviour:

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
      - url: http://ollama:11434
        name: local-ollama
        type: ollama
        priority: 100

      - url: http://lmstudio:1234
        name: lmstudio-gpu
        type: lmstudio
        priority: 90

      - url: http://vllm:8000
        name: vllm-cluster
        type: vllm
        priority: 80
```

## Usage Examples

### Basic Chat

```bash
# Start Crush CLI
crush

# Start chatting with local model
> Hello, can you help me with Python?

# Switch provider
> /switch
# Select from configured providers

# Change model within session
> /model qwen2.5-coder:32b
```

### Code Generation

```bash
# In Crush CLI
> Write a Python function that implements quicksort with type hints and docstrings

> Create a REST API endpoint in Go that handles user authentication with JWT
```

### Code Explanation

```bash
# Paste code and ask for explanation
> Explain this code:
[paste complex code snippet]

> What design patterns are used in this TypeScript class?
[paste class definition]
```

### Debugging Assistance

```bash
> I'm getting this error: TypeError: 'NoneType' object is not iterable
[paste relevant code]
> Help me debug this

> Why is my Go routine leaking?
[paste goroutine code]
```

### Provider Switching

```bash
# Switch between OpenAI and Anthropic endpoints
> /switch

# Or use command-line flag
crush --provider olla-anthropic

# List available providers
crush providers list
```

## Switching Between APIs

Crush CLI's key advantage is native dual API support:

### Why Use Multiple APIs?

**OpenAI Format** (default):
- Broader model compatibility
- Standard for most local backends
- Simpler request/response structure

**Anthropic Format**:
- Better structured responses with content blocks
- Native tool use format
- Cleaner streaming event structure

### Switching Providers

**Within Crush CLI**:
```bash
# Interactive provider selection
> /switch

# Or directly specify
> /provider olla-anthropic
```

**Command Line**:
```bash
# Start with specific provider
crush --provider olla-anthropic

# Start with specific model
crush --model qwen2.5-coder:32b --provider olla-openai
```

**Configuration Default**:
```json
{
  "default_provider": "olla-openai",
  "providers": { ... }
}
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

# Both translators enabled
translators:
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

### Recommended Models for Crush CLI

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

### Crush CLI Can't Connect to Olla

**Check configuration file**:
```bash
cat ~/.crush/config.json
# Verify base_url points to correct endpoint
```

**Test Olla directly**:
```bash
curl http://localhost:40114/internal/health

# If this fails, Olla isn't running
docker compose ps
```

**Check Crush CLI logs**:
```bash
# macOS/Linux
tail -f ~/.crush/logs/crush.log

# Or run Crush in debug mode
crush --debug
```

### No Models Available

**List models from Olla**:
```bash
# OpenAI endpoint
curl http://localhost:40114/olla/openai/v1/models | jq

# Anthropic endpoint
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
```

**Pull a model if empty**:
```bash
docker exec ollama ollama pull llama3.2:latest
```

### Provider Switch Not Working

**Verify multiple providers configured**:
```bash
cat ~/.crush/config.json | jq '.providers'
```

**Check provider format**:
```json
{
  "providers": {
    "provider-name": {
      "type": "openai",  // Must be "openai" or "anthropic"
      "base_url": "...",
      "api_key": "..."
    }
  }
}
```

**Test both endpoints**:
```bash
# OpenAI
curl http://localhost:40114/olla/openai/v1/models

# Anthropic
curl http://localhost:40114/olla/anthropic/v1/models
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

**From Crush CLI to Olla**:
```bash
# Test from host
curl http://localhost:40114/internal/health

# If this works but Crush fails, check firewall
```

**From Olla to Ollama (Docker)**:
```bash
# Test from Olla container
docker exec olla wget -q -O- http://ollama:11434/api/tags

# If this fails, check Docker network
docker network inspect crush-olla_default
```

### Streaming Issues

**Enable streaming profile**:
```yaml
proxy:
  profile: streaming
```

**Check Crush CLI streaming support**:
- Ensure you're using a recent version
- Streaming should work automatically with both OpenAI and Anthropic formats

**Test streaming directly**:
```bash
# OpenAI format
curl -N -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "messages": [{"role":"user","content":"Count to 5"}],
    "stream": true
  }'

# Anthropic format
curl -N -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 50,
    "messages": [{"role":"user","content":"Count to 5"}],
    "stream": true
  }'
```

### API Key Issues

Olla doesn't enforce API keys by default. If Crush CLI requires one:

```json
{
  "providers": {
    "olla-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "api_key": "not-required-but-can-be-anything"
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
      - url: http://host.docker.internal:11434
        name: ollama-local
        type: ollama
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

### Multiple Crush Profiles

Create different configuration profiles for different use cases:

**~/.crush/coding.json**:
```json
{
  "providers": {
    "coding": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "default_model": "qwen2.5-coder:32b"
    }
  },
  "default_provider": "coding"
}
```

**~/.crush/chat.json**:
```json
{
  "providers": {
    "chat": {
      "type": "anthropic",
      "base_url": "http://localhost:40114/olla/anthropic/v1",
      "default_model": "llama3.3:latest"
    }
  },
  "default_provider": "chat"
}
```

**Use with**:
```bash
crush --config ~/.crush/coding.json
crush --config ~/.crush/chat.json
```

### Integration with Development Tools

**Using Crush CLI in scripts**:

```bash
#!/bin/bash
# commit-msg-generator.sh

# Get diff
DIFF=$(git diff --cached)

# Generate commit message via Crush
COMMIT_MSG=$(echo "$DIFF" | crush --provider olla-openai --prompt "Generate a concise commit message for this diff:")

# Use the message
git commit -m "$COMMIT_MSG"
```

**Shell alias for quick access**:

```bash
# Add to ~/.bashrc or ~/.zshrc
alias code-review='crush --provider coding --prompt "Review this code for issues:"'
alias explain-error='crush --provider coding --prompt "Explain this error:"'
alias write-docs='crush --provider chat --prompt "Write documentation for:"'
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

# Ollama logs
docker compose logs -f ollama

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

- **GPU acceleration**: Use CUDA-enabled Ollama image for GPU support
- **Resource limits**: Set Docker memory/CPU limits to prevent host resource exhaustion
- **Connection pooling**: Use `olla` proxy engine for better connection handling
- **Streaming profile**: Enable for real-time response feel

### 3. Development Workflow

- **Local-first**: Configure highest priority for local backends
- **Fallback remotes**: Add lower-priority remote endpoints for reliability
- **Provider separation**: Use OpenAI for standard tasks, Anthropic for structured outputs
- **Configuration profiles**: Create separate configs for different workflows

### 4. Security

- **Network isolation**: Use Docker networks to isolate services
- **Rate limiting**: Enable in production to prevent abuse
- **No public exposure**: Don't expose Olla directly to the internet without authentication
- **API gateway**: Use nginx/Traefik with auth for external access

### 5. Cost Efficiency

- **Local models**: Save on API costs whilst maintaining privacy
- **Hybrid setup**: Use local for dev/test, cloud for production if needed
- **Model caching**: Keep frequently used models loaded
- **Resource sharing**: One Olla instance can serve multiple developers

## Next Steps

### Related Documentation

- **[OpenAI Chat Completions API Reference](../../api-reference/openai.md)** - OpenAI API documentation
- **[Anthropic Messages API Reference](../../api-reference/anthropic.md)** - Anthropic API documentation
- **[API Translation Concept](../../concepts/api-translation.md)** - How translation works
- **[Load Balancing](../../concepts/load-balancing.md)** - Understanding request distribution
- **[Model Routing](../../concepts/model-routing.md)** - How models are selected

### Integration Examples

- **[Crush CLI + vLLM Example](../../../../examples/crush-vllm/)** - High-performance backend setup
- **[Claude Code + Ollama Example](../../../../examples/claude-code-ollama/)** - Alternative CLI assistant
- **[OpenCode Integration](opencode.md)** - Predecessor to Crush CLI

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

**Common Resources**:
- [Crush CLI Repository](https://github.com/charmbracelet/crush)
- [Charmbracelet Projects](https://github.com/charmbracelet)
- [Olla Project Home](../../index.md)

**Quick Help**:
```bash
# Verify setup
curl http://localhost:40114/internal/health
curl http://localhost:40114/olla/openai/v1/models | jq
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Test OpenAI format
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:latest","messages":[{"role":"user","content":"Hi"}]}' | jq

# Test Anthropic format
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"llama3.2:latest","max_tokens":50,"messages":[{"role":"user","content":"Hi"}]}' | jq

# Check logs
docker compose logs -f olla
```
