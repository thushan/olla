# Load Balancing

> :memo: **Default Configuration**
> ```yaml
> proxy:
>   load_balancer: "priority"  # priority, round-robin, or least-connections
> ```
> **Supported**:
> 
> - `priority` _(default)_ - Routes to highest priority endpoint
> - `round-robin` - Cycles through endpoints evenly
> - `least-connections` - Routes to least busy endpoint
> 
> **Environment Variable**: `OLLA_PROXY_LOAD_BALANCER`

Olla provides multiple load balancing strategies to distribute requests across backend endpoints efficiently. Each strategy has specific use cases and characteristics to optimise for different deployment scenarios.

## Overview

Load balancing determines which backend endpoint receives each incoming request. The strategy you choose affects:

- **Request distribution** - How evenly load spreads across endpoints
- **Failover behaviour** - How quickly traffic shifts from unhealthy endpoints
- **Resource utilisation** - How efficiently backend resources are used
- **Response times** - Latency based on endpoint selection

## Available Strategies

### Priority (Recommended)

Priority-based selection routes to the highest-priority healthy endpoint first.

**How it works:**

1. Endpoints assigned priority values (0-100, higher = preferred)
2. Selects from highest priority tier only
3. Within same priority, uses weighted random selection
4. Automatically falls back to lower priorities if higher ones fail

**Best for:**

- Production deployments with primary/backup servers
- GPU vs CPU server hierarchies
- Geographic or network-based preferences
- Cost-optimised deployments (prefer cheaper endpoints)

**Configuration:**
```yaml
proxy:
  load_balancer: "priority"

discovery:
  static:
    endpoints:
      - url: "http://gpu-server:11434"
        name: "primary-gpu"
        type: "ollama"
        priority: 100  # Highest priority
        
      - url: "http://backup-gpu:11434"
        name: "backup-gpu"
        type: "ollama"
        priority: 90   # Second choice
        
      - url: "http://cpu-server:11434"
        name: "cpu-fallback"
        type: "ollama"
        priority: 50   # Last resort
```

### Round Robin

Distributes requests evenly across all healthy endpoints in sequence.

**How it works:**

1. Maintains internal counter
2. Selects next endpoint in rotation
3. Skips unhealthy endpoints
4. Wraps around at end of list

**Best for:**

- Homogeneous server deployments
- Equal capacity endpoints
- Development and testing
- Simple load distribution

**Configuration:**
```yaml
proxy:
  load_balancer: "round-robin"

discovery:
  static:
    endpoints:
      - url: "http://server1:11434"
        name: "ollama-1"
        type: "ollama"
        
      - url: "http://server2:11434"
        name: "ollama-2"
        type: "ollama"
        
      - url: "http://server3:11434"
        name: "ollama-3"
        type: "ollama"
```

### Least Connections

Routes to the endpoint with fewest active connections.

**How it works:**

1. Tracks active connections per endpoint
2. Selects endpoint with minimum connections
3. Updates count on connection start/end
4. Ideal for long-running requests

**Best for:**

- Variable request durations
- Streaming responses
- Mixed model sizes
- Preventing endpoint overload

**Configuration:**
```yaml
proxy:
  load_balancer: "least-connections"

discovery:
  static:
    endpoints:
      - url: "http://fast-server:8000"
        name: "vllm-fast"
        type: "vllm"
        
      - url: "http://slow-server:8000"
        name: "vllm-slow"
        type: "vllm"
```

## Strategy Comparison

| Strategy | Distribution | Failover Speed | Best Use Case | Complexity |
|----------|-------------|----------------|---------------|------------|
| Priority | Weighted by tier | Fast | Production with primary/backup | Low |
| Round Robin | Equal | Moderate | Homogeneous servers | Low |
| Least Connections | Dynamic | Fast | Variable workloads | Medium |

## Health-Aware Routing

All strategies respect endpoint health status:

### Health States

| Status | Routable | Weight | Description |
|--------|----------|--------|-------------|
| Healthy | ✅ Yes | 1.0 | Fully operational |
| Busy | ✅ Yes | 0.3 | Busy but working |
| Warming | ✅ Yes | 0.1 | Coming back online |
| Unhealthy | ❌ No | 0.0 | Failed health checks |
| Unknown | ❌ No | 0.0 | Not yet checked |

### Circuit Breaker Integration

Olla includes built-in circuit breaker functionality to prevent cascading failures. The circuit breaker automatically manages endpoint health without requiring configuration.

**Circuit states:**

- **Closed** - Normal operation, requests pass through
- **Open** - Endpoint marked unhealthy after consecutive failures
- **Half-Open** - Testing recovery with limited requests

!!! note "Circuit Breaker Behaviour"
    The circuit breaker thresholds are currently hardcoded in the implementation. Configuration support may be added in future versions.

## Advanced Configurations

### Multi-Tier Priority

Create sophisticated failover hierarchies:

```yaml
discovery:
  static:
    endpoints:
      # Tier 1: Local GPU servers
      - url: "http://localhost:11434"
        name: "local-gpu"
        priority: 100
        
      - url: "http://192.168.1.10:11434"
        name: "lan-gpu"
        priority: 95
        
      # Tier 2: Cloud GPU servers
      - url: "https://gpu1.cloud.example.com"
        name: "cloud-gpu-1"
        priority: 80
        
      - url: "https://gpu2.cloud.example.com"
        name: "cloud-gpu-2"
        priority: 80
        
      # Tier 3: CPU fallbacks
      - url: "http://cpu-server:11434"
        name: "cpu-fallback"
        priority: 50
```

### Model-Based Load Distribution

The load balancer works with model unification to distribute requests across endpoints that have the requested model available. When multiple endpoints have the same model, the configured load balancing strategy (priority, round-robin, or least-connections) determines which endpoint receives the request.

### Geographic Distribution

Use priority for region-based routing:

```yaml
endpoints:
  # US East - Primary
  - url: "https://us-east.example.com"
    name: "us-east-primary"
    priority: 100
    
  # US West - Secondary
  - url: "https://us-west.example.com"
    name: "us-west-secondary"
    priority: 90
    
  # EU - Tertiary
  - url: "https://eu.example.com"
    name: "eu-tertiary"
    priority: 70
```

## Monitoring Load Distribution

### Metrics Endpoints

Monitor load balancer effectiveness:

```bash
# Endpoint connection counts
curl http://localhost:40114/internal/status/endpoints

# Model routing statistics
curl http://localhost:40114/internal/stats/models

# Health status overview
curl http://localhost:40114/internal/health
```

### Response Headers

Track routing decisions via headers:

```http
X-Olla-Endpoint: gpu-server-1
X-Olla-Backend-Type: ollama
X-Olla-Request-ID: abc-123
X-Olla-Response-Time: 1.234s
```

### Logging

Enable debug logging for routing decisions:

```yaml
logging:
  level: debug
  
# Logs show:
# - Endpoint selection reasoning
# - Health state changes
# - Circuit breaker transitions
# - Connection tracking
```

## Performance Tuning

### Connection Pooling

Optimise connection reuse:

```yaml
proxy:
  connection_pool:
    max_idle_conns: 100
    max_idle_conns_per_host: 10
    idle_conn_timeout: 90s
```

### Timeout Configuration

Balance reliability and responsiveness:

```yaml
proxy:
  # Initial connection timeout
  connection_timeout: 10s
  
  # Request/response timeout
  response_timeout: 300s
  
  # Idle connection timeout
  idle_timeout: 90s
  
  # Health check specific
  health_check_timeout: 5s
```

### Buffering Strategies

Choose appropriate proxy profile:

```yaml
proxy:
  profile: "auto"  # auto, streaming, or standard
  
  # Auto profile adapts based on:
  # - Response size
  # - Streaming detection
  # - Content type
```

## Troubleshooting

### Uneven Distribution

**Issue**: Requests not spreading evenly

**Diagnosis:**
```bash
# Check endpoint stats
curl http://localhost:40114/internal/status/endpoints | jq '.endpoints[] | {name, requests}'
```

**Solutions:**

- Verify all endpoints are healthy
- Check priority values aren't identical
- Review connection tracking for least-connections
- Ensure round-robin counter isn't stuck

### Slow Failover

**Issue**: Takes too long to stop using failed endpoints

**Solutions:**
```yaml
health:
  check_interval: 5s      # Reduce for faster detection
  check_timeout: 2s       # Fail fast on timeouts
  
  circuit_breaker:
    failure_threshold: 3  # Lower for quicker opening
```

### Connection Exhaustion

**Issue**: "Too many connections" errors

**Solutions:**
```yaml
# Increase connection limits
proxy:
  connection_pool:
    max_idle_conns_per_host: 20  # Increase per-host limit
    
# Use least-connections to spread load
proxy:
  load_balancer: "least-connections"
```

## Best Practices

### 1. Match Strategy to Deployment

- **Priority**: Production with clear server tiers
- **Round Robin**: Development or identical servers
- **Least Connections**: Mixed workloads or streaming

### 2. Configure Health Checks

```yaml
health:
  check_interval: 10s    # Balance between speed and load
  check_timeout: 5s      # Allow for model loading
  unhealthy_threshold: 3 # Avoid flapping
```

### 3. Set Appropriate Priorities

```yaml
# Clear gaps between tiers
priority: 100  # Primary
priority: 75   # Secondary (clear gap)
priority: 50   # Tertiary (clear gap)
priority: 25   # Last resort
```

### 4. Monitor and Adjust

Regularly review:

- Request distribution patterns
- Endpoint utilisation
- Response time percentiles
- Error rates by endpoint

### 5. Plan for Failure

Always have:

- At least one backup endpoint
- Clear priority tiers
- Appropriate circuit breaker settings
- Monitoring and alerting

## Integration Examples

### Kubernetes Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ollama-backends
spec:
  selector:
    app: ollama
  ports:
    - port: 11434
  type: ClusterIP
---
# Olla configuration
discovery:
  static:
    endpoints:
      - url: "http://ollama-backends:11434"
        name: "k8s-ollama"
        type: "ollama"
```

### Docker Swarm

```yaml
# docker-compose.yml
services:
  ollama:
    image: ollama/ollama
    deploy:
      replicas: 3
      
  olla:
    image: thushan/olla
    environment:
      OLLA_LOAD_BALANCER: "round-robin"
```

### Consul Discovery

```yaml
discovery:
  consul:
    enabled: true
    service_name: "ollama"
    health_check: true
    
proxy:
  load_balancer: "least-connections"
```

## Next Steps

- [Health Checking](health-checking.md) - Configure health monitoring
- [Circuit Breaker](../development/circuit-breaker.md) - Prevent cascade failures
- [Model Unification](model-unification.md) - Unified model routing
- [Performance Tuning](../configuration/practices/performance.md) - Optimisation guide