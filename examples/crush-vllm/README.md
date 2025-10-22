# Crush CLI + vLLM Integration Example

This example demonstrates how to set up Crush CLI with Olla as a proxy/load balancer for vLLM high-performance inference engines. Crush CLI supports both OpenAI and Anthropic API formats, giving you the flexibility to switch between providers whilst maintaining a consistent experience.

## Architecture

```
┌─────────────┐    ┌──────────┐    ┌─────────────────┐
│  Crush CLI  │───▶│   Olla   │───▶│  vLLM Instance  │
│             │    │(Port     │    │  (Port 8000)    │
│ OpenAI or   │    │ 40114)   │    │                 │
│ Anthropic   │    │          │    │ GPU-optimised   │
│ Provider    │    │          │    │ PagedAttention  │
└─────────────┘    └──────────┘    └─────────────────┘
```

## Prerequisites

### GPU Requirements
vLLM requires NVIDIA GPU with compute capability 7.0 or higher:
- **Minimum**: RTX 2060 (6GB VRAM) for small models (3B-7B parameters)
- **Recommended**: RTX 3090/4090 (24GB VRAM) for medium models (13B-30B parameters)
- **Production**: A100/H100 (40GB+ VRAM) for large models (70B+ parameters)

### Software Requirements
- Docker with GPU support (NVIDIA Container Toolkit)
- Docker Compose v2.0+
- NVIDIA drivers 525.60.13 or newer
- Crush CLI installed ([Installation Guide](https://github.com/charmbracelet/crush))

## Quick Start

### 1. Verify GPU Support

```bash
# Check NVIDIA Docker runtime
docker run --rm --gpus all nvidia/cuda:12.1.0-base-ubuntu22.04 nvidia-smi
```

You should see your GPU listed with CUDA version information.

### 2. Start the Stack

```bash
# Clone or navigate to the example directory
cd examples/crush-vllm

# Pull and start services
docker compose up -d
```

This will:

- Start vLLM with GPU support
- Start Olla proxy configured for vLLM
- Download the model (first run only)

### 3. Wait for Model Loading

vLLM needs time to load the model into GPU memory:

```bash
# Monitor vLLM logs
docker logs vllm -f

# Wait for this message:
# "Avg prompt throughput: ... tokens/s, Avg generation throughput: ... tokens/s"
```

Model loading times:

- 3B model: ~15-30 seconds
- 7B model: ~30-60 seconds
- 13B model: ~60-120 seconds

### 4. Verify Services

```bash
# Check Olla health
curl http://localhost:40114/internal/health

# Check vLLM health
curl http://localhost:8000/health

# List available models via Olla
curl http://localhost:40114/olla/openai/v1/models

# List models via Anthropic endpoint
curl http://localhost:40114/olla/anthropic/v1/models
```

### 5. Configure Crush CLI

Create or update your Crush configuration file at `~/.crush/config.json`:

```bash
# Copy the example configuration
cp crush-config.example.json ~/.crush/config.json

# Edit the file to match your setup
# The example includes both OpenAI and Anthropic providers
```

### 6. Use Crush CLI

```bash
# Start a chat session with OpenAI provider
crush chat --provider olla-openai

# Or use Anthropic provider
crush chat --provider olla-anthropic

# Switch between providers at any time
crush model switch olla-anthropic

# List available models
crush model list

# Set default provider
crush config set default_provider olla-openai
```

## Configuration Files

### compose.yaml

Defines the Docker services:

- **vLLM**: High-performance inference engine with GPU support
- **Olla**: Proxy and load balancer with API translation

### olla.yaml

Olla configuration:

- High-performance engine (`olla`) for optimal throughput
- Streaming profile for real-time responses
- vLLM endpoint discovery and health checks
- Anthropic translator enabled for dual API support

### crush-config.example.json

Crush CLI configuration showing:

- Both `olla-openai` and `olla-anthropic` providers
- Model metadata (context window, cost, capabilities)
- Provider-specific settings
- Schema reference to Charm

## Model Configuration

### Changing the Model

Edit `compose.yaml` and update the vLLM model parameter:

```yaml
services:
  vllm:
    command:
      - "--model"
      - "meta-llama/Meta-Llama-3.1-8B-Instruct"  # Change this
```

Popular models for vLLM:

- `meta-llama/Meta-Llama-3.1-8B-Instruct` (Recommended, 8B params, 8K context)
- `meta-llama/Meta-Llama-3.2-3B-Instruct` (Smaller, 3B params, 8K context)
- `mistralai/Mistral-7B-Instruct-v0.3` (7B params, 32K context)
- `Qwen/Qwen2.5-7B-Instruct` (7B params, 32K context)

### GPU Memory Requirements

| Model Size | VRAM Required | Recommended GPU |
|------------|---------------|-----------------|
| 3B | 6-8 GB | RTX 3060 12GB |
| 7B-8B | 12-16 GB | RTX 3090, RTX 4080 |
| 13B | 24-32 GB | RTX 4090, A5000 |
| 30B+ | 48+ GB | A100, H100 |

## Provider Switching

One of Crush CLI's key features is seamless provider switching:

### Switch Providers Mid-Conversation

```bash
# Start with OpenAI format
crush chat --provider olla-openai

# Within the chat, switch to Anthropic format
/model switch olla-anthropic

# Switch back
/model switch olla-openai
```

### Compare Response Formats

```bash
# Test OpenAI endpoint
crush chat --provider olla-openai --prompt "Explain async/await in Python"

# Test Anthropic endpoint (same backend!)
crush chat --provider olla-anthropic --prompt "Explain async/await in Python"
```

Both providers route to the same vLLM backend through Olla's translation layer. The difference is purely in the API format used for communication.

## Monitoring

### Check Olla Status

```bash
# Overall health
curl http://localhost:40114/internal/health

# Endpoint status
curl http://localhost:40114/internal/status/endpoints

# Statistics
curl http://localhost:40114/internal/status/stats
```

### Monitor vLLM Performance

```bash
# Prometheus metrics
curl http://localhost:8000/metrics

# Key metrics to watch:
# - vllm:num_requests_running
# - vllm:num_requests_waiting
# - vllm:gpu_cache_usage_perc
# - vllm:time_to_first_token_seconds
```

### View Logs

```bash
# Olla logs
docker logs olla -f

# vLLM logs
docker logs vllm -f

# Combined logs
docker compose logs -f
```

## Advanced Configuration

### Multiple vLLM Instances

To add load balancing across multiple vLLM instances, edit `olla.yaml`:

```yaml
discovery:
  static:
    endpoints:
      # Primary vLLM instance (high priority)
      - url: "http://vllm:8000"
        name: "vllm-primary"
        type: "vllm"
        priority: 100

      # Secondary vLLM instance (medium priority)
      - url: "http://vllm-secondary:8000"
        name: "vllm-secondary"
        type: "vllm"
        priority: 75
```

Then add the secondary service to `compose.yaml`.

### Optimise vLLM Performance

Edit the vLLM command in `compose.yaml`:

```yaml
command:
  - "--model"
  - "meta-llama/Meta-Llama-3.1-8B-Instruct"
  - "--max-model-len"
  - "8192"                    # Adjust context window
  - "--gpu-memory-utilization"
  - "0.95"                    # Use 95% of GPU memory
  - "--max-num-seqs"
  - "256"                     # Increase concurrent sequences
  - "--enable-prefix-caching" # Enable prompt caching
```

### Custom Crush Prompts

Create custom system prompts in your Crush config:

```json
{
  "providers": {
    "olla-openai": {
      "type": "openai",
      "base_url": "http://localhost:40114/olla/openai/v1",
      "default_system_prompt": "You are a helpful coding assistant running locally via vLLM."
    }
  }
}
```

## Performance Tuning

### For Maximum Throughput

```yaml
# olla.yaml
proxy:
  engine: "olla"              # High-performance engine
  stream_buffer_size: 16384   # Larger buffer

# vLLM command
  - "--max-num-batched-tokens"
  - "8192"                    # Increase batch size
```

### For Low Latency

```yaml
# olla.yaml
proxy:
  engine: "olla"
  connection_timeout: 10s

# vLLM command
  - "--max-num-seqs"
  - "32"                      # Reduce concurrency
```

### For Memory-Constrained GPUs

```yaml
# vLLM command
  - "--gpu-memory-utilization"
  - "0.85"                    # Reduce GPU memory usage
  - "--max-model-len"
  - "4096"                    # Smaller context window
  - "--enforce-eager"         # Disable CUDA graphs (saves memory)
```

## Troubleshooting

### vLLM Out of Memory

**Symptoms**: Container crashes with CUDA out of memory error

**Solutions**:

1. Reduce model size (use 3B instead of 7B)
2. Decrease `--gpu-memory-utilization` to `0.8`
3. Reduce `--max-model-len` to `4096`
4. Add `--enforce-eager` to disable CUDA graphs

### Slow Model Loading

**Symptoms**: vLLM takes minutes to start

**Causes**:

- Large model download (first run)
- Model quantization/compilation
- Insufficient GPU memory causing swapping

**Solutions**:

1. Pre-download models: `docker compose pull`
2. Use smaller models for testing
3. Check GPU memory: `nvidia-smi`

### Crush Can't Connect

**Symptoms**: Connection refused or timeout errors

**Solutions**:
```bash
# Verify Olla is running
curl http://localhost:40114/internal/health

# Check Olla logs
docker logs olla

# Verify vLLM is healthy
curl http://localhost:8000/health

# Test the full chain
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","messages":[{"role":"user","content":"test"}],"max_tokens":10}'
```

### Provider Not Found

**Symptoms**: Crush says provider doesn't exist

**Solutions**:

1. Check config file location: `~/.crush/config.json`
2. Validate JSON syntax: `cat ~/.crush/config.json | jq`
3. Verify provider names match exactly
4. Review Crush logs: `crush --debug chat`

### Streaming Issues

**Symptoms**: Responses appear all at once instead of streaming

**Solutions**:

1. Ensure Olla is using streaming profile
2. Check vLLM supports streaming: `curl http://localhost:8000/health`
3. Verify Crush streaming settings in config
4. Test streaming directly:
```bash
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","messages":[{"role":"user","content":"Count to 10"}],"stream":true}'
```

## Testing

Run the included test script to verify the setup:

```bash
# Make executable
chmod +x test.sh

# Run tests
./test.sh
```

The test script checks:

1. vLLM health endpoint
2. Olla health endpoint
3. Model discovery through both APIs
4. OpenAI format completions
5. Anthropic format messages
6. Streaming functionality

## Next Steps

- **[vLLM Backend Integration](../../docs/content/integrations/backend/vllm.md)** - Full vLLM configuration guide
- **[Crush CLI Integration](../../docs/content/integrations/frontend/crush-cli.md)** - Complete Crush CLI setup
- **[Anthropic API Reference](../../docs/content/api-reference/anthropic.md)** - API documentation
- **[Olla Configuration Reference](../../docs/content/configuration/reference.md)** - All configuration options

## Related Examples

- [Claude Code + Ollama](../claude-code-ollama/) - Claude Code with Ollama backend
- [Claude Code + llama.cpp](../claude-code-llamacpp/) - Lightweight llama.cpp backend
- [OpenCode + LM Studio](../opencode-lmstudio/) - OpenCode integration

## Support

For issues with:
- **Olla**: [GitHub Issues](https://github.com/thushan/olla/issues)
- **vLLM**: [vLLM GitHub](https://github.com/vllm-project/vllm/issues)
- **Crush CLI**: [Crush GitHub](https://github.com/charmbracelet/crush/issues)
