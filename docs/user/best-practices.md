# Production Best Practices

This guide provides recommendations for operating Olla reliably in production environments, based on its actual capabilities.

## Architecture Best Practices

### High Availability Design

Deploy multiple Olla instances behind a load balancer for redundancy:

```
┌─────────────────────────────┐
│      Load Balancer          │
│    (nginx/HAProxy/ALB)      │
└──────────┬─────────────────┘
           │
    ┌──────┴──────┬────────┐
    │             │        │
┌───▼──┐    ┌────▼──┐ ┌───▼──┐
│Olla 1│    │Olla 2 │ │Olla 3│
└───┬──┘    └────┬──┘ └───┬──┘
    │            │        │
    └──────┬─────┴────────┘
           │
    ┌──────▼──────┐
    │  Endpoints  │
    │(Ollama/LMS) │
    └─────────────┘
```

**Key Principles:**
- Deploy at least 3 Olla instances for redundancy
- Use health checks at the load balancer level
- Each Olla instance is stateless and independent
- Configure consistent endpoint lists across instances

### Endpoint Organisation

Structure endpoints by priority for optimal routing:

```yaml
discovery:
  static:
    endpoints:
      # Primary: Local high-performance GPUs
      - name: "gpu-primary"
        url: "http://10.0.1.10:11434"
        type: "ollama"
        priority: 100  # Highest priority
        check_interval: "10s"
        
      # Secondary: Additional GPU servers
      - name: "gpu-secondary"
        url: "http://10.0.1.11:11434"
        type: "ollama"
        priority: 50
        check_interval: "10s"
        
      # Tertiary: Cloud endpoints (fallback)
      - name: "cloud-backup"
        url: "https://api.provider.com"
        type: "openai"
        priority: 10  # Lowest priority
        check_interval: "30s"
```

## Configuration Best Practices

### Production Configuration Template

```yaml
# production-config.yaml
server:
  host: "127.0.0.1"  # Bind to localhost, proxy handles external
  port: 40114
  read_timeout: "5m"
  write_timeout: "0s"  # No timeout for streaming
  shutdown_timeout: "30s"
  request_logging: false  # Reduce log volume
  
  # Request limits
  request_limits:
    max_body_size: 104857600    # 100MB
    max_header_size: 1048576    # 1MB
  
  # Rate limiting
  rate_limits:
    global_requests_per_minute: 10000
    per_ip_requests_per_minute: 100
    burst_size: 50
    health_requests_per_minute: 1000
    cleanup_interval: "5m"
    trust_proxy_headers: true  # Behind load balancer
    trusted_proxy_cidrs:
      - "10.0.0.0/8"
      - "172.16.0.0/12"

proxy:
  engine: "olla"  # High-performance engine
  load_balancer: "priority"
  connection_timeout: "30s"
  response_timeout: "30m"  # Long timeout for slow models
  read_timeout: "5m"
  max_retries: 3
  retry_backoff: "1s"
  stream_buffer_size: 65536  # 64KB for better streaming

discovery:
  type: "static"
  refresh_interval: "30s"
  model_discovery:
    enabled: true
    interval: "5m"
    timeout: "30s"
    concurrent_workers: 3

logging:
  level: "info"
  format: "json"  # Structured logs

model_registry:
  type: "memory"
  enable_unifier: true
  unification:
    enabled: true
    cache_ttl: "10m"

engineering:
  show_nerdstats: false  # Enable for debugging
```

### Environment Variables

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

# Rate limits
export OLLA_SERVER_GLOBAL_RATE_LIMIT=10000
export OLLA_SERVER_PER_IP_RATE_LIMIT=1000
```

## Operational Best Practices

### Monitoring Setup

Monitor these key metrics using the status endpoints:

```bash
# Create monitoring script
#!/bin/bash
# monitor-olla.sh

# Check health
health=$(curl -sf http://localhost:40114/internal/health | jq -r '.status')
if [ "$health" != "healthy" ]; then
    alert "Olla unhealthy"
fi

# Check endpoints
unhealthy=$(curl -sf http://localhost:40114/internal/status/endpoints | \
    jq -r '.endpoints[] | select(.status != "Healthy") | .name')
if [ -n "$unhealthy" ]; then
    alert "Unhealthy endpoints: $unhealthy"
fi

# Check error rate
stats=$(curl -sf http://localhost:40114/internal/status | jq '.stats')
total=$(echo "$stats" | jq -r '.total_requests')
failed=$(echo "$stats" | jq -r '.failed_requests')
if [ "$total" -gt 0 ]; then
    error_rate=$(awk "BEGIN {printf \"%.2f\", $failed/$total*100}")
    if (( $(echo "$error_rate > 5" | bc -l) )); then
        alert "High error rate: ${error_rate}%"
    fi
fi
```

### Key Metrics to Track

1. **System Health**
   - Endpoint status (healthy/unhealthy)
   - Circuit breaker states
   - Active connections per endpoint
   - Model availability

2. **Performance Metrics**
   - Request rate (requests/minute)
   - Error rate (failed/total)
   - Average latency
   - Model-specific statistics (P95, P99)

3. **Resource Usage**
   - Memory usage (via `/internal/process`)
   - CPU usage
   - Goroutine count
   - Connection pool usage

### Logging Best Practices

Configure structured JSON logging for easy parsing:

```yaml
logging:
  level: "info"
  format: "json"
```

Parse logs with jq:
```bash
# View errors
tail -f /var/log/olla.log | jq 'select(.level == "error")'

# Track specific request
tail -f /var/log/olla.log | jq 'select(.request_id == "abc-123")'

# Monitor latency
tail -f /var/log/olla.log | jq 'select(.latency_ms > 5000)'
```

## Security Best Practices

### Network Security

1. **Deploy Behind Reverse Proxy**
   - Use nginx/Caddy for TLS termination
   - Implement additional security headers
   - Hide Olla from direct internet access

2. **Configure Rate Limiting**
   ```yaml
   server:
     rate_limits:
       global_requests_per_minute: 10000
       per_ip_requests_per_minute: 100
       burst_size: 50
       trust_proxy_headers: true
   ```

3. **Restrict Access**
   - Use firewall rules to limit access
   - Deploy in private subnets
   - Use VPN for management access

### Request Validation

Configure appropriate limits:

```yaml
server:
  request_limits:
    max_body_size: 104857600    # 100MB max
    max_header_size: 1048576    # 1MB headers
```

### Secrets Management

Protect sensitive endpoint URLs:

```bash
# Don't hardcode API keys in URLs
# Bad:
url: "https://api.openai.com?key=sk-abc123"

# Good: Use environment variables
url: "https://api.openai.com"
# Then configure authentication at the endpoint level
```

## Performance Optimisation

### Proxy Engine Selection

Choose the right engine for your workload:

```yaml
# For high throughput
proxy:
  engine: "olla"  # Better connection pooling, circuit breakers

# For lower memory usage
proxy:
  engine: "sherpa"  # Simpler, less memory overhead
```

### Buffer Size Tuning

Adjust buffer sizes based on response patterns:

```yaml
# For streaming large responses
proxy:
  stream_buffer_size: 65536  # 64KB

# For smaller, faster responses
proxy:
  stream_buffer_size: 8192   # 8KB (default for Sherpa)
```

### Connection Timeout Tuning

Set appropriate timeouts:

```yaml
proxy:
  connection_timeout: "30s"   # Initial connection
  response_timeout: "30m"     # Total response time
  read_timeout: "5m"          # Time between chunks
```

## Deployment Best Practices

### Health Check Configuration

Configure load balancer health checks:

```nginx
# nginx health check
upstream olla_backend {
    server 127.0.0.1:40114 max_fails=3 fail_timeout=30s;
    server 127.0.0.1:40115 max_fails=3 fail_timeout=30s;
}

location /health {
    proxy_pass http://olla_backend/internal/health;
    proxy_connect_timeout 3s;
    proxy_read_timeout 3s;
    access_log off;
}
```

### Graceful Shutdown

Olla handles SIGTERM for graceful shutdown:

```bash
# Graceful restart
kill -TERM $(pgrep olla)
sleep 5  # Wait for connections to drain
./olla &
```

### Zero-Downtime Updates

Simple rolling update strategy:

```bash
#!/bin/bash
# Simple rolling update for 3 instances

for port in 40114 40115 40116; do
    echo "Updating instance on port $port"
    
    # Stop old instance
    pkill -f "OLLA_SERVER_PORT=$port"
    
    # Wait for shutdown
    sleep 5
    
    # Start new instance
    OLLA_SERVER_PORT=$port ./olla-new &
    
    # Wait for health
    until curl -sf "http://localhost:$port/internal/health"; do
        sleep 1
    done
    
    echo "Instance on port $port updated"
    sleep 10  # Stabilisation time
done
```

## Capacity Planning

### Resource Requirements

Based on actual usage patterns:

| Load Level | CPU | Memory | Connections |
|------------|-----|--------|-------------|
| Light (<100 RPS) | 2 cores | 2GB | 100 |
| Medium (100-1000 RPS) | 4-8 cores | 8GB | 500 |
| Heavy (>1000 RPS) | 16+ cores | 16GB | 1000+ |

### Sizing Calculation

```
# Simple sizing formula
Required Instances = (Peak RPS × Avg Response Time) / Connections per Instance

Example:
- Peak: 500 req/s
- Avg response: 3s
- Connections per instance: 500
- Required: (500 × 3) / 500 = 3 instances
- Deploy: 5 instances (with redundancy)
```

## Troubleshooting in Production

### Enable Debug Information

For production debugging:

```bash
# Temporary debug logging
OLLA_LOG_LEVEL=debug ./olla

# Enable profiling
./olla --profile
# Access at http://localhost:19841/debug/pprof/
```

### Common Issues and Solutions

| Issue | Check | Solution |
|-------|-------|----------|
| High latency | `/internal/status/endpoints` | Check endpoint health, increase timeouts |
| Memory growth | `/internal/process` | Switch to Sherpa engine, restart periodically |
| Connection errors | Active connections in status | Increase file descriptors, check limits |
| Model not found | `/olla/models` | Verify model discovery is working |
| Rate limiting | Security stats in status | Adjust limits or add trusted proxies |

### Production Checklist

Before going to production:

- [ ] Configure rate limiting appropriate for load
- [ ] Set up monitoring for all instances
- [ ] Configure health checks in load balancer
- [ ] Test graceful shutdown behaviour
- [ ] Document endpoint configuration
- [ ] Set appropriate timeouts for your models
- [ ] Configure trusted proxy CIDRs
- [ ] Disable request logging for performance
- [ ] Set up log aggregation
- [ ] Test failover scenarios
- [ ] Document rollback procedure
- [ ] Configure resource limits (systemd/container)

## Maintenance

### Regular Tasks

**Daily:**
- Monitor error rates
- Check endpoint health
- Review resource usage

**Weekly:**
- Review logs for anomalies
- Check circuit breaker events
- Verify model availability

**Monthly:**
- Update Olla version
- Review and optimise configuration
- Benchmark performance
- Audit security settings

### Performance Baseline

Establish baselines for your deployment:

```bash
# Benchmark script
#!/bin/bash
echo "=== Olla Performance Baseline ==="
echo "Date: $(date)"
echo "Version: $(./olla --version)"
echo ""

# Get current stats
stats=$(curl -sf http://localhost:40114/internal/status)
echo "Total Requests: $(echo $stats | jq '.stats.total_requests')"
echo "Error Rate: $(echo $stats | jq '.stats.failed_requests / .stats.total_requests * 100')%"
echo "Avg Latency: $(echo $stats | jq '.stats.average_latency')ms"
echo ""

# Endpoint health
echo "Endpoint Health:"
curl -sf http://localhost:40114/internal/status/endpoints | \
    jq -r '.endpoints[] | "\(.name): \(.status) - \(.average_latency)ms"'
```

Remember: Start simple, monitor continuously, and scale based on actual usage patterns rather than assumptions.