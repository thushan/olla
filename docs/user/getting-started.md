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

### Quick Install Script

```bash
# Download and install automatically
bash <(curl -s https://raw.githubusercontent.com/thushan/olla/main/install.sh)
```

### Manual Installation

#### Download Binary

```bash
# Download latest release
VERSION="v0.1.0"
ARCH="amd64" # or arm64
OS="linux" # or darwin, windows
curl -L "https://github.com/thushan/olla/releases/download/${VERSION}/olla-${OS}-${ARCH}" -o olla
chmod +x olla

# Verify installation
./olla --version
```

#### Build from Source

```bash
# Clone repository
git clone https://github.com/thushan/olla.git
cd olla

# Build standard version
make build

# Build optimised release version
make build-release

# Install to system (optional)
sudo make install
```

#### Container Installation

```bash
# Use pre-built container
docker run -t \
  --name olla \
  -p 40114:40114 \
  ghcr.io/thushan/olla:latest

# Or build from source
docker build -t olla:latest .
docker run -d \
  --name olla \
  -p 40114:40114 \
  -v $(pwd)/config.yaml:/config.yaml \
  olla:latest
```

#### Go Install

```bash
go install github.com/thushan/olla@latest
```

### System Requirements

| Deployment | CPU | RAM | Notes |
|-----------|-----|-----|-------|
| Development | 2 cores | 2GB | Light traffic |
| Small Production | 4 cores | 8GB | Moderate traffic |
| Large Production | 8+ cores | 16GB+ | High traffic |

**Operating Systems:** Linux, macOS, Windows  
**Go Version:** 1.24+ (for building from source)

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

## Deployment Options

### Development Deployment

For local development, run Olla directly:

```bash
# Run with default config
./olla

# Run with custom config
./olla -c my-config.yaml

# Run with environment overrides
OLLA_SERVER_PORT=8080 OLLA_LOG_LEVEL=debug ./olla
```

### Production Deployment

#### Running as a Service

**Linux (systemd):**

```bash
# Create user and directories
sudo useradd -r -s /bin/false olla
sudo mkdir -p /opt/olla
sudo cp olla config.yaml /opt/olla/
sudo chown -R olla:olla /opt/olla

# Create service file
sudo tee /etc/systemd/system/olla.service > /dev/null <<EOF
[Unit]
Description=Olla LLM Proxy
After=network.target
Documentation=https://github.com/thushan/olla

[Service]
Type=simple
User=olla
Group=olla
WorkingDirectory=/opt/olla
ExecStart=/opt/olla/olla
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable olla
sudo systemctl start olla
```

**Docker Deployment:**

```bash
# Production container with custom config
docker run -d \
  --name olla-prod \
  --restart unless-stopped \
  -p 40114:40114 \
  -v /etc/olla/config.yaml:/config.yaml \
  -e OLLA_CONFIG_FILE=/config.yaml \
  -e OLLA_LOG_LEVEL=info \
  -e OLLA_LOG_FORMAT=json \
  ghcr.io/thushan/olla:latest

# Or use Docker Compose
tee docker-compose.yml > /dev/null <<EOF
version: '3.8'
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    container_name: olla
    restart: unless-stopped
    ports:
      - "40114:40114"
    volumes:
      - ./config.yaml:/config.yaml
    environment:
      - OLLA_CONFIG_FILE=/config.yaml
      - OLLA_LOG_LEVEL=info
      - OLLA_LOG_FORMAT=json
EOF

docker-compose up -d
```

#### Reverse Proxy Setup

Deploy behind a reverse proxy for TLS and load balancing:

**Nginx:**

```nginx
upstream olla_backend {
    server 127.0.0.1:40114;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://olla_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        
        # LLM-optimised timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
        proxy_buffering off;
    }
}
```

**Caddy:**

```
api.example.com {
    reverse_proxy localhost:40114 {
        health_uri /internal/health
        health_interval 10s
        flush_interval -1
    }
}
```

#### High Availability

Run multiple Olla instances for redundancy:

```bash
# Multiple instances on different ports
OLLA_SERVER_PORT=40114 ./olla &
OLLA_SERVER_PORT=40115 ./olla &
OLLA_SERVER_PORT=40116 ./olla &

# Configure load balancer to distribute across instances
```

### Environment Configuration

Use environment variables for deployment-specific settings:

```bash
# Server settings
export OLLA_SERVER_HOST=0.0.0.0
export OLLA_SERVER_PORT=40114
export OLLA_CONFIG_FILE=/etc/olla/config.yaml

# Logging
export OLLA_LOG_LEVEL=info
export OLLA_LOG_FORMAT=json

# Proxy settings  
export OLLA_PROXY_ENGINE=olla
export OLLA_PROXY_LOAD_BALANCER=priority

# Rate limiting
export OLLA_SERVER_GLOBAL_RATE_LIMIT=10000
export OLLA_SERVER_PER_IP_RATE_LIMIT=1000
```

## Security Considerations

### Network Security

```yaml
server:
  # Bind to localhost only if behind reverse proxy
  host: "127.0.0.1"
  
  # Configure rate limiting
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    burst_size: 50
    trust_proxy_headers: true  # When behind reverse proxy
  
  # Request size limits
  request_limits:
    max_body_size: 104857600  # 100MB
    max_header_size: 1048576  # 1MB
```

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

### TLS/SSL

Olla doesn't handle TLS directly - use a reverse proxy:
- nginx, Caddy, HAProxy for self-managed TLS
- Cloud load balancers (AWS ALB, GCP LB) for managed TLS
- Service mesh (Istio, Linkerd) for zero-config TLS

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
2. **Deploy to Production**: See the deployment sections above for production best practices
3. **Monitor Performance**: Learn about [Monitoring and Observability](monitoring.md)
4. **Troubleshoot Issues**: Refer to the [Troubleshooting Guide](troubleshooting.md)

## Getting Help

- **Issues**: Report bugs at https://github.com/sammcj/olla/issues
- **Discussions**: Join our community discussions
- **Documentation**: Full docs at https://olla.sammcj.com

Remember: Start simple with Sherpa engine and round-robin balancing, then optimise based on your actual usage patterns. Olla is designed to grow with your needs.