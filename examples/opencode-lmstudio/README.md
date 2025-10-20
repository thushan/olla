# OpenCode + LM Studio Integration Example

This example demonstrates how to set up OpenCode (SST fork) with Olla as a proxy to LM Studio for local AI coding assistance.

## Architecture

```
┌─────────────┐    ┌──────────┐    ┌─────────────────┐
│  OpenCode   │───▶│   Olla   │───▶│   LM Studio     │
│   (Local)   │    │(Container│    │   (Host)        │
│             │    │ :40114)  │    │   :1234         │
└─────────────┘    └──────────┘    └─────────────────┘
                    Anthropic
                    Translator
```

## Endpoint Options

OpenCode can connect to Olla using either endpoint:

1. **OpenAI endpoint** (simpler, direct passthrough):
   ```json
   "baseURL": "http://localhost:40114/olla/openai/v1"
   ```

2. **Anthropic endpoint** (uses Olla's Anthropic translator):
   ```json
   {
     "npm": "@ai-sdk/anthropic",
     "options": {
       "baseURL": "http://localhost:40114/olla/anthropic/v1"
     }
   }
   ```

This example demonstrates both options. See `opencode-config.example.json` for details.

## Prerequisites

1. **LM Studio** installed and running on your host machine
   - Download from: https://lmstudio.ai/
   - Enable server mode (Settings → Server → Start Server)
   - Default port: 1234
   - Load a model (e.g., Qwen, Llama, Mistral)

2. **Docker** installed for running Olla

3. **OpenCode** installed:
   ```bash
   npm install -g @sst/opencode
   ```

## Quick Start

### 1. Start LM Studio

1. Open LM Studio
2. Download and load a model (e.g., `qwen2.5-coder-7b-instruct`)
3. Go to Developer → Server
4. Click "Start Server" (default: http://localhost:1234)
5. Verify it's running:
   ```bash
   curl http://localhost:1234/v1/models
   ```

### 2. Configure Olla

The `olla.yaml` configuration is already set up to connect to LM Studio on your host:

**For Windows/Mac** (using `host.docker.internal`):
```yaml
endpoints:
  - url: "http://host.docker.internal:1234"
    name: "lm-studio"
    type: "lmstudio"
```

**For Linux** (using host network bridge IP):
```yaml
endpoints:
  - url: "http://172.17.0.1:1234"  # Default Docker bridge IP
    name: "lm-studio"
    type: "lmstudio"
```

Edit `olla.yaml` if your setup differs.

### 3. Start Olla

```bash
docker compose up -d
```

### 4. Verify Connectivity

Test that Olla can reach LM Studio:

```bash
# Check Olla health
curl http://localhost:40114/internal/health

# Check LM Studio endpoint status
curl http://localhost:40114/internal/status/endpoints

# List models via OpenAI endpoint
curl http://localhost:40114/olla/openai/v1/models

# List models via Anthropic endpoint
curl http://localhost:40114/olla/anthropic/v1/models
```

### 5. Configure OpenCode

Copy the example configuration:

```bash
# Linux/Mac
mkdir -p ~/.opencode
cp opencode-config.example.json ~/.opencode/config.json

# Windows (PowerShell)
New-Item -ItemType Directory -Force -Path $env:USERPROFILE\.opencode
Copy-Item opencode-config.example.json $env:USERPROFILE\.opencode\config.json
```

Edit `~/.opencode/config.json` to choose your preferred endpoint (OpenAI or Anthropic).

### 6. Use OpenCode

```bash
# Start OpenCode with your configured provider
opencode

# Or specify a model
opencode --model qwen2.5-coder:7b
```

## Configuration Details

### OpenCode Configuration

See `opencode-config.example.json` for two provider configurations:

**Option 1: OpenAI-Compatible (Recommended)**
```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "olla-openai": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:40114/olla/openai/v1"
      }
    }
  }
}
```

**Option 2: Anthropic Format**
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

### Olla Configuration

The `olla.yaml` file includes:

- **Anthropic translator enabled**: For use with Anthropic endpoint
- **LM Studio endpoint**: Configured to reach host machine
- **Model discovery**: Automatically discovers models from LM Studio

## Networking Notes

### Windows/Mac Users

Use `host.docker.internal` to access services on your host machine from Docker:

```yaml
- url: "http://host.docker.internal:1234"
```

This is automatically configured in the provided `olla.yaml`.

### Linux Users

Docker on Linux doesn't support `host.docker.internal` by default. Use one of these approaches:

**Option 1: Docker bridge IP** (default, already in olla.yaml):
```yaml
- url: "http://172.17.0.1:1234"
```

**Option 2: Host network mode** (simpler, but less isolated):
```yaml
services:
  olla:
    network_mode: "host"
    # Remove ports section when using host mode
```

**Option 3: Custom bridge with host.docker.internal**:
```yaml
services:
  olla:
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

Then use `http://host.docker.internal:1234` in `olla.yaml`.

## Testing

Run the included test script:

```bash
chmod +x test.sh
./test.sh
```

The test script will:

1. Check if LM Studio is running on the host
2. Verify Olla container is running
3. Test Olla → LM Studio connectivity
4. Test both OpenAI and Anthropic endpoints
5. Test chat completion requests

## Troubleshooting

### LM Studio Not Reachable

**Symptom**: `curl http://localhost:1234/v1/models` fails from within Olla container.

**Solutions**:

1. **Verify LM Studio server is running**:
   - Open LM Studio
   - Go to Developer → Server
   - Check "Server running" status
   - Try from host: `curl http://localhost:1234/v1/models`

2. **Check firewall settings**:
   - Ensure port 1234 is not blocked
   - Allow LM Studio through your firewall

3. **Linux users**: Verify Docker bridge IP:
   ```bash
   ip addr show docker0 | grep inet
   ```
   Update `olla.yaml` if it differs from 172.17.0.1

4. **Mac/Windows users**: Ensure Docker Desktop is up to date for `host.docker.internal` support

### Models Not Appearing

**Symptom**: `curl http://localhost:40114/olla/openai/v1/models` returns empty list.

**Solutions**:

1. **Load a model in LM Studio**:
   - Models must be loaded (not just downloaded)
   - Click "Load" in LM Studio interface

2. **Check endpoint health**:
   ```bash
   curl http://localhost:40114/internal/status/endpoints
   ```
   Look for "healthy": true and "models_loaded": > 0

3. **Wait for discovery**:
   - Model discovery runs every 5 minutes
   - Check Olla logs: `docker logs olla`

4. **Trigger manual discovery**:
   ```bash
   docker restart olla
   ```

### OpenCode Connection Issues

**Symptom**: OpenCode fails to connect or shows authentication errors.

**Solutions**:

1. **Check OpenCode configuration**:
   ```bash
   # Linux/Mac
   cat ~/.opencode/config.json

   # Windows
   type %USERPROFILE%\.opencode\config.json
   ```

2. **Verify Olla is accessible from host**:
   ```bash
   curl http://localhost:40114/olla/openai/v1/models
   ```

3. **Check API key** (if using Anthropic provider):
   - Set to any value or "not-required"
   - OpenCode needs the field, but Olla doesn't validate it for local use

4. **Test with curl first**:
   ```bash
   # OpenAI endpoint
   curl http://localhost:40114/olla/openai/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "qwen2.5-coder:7b",
       "messages": [{"role": "user", "content": "Hello!"}]
     }'

   # Anthropic endpoint
   curl http://localhost:40114/olla/anthropic/v1/messages \
     -H "Content-Type: application/json" \
     -d '{
       "model": "qwen2.5-coder:7b",
       "max_tokens": 1024,
       "messages": [{"role": "user", "content": "Hello!"}]
     }'
   ```

### Performance Issues

**LM Studio is single-threaded**:

- Handles one request at a time
- Configure in `olla.yaml`: `max_concurrent_requests: 1`
- This is already set in the provided configuration

**Slow responses**:

- Check LM Studio GPU/CPU usage
- Ensure model fits in available memory
- Consider smaller quantized models (Q4_K_M instead of Q8_0)

### Docker Issues

**Container won't start**:
```bash
# Check logs
docker logs olla

# Check if port 40114 is available
netstat -an | grep 40114  # Linux/Mac
netstat -an | findstr 40114  # Windows

# Restart with fresh state
docker compose down
docker compose up -d
```

## Advanced Configuration

### Using Multiple Models

LM Studio can only load one model at a time, but you can switch models:

1. Stop requests in OpenCode
2. Unload current model in LM Studio
3. Load new model
4. Wait for Olla discovery (or restart Olla)
5. Use new model in OpenCode

### Custom Model Names

If LM Studio shows models with full paths, configure model name mapping in `olla.yaml`:

```yaml
model_registry:
  unification:
    enabled: true
```

This normalises model names across endpoints.

### Adjusting Timeouts

For large models with slow generation:

```yaml
proxy:
  response_timeout: 1800s  # 30 minutes
  read_timeout: 600s       # 10 minutes
```

### Enabling Debug Logging

For troubleshooting:

```yaml
logging:
  level: "debug"
```

Then check logs:
```bash
docker logs olla -f
```

## Monitoring

### Check Olla Stats

```bash
# Endpoint health and model count
curl http://localhost:40114/internal/status/endpoints | jq

# Overall health
curl http://localhost:40114/internal/health | jq

# Available models
curl http://localhost:40114/olla/openai/v1/models | jq
```

### View Logs

```bash
# Olla logs
docker logs olla -f

# LM Studio logs
# Available in LM Studio GUI: Developer → Logs
```

## Related Examples

- [Claude Code + Ollama](../claude-code-ollama/) - Similar setup with Claude Code
- [OpenWebUI + Ollama](../ollama-openwebui/) - Web interface alternative

## Support

For issues with:

- **Olla**: Check the [Olla GitHub repository](https://github.com/thushan/olla)
- **OpenCode**: Check the [OpenCode GitHub repository](https://github.com/sst/opencode)
- **LM Studio**: Check the [LM Studio website](https://lmstudio.ai/)

## License

This example is part of the Olla project and follows the same license.
