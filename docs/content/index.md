<div align="center">
  <img src="assets/images/banner.png" alt="Olla - LLM Proxy & Load Balancer" style="max-width: 100%; height: auto;">
  <p>
    <a href="https://github.com/thushan/olla/blob/master/LICENSE"><img src="https://img.shields.io/github/license/thushan/olla" alt="License"></a>
    <a href="https://golang.org/"><img src="https://img.shields.io/github/go-mod/go-version/thushan/olla" alt="Go"></a>
    <a href="https://github.com/thushan/olla/actions/workflows/ci.yml"><img src="https://github.com/thushan/olla/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/thushan/olla"><img src="https://goreportcard.com/badge/github.com/thushan/olla" alt="Go Report Card"></a>
    <a href="https://github.com/thushan/olla/releases/latest"><img src="https://img.shields.io/github/release/thushan/olla" alt="Latest Release"></a> <br />
    <a href="https://ollama.com"><img src="https://img.shields.io/badge/Ollama-native-lightgreen.svg" alt="Ollama: Native Support"></a> 
    <a href="https://lmstudio.ai/"><img src="https://img.shields.io/badge/LM Studio-native-lightgreen.svg" alt="LM Studio: Native Support"></a> 
    <a href="https://github.com/vllm-project/vllm"><img src="https://img.shields.io/badge/vLLM-openai-lightblue.svg" alt="vLLM: OpenAI Compatible"></a> 
    <a href="https://github.com/lemonade-sdk/lemonade"><img src="https://img.shields.io/badge/Lemonade-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
    <a href="https://github.com/InternLM/lmdeploy"><img src="https://img.shields.io/badge/LM Deploy-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
  </P>
</div>

Olla is a high-performance, low-overhead, low-latency proxy, model unifier and load balancer for managing LLM infrastructure. 

It intelligently routes LLM requests across local and remote inference nodes - including [Ollama](https://github.com/ollama/ollama), [LM Studio](https://lmstudio.ai/) and OpenAI-compatible endpoints like [vLLM](https://github.com/vllm-project/vllm). Olla provides model discovery and unified model catalogues within each provider, enabling seamless routing to available models on compatible endpoints.

## Key Features

- **Unified Model Registry**: Unifies models registered across instances (of the same type - Eg. Ollama or LMStudio)
- **Dual Proxy Engines**: Choose between Sherpa (simple, maintainable) and Olla (high-performance with advanced features)
- **Intelligent Load Balancing**: Priority-based, round-robin, and least-connections strategies
- **Health Monitoring**: Circuit breakers and automatic failover
- **High Performance**: Connection pooling, object pooling, and lock-free statistics
- **Security**: Built-in rate limiting and request validation
- **Observability**: Comprehensive metrics and request tracing

## Use Cases

### 🏠 Home Lab & Personal Use

Perfect for enthusiasts running multiple LLM instances:

- **Multi-GPU Setups**: Route between different models on various GPUs
- **Model Experimentation**: Easy switching between Ollama, LM Studio and OpenAI backends
- **Resource Management**: Automatic failover when local resources are busy
- **Cost Optimisation**: Priority routing (local first, cloud fallback)

```yaml
# Home lab config - local first, home-lab second
endpoints:
  - name: "rtx-4090-mobile"
    url: "http://localhost:11434" 
    priority: 100  # Highest priority
  - name: "home-lab-rtx-6000"
    url: "https://192.168.0.1:11434"
    priority: 10   # Fallback only
```

### 🏢 Business & Teams

Streamline AI infrastructure for growing teams:

- **Department Isolation**: Route different teams to appropriate endpoints
- **Budget Controls**: Rate limiting and usage tracking per team
- **High Availability**: Load balancing across multiple inference servers
- **Development Staging**: Separate dev/staging/prod model routing

```yaml
# Business config - load balanced production
load_balancer: "least-connections"
rate_limits:
  per_ip_requests_per_minute: 100
  global_requests_per_minute: 1000
```

### 🏭 Enterprise & Production

Mission-critical AI infrastructure at scale:

- **Multi-Region Deployment**: Geographic load balancing and failover
- **Enterprise Security**: Rate limiting, request validation, audit trails  
- **Performance Monitoring**: Circuit breakers, health checks, metrics
- **Vendor Diversity**: Mix of cloud providers and on-premise infrastructure

```yaml
# Enterprise config - high performance, observability
proxy:
  engine: "olla"  # High-performance engine
  max_retries: 3
server:
  request_logging: true
  rate_limits:
    global_requests_per_minute: 10000
```

---

## Quick Start

Get up and running with Olla in minutes:

=== "Using Docker"
    ```bash
    # If you have ollama or lmstudio locally
    docker run -t -p 40114:40114 ghcr.io/thushan/olla:latest
    ```

=== "Using Go"

    ```bash
    go install github.com/thushan/olla@latest
    olla
    ```

=== "From Source"

    ```bash
    git clone https://github.com/thushan/olla.git
    cd olla
    make build-release
    ./olla
    ```

## Response Headers

Olla provides detailed response headers for observability:

| Header | Description |
|--------|-------------|
| `X-Olla-Endpoint` | Backend endpoint name |
| `X-Olla-Model` | Model used for the request |
| `X-Olla-Backend-Type` | Backend type (ollama/openai/lmstudio) |
| `X-Olla-Request-ID` | Unique request identifier |
| `X-Olla-Response-Time` | Total processing time |

## Why Olla?

- **Production Ready**: Built for high-throughput production environments
- **Flexible**: Works with any OpenAI-compatible endpoint
- **Observable**: Rich metrics and tracing out of the box
- **Reliable**: Circuit breakers and automatic failover
- **Fast**: Optimised for minimal latency and maximum throughput

## Next Steps

- [Installation Guide](getting-started/installation.md) - Get Olla installed
- [Quick Start](getting-started/quickstart.md) - Basic setup and configuration
- [Architecture Overview](architecture/overview.md) - Understand how Olla works
- [Configuration Reference](config/reference.md) - Complete configuration options

## Community

- 🐛 [Report Issues](https://github.com/thushan/olla/issues)
- 💡 [Feature Requests](https://github.com/thushan/olla/discussions)
- 📖 [Documentation](https://thushan.github.io/olla/)
- ⭐ [Star on GitHub](https://github.com/thushan/olla)