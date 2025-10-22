---
title: Claude Code + Olla (Anthropic API) Integration
description: Configure Claude Code to use local LLM models via Olla's Anthropic Messages API translator. Load-balancing, failover, streaming, and model unification for Ollama, LM Studio, vLLM, and other OpenAI-compatible backends.
keywords: Claude Code, Olla, Anthropic API, Messages API, local LLM, Ollama, vLLM, LM Studio, load balancing, API translation
---

# Claude Code Integration with Anthropic API

Claude Code can connect to Olla's Anthropic Messages API translation endpoint, enabling you to use Anthropic's official CLI coding assistant with local LLM infrastructure—no cloud API costs.

**Set in Claude Code:**

```bash
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"

export DEFAULT_MODEL="openai/gpt-oss-120b" # the model you want to target
export ANTHROPIC_MODEL="${DEFAULT_MODEL}"
export ANTHROPIC_SMALL_FAST_MODEL="${DEFAULT_MODEL}"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="${DEFAULT_MODEL}"
export ANTHROPIC_DEFAULT_SONNET_MODEL="${DEFAULT_MODEL}"
export ANTHROPIC_DEFAULT_OPUS_MODEL="${DEFAULT_MODEL}"

export ANTHROPIC_AUTH_TOKEN="not-really-needed"

# Some options to help Claude Code work better
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export API_TIMEOUT_MS=3000000
```

You can of course customise individual model choices too.

**What you get via Olla**

* Consistent Anthropic Messages API at `/olla/anthropic/v1/*`
* Load-balancing and health checks
* Streaming passthrough
* Unified `/v1/models` across providers
* Seamless Anthropic-to-OpenAI format translation
* Fallback and self-healing for backends that fail etc

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://docs.claude.com/en/docs/claude-code">Claude Code</a> (Anthropic's Official CLI)</td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Frontend UI / CLI Coding Assistant</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>Anthropic Messages API Compatibility</td>
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
            Set <code>ANTHROPIC_API_BASE_URL</code> to Olla Anthropic endpoint <br/>
            <pre>export ANTHROPIC_API_BASE_URL="http://localhost:40114/olla/anthropic"</pre>
        </td>
    </tr>
    <tr>
        <th>Example</th>
        <td>
            Complete working examples available in <code>examples/claude-code-*/</code>
        </td>
    </tr>
</table>

## What is Claude Code?

Claude Code is Anthropic's official command-line coding assistant that provides:

- **Intelligent Code Generation**: Context-aware code suggestions and completions
- **Multi-file Editing**: Understands and modifies entire codebases
- **Terminal Integration**: Works directly in your development environment
- **Real-time Collaboration**: Iterative coding with natural language

**Official Documentation**: [https://docs.claude.com/en/docs/claude-code](https://docs.claude.com/en/docs/claude-code)

By default, Claude Code connects to Anthropic's cloud API. With Olla's API translation, you can redirect it to local models whilst maintaining full compatibility.

## Architecture

```
┌──────────────┐  Anthropic API   ┌──────────┐    OpenAI API   ┌─────────────────┐
│ Claude Code  │─────────────────▶│   Olla   │───────────────▶│ Ollama :11434  │
│   (CLI)      │  /anthropic/*    │  :40114  │  /v1/*          └─────────────────┘
│              │                  │          │                 ┌────────────────┐
│              │                  │  • API   │───────────────▶│ LM Studio :1234 │
│              │                  │    Translation             └─────────────────┘
│              │                  │  • Load Balancing          ┌─────────────────┐
│              │◀────────────────│  • Health Checks │────────▶│ vLLM :8000      │
└──────────────┘ Anthropic format  └──────────┘                └───────────────┘
                                           │
                                           ├─ Translates Anthropic → OpenAI
                                           ├─ Routes to healthy backend
                                           └─ Translates OpenAI → Anthropic
```

## Prerequisites

Before starting, ensure you have:

1. **Claude Code Installed**
      * Follow [Anthropic's installation guide](https://docs.claude.com/en/docs/claude-code/installation)
      * Verify: `claude-code --version`

2. **Olla Running**

      * Installed and configured (see [Installation Guide](../../getting-started/installation.md))
      * Anthropic translation enabled (see `config.yaml`)

3. **At Least One Backend**
   
      * Ollama, LM Studio, vLLM, SGLang, llama.cpp or any OpenAI-compatible endpoint
      * With at least one model loaded/available

4. **Docker & Docker Compose** (for examples)
   
      * Required only if following Docker-based quick start

## Quick Start (Docker Compose)

### 1. Create Project Directory

```bash
mkdir claude-code-olla
cd claude-code-olla
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

# Anthropic API translation (disabled by default)
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
docker exec ollama ollama pull llama4:latest

# Or a coding-focused model:
docker exec ollama ollama pull qwen2.5-coder:32b
```

### 5. Verify Olla Setup

```bash
# Health check
curl http://localhost:40114/internal/health

# List available models
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Test message (non-streaming)
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 100,
    "messages": [{"role":"user","content":"Hello from Olla"}]
  }' | jq
```

### 6. Configure Claude Code

**Option A: Environment Variables (Recommended)**

```bash
export ANTHROPIC_API_BASE_URL="http://localhost:40114/olla/anthropic"
export ANTHROPIC_API_KEY="not-required"  # Optional
... # Add others
```

Add to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.) to make permanent:

```bash
echo 'export ANTHROPIC_API_BASE_URL="http://localhost:40114/olla/anthropic"' >> ~/.bashrc
source ~/.bashrc
```

**Option B: Configuration File**

If Claude Code supports configuration files, create/edit the config:

**macOS/Linux**: `~/.config/claude-code/config.json`
**Windows**: `%APPDATA%\claude-code\config.json`

```json
{
  "api": {
    "baseURL": "http://localhost:40114/olla/anthropic",
    "apiKey": "not-required"
  }
}
```

> **Note**: Configuration file format may vary by Claude Code version. Check [official documentation](https://docs.claude.com/en/docs/claude-code) for your version.

### 7. Start Claude Code

```bash
claude-code
```

Try prompts like:

- "Write a Python function to calculate factorial"
- "Explain this code: [paste code]"
- "Help me debug this error: [paste error]"

## Configuration Options

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ANTHROPIC_API_BASE_URL` | Yes | - | Olla's Anthropic endpoint URL |
| `ANTHROPIC_API_KEY` | No | - | API key (not enforced by Olla) |
| `ANTHROPIC_VERSION` | No | `2023-06-01` | API version header |

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

### Basic Code Generation

```bash
# In Claude Code CLI
> Write a Python function that calculates the Fibonacci sequence recursively
```

### Multi-file Code Editing

```bash
# Claude Code can read and modify multiple files
> Refactor the user authentication in auth.py to use environment variables
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

If Claude Code supports model selection:

```bash
# Some versions allow model specification
> Use model qwen2.5-coder:32b to write a sorting algorithm
```

Or configure default model in Olla by ensuring only desired models are available.

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

### Recommended Models for Claude Code

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

### Claude Code Can't Connect to Olla

**Check environment variable**:
```bash
echo $ANTHROPIC_API_BASE_URL
# Should output: http://localhost:40114/olla/anthropic
```

**Test Olla directly**:
```bash
curl http://localhost:40114/internal/health

# If this fails, Olla isn't running
docker compose ps
```

**Check Claude Code logs** (location varies by OS):
```bash
# macOS/Linux
tail -f ~/.config/claude-code/logs/client.log

# Windows
type %APPDATA%\claude-code\logs\client.log
```

### No Models Available

**List models from Olla**:
```bash
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
docker exec ollama ollama pull llama4:latest
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
docker exec ollama ollama pull llama4:latest
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

**From Claude Code to Olla**:
```bash
# Test from host
curl http://localhost:40114/internal/health

# If this works but Claude Code fails, check firewall
```

**From Olla to Ollama (Docker)**:
```bash
# Test from Olla container
docker exec olla wget -q -O- http://ollama:11434/api/tags

# If this fails, check Docker network
docker network inspect claude-code-olla_default
```

### Streaming Issues

**Enable streaming profile**:
```yaml
proxy:
  profile: streaming
```

**Check Claude Code streaming support**:

- Ensure you're using a recent version
- Some older versions may have limited streaming support

**Test streaming directly**:
```bash
curl -N -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "llama4:latest",
    "max_tokens": 50,
    "messages": [{"role":"user","content":"Count to 5"}],
    "stream": true
  }'
```

### API Key Issues

Olla doesn't enforce API keys by default. If Claude Code requires one:

```bash
export ANTHROPIC_API_KEY="dummy-key-not-validated"
```

Or any placeholder value will work.

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

### Custom System Prompts

Claude Code allows system prompt customisation. Set via environment:

```bash
export CLAUDE_SYSTEM_PROMPT="You are an expert Go developer specialising in high-performance, concurrent systems. Always provide idiomatic Go code with proper error handling."
```

> **Note**: Variable name may differ by Claude Code version. Check official docs.

### Integration with CI/CD

**Using Claude Code in CI pipelines**:

```yaml
# .github/workflows/code-review.yml
name: AI Code Review

on: [pull_request]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install Claude Code
        run: |
          # Install Claude Code (check official docs for method)

      - name: Configure for Olla
        env:
          ANTHROPIC_API_BASE_URL: ${{ secrets.OLLA_URL }}
        run: |
          echo "Configured Olla endpoint"

      - name: Run AI Review
        run: |
          claude-code review --diff="${{ github.event.pull_request.diff_url }}"
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
- **Model isolation**: Separate models for different tasks (code vs chat vs analysis)
- **Version control**: Keep `olla.yaml` in your project repo

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

## Next Steps

### Related Documentation

- **[Anthropic Messages API Reference](../../api-reference/anthropic.md)** - Complete API documentation
- **[API Translation Concept](../../concepts/api-translation.md)** - How translation works
- **[Load Balancing](../../concepts/load-balancing.md)** - Understanding request distribution
- **[Model Routing](../../concepts/model-routing.md)** - How models are selected

### Integration Examples

- **[Claude Code + Ollama Example](https://github.com/thushan/olla/tree/main/examples/claude-code-ollama/)** - Complete Docker setup
- **[Claude Code + llama.cpp Example](https://github.com/thushan/olla/tree/main/examples/claude-code-llamacpp/)** - Lightweight backend
- **[OpenCode Integration](opencode.md)** - Alternative AI coding assistant
- **[Crush CLI Integration](crush-cli.md)** - Terminal AI assistant

### Backend Guides

- **[Ollama Integration](../backend/ollama.md)** - Ollama-specific configuration
- **[LM Studio Integration](../backend/lmstudio.md)** - LM Studio setup
- **[vLLM Integration](../backend/vllm.md)** - High-performance inference

### Advanced Topics

- **[Health Checking](../../concepts/health-checking.md)** - Endpoint monitoring
- **[Circuit Breaking](../../development/circuit-breaker.md)** - Failure handling
- **[Provider Metrics](../../concepts/provider-metrics.md)** - Performance metrics

---

## Support

**Community**:

- GitHub Issues: [https://github.com/thushan/olla/issues](https://github.com/thushan/olla/issues)
- Discussions: [https://github.com/thushan/olla/discussions](https://github.com/thushan/olla/discussions)

**Common Resources**:

- [Claude Code Official Docs](https://docs.claude.com/en/docs/claude-code)
- [Anthropic API Reference](https://docs.anthropic.com/en/api)
- [Olla Project Home](../../index.md)

**Quick Help**:
```bash
# Verify setup
curl http://localhost:40114/internal/health
curl http://localhost:40114/olla/anthropic/v1/models | jq

# Test message
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model":"llama4:latest","max_tokens":50,"messages":[{"role":"user","content":"Hi"}]}' | jq

# Check logs
docker compose logs -f olla
```
