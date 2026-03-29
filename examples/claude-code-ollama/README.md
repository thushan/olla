# Claude Code + Olla + Ollama Integration Example

This example demonstrates how to use Claude Code with Olla as an Anthropic API translator, routing requests to a local Ollama instance.

## Architecture

```
┌──────────────┐    Anthropic API     ┌──────────┐    OpenAI API    ┌─────────────┐
│ Claude Code  │───────────────────────▶│   Olla   │──────────────────▶│   Ollama    │
│              │   /anthropic/v1/*     │  :40114  │   /v1/*          │   :11434    │
│              │◀───────────────────────│          │◀──────────────────│             │
└──────────────┘    Anthropic format   └──────────┘   OpenAI format  └─────────────┘
                                            │
                                            ├─ Translate request
                                            ├─ Route to backend
                                            ├─ Translate response
                                            └─ Return to client
```

## What This Example Includes

- ✅ Docker Compose setup with Olla and Ollama
- ✅ Pre-configured Olla with Anthropic translation enabled
- ✅ Example configuration for Claude Code
- ✅ Test script to verify setup
- ✅ Ready-to-use configuration files

## Prerequisites

- Docker and Docker Compose installed
- Claude Code installed ([Installation Guide](https://docs.claude.com/en/docs/claude-code/installation))
- Basic understanding of Docker and LLMs

## Quick Start

### 1. Start Services

```bash
docker compose up -d
```

Wait for services to start (~30 seconds).

### 2. Pull a Model

```bash
# Pull Llama 3.2 (recommended for coding)
docker exec ollama ollama pull llama3.2:latest

# Or use Qwen 2.5 Coder for better code generation
docker exec ollama ollama pull qwen2.5-coder:32b
```

### 3. Verify Setup

```bash
./test.sh
```

You should see:

- ✅ Olla health check passing
- ✅ Models listed
- ✅ Non-streaming message working
- ✅ Streaming message working

### 4. Configure Claude Code

**Option A: Environment Variable** (Recommended)
```bash
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"
export ANTHROPIC_API_KEY="not-required"  # Optional, Olla doesn't enforce this
```

**Option B: Configuration File**

Create or edit Claude Code's configuration file with the example provided:

```bash
# Copy example config (adjust path for your OS)
# macOS/Linux:
cp claude-code-config.example.json ~/.claude-code/config.json

# Windows:
# Copy to %USERPROFILE%\.claude-code\config.json
```

Edit the config to point to Olla:
```json
{
  "api": {
    "baseURL": "http://localhost:40114/olla/anthropic/v1"
  }
}
```

### 5. Use Claude Code

```bash
# Start Claude Code
claude

# In Claude Code, try:
# "Write a Python function to calculate factorial"
# "Explain this code: [paste some code]"
# "Help me debug this error: [paste error]"
```

## Files

| File | Description |
|------|-------------|
| `compose.yaml` | Docker Compose configuration for Olla + Ollama |
| `olla.yaml` | Olla configuration with Anthropic translation enabled |
| `test.sh` | Automated test script to verify setup |
| `claude-code-config.example.json` | Example Claude Code configuration |
| `README.md` | This file |

## Verification Steps

### Check Olla Health

```bash
curl http://localhost:40114/internal/health
```

Expected output:
```json
{
  "status": "healthy",
  "timestamp": "2025-01-20T12:00:00Z"
}
```

### Check Available Models

```bash
curl http://localhost:40114/olla/anthropic/v1/models | jq
```

Expected output:
```json
{
  "object": "list",
  "data": [
    {
      "id": "llama3.2:latest",
      "object": "model",
      "created": 1704067200,
      "owned_by": "ollama"
    }
  ]
}
```

### Test Chat Completion

**Non-Streaming**:
```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 100,
    "messages": [
      {"role": "user", "content": "Say hello in one sentence"}
    ]
  }' | jq
```

**Streaming**:
```bash
curl -N -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:latest",
    "max_tokens": 50,
    "messages": [
      {"role": "user", "content": "Count from 1 to 3"}
    ],
    "stream": true
  }'
```

## Customisation

### Using Different Models

Edit the test commands or Claude Code prompts to specify different models:

```bash
# In your requests
"model": "qwen2.5-coder:32b"
```

Or pull additional models:
```bash
docker exec ollama ollama pull mistral-nemo:latest
docker exec ollama ollama pull codellama:latest
```

### Adding Multiple Ollama Instances

Edit `olla.yaml`:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://ollama:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100

      - url: "http://192.168.1.100:11434"  # Another machine
        name: "remote-ollama"
        type: "ollama"
        priority: 50                        # Lower priority (fallback)
```

### Adjusting Performance Settings

For faster responses, edit `olla.yaml`:

```yaml
proxy:
  engine: "olla"              # High-performance engine
  profile: "streaming"        # Low-latency streaming
  load_balancer: "priority"   # Use highest priority endpoint first
```

### GPU Support (Optional)

Uncomment the GPU section in `compose.yaml`:

```yaml
services:
  ollama:
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

## Troubleshooting

### Services Won't Start

**Check logs**:

```bash
docker compose logs olla
docker compose logs ollama
```

**Common issues**:

- Port 40114 or 11434 already in use
- Docker daemon not running
- Insufficient disk space

**Solution**:

```bash
# Stop and remove containers
docker compose down -v

# Check ports
netstat -an | grep 40114
netstat -an | grep 11434

# Start again
docker compose up -d
```

### No Models Available

**Check if models are pulled**:
```bash
docker exec ollama ollama list
```

**Pull a model**:
```bash
docker exec ollama ollama pull llama3.2:latest
```

### Claude Code Can't Connect

**Check Claude Code configuration**:
```bash
# Verify environment variable
echo $ANTHROPIC_BASE_URL

# Should output:
# http://localhost:40114/olla/anthropic/v1
```

**Test Olla directly**:
```bash
curl http://localhost:40114/internal/health
```

**Check Claude Code logs** (location varies by OS):
```bash
# macOS/Linux
tail -f ~/.claude-code/logs/client.log

# Windows
# Check %USERPROFILE%\.claude-code\logs\client.log
```

### Slow Responses

**Causes**:

- Model too large for hardware
- CPU-only inference (no GPU)
- Ollama not optimised

**Solutions**:

1. **Use smaller model**:
   ```bash
   docker exec ollama ollama pull llama3.2:3b  # Smaller, faster
   ```

2. **Enable GPU** (see GPU Support section above)

3. **Adjust Ollama settings**:
   ```bash
   # Inside ollama container
   export OLLAMA_NUM_PARALLEL=2
   export OLLAMA_MAX_LOADED_MODELS=1
   ```

4. **Optimise Olla**:
   Edit `olla.yaml`:
   ```yaml
   proxy:
     engine: "olla"
     profile: "streaming"
   ```

### Connection Refused

**From Claude Code to Olla**:

```bash
# Test from host
curl http://localhost:40114/internal/health

# If this works, Claude Code issue
# If this fails, Olla isn't running
docker compose ps
```

**From Olla to Ollama**:

```bash
# Test from Olla container
docker exec olla wget -q -O- http://ollama:11434/api/tags
```

## Advanced Usage

### Custom System Prompts

Claude Code allows custom system prompts. Example:

```bash
export CLAUDE_SYSTEM_PROMPT="You are an expert Go developer specialising in high-performance code."
```

### Model Selection in Claude Code

You can specify which model to use (if Claude Code supports it):

```bash
# Some versions of Claude Code allow model selection
# Check Claude Code documentation for your version
```

Or ensure only one model is loaded in Ollama:

```bash
docker exec ollama ollama list
docker exec ollama ollama rm <model-name>  # Remove unwanted models
```

### Monitoring

**Watch Olla logs**:
```bash
docker compose logs -f olla
```

**Watch Ollama logs**:
```bash
docker compose logs -f ollama
```

**Check Olla statistics**:
```bash
curl http://localhost:40114/internal/status | jq
```

### Production Considerations

This example is for **local development**. For production:

1. **Add authentication** (reverse proxy with API keys)
2. **Enable HTTPS** (TLS termination)
3. **Configure resource limits**:
   ```yaml
   services:
     olla:
       deploy:
         resources:
           limits:
             cpus: '2'
             memory: 4G
   ```
4. **Set up monitoring** (Prometheus, Grafana)
5. **Configure backups** (for Ollama models)

## Next Steps

- **[Claude Code Documentation](../../docs/content/integrations/frontend/claude-code.md)** - Complete integration guide
- **[Anthropic API Reference](../../docs/content/api-reference/anthropic.md)** - API documentation
- **[Olla Configuration Reference](../../docs/content/configuration/reference.md)** - All configuration options
- **[Model Routing](../../docs/content/concepts/model-routing.md)** - Understanding request routing

## Related Examples

- [Claude Code + llama.cpp](../claude-code-llamacpp/) - Using llama.cpp backend
- [OpenCode + LM Studio](../opencode-lmstudio/) - OpenCode integration
- [Crush CLI + vLLM](../crush-vllm/) - High-performance backend

## Support

- **Issues**: [GitHub Issues](https://github.com/thushan/olla/issues)
- **Discussions**: [GitHub Discussions](https://github.com/thushan/olla/discussions)
- **Documentation**: [Olla Docs](https://thushan.github.io/olla/)
