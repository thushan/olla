---
title: OpenWebUI Integration with Olla - LLM Web Interface
description: Connect OpenWebUI to multiple Ollama instances through Olla proxy. Load balancing, failover, and unified model management for your LLM web interface.
keywords: OpenWebUI, Olla, Ollama, LLM interface, web UI, load balancing, model management
---

# OpenWebUI Integration

OpenWebUI is a powerful web interface for interacting with LLMs. Olla acts as a proxy between OpenWebUI and your Ollama backends, providing load balancing, failover and unified model management across multiple Ollama instances.

You can find an example integration of OpenWebUI with Olla and Ollama instances in <code>examples/ollama-openwebui</code> - see [latest in Github](https://github.com/thushan/olla/tree/main/examples/ollama-openwebui).

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://github.com/open-webui/open-webui">github.com/open-webui/open-webui</a></td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Frontend UI</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>Ollama API Compatibility</td>
    </tr>
    <tr>
        <th>
          Features Supported <br/>
          <small>(via Olla)</small>
        </th>
        <td>
            <ul>
                <li>Chat Interface</li>
                <li>Model Selection</li>
                <li>Streaming Responses</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Configuration</th>
        <td>
            Set <code>OLLAMA_BASE_URL</code> to Olla endpoint <br/>
            ```
            export OLLAMA_BASE_URL="http://localhost:40114/olla/ollama"  
            ```
        </td>
    </tr>
    <tr>
        <th>Example</th>
        <td>
            You can find an example of integration in <code>examples/ollama-openwebui</code>
        </td>
    </tr>
</table>

## Architecture

```
┌─────────────┐    ┌──────────┐    ┌─────────────────┐
│  OpenWebUI │───▶│   Olla   │───▶│ Ollama Instance │
│ (Port 3000)│    │(Port     │    │  (Primary)       │
│            │    │ 40114)   │    │                  │
└─────────────┘    └──────────┘    └─────────────────┘
                          │
                          ├──────▶┌──────────────────┐
                          │       │ Ollama Instance 2│
                          │       │  (Fallback)      │
                          │       └──────────────────┘
                          │
                          └──────▶┌─────────────────┐
                                  │ Ollama Instance 3│
                                  │  (GPU)         │
                                  └─────────────────┘
```

## Quick Start

### Docker Compose Setup

1. **Create `compose.yaml`**:

```yaml
services:
  # Olla proxy/load balancer
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
      timeout: 5s
      interval: 30s
      retries: 3
      start_period: 10s

  # OpenWebUI interface
  openwebui:
    image: ghcr.io/open-webui/open-webui:main
    container_name: openwebui
    restart: unless-stopped
    ports:
      - "3000:8080"
    volumes:
      - openwebui_data:/app/backend/data
    environment:
      # Point to Olla instead of direct Ollama
      - OLLAMA_BASE_URL=http://olla:40114/olla/ollama
      - WEBUI_NAME=Olla + OpenWebUI
      - WEBUI_URL=http://localhost:3000
    depends_on:
      olla:
        condition: service_healthy

volumes:
  openwebui_data:
    driver: local
```

2. **Create `olla.yaml`** configuration - copy the existing `olla.yaml`, below is just for brevity:

```yaml
server:
  host: "0.0.0.0"
  port: 40114

proxy:
  engine: "sherpa"
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://192.168.1.100:11434"
        name: "main-ollama"
        type: "ollama"
        priority: 100
      
      - url: "http://192.168.1.101:11434"
        name: "backup-ollama"
        type: "ollama"
        priority: 50
```

3. **Start the stack**:

```bash
docker compose up -d
```

4. **Access OpenWebUI** at http://localhost:3000

## Configuration Options

### Basic Configuration

The minimal configuration requires setting the Ollama base URL:

```yaml
environment:
  - OLLAMA_BASE_URL=http://olla:40114/olla/ollama
```

### Advanced Configuration

```yaml
environment:
  # Olla connection
  - OLLAMA_BASE_URL=http://olla:40114/olla/ollama
  
  # OpenWebUI settings
  - WEBUI_NAME=My AI Assistant
  - WEBUI_URL=http://localhost:3000
  - WEBUI_SECRET_KEY=change-this-secret-key
  
  # Default models
  - DEFAULT_MODELS=llama3.2:latest,mistral:latest
  
  # User management
  - DEFAULT_USER_ROLE=user
  - ENABLE_SIGNUP=true
  
  # Features
  - ENABLE_RAG_WEB_SEARCH=true
  - RAG_WEB_SEARCH_ENGINE=duckduckgo
```

See Ollama documentation for more details.

## Using Multiple Backends

Olla enables OpenWebUI to use multiple backend types simultaneously:

### Mixed Backend Configuration

```yaml
discovery:
  static:
    endpoints:
      # Primary Ollama instance
      - url: "http://gpu-server:11434"
        name: "ollama-gpu"
        type: "ollama"
        priority: 100
      
      # LM Studio for specific models
      - url: "http://workstation:1234"
        name: "lmstudio"
        type: "lm-studio"
        priority: 80
      
      # vLLM for high throughput
      - url: "http://vllm-server:8000"
        name: "vllm"
        type: "vllm"
        priority: 60
```

### Model Unification

OpenWebUI sees a unified model list across all backends:

```bash
# Check unified models
curl http://localhost:40114/olla/ollama/api/tags

# Response includes models from all Ollama-type endpoints
{
  "models": [
    {"name": "llama3.2:latest", "size": 2023547950, ...},
    {"name": "mistral:latest", "size": 4113487360, ...},
    {"name": "codellama:13b", "size": 7365960704, ...}
  ]
}
```

## Standalone Setup

### Without Docker

1. **Start Olla**:

```bash
olla --config olla.yaml
```

2. **Start OpenWebUI**:

```bash
docker run -d \
  --name openwebui \
  -p 3000:8080 \
  -v openwebui_data:/app/backend/data \
  -e OLLAMA_BASE_URL=http://host.docker.internal:40114/olla/ollama \
  ghcr.io/open-webui/open-webui:main
```

### With Existing OpenWebUI

Update your existing OpenWebUI configuration:

```bash
# Stop OpenWebUI
docker stop openwebui

# Update environment
docker run -d \
  --name openwebui \
  -p 3000:8080 \
  -v openwebui_data:/app/backend/data \
  -e OLLAMA_BASE_URL=http://your-olla-host:40114/olla/ollama \
  ghcr.io/open-webui/open-webui:main
```

## Monitoring

### Check Health

```bash
# Olla health
curl http://localhost:40114/internal/health

# Endpoint status
curl http://localhost:40114/internal/status/endpoints

# Available models
curl http://localhost:40114/olla/ollama/api/tags
```

### View Logs

```bash
# Olla logs
docker logs olla -f

# OpenWebUI logs
docker logs openwebui -f
```

### Monitor Performance

Check response headers for routing information:

```bash
curl -I http://localhost:40114/olla/ollama/api/tags

# Headers show:
# X-Olla-Endpoint: main-ollama
# X-Olla-Backend-Type: ollama
# X-Olla-Response-Time: 45ms
```

## Troubleshooting

### Models Not Appearing

**Issue**: OpenWebUI doesn't show any models

**Solution**:

1. Verify Olla is healthy:
   ```bash
   curl http://localhost:40114/internal/health
   ```

2. Check endpoints are discovered:
   ```bash
   curl http://localhost:40114/internal/status/endpoints
   ```

3. Verify models are available:
   ```bash
   curl http://localhost:40114/olla/ollama/api/tags
   ```

4. Check OpenWebUI logs:
   ```bash
   docker logs openwebui | grep -i error
   ```

### Connection Refused

**Issue**: OpenWebUI can't connect to Olla

**Solution**:

1. Verify network connectivity:
   ```bash
   docker exec openwebui ping olla
   ```

2. Check Olla is listening:
   ```bash
   netstat -an | grep 40114
   ```

3. Verify environment variable:
   ```bash
   docker exec openwebui env | grep OLLAMA_BASE_URL
   ```

### Slow Response Times

**Issue**: Chat responses are slow

**Solution**:

1. Ensure that Proxy Profile is set correctly:
   ```yaml
   proxy:
    profile: "auto" # or "streaming"
   ```
2. Switch to high-performance engine:
   ```yaml
   proxy:
     engine: "olla"  # Instead of "sherpa"
   ```

3. Use appropriate load balancer:
   ```yaml
   proxy:
     load_balancer: "least-connections"
   ```

4. Increase timeouts:
   ```yaml
   proxy:
     response_timeout: 1200s  # 20 minutes
   ```

### Docker Networking Issues

**Issue**: Containers can't communicate

**Solution**:

For Ollama on Docker host:
```yaml
endpoints:
  - url: "http://host.docker.internal:11434"  # macOS/Windows
  - url: "http://172.17.0.1:11434"            # Linux
```

For remote instances:
```yaml
endpoints:
  - url: "http://192.168.1.100:11434"  # Use actual IP
```

## Advanced Features

### GPU Support

Add GPU-enabled Ollama to the stack:

```yaml
services:
  ollama-gpu:
    image: ollama/ollama:latest
    container_name: ollama-gpu
    restart: unless-stopped
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]

  olla:
    # ... existing config
    depends_on:
      - ollama-gpu
```

Update `olla.yaml`:
```yaml
endpoints:
  - url: "http://ollama-gpu:11434"
    name: "local-gpu"
    type: "ollama"
    priority: 100
```

### Authentication

!!! warning "Authentication Not Supported"
    Olla does not currently support authentication headers for endpoints. If your API requires authentication, you'll need to:
    
    - Use a reverse proxy that adds authentication
    - Wait for this feature to be implemented
    - Access only public/local endpoints

### Custom Networks

Create isolated networks:

```yaml
networks:
  olla-net:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16

services:
  olla:
    networks:
      - olla-net
  
  openwebui:
    networks:
      - olla-net
```

## Best Practices

### 1. Use Priority Load Balancing

Configure priorities based on cost and performance:

```yaml
endpoints:
  # Free/local first
  - url: "http://localhost:11434"
    priority: 100
  
  # Backup/cloud
  - url: "https://api.provider.com"
    priority: 10
```

### 2. Monitor Health

Set up health check alerts:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://ollama:11434"
        check_interval: 10s
        check_timeout: 2s
```

### 3. Configure Appropriate Timeouts

For large models:

```yaml
proxy:
  response_timeout: 1800s  # 30 minutes
  read_timeout: 600s       # 10 minutes
```

### 4. Use Volumes for Persistence

```yaml
volumes:
  - ./olla-config:/app/config:ro
  - ./olla-logs:/app/logs
  - openwebui_data:/app/backend/data
```

## Integration with Other Tools

### Nginx Reverse Proxy

```nginx
server {
    listen 80;
    server_name ai.example.com;

    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
    }

    location /olla/ {
        proxy_pass http://localhost:40114;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }
}
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: olla
spec:
  replicas: 1
  selector:
    matchLabels:
      app: olla
  template:
    metadata:
      labels:
        app: olla
    spec:
      containers:
      - name: olla
        image: ghcr.io/thushan/olla:latest
        ports:
        - containerPort: 40114
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: olla.yaml
      volumes:
      - name: config
        configMap:
          name: olla-config
---
apiVersion: v1
kind: Service
metadata:
  name: olla
spec:
  selector:
    app: olla
  ports:
  - port: 40114
    targetPort: 40114
```

## Example Repository

A complete example is available at:
[github.com/thushan/olla/examples/ollama-openwebui](https://github.com/thushan/olla/tree/main/examples/ollama-openwebui)

## Next Steps

- [Configuration Reference](../../configuration/reference.md) - Complete Olla configuration
- [Load Balancing](../../concepts/load-balancing.md) - Configure load balancing strategies
- [Model Unification](../../concepts/model-unification.md) - Understand model management