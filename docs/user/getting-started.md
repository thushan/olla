# Getting Started with Olla

Olla is a high-performance proxy and load balancer designed specifically for LLM infrastructure. It intelligently routes requests across multiple inference endpoints—whether they're local Ollama instances, LM Studio servers, or cloud-based OpenAI-compatible APIs.

## Why Olla?

If you're running LLMs in production, you've likely encountered these challenges:

- **Single Point of Failure**: One Ollama instance goes down, everything stops
- **Inefficient Resource Usage**: Some GPUs sit idle while others are overloaded
- **Platform Lock-in**: Can't easily mix Ollama, LM Studio, and cloud endpoints
- **No Fallback Options**: When your primary endpoint fails, requests just fail

Olla solves these problems by providing intelligent routing, automatic failover, and unified access to heterogeneous LLM infrastructure.

## Quick Start

### Installation

#### Download Binary

Download the latest release for your platform:

```bash
# Linux/macOS
curl -L https://github.com/sammcj/olla/releases/latest/download/olla-$(uname -s)-$(uname -m) -o olla
chmod +x olla

# Windows
# Download from https://github.com/sammcj/olla/releases
```

#### Build from Source

```bash
git clone https://github.com/sammcj/olla.git
cd olla
make build
```

### Basic Configuration

Create a `config.yaml` file:

```yaml
server:
  host: 0.0.0.0
  port: 8080

proxy:
  engine: "sherpa"  # Start with the simple engine
  load_balancer: "round_robin"

discovery:
  endpoints:
    - name: "local-ollama"
      url: "http://localhost:11434"
      platform: "ollama"
      priority: 1
```

### Start Olla

```bash
./olla

# Or with a custom config file
./olla -c /path/to/config.yaml

# Enable debug logging
./olla -d
```

### Test Your Setup

```bash
# Check health
curl http://localhost:8080/internal/health

# List available models
curl http://localhost:8080/api/tags

# Make an inference request
curl http://localhost:8080/api/generate \
  -d '{
    "model": "llama3.2",
    "prompt": "Why is the sky blue?"
  }'
```

## Understanding Olla's Components

### Proxy Engines

Olla offers two proxy engines:

**Sherpa** (Recommended for getting started)
- Simple and maintainable
- Perfect for development and small deployments
- Lower memory usage
- Easier to debug

**Olla** (For production at scale)
- High-performance with connection pooling
- Circuit breakers for failure isolation
- Optimised for streaming LLM responses
- Better for 10+ endpoints or high traffic

### Load Balancing Strategies

**Round Robin**
- Cycles through endpoints in order
- Good for identical endpoints
- Predictable behaviour

**Priority** (Recommended)
- Routes to highest priority endpoints first
- Falls back to lower priorities when needed
- Perfect for mixing local and cloud endpoints

**Least Connections**
- Routes to the endpoint with fewest active connections
- Prevents overloading busy endpoints
- Best for heterogeneous hardware

## Common Use Cases

### Local Development Setup

Spread load across multiple local Ollama instances:

```yaml
discovery:
  endpoints:
    - name: "ollama-1"
      url: "http://localhost:11434"
      platform: "ollama"
      
    - name: "ollama-2"
      url: "http://localhost:11435"
      platform: "ollama"
      
    - name: "lm-studio"
      url: "http://localhost:1234"
      platform: "lmstudio"

proxy:
  load_balancer: "round_robin"
```

### Production with Failover

Use local GPUs with cloud backup:

```yaml
discovery:
  endpoints:
    # Primary: Local GPU cluster
    - name: "gpu-cluster-1"
      url: "http://10.0.1.10:11434"
      platform: "ollama"
      priority: 1
      
    - name: "gpu-cluster-2"
      url: "http://10.0.1.11:11434"
      platform: "ollama"
      priority: 1
      
    # Fallback: Cloud API
    - name: "openai-backup"
      url: "https://api.openai.com"
      platform: "openai"
      priority: 10
      headers:
        Authorization: "Bearer ${OPENAI_API_KEY}"

proxy:
  engine: "olla"  # High-performance engine
  load_balancer: "priority"
```

### Multi-Platform Setup

Mix different LLM platforms seamlessly:

```yaml
discovery:
  endpoints:
    - name: "ollama-llama"
      url: "http://localhost:11434"
      platform: "ollama"
      tags:
        models: "llama,mistral"
        
    - name: "lmstudio-code"
      url: "http://localhost:1234"
      platform: "lmstudio"
      tags:
        models: "codellama,starcoder"
        
    - name: "groq-fast"
      url: "https://api.groq.com/openai/v1"
      platform: "openai"
      headers:
        Authorization: "Bearer ${GROQ_API_KEY}"
      tags:
        models: "mixtral,llama3-70b"
```

## Working with Models

### Model Unification

Olla automatically unifies models across platforms. For example:
- Ollama: `llama3.2:3b-instruct-q4_K_M`
- LM Studio: `Meta-Llama-3.2-3B-Instruct-Q4_K_M.gguf`
- OpenAI: `meta-llama-3.2-3b-instruct`

All resolve to the same model, and Olla routes to any available endpoint.

### Model Aliases

Configure friendly names for models:

```yaml
model_aliases:
  "llama3": "llama3.2:8b-instruct-q4_K_M"
  "code": "codellama:13b-code-q4_K_M"
  "vision": "llava:13b-v1.6"
```

### Checking Available Models

```bash
# List all models across all endpoints
curl http://localhost:8080/internal/status/models

# Response shows unified view:
{
  "models": [
    {
      "name": "llama3.2:8b-instruct-q4_K_M",
      "endpoints": ["ollama-1", "ollama-2", "lmstudio"],
      "state": "loaded",
      "size": "4.7GB"
    }
  ]
}
```

## Monitoring Your Deployment

### Health Checks

Olla continuously monitors endpoint health:

```bash
# Check overall health
curl http://localhost:8080/internal/health

# Get detailed endpoint status
curl http://localhost:8080/internal/status/endpoints
```

### Metrics

Access Prometheus metrics:

```bash
curl http://localhost:8080/metrics

# Key metrics to watch:
# - olla_requests_total
# - olla_request_duration_seconds
# - olla_endpoint_health
# - olla_active_connections
```

### Logging

Control log verbosity:

```yaml
logging:
  level: "info"  # debug, info, warn, error
  format: "json" # json or text
```

## Performance Tuning

### For High Throughput

```yaml
proxy:
  engine: "olla"
  max_idle_conns: 200
  max_conns_per_host: 100
  
server:
  read_timeout: 10m    # LLMs can be slow
  write_timeout: 10m
  max_request_size: 10485760  # 10MB
```

### For Low Latency

```yaml
proxy:
  load_balancer: "least_connections"
  
health:
  interval: 10s  # Faster health checks
  timeout: 2s
  
circuit_breaker:
  failure_threshold: 3  # Fail fast
  recovery_timeout: 15s
```

### For Large Models

```yaml
# Increase timeouts for models that take time to load
platforms:
  ollama:
    timeout: 5m  # Allow 5 minutes for model loading
    
discovery:
  endpoints:
    - name: "large-model-server"
      url: "http://gpu-server:11434"
      health_check:
        timeout: 30s  # Longer health check timeout
```

## Security Considerations

### API Key Management

Use environment variables for sensitive data:

```yaml
discovery:
  endpoints:
    - name: "openai"
      url: "https://api.openai.com"
      headers:
        Authorization: "Bearer ${OPENAI_API_KEY}"
```

```bash
# Set environment variable
export OPENAI_API_KEY="sk-..."
./olla
```

### Rate Limiting

Protect your infrastructure:

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_minute: 100
    burst: 20
    
  ip_whitelist:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
```

### TLS/HTTPS

For production, run Olla behind a reverse proxy with TLS:

```nginx
server {
    listen 443 ssl;
    server_name ollama.company.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
    }
}
```

## Integration Examples

### Python Client

```python
import requests

# Olla works with any Ollama-compatible client
response = requests.post('http://localhost:8080/api/generate', json={
    'model': 'llama3.2',
    'prompt': 'Explain quantum computing',
    'stream': False
})

print(response.json()['response'])
```

### Streaming with Python

```python
import requests
import json

response = requests.post('http://localhost:8080/api/generate', 
    json={
        'model': 'llama3.2',
        'prompt': 'Write a story about a robot',
        'stream': True
    },
    stream=True
)

for line in response.iter_lines():
    if line:
        data = json.loads(line)
        print(data['response'], end='', flush=True)
```

### Using with LangChain

```python
from langchain_ollama import Ollama

# Point to Olla instead of Ollama directly
llm = Ollama(
    base_url="http://localhost:8080",
    model="llama3.2"
)

response = llm.invoke("What is the meaning of life?")
```

## Next Steps

1. **Explore Advanced Configuration**: See the [Configuration Reference](configuration.md) for all options
2. **Deploy to Production**: Check the [Deployment Guide](deployment.md) for production best practices
3. **Monitor Performance**: Learn about [Monitoring and Observability](monitoring.md)
4. **Troubleshoot Issues**: Refer to the [Troubleshooting Guide](troubleshooting.md)

## Getting Help

- **Issues**: Report bugs at https://github.com/sammcj/olla/issues
- **Discussions**: Join our community discussions
- **Documentation**: Full docs at https://olla.sammcj.com

Remember: Start simple with Sherpa engine and round-robin balancing, then optimise based on your actual usage patterns. Olla is designed to grow with your needs.