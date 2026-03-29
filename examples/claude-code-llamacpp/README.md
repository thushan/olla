# Claude Code + Olla + llama.cpp Integration Example

Use Claude Code with a lightweight llama.cpp server backend through Olla's Anthropic API translation.

## Architecture

```
┌──────────────┐    Anthropic API     ┌──────────┐    OpenAI API    ┌──────────────┐
│ Claude Code  │───────────────────────▶│   Olla   │──────────────────▶│ llama.cpp    │
│              │   /anthropic/v1/*     │  :40114  │   /v1/*          │   :8080      │
│              │◀───────────────────────│          │◀──────────────────│              │
└──────────────┘                       └──────────┘                   └──────────────┘
```

## Prerequisites

- Docker and Docker Compose
- Claude Code installed
- Downloaded GGUF model file (see Model Setup below)

## Model Setup

Download a GGUF model before starting:

```bash
# Create models directory
mkdir -p models

# Download a model (choose one) - easier with hugging face CLI:

# Qwen (modern coding-capable baseline)
wget -P models https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q4_K_M.gguf


# Mistral successor (Magistral family – 2025 line)
wget -P models https://huggingface.co/mistralai/Magistral-Small-2509-GGUF/resolve/main/Magistral-Small-2509-Q5_K_M.gguf

```

**Update `compose.yaml`** to reference your chosen model (see compose.yaml below).

## Quick Start

### 1. Download Model

See "Model Setup" section above.

### 2. Update compose.yaml

Edit `compose.yaml` and update the model path in the llama-cpp service:

```yaml
command:
  - "--model"
  - "/models/YOUR-MODEL-NAME.gguf"  # Update this line
```

### 3. Start Services

```bash
docker compose up -d
```

Wait ~10-30 seconds for llama.cpp to load the model.

### 4. Verify Setup

```bash
./test.sh
```

### 5. Configure Claude Code

```bash
export ANTHROPIC_BASE_URL="http://localhost:40114/olla/anthropic"
```

### 6. Use Claude Code

```bash
claude
```

## Files

| File | Description |
|------|-------------|
| `compose.yaml` | Docker Compose for Olla + llama.cpp |
| `olla.yaml` | Olla configuration |
| `test.sh` | Test script |
| `README.md` | This file |

## Configuration

### llama.cpp Server Options

Edit `compose.yaml` to customise llama.cpp:

```yaml
services:
  llama-cpp:
    command:
      - "--model"
      - "/models/your-model.gguf"
      - "--ctx-size"
      - "8192"              # Context window size
      - "--n-gpu-layers"
      - "0"                 # CPU-only (set > 0 for GPU)
      - "--threads"
      - "8"                 # CPU threads
      - "--batch-size"
      - "512"
      - "--port"
      - "8080"
      - "--host"
      - "0.0.0.0"
```

**Key Parameters**:

- `--ctx-size`: Context window (2048, 4096, 8192, etc.)
- `--n-gpu-layers`: GPU layers (0 = CPU only, 35 = full GPU)
- `--threads`: CPU threads (match your CPU cores)
- `--batch-size`: Batch size (higher = faster, more memory)

### GPU Support

For NVIDIA GPU:

```yaml
services:
  llama-cpp:
    image: ghcr.io/ggml-org/llama.cpp:server-cuda
    command:
      - "--n-gpu-layers"
      - "35"                # Use GPU for all layers
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

For AMD GPU:
```yaml
services:
  llama-cpp:
    image: ghcr.io/ggml-org/llama.cpp:server-rocm
```

For Apple Metal:
```yaml
services:
  llama-cpp:
    image: ghcr.io/ggml-org/llama.cpp:server-metal
```

## Troubleshooting

### Model Not Loading

**Check logs**:
```bash
docker compose logs llama-cpp
```

**Common issues**:

- Model path incorrect in compose.yaml
- Model file not in `./models/` directory
- Insufficient memory for model

**Solutions**:
```bash
# Verify model file exists
ls -lh models/

# Check model path in compose.yaml matches filename
grep "model" compose.yaml

# Try smaller model (3B instead of 7B)
```

### Out of Memory

**Symptoms**: llama.cpp crashes or fails to start

**Solutions**:
1. **Use smaller model**:
   - 3B models: ~4GB RAM
   - 7B models: ~8-16GB RAM (depending on quantisation)

2. **Use higher quantisation**:
   - Q4_K_M: Smaller, lower quality
   - Q5_K_M: Medium (recommended)
   - Q8_0: Larger, higher quality

3. **Reduce context size**:
   ```yaml
   - "--ctx-size"
   - "2048"              # Reduce from 8192
   ```

### Slow Performance

**For CPU**:
```yaml
- "--threads"
- "12"                  # Use more CPU threads
- "--batch-size"
- "1024"                # Increase batch size
```

**Use GPU** (if available):
```yaml
- "--n-gpu-layers"
- "35"                  # Offload all layers to GPU
```

### Connection Refused

**Check llama.cpp is running**:
```bash
curl http://localhost:8080/health
```

**Check llama.cpp logs**:
```bash
docker compose logs llama-cpp
```

## Advanced Usage

### Multiple Models

Run multiple llama.cpp instances on different ports:

```yaml
services:
  llama-cpp-small:
    image: ghcr.io/ggml-org/llama.cpp:server
    ports:
      - "8080:8080"
    volumes:
      - ./models:/models
    command:
      - "--model"
      - "/models/llama-3.2-3b.gguf"
      - "--port"
      - "8080"

  llama-cpp-large:
    image: ghcr.io/ggml-org/llama.cpp:server
    ports:
      - "8081:8081"
    volumes:
      - ./models:/models
    command:
      - "--model"
      - "/models/qwen2.5-coder-7b.gguf"
      - "--port"
      - "8081"
```

Update `olla.yaml`:
```yaml
discovery:
  static:
    endpoints:
      - url: "http://llama-cpp-small:8080"
        name: "small-model"
        type: "llamacpp"
        priority: 100

      - url: "http://llama-cpp-large:8081"
        name: "large-model"
        type: "llamacpp"
        priority: 50
```

## Next Steps

- **[llama.cpp Backend Integration](../../docs/content/integrations/backend/llamacpp.md)** - Full llama.cpp guide
- **[Claude Code Integration](../../docs/content/integrations/frontend/claude-code.md)** - Claude Code setup
- **[Anthropic API Reference](../../docs/content/api-reference/anthropic.md)** - API documentation

## Related Examples

- [Claude Code + Ollama](../claude-code-ollama/) - Easier setup with Ollama
- [OpenCode + LM Studio](../opencode-lmstudio/) - OpenCode integration
- [Crush CLI + vLLM](../crush-vllm/) - High-performance backend
