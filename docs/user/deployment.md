# Deployment Guide

This guide covers deploying Olla in various environments. Olla is designed as a stateless proxy that can be deployed behind load balancers and reverse proxies for production use.

## System Requirements

### Minimum Requirements

- **CPU**: 2 cores
- **RAM**: 2GB
- **Disk**: 100MB for application
- **OS**: Linux, macOS, Windows
- **Go**: 1.24+ (for building from source)

### Recommended Production Requirements

- **CPU**: 4-8 cores
- **RAM**: 8-16GB
- **Network**: High bandwidth for LLM traffic
- **OS**: Linux (Ubuntu 22.04 LTS, RHEL 8+)

### Sizing Guidelines

| Workload | CPU | RAM | Notes |
|----------|-----|-----|-------|
| Development | 2 cores | 2GB | Light traffic |
| Small Production | 4 cores | 8GB | Moderate traffic |
| Large Production | 8+ cores | 16GB+ | High traffic |

## Installation Methods

### Binary Installation

Use the provided install script:

```bash
# Download and run install script
curl -fsSL https://raw.githubusercontent.com/thushan/olla/main/install.sh | bash

# Or download specific version
VERSION="v0.1.0"
ARCH="amd64" # or arm64
OS="linux" # or darwin, windows
curl -L "https://github.com/thushan/olla/releases/download/${VERSION}/olla-${OS}-${ARCH}" -o olla
chmod +x olla

# Verify installation
./olla --version
```

### Building from Source

```bash
# Clone repository
git clone https://github.com/thushan/olla.git
cd olla

# Build
make build

# Build optimised release version
make build-release

# Install to system
sudo make install
```

### Docker Deployment

Olla includes a Dockerfile for containerised deployments:

```bash
# Build Docker image
docker build -t olla:latest .

# Run with custom config
docker run -d \
  --name olla \
  -p 40114:40114 \
  -v $(pwd)/config.yaml:/config.yaml \
  -e OLLA_CONFIG_FILE=/config.yaml \
  olla:latest

# Run with Docker host networking (to access local services)
docker run -d \
  --name olla \
  --network host \
  -v $(pwd)/config-docker.yaml:/config.yaml \
  olla:latest
```

### Docker Configuration

For Docker deployments, use `host.docker.internal` to access services on the host:

```yaml
# config-docker.yaml
discovery:
  static:
    endpoints:
      - name: "host-ollama"
        url: "http://host.docker.internal:11434"
        type: "ollama"
        priority: 100
```

## Basic Deployment

### Single Server Setup

The simplest deployment runs Olla on a single server:

```bash
# Create configuration
cat > config.yaml <<EOF
server:
  host: "0.0.0.0"
  port: 40114

proxy:
  engine: "sherpa"  # or "olla" for high-performance
  load_balancer: "priority"

discovery:
  static:
    endpoints:
      - name: "local-ollama"
        url: "http://localhost:11434"
        type: "ollama"
        priority: 100
      - name: "gpu-server"
        url: "http://10.0.1.10:11434"
        type: "ollama"
        priority: 50
EOF

# Run Olla
./olla
```

### Running as a Service

#### Linux (systemd)

Create a systemd service file:

```ini
# /etc/systemd/system/olla.service
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

# Security
NoNewPrivileges=true
PrivateTmp=true

# Resource limits
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Install and start:

```bash
# Create user
sudo useradd -r -s /bin/false olla

# Install binary
sudo mkdir -p /opt/olla
sudo cp olla /opt/olla/
sudo cp config.yaml /opt/olla/
sudo chown -R olla:olla /opt/olla

# Install service
sudo cp olla.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable olla
sudo systemctl start olla

# Check status
sudo systemctl status olla
sudo journalctl -u olla -f
```

#### macOS (launchd)

Create a launch daemon:

```xml
<!-- ~/Library/LaunchAgents/com.olla.proxy.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.olla.proxy</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/olla</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/olla.log</string>
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/olla.error.log</string>
</dict>
</plist>
```

Load and start:

```bash
launchctl load ~/Library/LaunchAgents/com.olla.proxy.plist
launchctl start com.olla.proxy
```

## Production Deployment

### Reverse Proxy Setup

For production, deploy Olla behind a reverse proxy for TLS termination and load balancing.

#### Nginx Configuration

```nginx
upstream olla_backend {
    server 127.0.0.1:40114;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;
    
    # TLS configuration (add your certificates)
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://olla_backend;
        proxy_http_version 1.1;
        
        # Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for LLM responses
        proxy_connect_timeout 60s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
        
        # Disable buffering for streaming
        proxy_buffering off;
        
        # Connection reuse
        proxy_set_header Connection "";
    }
    
    # Health check endpoint
    location /internal/health {
        proxy_pass http://olla_backend;
        access_log off;
    }
}
```

#### Caddy Configuration

```
api.example.com {
    reverse_proxy localhost:40114 {
        # Health check
        health_uri /internal/health
        health_interval 10s
        
        # Timeouts for LLM
        transport http {
            dial_timeout 60s
            response_header_timeout 600s
        }
        
        # Streaming
        flush_interval -1
    }
}
```

### Multiple Instance Deployment

For high availability, run multiple Olla instances:

```bash
# Instance 1
OLLA_SERVER_PORT=40114 ./olla &

# Instance 2
OLLA_SERVER_PORT=40115 ./olla &

# Instance 3
OLLA_SERVER_PORT=40116 ./olla &
```

Configure your load balancer to distribute traffic across instances.

### Environment Configuration

Use environment variables for deployment-specific settings:

```bash
# Server configuration
export OLLA_SERVER_HOST=0.0.0.0
export OLLA_SERVER_PORT=8080
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

## Monitoring

### Health Checks

Olla provides several endpoints for monitoring:

```bash
# Basic health check
curl http://localhost:40114/internal/health

# Detailed status
curl http://localhost:40114/internal/status

# Endpoint status
curl http://localhost:40114/internal/status/endpoints

# Process metrics
curl http://localhost:40114/internal/process
```

### Logging

Configure structured logging for production:

```yaml
logging:
  level: "info"
  format: "json"  # Use JSON for log aggregation
```

### Performance Profiling

Enable pprof for performance debugging:

```bash
# Enable profiling
OLLA_ENABLE_PPROF=true ./olla

# Access profiling endpoints
go tool pprof http://localhost:40114/debug/pprof/heap
go tool pprof http://localhost:40114/debug/pprof/profile
```

## Security Considerations

### Network Security

1. **Firewall Rules**: Restrict access to Olla's port

```bash
# Only allow from trusted networks
iptables -A INPUT -p tcp --dport 40114 -s 10.0.0.0/8 -j ACCEPT
iptables -A INPUT -p tcp --dport 40114 -j DROP
```

2. **Rate Limiting**: Configure appropriate limits

```yaml
server:
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    burst_size: 50
```

3. **Request Size Limits**: Prevent abuse

```yaml
server:
  request_limits:
    max_body_size: 104857600  # 100MB
    max_header_size: 1048576  # 1MB
```

### TLS/SSL

Olla doesn't implement TLS directly. Use a reverse proxy for TLS termination:

- nginx, Caddy, or HAProxy for TLS
- Cloud load balancers (AWS ALB, GCP LB)
- Service mesh (Istio, Linkerd)

## Performance Tuning

### OS Tuning

```bash
# Increase file descriptor limits
ulimit -n 65535

# Network tuning (Linux)
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
```

### Olla Configuration

For high-performance deployments:

```yaml
proxy:
  engine: "olla"  # High-performance engine
  stream_buffer_size: 65536  # 64KB buffers

server:
  read_timeout: "5m"
  write_timeout: "0s"  # No timeout for streaming
```

## Troubleshooting

### Common Issues

1. **Port Already in Use**
```bash
# Find process using port
lsof -i :40114
# Or
netstat -tulpn | grep 40114
```

2. **Cannot Connect to Endpoints**
```bash
# Test endpoint connectivity
curl http://localhost:11434/api/tags

# Check Olla logs
journalctl -u olla -n 100
```

3. **High Memory Usage**
- Check active connections: `/internal/status`
- Review buffer sizes in configuration
- Enable pprof and analyse heap profile

4. **Slow Response Times**
- Check endpoint health: `/internal/status/endpoints`
- Review load balancer settings
- Monitor endpoint latencies: `/internal/stats/models`

### Debug Mode

Enable debug logging for troubleshooting:

```bash
OLLA_LOG_LEVEL=debug ./olla
```

## Best Practices

1. **Start Simple**: Begin with a single instance, add complexity gradually
2. **Monitor Health**: Set up automated health checks
3. **Log Aggregation**: Use structured JSON logging
4. **Graceful Shutdown**: Olla handles SIGTERM for clean shutdown
5. **Configuration Management**: Version control your configs
6. **Resource Limits**: Set appropriate OS and container limits
7. **Regular Updates**: Keep Olla updated for bug fixes and features

## Example Production Setup

Here's a complete example for a production deployment:

```yaml
# /etc/olla/config.yaml
server:
  host: "127.0.0.1"  # Only listen on localhost
  port: 40114
  request_logging: false  # Reduce log volume
  
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    burst_size: 100
    trust_proxy_headers: true  # Trust X-Forwarded-For from nginx

proxy:
  engine: "olla"
  load_balancer: "priority"
  connection_timeout: "30s"
  response_timeout: "30m"  # Long timeout for slow models

discovery:
  static:
    endpoints:
      - name: "primary-gpu"
        url: "http://10.0.1.10:11434"
        type: "ollama"
        priority: 100
        check_interval: "10s"
        
      - name: "secondary-gpu"
        url: "http://10.0.1.11:11434"
        type: "ollama"
        priority: 50
        check_interval: "10s"

logging:
  level: "info"
  format: "json"
```

Deploy behind nginx for TLS and let nginx handle external traffic while Olla focuses on intelligent LLM routing.