# Olla User Guide

## Installation

### Docker (Recommended)

```bash
# Basic setup
docker run -d --name olla \
  -p 40114:40114 \
  ghcr.io/thushan/olla:latest

# With custom config
docker run -d --name olla \
  -p 40114:40114 \
  -v ./config.yaml:/app/config.yaml \
  -e OLLA_CONFIG=/app/config.yaml \
  ghcr.io/thushan/olla:latest
```

### Binary Installation

1. Download the latest release:
```bash
# Linux
wget https://github.com/thushan/olla/releases/latest/download/olla-linux-amd64

# macOS
wget https://github.com/thushan/olla/releases/latest/download/olla-darwin-amd64

# Windows
wget https://github.com/thushan/olla/releases/latest/download/olla-windows-amd64.exe
```

2. Make it executable (Linux/macOS):
```bash
chmod +x olla-*
```

3. Run with configuration:
```bash
./olla -config config.yaml
```

### From Source

```bash
git clone https://github.com/thushan/olla
cd olla
make build
./bin/olla -config config.yaml
```

## Configuration

### Minimal Configuration

Create `config.yaml`:
```yaml
discovery:
  static:
    endpoints:
      - name: local-ollama
        url: http://localhost:11434
        type: ollama
```

### Typical Home Setup

```yaml
server:
  host: 0.0.0.0  # Allow network access
  port: 40114

proxy:
  engine: sherpa  # Simple engine for home use
  load_balancer: priority

discovery:
  static:
    endpoints:
      - name: gaming-pc
        url: http://192.168.1.100:11434
        type: ollama
        priority: 100  # Primary
        
      - name: old-laptop
        url: http://192.168.1.150:11434  
        type: ollama
        priority: 50   # Backup
        
      - name: lm-studio
        url: http://192.168.1.100:1234
        type: lmstudio
        priority: 75

logging:
  level: info
```

### Production Configuration

```yaml
server:
  host: 0.0.0.0
  port: 40114
  request_limits:
    max_body_size: 10485760  # 10MB
    max_header_size: 65536   # 64KB
  rate_limits:
    per_ip_requests_per_minute: 30
    global_requests_per_minute: 500

proxy:
  engine: olla  # High-performance engine
  load_balancer: priority
  response_timeout: 600s
  # Olla engine specific
  max_idle_conns: 200
  max_conns_per_host: 100

discovery:
  static:
    endpoints:
      - name: gpu-server-1
        url: http://10.0.1.10:11434
        type: ollama
        priority: 100
        
      - name: gpu-server-2
        url: http://10.0.1.11:11434
        type: ollama
        priority: 100
        
      - name: cpu-backup
        url: http://10.0.1.20:11434
        type: ollama
        priority: 25

health:
  check_interval: 30s
  timeout: 5s
  failure_threshold: 3

logging:
  level: info
  format: json
```

## Usage Examples

### Python (OpenAI Client)

```python
from openai import OpenAI

# For Ollama backends
client = OpenAI(
    base_url="http://localhost:40114/olla/ollama/v1",
    api_key="not-needed"  # Olla doesn't require auth
)

# For LM Studio backends
client = OpenAI(
    base_url="http://localhost:40114/olla/lmstudio/v1",
    api_key="not-needed"
)

# Chat completion
response = client.chat.completions.create(
    model="llama3.2",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Explain quantum computing in simple terms."}
    ]
)

print(response.choices[0].message.content)

# Check which backend handled it
print(f"Handled by: {response.headers.get('X-Olla-Endpoint')}")
```

### JavaScript/TypeScript

```javascript
import OpenAI from 'openai';

// For Ollama backends
const openai = new OpenAI({
  baseURL: 'http://localhost:40114/olla/ollama/v1',
  apiKey: 'not-needed',
});

// For OpenAI-compatible backends (vLLM, etc.)
const openai = new OpenAI({
  baseURL: 'http://localhost:40114/olla/openai/v1',
  apiKey: 'not-needed',
});

async function chat() {
  const response = await openai.chat.completions.create({
    model: 'llama3.2',
    messages: [{ role: 'user', content: 'Hello!' }],
  });
  
  console.log(response.choices[0].message.content);
}
```

### cURL

```bash
# Chat completion via Ollama
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Chat completion via LM Studio
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "TheBloke/Llama-2-7B-Chat-GGUF",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# List available models
curl http://localhost:40114/olla/models | jq

# List models from Ollama only
curl http://localhost:40114/olla/models?format=ollama | jq

# Check system status
curl http://localhost:40114/internal/status | jq
```

### Ollama Native API

```bash
# Use Ollama's native generate API
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "prompt": "Why is the sky blue?"
  }'

# List models in Ollama format
curl http://localhost:40114/olla/ollama/api/tags | jq
```

## Model Management

### List Available Models

```bash
# All models across all endpoints
curl http://localhost:40114/olla/models

# Models from specific endpoint
curl http://localhost:40114/olla/models?endpoint=gaming-pc

# Only loaded models
curl http://localhost:40114/olla/models?available=true

# Filter by family
curl http://localhost:40114/olla/models?family=llama
```

### Model Routing

Olla automatically routes to endpoints that have your requested model:

```python
# Request specific model
response = client.chat.completions.create(
    model="llama3.2:3b",  # Routes to any endpoint with this model
    messages=[...]
)

```

## Monitoring

### Health Checks

```bash
# Simple health check
curl http://localhost:40114/internal/health
# Returns: 200 OK if healthy

# Detailed status
curl http://localhost:40114/internal/status | jq
```

### Response Headers

Every request includes diagnostic headers:
```
X-Olla-Endpoint: gaming-pc        # Which backend handled it
X-Olla-Model: llama3.2:3b         # Actual model used
X-Olla-Backend-Type: ollama       # Backend type
X-Olla-Request-ID: req_abc123     # For log correlation
X-Olla-Response-Time: 523ms       # Total time
```

### Metrics

```bash
# Model usage statistics
curl http://localhost:40114/internal/stats/models | jq

# Process statistics
curl http://localhost:40114/internal/process | jq
```

## Common Patterns

### Multi-Environment Setup

```yaml
# Development
endpoints:
  - name: local-ollama
    url: http://localhost:11434
    priority: 100

# Production  
endpoints:
  - name: gpu-cluster
    url: http://gpu.internal:11434
    priority: 100
  - name: cpu-fallback
    url: http://cpu.internal:11434
    priority: 10
```

### Rate Limiting by Use Case

```bash
# Personal use - relaxed limits
OLLA_SERVER_PER_IP_RATE_LIMIT=60
OLLA_SERVER_RATE_BURST_SIZE=10

# Shared team - moderate limits
OLLA_SERVER_PER_IP_RATE_LIMIT=30
OLLA_SERVER_RATE_BURST_SIZE=5

# Public API - strict limits
OLLA_SERVER_PER_IP_RATE_LIMIT=10
OLLA_SERVER_RATE_BURST_SIZE=2
```

### Debugging Issues

```bash
# Enable debug logging
export OLLA_LOG_LEVEL=debug
./olla -config config.yaml

# Test specific endpoint
curl -H "X-Olla-Endpoint: gaming-pc" \
  http://localhost:40114/olla/ollama/v1/chat/completions \
  -d '{"model": "llama3.2", "messages": [...]}'

# Check rate limit headers
curl -i http://localhost:40114/olla/models | grep X-RateLimit
```

## Troubleshooting

### "No healthy endpoints available"
1. Check endpoints are running: `curl http://endpoint-url/api/tags`
2. Verify URLs in config.yaml
3. Check Olla logs for health check failures
4. Ensure network connectivity

### "Model not found"
1. List available models: `curl http://localhost:40114/olla/models`
2. Verify model is loaded on at least one endpoint
3. Check model name spelling and format

### High latency
1. Check response time header: `X-Olla-Response-Time`
2. Verify primary endpoints are healthy
3. Consider switching to Olla engine for better performance
4. Check connection pool settings

### Rate limit errors
1. Check current limits: response includes `X-RateLimit-*` headers
2. Increase limits in configuration
3. Use burst size for temporary spikes
4. Consider per-IP vs global limits

## Best Practices

1. **Use Priority Load Balancing**: Set higher priorities for faster/local endpoints
2. **Monitor Health Endpoints**: Regular checks help detect issues early
3. **Set Appropriate Timeouts**: LLM requests can take minutes, set timeouts accordingly
4. **Use Response Headers**: Track which endpoints handle requests for debugging
5. **Configure Rate Limits**: Protect your infrastructure while allowing legitimate use
6. **Regular Model Sync**: Ensure endpoints have consistent model availability

## Next Steps

- Explore [API Reference](api/) for detailed endpoint documentation
- Read [Technical Overview](technical.md) for architecture details
- Check [examples/](../examples/) for more configuration samples