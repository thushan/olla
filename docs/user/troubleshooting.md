# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Olla. Start with the quick diagnostics, then dive into specific problem areas.

## Quick Diagnostics

Run these checks first when encountering issues:

```bash
# 1. Check if Olla is running
curl http://localhost:40114/internal/health

# 2. Check system status
curl http://localhost:40114/internal/status | jq

# 3. Check endpoint status
curl http://localhost:40114/internal/status/endpoints | jq

# 4. List available models
curl http://localhost:40114/olla/models | jq

# 5. Check version
curl http://localhost:40114/version
```

## Common Issues

### Olla Won't Start

**Symptom:** Olla fails to start or exits immediately

**Check #1: Port Already in Use**
```bash
# Default port is 40114
lsof -i :40114
# or
netstat -tlnp | grep 40114

# Solution: Change port in config or via environment
OLLA_SERVER_PORT=8080 ./olla
```

**Check #2: Configuration Errors**
```bash
# Run with debug logging to see config loading issues
OLLA_LOG_LEVEL=debug ./olla

# Common errors:
# - Invalid YAML syntax
# - Missing required fields
# - Invalid endpoint URLs
# - Unreachable endpoints
```

**Check #3: File Permissions**
```bash
# Check config file permissions
ls -la config.yaml

# If using systemd, check service user permissions
sudo -u olla cat /etc/olla/config.yaml
```

### No Endpoints Available

**Symptom:** Error: "no healthy endpoints available"

**Check #1: Endpoint Connectivity**
```bash
# Test each endpoint directly
curl http://localhost:11434/api/tags
curl http://gpu-server:11434/health

# Check detailed endpoint status
curl http://localhost:40114/internal/status/endpoints | jq
```

**Check #2: Health Check Settings**
```yaml
discovery:
  static:
    endpoints:
      - name: "slow-endpoint"
        url: "http://gpu:11434"
        check_timeout: "10s"  # Increase for slow endpoints
        check_interval: "30s" # Check less frequently
```

**Check #3: Circuit Breaker State**
```bash
# Check if endpoints are circuit-broken
curl http://localhost:40114/internal/status | jq '.endpoints[] | select(.status != "Healthy")'

# Circuit breakers automatically recover after timeout
# Default recovery happens after successful health check
```

### Model Not Found

**Symptom:** Error: "model 'llama3.2' not found on any endpoint"

**Check #1: Model Availability**
```bash
# List all unified models
curl http://localhost:40114/olla/models | jq '.models[].unified_name'

# Check specific model
curl http://localhost:40114/olla/models/llama3.2 | jq

# List models on specific endpoint
curl "http://localhost:40114/olla/models?endpoint=local-ollama" | jq
```

**Check #2: Model Aliases**
```bash
# Models have multiple aliases
curl http://localhost:40114/olla/models/llama3.2 | jq '.aliases'

# Try different variations:
# - llama3.2
# - llama3.2:latest
# - llama-3.2-7b
```

**Check #3: Endpoint Model Discovery**
```bash
# Check if model discovery is finding models
curl http://localhost:40114/internal/status/models | jq

# Verify endpoint has the model
curl http://endpoint:11434/api/tags | jq '.models[].name'
```

### Slow Response Times

**Symptom:** Requests take too long or timeout

**Check #1: Endpoint Performance**
```bash
# Check endpoint latency statistics
curl http://localhost:40114/internal/status/endpoints | jq '.endpoints[] | {name, average_latency, total_requests}'

# Check model-specific statistics
curl http://localhost:40114/internal/stats/models | jq
```

**Check #2: Timeout Configuration**
```yaml
# Increase timeouts for large models
proxy:
  response_timeout: "30m"  # Increase from default 10m
  read_timeout: "5m"       # Increase from default 2m

server:
  write_timeout: "0s"      # Disable for streaming
```

**Check #3: Resource Usage**
```bash
# Check Olla process statistics
curl http://localhost:40114/internal/process | jq

# Enable detailed stats (requires restart)
# Set engineering.show_nerdstats: true in config
```

### Connection Errors

**Symptom:** "connection refused" or "connection reset"

**Check #1: Active Connections**
```bash
# Check current connection count
curl http://localhost:40114/internal/status | jq '.stats'

# Check per-endpoint connections
curl http://localhost:40114/internal/status/endpoints | jq '.endpoints[] | {name, active_connections}'
```

**Check #2: File Descriptor Limits**
```bash
# Check current limit
ulimit -n

# For systemd services, check service limits
systemctl show olla | grep LimitNOFILE
```

**Check #3: Proxy Engine**
```bash
# Olla engine has better connection pooling
# Check current engine
curl http://localhost:40114/internal/status | jq '.config.proxy_engine'

# Switch to Olla engine for better performance
# Set proxy.engine: "olla" in config
```

### Rate Limiting Issues

**Symptom:** HTTP 429 "rate limit exceeded" errors

**Check #1: Current Rate Limit Status**
```bash
# Check security statistics
curl http://localhost:40114/internal/status | jq '.security'

# See rate limit configuration
grep -A5 "rate_limits:" config.yaml
```

**Check #2: Adjust Limits**
```yaml
server:
  rate_limits:
    global_requests_per_minute: 10000  # Increase global limit
    per_ip_requests_per_minute: 1000   # Increase per-IP limit
    burst_size: 100                    # Allow larger bursts
```

**Check #3: Trusted Proxies**
```yaml
server:
  rate_limits:
    trust_proxy_headers: true
    trusted_proxy_cidrs:
      - "10.0.0.0/8"        # Trust internal network
      - "172.16.0.0/12"
```

## Debugging Techniques

### Enable Debug Logging

```bash
# Via environment variable
OLLA_LOG_LEVEL=debug ./olla

# Or in config
logging:
  level: "debug"
  format: "text"  # More readable than JSON for debugging
```

### Enable Profiling

```bash
# Start with profiling enabled
./olla --profile
# or
OLLA_ENABLE_PROFILER=true ./olla

# Access profiling data
go tool pprof http://localhost:19841/debug/pprof/profile?seconds=30
go tool pprof http://localhost:19841/debug/pprof/heap
go tool pprof http://localhost:19841/debug/pprof/goroutine

# View in browser
open http://localhost:19841/debug/pprof/
```

### Monitor in Real-Time

```bash
# Watch status updates
watch -n 1 'curl -s http://localhost:40114/internal/status | jq .'

# Monitor specific endpoint
watch -n 1 'curl -s http://localhost:40114/internal/status/endpoints | jq ".endpoints[] | select(.name==\"local-ollama\")"'

# Watch model statistics
watch -n 5 'curl -s http://localhost:40114/internal/stats/models | jq .'
```

### Trace Requests

Add request IDs to track through the system:

```bash
# Add X-Request-ID header
curl -H "X-Request-ID: debug-123" http://localhost:40114/olla/api/generate \
  -d '{"model":"llama3.2","prompt":"test"}'

# Then search logs
grep "debug-123" /var/log/olla.log
```

## Platform-Specific Issues

### Docker Deployments

**Cannot reach host services:**
```yaml
# Use host.docker.internal for Docker Desktop
discovery:
  static:
    endpoints:
      - url: "http://host.docker.internal:11434"
```

**Permission issues:**
```bash
# Run with specific user
docker run --user $(id -u):$(id -g) olla

# Or ensure config is readable
chmod 644 config.yaml
```

### Ollama Endpoints

**Model not loading:**
```bash
# Check Ollama logs
journalctl -u ollama -n 50

# Check available models
curl http://localhost:11434/api/tags

# Pull model if missing
curl -X POST http://localhost:11434/api/pull -d '{"name":"llama3.2"}'
```

**Connection issues:**
```bash
# Verify Ollama is listening on all interfaces
# Not just localhost if Olla runs in Docker
ss -tlnp | grep 11434
```

### High Memory Usage

**Check memory statistics:**
```bash
# Enable nerd stats
# Set engineering.show_nerdstats: true

# Check process memory
curl http://localhost:40114/internal/process | jq '.memory_mb'

# If using Olla engine, connection pools use more memory
# Switch to Sherpa for lower memory usage
```

## Recovery Procedures

### Restart Olla

```bash
# Graceful shutdown (handles active requests)
kill -TERM $(pgrep olla)

# For systemd
systemctl restart olla

# Force restart if hung
kill -9 $(pgrep olla)
```

### Circuit Breaker Recovery

Circuit breakers automatically recover when endpoints become healthy:

1. Breaker opens after threshold failures (default: 3)
2. After timeout, breaker enters "half-open" state
3. Next successful request closes the breaker
4. Failed request re-opens the breaker

No manual intervention needed - just fix the underlying endpoint issue.

### Emergency Fallback

Add a high-priority cloud endpoint for emergencies:

```yaml
discovery:
  static:
    endpoints:
      # Existing endpoints...
      - name: "emergency-cloud"
        url: "https://api.openai.com"
        type: "openai"
        priority: 200  # Higher priority
        # Only used when all others fail
```

## Test Scripts

Olla includes test scripts for troubleshooting:

```bash
# Test rate limiting
./test/scripts/security/test-request-rate-limits.sh

# Test request size limits
./test/scripts/security/test-request-size-limits.sh

# Load testing
./test/scripts/load/test-load-limits.sh

# Chaos testing
./test/scripts/load/test-load-chaos.sh
```

## Getting Help

### Collect Diagnostic Information

```bash
# Create diagnostic bundle
mkdir olla-diag && cd olla-diag

# System info
uname -a > system.txt
date >> system.txt

# Olla info
olla --version > version.txt 2>&1
cp /path/to/config.yaml .

# Current status
curl -s http://localhost:40114/internal/status > status.json
curl -s http://localhost:40114/internal/status/endpoints > endpoints.json
curl -s http://localhost:40114/internal/status/models > models.json
curl -s http://localhost:40114/internal/process > process.json

# Create archive
tar -czf olla-diag-$(date +%Y%m%d-%H%M%S).tar.gz *
```

### Reporting Issues

When reporting issues on GitHub:

1. Check existing issues first
2. Include:
   - Olla version (`olla --version`)
   - Configuration (remove sensitive data)
   - Error messages and logs
   - Steps to reproduce
3. Attach diagnostic bundle if applicable

### Common Solutions Summary

| Problem | Quick Fix |
|---------|-----------|
| Port in use | Change port: `OLLA_SERVER_PORT=8081 ./olla` |
| No endpoints available | Check endpoint URLs and connectivity |
| Model not found | Use `/olla/models` to see available models |
| Slow responses | Increase timeouts in config |
| Rate limited | Increase limits or add trusted proxies |
| High memory | Switch to Sherpa engine |
| Connection errors | Check file descriptor limits |

Remember: Enable debug logging (`OLLA_LOG_LEVEL=debug`) for detailed troubleshooting information.