# OpenWebUI + Olla Integration Example

This example demonstrates how to set up OpenWebUI with Olla as a proxy/load balancer for multiple Ollama instances.

## Architecture

```
┌─────────────┐    ┌──────────┐    ┌─────────────────┐
│  OpenWebUI  │───▶│   Olla   │───▶│ Ollama Instance │
│ (Port 3000) │    │(Port     │    │  (External)     │
│             │    │ 40114)   │    │                 │
└─────────────┘    └──────────┘    └─────────────────┘
                           │
                           ├──────▶┌─────────────────┐
                           │       │ Ollama Instance │
                           │       │  (External)     │
                           │       └─────────────────┘
                           │
                           └──────▶┌─────────────────┐
                                   │ Ollama Instance │
                                   │  (External)     │
                                   └─────────────────┘
```

## Quick Start

> [!NOTE]  
> Olla runs in the container under `/app` so you will have to override within that folder.
>
> Eg. the `config.yaml` is in `/app/config.yaml`. Logs are available in `/app/logs/`.

1. **Edit `olla.yaml`** - Add your Ollama server URLs:
   ```yaml
   discovery:
     type: "static"
     static:
       endpoints:
         - url: "http://192.168.1.100:11434"  # Your Ollama server
           name: "my-ollama"
           type: "ollama"
           priority: 100
           model_url: "/api/tags"
           health_check_url: "/"
           check_interval: 2s
           check_timeout: 1s
   ```

2. **Start the stack**:
   ```bash
   docker compose up -d
   ```

3. **Access OpenWebUI** at http://localhost:3000

**Files needed:**
- `compose.yaml` (provided)
- `olla.yaml` (edit with your Ollama servers)

## How It Works

This Docker Compose stack runs:
- **Olla** - Proxy that load balances across your Ollama servers
- **OpenWebUI** - Web interface that connects to Olla instead of directly to Ollama

The `olla.yaml` file gets mounted to `/app/config.yaml` inside the Olla container.

### Adding More Ollama Servers

Edit `olla.yaml` to add more endpoints:
```yaml
discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://server1:11434"
        name: "server1"
        type: "ollama"
        priority: 100
      - url: "http://server2:11434"
        name: "server2"
        type: "ollama"
        priority: 50
```

## Adding Ollama Instances

To add more Ollama instances, edit the `olla.yaml` file:

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      # High-priority local instance
      - url: "http://host.docker.internal:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
      
      # Medium-priority remote instance
      - url: "http://gpu-server.local:11434"
        name: "gpu-server"
        type: "ollama"
        priority: 75
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
      
      # Low-priority backup instance
      - url: "http://backup.local:11434"
        name: "backup-ollama"
        type: "ollama"
        priority: 25
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

## Load Balancing Strategies

You can change the load balancing strategy in `olla.yaml`:

```yaml
proxy:
  load_balancer: "priority"  # Options: priority, round-robin, least-connections
```

- **Priority**: Routes to highest priority healthy endpoint (recommended for mixed hardware)
- **Round-robin**: Even distribution across all healthy endpoints
- **Least-connections**: Routes to endpoint with fewest active requests

## Monitoring

### Check Olla Health
```bash
curl http://localhost:40114/internal/health
```

### Check Endpoint Status
```bash
curl http://localhost:40114/internal/status/endpoints
```

### Check Available Models
```bash
curl http://localhost:40114/internal/status/models
```

### View Unified Models (What OpenWebUI Sees)
```bash
curl http://localhost:40114/olla/models
```

## Troubleshooting

### OpenWebUI Can't Connect to Models

1. **Check Olla health**:
   ```bash
   docker logs olla
   curl http://localhost:40114/internal/health
   ```

2. **Verify endpoint configuration**:
   ```bash
   curl http://localhost:40114/internal/status/endpoints
   ```

3. **Check if models are discovered**:
   ```bash
   curl http://localhost:40114/olla/ollama/api/tags
   ```

### Ollama Endpoints Not Healthy

1. **Verify Ollama instances are accessible**:
   ```bash
   # Test direct connection to your Ollama instance
   curl http://your-ollama-host:11434/
   ```

2. **Check Docker networking**:
   - Use `host.docker.internal` for Ollama running on Docker host
   - Use actual IP addresses for remote instances
   - Ensure firewall allows connections

3. **Review Olla logs**:
   ```bash
   docker logs olla -f
   ```

### Performance Issues

1. **Switch to high-performance engine**:
   ```yaml
   proxy:
     engine: "olla"  # Instead of "sherpa"
   ```

2. **Adjust timeouts**:
   ```yaml
   proxy:
     connection_timeout: 60s
     response_timeout: 1200s  # 20 minutes for very long responses
   ```

## Environment Variables

You can override configuration using environment variables:

```yaml
services:
  olla:
    environment:
      - OLLA_SERVER_HOST=0.0.0.0
      - OLLA_SERVER_PORT=40114
      - OLLA_PROXY_ENGINE=sherpa
      - OLLA_LOGGING_LEVEL=info
```

## Advanced Configuration

### Custom OpenWebUI Settings

```yaml
services:
  openwebui:
    environment:
      - OLLAMA_BASE_URL=http://olla:40114/olla/ollama
      - WEBUI_NAME=My Olla Setup
      - WEBUI_URL=http://localhost:3000
      - WEBUI_SECRET_KEY=your-secret-key-here
      - DEFAULT_MODELS=llama3.2:latest,mistral:latest
      - DEFAULT_USER_ROLE=user
```

### GPU Support for Local Ollama

If you want to also run a local Ollama instance with GPU support:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    container_name: ollama
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
      - ollama
```

Then update `olla.yaml` to use `http://ollama:11434` instead of `host.docker.internal`.

## Support

For issues with:
- **Olla**: Check the [Olla GitHub repository](https://github.com/thushan/olla)
- **OpenWebUI**: Check the [OpenWebUI GitHub repository](https://github.com/open-webui/open-webui)
- **Ollama**: Check the [Ollama GitHub repository](https://github.com/ollama/ollama)