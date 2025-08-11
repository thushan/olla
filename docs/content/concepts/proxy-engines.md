---
title: Proxy Engines - Sherpa vs Olla Performance Comparison
description: Choose between Sherpa (simple, maintainable) and Olla (high-performance) proxy engines. Detailed comparison of memory usage, connection handling, and streaming performance.
keywords: olla proxy engine, sherpa engine, high performance proxy, llm proxy performance, connection pooling
---

# Proxy Engines - Choosing the Right Engine for Your Workload

> :memo: **Default Configuration**
> ```yaml
> proxy:
>   engine: "sherpa"  # sherpa or olla
>   stream_buffer_size: 8192  # Buffer size in bytes
> ```
> **Supported**:
> 
> - `sherpa` _(default)_ - Simple, maintainable proxy
> - `olla` - High-performance proxy
> 
> **Environment Variables**: 
> - `OLLA_PROXY_ENGINE`
> - `OLLA_PROXY_STREAM_BUFFER_SIZE`

Olla offers two proxy engines: **Sherpa** for simplicity and **Olla** for high performance. This guide helps you choose the right engine for your needs.

## Quick Decision Guide

**Use Sherpa if you:**

- Are running in development or testing
- Have moderate traffic (fewer than 100 concurrent users)
- Want simpler debugging and troubleshooting
- Have limited memory resources

**Use Olla if you:**

- Are running in production
- Need to handle high traffic volumes
- Require optimal streaming performance
- Want advanced features like connection pooling

Olla ships with the default setting of using the `Sherpa` engine for a wide variety of use-cases.

## Engine Comparison

| Aspect | Sherpa (default) | Olla |
|--------|--------|------|
| **Performance** | Good for moderate loads | Excellent for high loads |
| **Memory Usage** | Lower memory footprint | Higher due to pooling |
| **Connection Handling** | Shared transport with keep-alive | Per-endpoint connection pools |
| **Streaming** | Standard HTTP streaming (8KB buffer) | Optimised for LLM streaming (64KB buffer) |
| **Best For** | Development, small deployments | Production, enterprise use |

## Configuration

Set your chosen engine in the configuration file:

```yaml
proxy:
  engine: "sherpa"  # or "olla" for production
  profile: "auto"   # Works with both engines
  stream_buffer_size: 8192  # Optional: tune for your workload
```

For streaming optimisation with either engine, see [Proxy Profiles](proxy-profiles.md).

## Sherpa Engine

**Sherpa** is the simple, reliable choice for getting started with Olla.

### When Sherpa is the Right Choice

- **Development environments** where you're testing integrations
- **Small deployments** with a handful of users
- **Limited resources** where memory usage needs to be minimal
- **Debugging scenarios** where simpler code paths help troubleshooting

### Performance Expectations

With Sherpa, expect:

- Reliable performance for typical workloads
- Lower memory usage due to simpler architecture
- Good streaming support for LLM responses
- Straightforward request handling

## Olla Engine

**Olla** is the high-performance engine designed for production workloads.

### When Olla is the Right Choice

- **Production deployments** serving real users
- **High traffic** scenarios with many concurrent requests
- **Streaming-heavy workloads** like chat applications
- **Enterprise environments** requiring maximum performance

### Performance Benefits

With Olla, you get:

- Connection pooling reduces latency by reusing connections
- Optimised buffering for better streaming performance
- Lower per-request overhead through resource pooling
- Better throughput under high load

### Resource Considerations

The Olla engine uses more memory due to:

- Connection pools maintained per backend  
- Buffer pools for efficient streaming
- Request/response object pooling
- Atomic statistics tracking

This extra memory investment pays off through significantly better performance under load.

## Migration Between Engines

Switching engines is seamless - just change the configuration:

```yaml
# Development
proxy:
  engine: "sherpa"

# Production
proxy:
  engine: "olla"
```

No other changes are needed. Both engines:

- Support the same configuration options
- Work with all [proxy profiles](proxy-profiles.md)
- Are compatible with all backends
- Provide identical functionality

## Stream Buffer Size

One of the key performance parameters is `stream_buffer_size`, which controls how data is chunked during streaming operations. This setting significantly impacts both latency and throughput.

### Understanding Buffer Sizes

The buffer size determines how much data is read from the backend before forwarding to the client:

| Buffer Size | First Token Latency | Throughput | Memory per Request | Best For |
|-------------|-------------------|------------|-------------------|----------|
| **2KB** | Fastest (~5ms) | Lower | Minimal | Interactive chat with immediate feedback |
| **4KB** | Fast (~10ms) | Moderate | Low | Balanced chat applications |
| **8KB** (Sherpa default) | Moderate (~20ms) | Good | Moderate | General-purpose workloads |
| **16KB** | Slower (~40ms) | Better | Higher | Bulk operations, embeddings |
| **64KB** (Olla default) | Slowest (~150ms) | Best | Highest | High-throughput batch processing |

### How Engines Use Buffers

**Sherpa Engine (8KB default):**
- Allocates buffers from a shared pool
- Single buffer per active request
- Optimised for moderate concurrency
- Lower memory footprint

**Olla Engine (64KB default):**
- Per-endpoint buffer pools
- Multiple buffers pre-allocated
- Optimised for high concurrency
- Better throughput at cost of memory

### Tuning Buffer Size

```yaml
# Interactive chat - prioritise low latency
proxy:
  engine: "sherpa"
  stream_buffer_size: 4096  # 4KB for faster first token
  profile: "streaming"

# High-throughput API - prioritise throughput
proxy:
  engine: "olla"
  stream_buffer_size: 65536  # 64KB for maximum throughput
  profile: "auto"

# Balanced workload - default settings
proxy:
  engine: "sherpa"
  stream_buffer_size: 8192  # 8KB balanced approach
  profile: "auto"
```

### Performance Impact

Buffer size affects streaming in several ways:

1. **Latency to First Token**: Smaller buffers deliver tokens faster
2. **System Call Overhead**: Larger buffers mean fewer read/write operations
3. **Memory Usage**: Larger buffers consume more memory per connection
4. **Network Efficiency**: Larger buffers can better utilise network bandwidth

### Recommendations by Use Case

**Real-time Chat Applications:**
```yaml
stream_buffer_size: 2048  # 2KB - Minimise latency
```

**Standard API Serving:**
```yaml
stream_buffer_size: 8192  # 8KB - Balanced performance
```

**Batch Processing & Embeddings:**
```yaml
stream_buffer_size: 32768  # 32KB - Maximise throughput
```

**High-Volume Production:**
```yaml
stream_buffer_size: 65536  # 64KB - Optimal for Olla engine
```

## Performance Tuning

### For Sherpa

Sherpa works well out of the box. For best results:

```yaml
proxy:
  engine: "sherpa"
  profile: "auto"  # Let Olla detect the best streaming mode
  stream_buffer_size: 8192  # 8KB default, adjust based on use case
  connection_timeout: 30s
```

### For Olla

Olla benefits from tuning for your workload:

```yaml
proxy:
  engine: "olla"
  profile: "auto"  # Or "streaming" for chat applications
  stream_buffer_size: 65536  # 64KB for high throughput
  connection_timeout: 60s  # Longer reuse for connection pooling
```

See [Performance Best Practices](../configuration/practices/performance.md) for detailed tuning guidance including buffer size optimisation.

## Monitoring Your Choice

Both engines expose the same monitoring endpoints:

```bash
# Check which engine is running
curl http://localhost:40114/internal/status | jq '.proxy.engine'
```

Monitor these metrics to validate your engine choice:

- **Response times**: Should meet your SLA requirements
- **Memory usage**: Should fit within your resource limits
- **Error rates**: Should remain low under normal load

## Common Questions

### Can I switch engines without downtime?

Yes, but you'll need to restart Olla with the new configuration. Consider running multiple instances behind a load balancer for zero-downtime updates.

### Which engine do most users choose?

- Development: Most use Sherpa for simplicity
- Production: Most use Olla for performance

The `auto` proxy profile works great with both but learn more about [proxy profiles](proxy-profiles.md).

### Does engine choice affect my LLM backends?

No, both engines work identically with all supported backends (Ollama, LM Studio, vLLM, etc.). The difference is purely in how Olla handles the proxying.

## Next Steps

- Configure [Proxy Profiles](proxy-profiles.md) for optimal streaming
- Set up [Load Balancing](load-balancing.md) for multiple backends
- Review [Performance Best Practices](../configuration/practices/performance.md)
- Monitor with [Health Checking](health-checking.md)