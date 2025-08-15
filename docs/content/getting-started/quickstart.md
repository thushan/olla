---
title: Quick Start Guide - Get Olla Running in Minutes
description: Learn how to install and configure Olla for proxying and load balancing LLM backends including Ollama, LM Studio, and vLLM. Step-by-step setup with examples.
keywords: olla quickstart, llm proxy setup, ollama configuration, lm studio proxy, vllm load balancer
---

# Quick Start

Get Olla up and running with this quick start guide.

## Prerequisites

- [Olla installed](installation.md) on your system
- At least one [compatible LLM](../integrations/overview.md) endpoint running

## Basic Setup

### 1. Create Configuration

Create a `config.yaml` file:

```yaml
server:
  host: "0.0.0.0"
  port: 40114
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
        # health_check_url: "/"  # Optional, defaults to provider-specific path

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
{"level":"info","msg":"Starting Olla proxy server","port":40114}
{"level":"info","msg":"Health check passed","endpoint":"local-ollama"}
{"level":"info","msg":"Server ready","endpoints":1}
```

### 3. Test the Proxy

Check that Olla is running:

```bash
curl http://localhost:40114/internal/health
```

List available models through the proxy:

```bash
# For Ollama endpoints
curl http://localhost:40114/olla/ollama/api/tags

# For OpenAI-compatible endpoints
curl http://localhost:40114/olla/ollama/v1/models
```

## Example Requests

### Chat Completion (OpenAI-compatible)

```bash
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
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
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "prompt": "Why is the sky blue?"
  }'
```

### Streaming Response

```bash
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
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
```

## Monitoring

Monitor Olla's performance:

```bash
# Health status
curl http://localhost:40114/internal/health

# System status and statistics
curl http://localhost:40114/internal/status
```

Response headers provide request tracing:

```bash
curl -I http://localhost:40114/olla/ollama/v1/models
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
  # Note: Automatic retry on connection failures is built-in
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

## Learn More

### Core Concepts

- **[Proxy Engines](../concepts/proxy-engines.md)** - Compare Sherpa vs Olla engines
- **[Load Balancing](../concepts/load-balancing.md)** - Priority, round-robin, and least-connections strategies
- **[Model Unification](../concepts/model-unification.md)** - How models are aggregated across endpoints
- **[Health Checking](../concepts/health-checking.md)** - Automatic endpoint monitoring
- **[Profile System](../concepts/profile-system.md)** - Customise backend behaviour

### Configuration

- **[Configuration Overview](../configuration/overview.md)** - Complete configuration guide
- **[Proxy Profiles](../concepts/proxy-profiles.md)** - Auto, streaming, and standard profiles
- **[Best Practices](../configuration/practices/overview.md)** - Production recommendations

### Next Steps

- [Backend Integrations](../integrations/overview.md) - Connect Ollama, LM Studio, vLLM
- [Architecture Overview](../development/architecture.md) - Deep dive into Olla's design
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