# Load Balancing Strategies

Olla provides three load balancing strategies to distribute requests across your LLM endpoints. Each strategy is optimised for different deployment scenarios and infrastructure patterns.

## Strategy Overview

| Strategy | Best For | Use Case |
|----------|----------|----------|
| **Priority** | Home labs, tiered infrastructure | Local hardware → cloud fallback |
| **Least Connections** | Production, mixed workloads | Dynamic load adaptation |
| **Round Robin** | Identical hardware, development | Simple, predictable distribution |

## Priority Balancer (Recommended)

The priority balancer routes requests to the highest priority healthy endpoint, providing controlled failover and cost optimisation.

### How It Works

Always routes requests to the highest priority healthy endpoint. If that endpoint becomes unavailable, automatically fails over to the next highest priority.

### Configuration

```yaml
proxy:
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-gpu"
        type: "ollama"
        priority: 100        # Highest priority - always tried first
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://192.168.1.10:11434" 
        name: "backup-cpu"
        type: "ollama"
        priority: 50         # Fallback option
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

### Use Cases

**Home Lab Setup**
```yaml
endpoints:
  - name: "gaming-pc"      # Primary: RTX 4090
    priority: 100
  - name: "old-laptop"     # Backup: CPU only  
    priority: 50
```

**Enterprise Tiered Infrastructure**
```yaml
endpoints:
  - name: "a100-cluster"   # Tier 1: High-end GPUs
    priority: 100
  - name: "rtx4090-pool"   # Tier 2: Mid-range GPUs
    priority: 75
  - name: "cpu-farm"       # Tier 3: CPU inference
    priority: 25
```

**Cost-Optimised Cloud**
```yaml
endpoints:
  - name: "local-dev"      # Free: Local development
    priority: 100
  - name: "groq-fast"      # Cheap: Fast inference
    priority: 50  
  - name: "openai-gpt4"    # Expensive: Premium models
    priority: 10
```

### Multiple Same-Priority Endpoints

When multiple endpoints share the same priority, Olla automatically balances between them based on their health status, giving preference to healthier endpoints.

## Least Connections Balancer

Routes requests to the endpoint with the least active connections, automatically adapting to endpoint performance and load.

### How It Works

Automatically routes new requests to whichever healthy endpoint currently has the fewest active requests. This ensures busy or slower endpoints don't get overwhelmed.

### Configuration

```yaml
proxy:
  load_balancer: "least_connections"
```

### Use Cases

**Mixed Hardware Performance**
```yaml
endpoints:
  - name: "rtx4090-fast"    # Processes requests quickly
    url: "http://gpu1:11434"
  - name: "rtx3060-slow"    # Takes longer per request
    url: "http://gpu2:11434"
# Least connections automatically gives more traffic to the faster GPU
```

**Variable Request Types**
- Chat completion requests (fast)
- Code generation requests (medium)  
- Document analysis requests (slow)

Least connections ensures slower requests don't bottleneck faster ones.

### Benefits

- **Automatic adaptation** to endpoint performance differences
- **Prevents overload** of slower endpoints
- **Optimal resource utilisation** across heterogeneous hardware
- **No manual tuning** required for capacity differences

## Round Robin Balancer

Distributes requests evenly across all healthy endpoints in rotation.

### How It Works

Distributes requests evenly by cycling through healthy endpoints in order. Each new request goes to the next endpoint in the rotation.

### Configuration

```yaml
proxy:
  load_balancer: "round_robin"
```

### Use Cases

**Identical Hardware**
```yaml
endpoints:
  - name: "gpu-node-1"
    url: "http://gpu1:11434"
  - name: "gpu-node-2" 
    url: "http://gpu2:11434"
  - name: "gpu-node-3"
    url: "http://gpu3:11434"
# All nodes have identical RTX 4090s - fair distribution
```

**Development and Testing**
- Predictable request routing for debugging
- Even load distribution for performance testing
- Simple behaviour for development environments

### Benefits

- **Predictable routing** - easy to debug and monitor
- **Fair distribution** - each endpoint gets equal traffic
- **Simple implementation** - minimal overhead
- **No complex logic** - works well for basic setups


## Configuration Examples

### Development Environment

```yaml
proxy:
  load_balancer: "round_robin"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-1"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
      - url: "http://localhost:11435" 
        name: "local-2"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

### Production High-Availability

```yaml
proxy:
  load_balancer: "priority"

discovery:
  type: "static"
  static:
    endpoints:
      # Primary datacenter
      - url: "http://10.1.1.10:11434"
        name: "dc1-primary"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://10.1.1.11:11434"
        name: "dc1-secondary"
        type: "ollama"
        priority: 100          # Same priority as primary
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      # Backup datacenter  
      - url: "http://10.2.1.10:11434"
        name: "dc2-backup"
        type: "ollama"
        priority: 50
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

### Mixed Performance Cluster

```yaml
proxy:
  load_balancer: "least_connections"

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://a100-server:11434"
        name: "high-end-gpu"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://rtx4090-server:11434"
        name: "mid-range-gpu"  
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      - url: "http://cpu-server:11434"
        name: "cpu-only"
        type: "ollama"
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

## Monitoring and Debugging

### Health Check Integration

All strategies automatically exclude unhealthy endpoints. You can check which endpoints are healthy by visiting:
- `http://localhost:40114/internal/status/endpoints`

### Request Tracking

Every response includes a header showing which endpoint handled your request:
- `X-Olla-Endpoint: gpu-server-1`

### Statistics

View load balancing statistics at:
- `http://localhost:40114/internal/status`

## Troubleshooting

### Common Issues

**All requests go to one endpoint (Priority balancer)**
- Check that endpoints have different priorities
- Verify lower priority endpoints are healthy
- Confirm weighted selection is working with same-priority endpoints

**Uneven distribution (Round Robin)**
- Verify all endpoints are reporting as healthy
- Check that endpoint count isn't changing during operation
- Ensure endpoints have similar performance characteristics

**Slow responses (Least Connections)**
- Monitor connection counts - may indicate endpoint performance issues
- Check for connection leaks in client applications
- Verify endpoint health and response times

### Best Practices

1. **Start with Priority** for most deployments
2. **Use Least Connections** for mixed hardware
3. **Reserve Round Robin** for identical endpoints
4. **Monitor endpoint health** regardless of strategy
5. **Test failover behaviour** under load
6. **Configure appropriate priorities** for cost/performance trade-offs

## Priority Guidelines

### Common Priority Schemes

**Cost-Optimised**
- 100: Free/local resources
- 50: Cheap cloud APIs  
- 10: Expensive premium APIs

**Performance-Optimised**
- 100: High-end GPUs (A100s, H100s)
- 75: Mid-range GPUs (RTX 4090s)
- 50: Older GPUs (RTX 3090s)
- 25: CPU inference

**Location-Based**
- 100: Local datacenter
- 50: Regional datacenter
- 10: Cross-region backup

Choose the strategy that best matches your infrastructure setup and operational priorities.