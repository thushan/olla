# Load Balancing Strategies

Olla provides three load balancing algorithms, each suited to different operational requirements. The load balancer sits at the heart of the proxy, making routing decisions that directly impact performance, reliability, and resource utilisation.

## Architecture Overview

All load balancers implement the EndpointSelector interface:

```go
type EndpointSelector interface {
    Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error)
    Name() string
    IncrementConnections(endpoint *domain.Endpoint)
    DecrementConnections(endpoint *domain.Endpoint)
}
```

This interface provides endpoint selection with connection tracking capabilities. Statistics are collected through a separate StatsCollector interface.

## Priority Balancer (Recommended)

The priority balancer is our recommended strategy for production deployments. It provides intelligent failover while respecting your infrastructure hierarchy.

### How It Works

```go
type PrioritySelector struct {
    statsCollector ports.StatsCollector
}

func (p *PrioritySelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
    if len(endpoints) == 0 {
        return nil, fmt.Errorf("no endpoints available")
    }
    
    // Filter only routable endpoints
    routable := make([]*domain.Endpoint, 0, len(endpoints))
    for _, endpoint := range endpoints {
        if endpoint.Status.IsRoutable() {
            routable = append(routable, endpoint)
        }
    }
    
    if len(routable) == 0 {
        return nil, fmt.Errorf("no routable endpoints available")
    }
    
    // Sort by priority (highest first)
    sort.Slice(routable, func(i, j int) bool {
        return routable[i].Priority > routable[j].Priority
    })
    
    // Get the highest priority tier
    highestPriority := routable[0].Priority
    highestPriorityEndpoints := make([]*domain.Endpoint, 0)
    
    for _, endpoint := range routable {
        if endpoint.Priority == highestPriority {
            highestPriorityEndpoints = append(highestPriorityEndpoints, endpoint)
        } else {
            break // Since sorted, we can break early
        }
    }
    
    // Use weighted selection within the priority group
    return p.weightedSelect(highestPriorityEndpoints), nil
}
```

### Priority Groups

Endpoints are organised into priority tiers:

```yaml
discovery:
  endpoints:
    - name: "local-ollama"
      url: "http://localhost:11434"
      priority: 1  # Highest priority
      
    - name: "gpu-server-1"
      url: "http://10.0.1.10:11434"
      priority: 2  # Secondary
      
    - name: "gpu-server-2"
      url: "http://10.0.1.11:11434"
      priority: 2  # Same tier as gpu-server-1
      
    - name: "cloud-backup"
      url: "https://api.provider.com"
      priority: 3  # Last resort
```

### Weighted Selection

Within a priority group, selection is weighted by endpoint status:

```go
func (p *PrioritySelector) weightedSelect(endpoints []*domain.Endpoint) *domain.Endpoint {
    if len(endpoints) == 1 {
        return endpoints[0]
    }
    
    // Calculate total weight based on endpoint status
    totalWeight := 0.0
    for _, endpoint := range endpoints {
        totalWeight += endpoint.Status.GetTrafficWeight()
    }
    
    // All endpoints have 0 weight, fallback to random selection
    if totalWeight == 0 {
        return endpoints[rand.Intn(len(endpoints))]
    }
    
    // Weighted random selection
    r := rand.Float64() * totalWeight
    weightSum := 0.0
    
    for _, endpoint := range endpoints {
        weightSum += endpoint.Status.GetTrafficWeight()
        if r <= weightSum {
            return endpoint
        }
    }
    
    // Fallback (shouldn't reach here)
    return endpoints[len(endpoints)-1]
}
```

The traffic weights are defined by endpoint status:
- `StatusHealthy`: 1.0 (full traffic)
- `StatusBusy`: 0.3 (reduced traffic)
- `StatusWarming`: 0.1 (minimal traffic)
- Other statuses: 0.0 (no traffic)

### Use Cases

The priority balancer excels in:

1. **Hybrid Infrastructure**: Mix local, private cloud, and public cloud endpoints
2. **Cost Optimization**: Use expensive endpoints only when necessary
3. **Compliance Requirements**: Keep sensitive data on specific endpoints
4. **Progressive Rollouts**: Test new endpoints at lower priority

### Configuration Example

```yaml
proxy:
  load_balancer: "priority"
  
discovery:
  endpoints:
    # Local development
    - name: "local"
      url: "http://localhost:11434"
      priority: 1
      platform: "ollama"
      
    # Primary production
    - name: "prod-primary"
      url: "http://gpu-cluster.internal:11434"
      priority: 2
      platform: "ollama"
      
    # Failover
    - name: "prod-secondary"
      url: "http://gpu-cluster-2.internal:11434"
      priority: 3
      platform: "ollama"
      
    # Cloud backup (expensive)
    - name: "openai-backup"
      url: "https://api.openai.com"
      priority: 10
      platform: "openai"
```

## Round Robin Balancer

Round-robin provides simple, predictable load distribution across all healthy endpoints.

### Implementation

```go
type RoundRobinSelector struct {
    statsCollector ports.StatsCollector
    counter        uint64
}

func (r *RoundRobinSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
    if len(endpoints) == 0 {
        return nil, fmt.Errorf("no endpoints available")
    }
    
    // Filter routable endpoints
    routable := make([]*domain.Endpoint, 0, len(endpoints))
    for _, endpoint := range endpoints {
        if endpoint.Status.IsRoutable() {
            routable = append(routable, endpoint)
        }
    }
    
    if len(routable) == 0 {
        return nil, fmt.Errorf("no routable endpoints available")
    }
    
    // Atomic increment and modulo
    current := atomic.AddUint64(&r.counter, 1) - 1
    index := current % uint64(len(routable))
    
    return routable[index], nil
}
```

### Characteristics

- **Predictable**: Requests cycle through endpoints in order
- **Fair**: Equal distribution regardless of endpoint capacity
- **Stateless**: No consideration of endpoint performance
- **Simple**: Minimal overhead and easy to debug

### Use Cases

Round-robin works well for:

1. **Homogeneous Clusters**: Identical endpoints with same capacity
2. **Development Environments**: Predictable behaviour for testing
3. **Even Load Distribution**: When all endpoints should share load equally
4. **Simple Requirements**: When sophisticated routing isn't needed

### Limitations

- Doesn't account for endpoint capacity differences
- No consideration of latency or success rates
- Can overload slower endpoints
- No built-in failover hierarchy

## Least Connections Balancer

The least connections strategy routes requests to the endpoint with the fewest active connections, helping prevent overload.

### Implementation

```go
type LeastConnectionsSelector struct {
    statsCollector ports.StatsCollector
}

func (l *LeastConnectionsSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
    if len(endpoints) == 0 {
        return nil, fmt.Errorf("no endpoints available")
    }
    
    // Filter routable endpoints
    routable := make([]*domain.Endpoint, 0, len(endpoints))
    for _, endpoint := range endpoints {
        if endpoint.Status.IsRoutable() {
            routable = append(routable, endpoint)
        }
    }
    
    if len(routable) == 0 {
        return nil, fmt.Errorf("no routable endpoints available")
    }
    
    // Get current connection counts from stats collector
    connectionStats := l.statsCollector.GetConnectionStats()
    
    // Find endpoint with least number of connections
    var selected *domain.Endpoint
    minConnections := int64(-1)
    
    for _, endpoint := range routable {
        key := endpoint.URL.String()
        connections := connectionStats[key] // Will be 0 if not found
        
        if minConnections == -1 || connections < minConnections {
            minConnections = connections
            selected = endpoint
        }
    }
    
    return selected, nil
}
```

### Connection Tracking

Connection tracking is managed through the StatsCollector:

```go
// In the proxy implementation
func (p *Proxy) proxyRequest(endpoint *domain.Endpoint, req *http.Request) (*http.Response, error) {
    selector.IncrementConnections(endpoint)
    defer selector.DecrementConnections(endpoint)
    
    // Perform request
    return p.client.Do(req)
}
```

The selector delegates to the StatsCollector which maintains connection counts per endpoint URL.

### Characteristics

- **Dynamic**: Adapts to real-time load
- **Protective**: Prevents endpoint overload
- **Responsive**: Quickly shifts load from busy endpoints
- **Fair**: Accounts for request duration, not just count

### Use Cases

Least connections excels when:

1. **Variable Request Duration**: LLM requests with different model sizes
2. **Heterogeneous Capacity**: Endpoints with different GPU counts
3. **Bursty Traffic**: Sudden load spikes need distribution
4. **Long-Running Requests**: Streaming responses that hold connections


## Choosing the Right Strategy

### Decision Matrix

| Scenario | Recommended Strategy | Reasoning |
|----------|---------------------|-----------|
| Mixed local/cloud endpoints | Priority | Cost and latency optimisation |
| Identical GPU servers | Round Robin | Simple and fair |
| Variable model sizes | Least Connections | Adapts to request duration |
| Development/testing | Round Robin | Predictable behaviour |
| High availability production | Priority | Controlled failover |
| Heterogeneous hardware | Least Connections | Accounts for capacity |

### Performance Comparison

Based on our benchmarks with 10 endpoints and mixed workloads:

```
Priority Balancer:
- Selection latency: 0.8µs average
- Memory overhead: 2KB per endpoint
- Failover time: <1ms

Round Robin:
- Selection latency: 0.2µs average
- Memory overhead: 8 bytes total
- No failover hierarchy

Least Connections:
- Selection latency: 1.2µs average
- Memory overhead: 4 bytes per endpoint
- Adaptive to load
```

## Advanced Configuration

### Priority with Health Checks

```yaml
proxy:
  load_balancer: "priority"
  
health:
  interval: 30s
  timeout: 5s
  threshold: 3  # Failures before unhealthy
  
discovery:
  endpoints:
    - name: "primary"
      priority: 1
      health_check:
        path: "/api/tags"  # Ollama-specific
        expected_status: 200
```

### Least Connections with Limits

```yaml
proxy:
  load_balancer: "least_connections"
  
discovery:
  endpoints:
    - name: "small-gpu"
      max_connections: 10
      
    - name: "large-gpu"
      max_connections: 50
```

### Custom Weighting

For advanced use cases, you can implement custom weighting:

```go
type CustomBalancer struct {
    baseBalancer LoadBalancer
}

func (c *CustomBalancer) Select(endpoints []domain.Endpoint) *domain.Endpoint {
    // Apply custom logic
    scored := c.scoreEndpoints(endpoints)
    return c.baseBalancer.Select(scored)
}

func (c *CustomBalancer) scoreEndpoints(endpoints []domain.Endpoint) []domain.Endpoint {
    // Custom scoring based on:
    // - Time of day (prefer local during business hours)
    // - Cost metrics
    // - SLA requirements
    // - Model availability
}
```

## Monitoring and Debugging

### Key Metrics

Monitor these metrics for each strategy:

**Priority Balancer**
```
load_balancer.priority.selections_by_tier{tier="1"}: 8532
load_balancer.priority.selections_by_tier{tier="2"}: 467
load_balancer.priority.selections_by_tier{tier="3"}: 12
load_balancer.priority.failover_events: 23
```

**Round Robin**
```
load_balancer.round_robin.selections{endpoint="gpu-1"}: 3341
load_balancer.round_robin.selections{endpoint="gpu-2"}: 3339
load_balancer.round_robin.selections{endpoint="gpu-3"}: 3342
load_balancer.round_robin.cycle_time: 0.8ms
```

**Least Connections**
```
load_balancer.least_conn.selections{endpoint="gpu-1"}: 2841
load_balancer.least_conn.rebalance_events: 156
load_balancer.least_conn.overload_skips: 23
```

### Debug Logging

Enable debug logging to trace routing decisions:

```yaml
logging:
  level: debug
  
# Logs will show:
# DEBUG Selected endpoint=gpu-1 strategy=priority tier=1 healthy_count=3
# DEBUG Skipped endpoint=gpu-2 reason=unhealthy last_check=2024-01-15T10:30:00Z
# DEBUG Failover from tier=1 to tier=2 reason=no_healthy_endpoints
```

## Best Practices

1. **Start with Priority**: It's the most flexible and forgiving strategy

2. **Monitor Actively**: Watch for imbalanced load distribution

3. **Test Failover**: Regularly verify your failover paths work

4. **Configure Health Checks**: Accurate health status is critical for all strategies

5. **Consider Costs**: Factor in API costs when designing priority tiers

6. **Plan for Growth**: Choose a strategy that scales with your infrastructure

## Troubleshooting Common Issues

### Uneven Load Distribution

**Symptoms**: Some endpoints handle more traffic than others

**Solutions**:
- Verify all endpoints are marked healthy
- Check for connection limit differences
- Review priority configuration
- Consider switching strategies

### Slow Failover

**Symptoms**: Requests fail before trying other endpoints

**Solutions**:
- Reduce health check interval
- Lower failure threshold
- Implement circuit breakers
- Use more aggressive timeouts

### Endpoint Starvation

**Symptoms**: Some endpoints receive no traffic

**Solutions**:
- Check priority configuration
- Verify endpoint health status
- Review connection limits
- Consider round-robin for fair distribution

The load balancer is a critical component that directly impacts your LLM infrastructure's performance, reliability, and cost. Choose wisely based on your specific requirements, and don't hesitate to switch strategies as your needs evolve.