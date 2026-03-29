---
title: Frequently Asked Questions - Olla FAQ
description: Common questions and answers about Olla proxy configuration, troubleshooting, and best practices.
keywords: olla faq, troubleshooting, common questions, proxy help
---

# Frequently Asked Questions

## General Questions

### What is Olla?

Olla is a high-performance proxy and load balancer specifically designed for LLM infrastructure. It intelligently routes requests across multiple LLM backends (Ollama, LM Studio, vLLM, SGLang, Lemonade SDK, LiteLLM, and OpenAI-compatible endpoints) while providing load balancing, health checking, and unified model management.

See how Olla compares to [other tools](compare/overview.md) in the ecosystem.

### Why use Olla instead of connecting directly to backends?

Olla provides several benefits:

- **High availability**: Automatic failover between multiple backends
- **Load balancing**: Distribute requests across multiple GPUs/nodes
- **Unified interface**: Single endpoint for all your LLM services
- **Health monitoring**: Automatic detection and recovery from failures
- **Performance optimisation**: Connection pooling and streaming optimisation

### Which proxy engine should I use?

- **Sherpa (default)**: Use for development, testing, or moderate traffic (< 100 concurrent users)
- **Olla**: Use for production, high traffic, or when you need optimal streaming performance

See [Proxy Engines](concepts/proxy-engines.md) for detailed comparison.

## Configuration

### How do I configure multiple backends?

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
      
      - url: "http://192.168.1.50:11434"
        name: "remote-ollama"
        type: "ollama"
        priority: 80
      
      - url: "http://lmstudio.local:1234"
        name: "lmstudio"
        type: "lm-studio"
        priority: 60
```

Higher priority endpoints are preferred when available.

### What is stream_buffer_size and how should I tune it?

`stream_buffer_size` controls how data is chunked during streaming. It's a crucial performance parameter:

- **Small buffers (2-4KB)**: Lower latency, faster first token for chat
- **Medium buffers (8KB)**: Balanced performance for general use
- **Large buffers (16-64KB)**: Higher throughput for batch processing

See [Stream Buffer Size](concepts/proxy-engines.md#stream-buffer-size) for detailed tuning guide.

### Can I use environment variables for configuration?

Yes, most settings support environment variables:

```bash
OLLA_SERVER_PORT=8080
OLLA_PROXY_ENGINE=olla
OLLA_LOG_LEVEL=debug
```

However, some settings like `proxy.profile` must be set in the YAML configuration file.

## Troubleshooting

### Streaming responses arrive all at once

This usually means `write_timeout` is not set to 0:

```yaml
server:
  write_timeout: 0s  # Required for streaming
```

Also ensure your client supports streaming. For curl, use the `-N` flag.

### Circuit breaker keeps opening

The circuit breaker opens after 3 consecutive failures. Common causes:

1. **Backend is actually down**: Check if the backend is running
2. **Network issues**: Verify connectivity to the backend
3. **Timeout too short**: Increase `check_timeout` in endpoint configuration
4. **Backend overloaded**: The backend might be too slow to respond

### High memory usage

Try these optimisations:

1. Use Sherpa engine instead of Olla (lower memory footprint)
2. Reduce `stream_buffer_size`
3. Lower request size limits
4. Reduce model registry cache time

```yaml
proxy:
  engine: "sherpa"
  stream_buffer_size: 4096  # Smaller buffer

server:
  request_limits:
    max_body_size: 5242880  # 5MB instead of default 100MB
```

### Models not appearing

Model discovery is enabled by default. If models aren't being discovered:

1. Verify it hasn't been explicitly disabled in your configuration:
   ```yaml
   discovery:
     model_discovery:
       enabled: false  # Remove this line or set to true
   ```

2. Verify endpoints are healthy:
   ```bash
   curl http://localhost:40114/internal/status
   ```

3. Check backend APIs directly:
   ```bash
   # Ollama
   curl http://localhost:11434/api/tags
   
   # LM Studio
   curl http://localhost:1234/v1/models
   ```

## Performance

### How many requests can Olla handle?

Performance depends on your configuration:

- **Sherpa engine**: ~1,000 req/s for simple requests
- **Olla engine**: ~10,000 req/s with connection pooling
- Actual LLM inference will be the bottleneck, not Olla

### How do I optimise for low latency?

For minimal latency to first token:

```yaml
proxy:
  engine: "sherpa"
  profile: "streaming"
  stream_buffer_size: 2048  # 2KB for fastest response
  
server:
  write_timeout: 0s
```

### How do I optimise for high throughput?

For maximum throughput:

```yaml
proxy:
  engine: "olla"
  profile: "auto"
  stream_buffer_size: 65536  # 64KB for batch processing
  
discovery:
  model_discovery:
    enabled: false  # Disable if not needed
    
server:
  request_logging: false  # Reduce overhead
```

## Integration

### Does Olla work with OpenAI SDK?

Yes, Olla provides OpenAI-compatible endpoints (similar to [LocalAI](compare/localai.md)):

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:40114/olla/ollama/v1",
    api_key="not-needed"  # Ollama doesn't require API keys
)

response = client.chat.completions.create(
    model="llama3.2",
    messages=[{"role": "user", "content": "Hello"}]
)
```

### How does Olla compare to LiteLLM?

[LiteLLM](compare/litellm.md) is an API translation layer for cloud providers, while Olla is an infrastructure proxy for self-hosted endpoints. They solve different problems and work well together - LiteLLM for cloud APIs, Olla for local infrastructure reliability.

### Can I use Olla with LangChain?

Yes, configure LangChain to use Olla's endpoint:

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:40114/olla/ollama/v1",
    api_key="not-needed",
    model="llama3.2"
)
```

### Does Olla support embeddings?

Yes, Olla proxies embedding requests:

```bash
curl http://localhost:40114/olla/ollama/v1/embeddings \
  -d '{"model":"nomic-embed-text","input":"Hello world"}'
```

## Deployment

### Can Olla deploy models like GPUStack?

No, Olla doesn't deploy models. It routes to existing endpoints. For model deployment across GPU clusters, use [GPUStack](compare/gpustack.md). Olla can then provide routing and failover for GPUStack-managed endpoints.

### Can I run multiple Olla instances?

Yes, you can run multiple instances for high availability:

```nginx
upstream olla_cluster {
    least_conn;
    server olla1:40114;
    server olla2:40114;
}
```

### How do I monitor Olla?

Olla provides several monitoring endpoints:

- `/internal/health` - Basic health check
- `/internal/status` - Detailed status and statistics
- `/internal/status/models` - Model registry information

### What's the recommended production configuration?

```yaml
server:
  request_logging: false  # Reduce overhead
  
proxy:
  engine: "olla"  # High-performance engine
  profile: "auto"
  load_balancer: "least-connections"
  
logging:
  level: "warn"
  format: "json"
  
discovery:
  model_discovery:
    interval: 15m  # Less frequent discovery
```

## Common Issues

### "No healthy endpoints available"

This means all backends are failing health checks. Check:

1. Backends are running
2. URLs are correct in configuration
3. Network connectivity
4. Firewall rules

### "Circuit breaker open"

The circuit breaker has tripped after multiple failures. It will automatically retry after 30 seconds. To manually reset, restart Olla.

### Response headers missing

Olla adds several headers to responses:

- `X-Olla-Endpoint`: Which backend served the request
- `X-Olla-Model`: Model used
- `X-Olla-Response-Time`: Total processing time

If missing, check you're using the `/olla/` prefix in your requests.

### Connection refused errors

Common causes:

1. Olla isn't running on the expected port
2. Firewall blocking the port
3. Binding to localhost vs 0.0.0.0
4. Another service using the port

Check with:
```bash
netstat -tlnp | grep 40114
```

## Best Practices

### Should I use auto proxy profile?

Yes, the `auto` profile intelligently detects whether to stream or buffer based on content type. It's the recommended default for most workloads.

### How often should health checks run?

Balance detection speed vs overhead:

- **Production**: 30-60 seconds
- **Development**: 10-30 seconds
- **Critical systems**: 5-10 seconds

### Should I enable request logging?

Only in development or when debugging. Request logging significantly impacts performance in production.

### How many endpoints should I configure?

- **Minimum**: 2 for redundancy
- **Typical**: 3-5 endpoints
- **Maximum**: No hard limit, but more endpoints increase health check overhead

### Should I use Olla with other tools?

Yes! Olla works well in combination with other tools:

- Use [LiteLLM](compare/litellm.md) for cloud API access
- Use [GPUStack](compare/gpustack.md) for GPU cluster management
- Use [LocalAI](compare/localai.md) for OpenAI compatibility
- See [integration patterns](compare/integration-patterns.md) for architectures

## Getting Help

### Where can I get support?

1. Check this FAQ first
2. Review the [documentation](/)
3. Search [GitHub Issues](https://github.com/thushan/olla/issues)
4. Create a new issue with details

### How do I report a bug?

Create a GitHub issue with:

1. Olla version (`olla version`)
2. Configuration (sanitised)
3. Steps to reproduce
4. Expected vs actual behaviour
5. Relevant logs

### Can I contribute?

Yes! See the [Contributing Guide](development/contributing.md) for details on:

- Code standards
- Testing requirements
- Pull request process
- Development setup