# Quick Start

Get Olla up and running in minutes with this quick start guide.

## Prerequisites

- [Olla installed](installation.md) on your system
- At least one LLM endpoint running (Ollama, LM Studio, or OpenAI-compatible)

## Basic Setup

### 1. Create Configuration

Create a `config.yaml` file:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  request_logging: true

proxy:
  engine: "sherpa"  # or "olla" for high-performance
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        health_check_url: "/"

logging:
  level: "info"
  format: "json"
```

### 2. Start Olla

```bash
olla --config config.yaml
```

You should see output similar to:

```json
{"level":"info","msg":"Starting Olla proxy server","port":8080}
{"level":"info","msg":"Health check passed","endpoint":"local-ollama"}
{"level":"info","msg":"Server ready","endpoints":1}
```

### 3. Test the Proxy

Check that Olla is running:

```bash
curl http://localhost:8080/internal/health
```

List available models through the proxy:

```bash
# For Ollama endpoints
curl http://localhost:8080/olla/api/tags

# For OpenAI-compatible endpoints
curl http://localhost:8080/olla/v1/models
```

## Example Requests

### Chat Completion (OpenAI-compatible)

```bash
curl -X POST http://localhost:8080/olla/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

### Ollama Generate

```bash
curl -X POST http://localhost:8080/olla/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "prompt": "Why is the sky blue?"
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:8080/olla/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Tell me a story"}
    ],
    "stream": true
  }'
```

## Multiple Endpoints Configuration

Configure multiple LLM endpoints with load balancing:

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      # High priority local Ollama
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        
      # Medium priority LM Studio
      - url: "http://localhost:1234"
        name: "local-lm-studio"
        type: "lm-studio"
        priority: 50
        
      # Low priority remote endpoint
      - url: "https://api.example.com"
        name: "remote-api"
        type: "openai"
        priority: 10
        headers:
          authorization: "Bearer your-api-key"
```

## Monitoring

Monitor Olla's performance:

```bash
# Health status
curl http://localhost:8080/internal/health

# System status and statistics
curl http://localhost:8080/internal/status
```

Response headers provide request tracing:

```bash
curl -I http://localhost:8080/olla/v1/models
```

Look for these headers:
- `X-Olla-Endpoint`: Which backend handled the request
- `X-Olla-Backend-Type`: Type of backend (ollama/openai/lmstudio)
- `X-Olla-Request-ID`: Unique request identifier
- `X-Olla-Response-Time`: Total processing time

## Common Configuration Options

### High-Performance Setup

For production environments, use the Olla engine:

```yaml
proxy:
  engine: "olla"  # High-performance engine
  load_balancer: "least-connections"
  connection_timeout: 30s
  max_retries: 3
```

### Rate Limiting

Protect your endpoints with rate limiting:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 100
    burst_size: 50
```

### Request Size Limits

Set appropriate request limits:

```yaml
server:
  request_limits:
    max_body_size: 52428800   # 50MB
    max_header_size: 524288   # 512KB
```

## Next Steps

- [Configuration Reference](../config/reference.md) - Complete configuration options
- [Architecture Overview](../architecture/overview.md) - Understand how Olla works
- [Load Balancing](../architecture/load-balancing.md) - Advanced load balancing strategies
- [Development Guide](../development/contributing.md) - Contribute to Olla

## Troubleshooting

### Endpoint Not Responding

Check your endpoint URLs and ensure the services are running:

```bash
# Test direct access to your LLM endpoint
curl http://localhost:11434/api/tags
```

### Health Checks Failing

Verify health check URLs are correct for your endpoint type:

- **Ollama**: Use `/` or `/api/version`
- **LM Studio**: Use `/` or `/v1/models`  
- **OpenAI-compatible**: Use `/v1/models`

### High Latency

Consider switching to the high-performance Olla engine:

```yaml
proxy:
  engine: "olla"
  load_balancer: "least-connections"
```

For more detailed troubleshooting, check the logs and [open an issue](https://github.com/thushan/olla/issues) if needed.