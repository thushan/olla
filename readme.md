<div align="center">
    <img src="assets/images/banner.png" width="480" height="249" alt="Olla - Smart LLM Load Balancer & Proxy" />
  <p>
    <a href="https://github.com/thushan/olla/blob/master/LICENSE"><img src="https://img.shields.io/github/license/thushan/olla" alt="License"></a>
    <a href="https://github.com/thushan/olla/actions/workflows/ci.yml"><img src="https://github.com/thushan/olla/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/thushan/olla"><img src="https://goreportcard.com/badge/github.com/thushan/olla" alt="Go Report Card"></a>
    <a href="https://github.com/thushan/olla/releases/latest"><img src="https://img.shields.io/github/release/thushan/olla" alt="Latest Release"></a> <br />
    <a href="https://ollama.com"><img src="https://img.shields.io/badge/Ollama-native-lightgreen.svg" alt="Ollama: Native Support"></a> 
    <a href="https://lmstudio.ai/"><img src="https://img.shields.io/badge/LM Studio-native-lightgreen.svg" alt="LM Studio: Native Support"></a> 
    <a href="https://github.com/vllm-project/vllm"><img src="https://img.shields.io/badge/vLLM-native-lightgreen.svg" alt="vLLM: Native Support"></a> 
    <a href="https://github.com/lemonade-sdk/lemonade"><img src="https://img.shields.io/badge/Lemonade-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
    <a href="https://github.com/InternLM/lmdeploy"><img src="https://img.shields.io/badge/LM Deploy-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
  </P>
  <p>
    <div align="center">
    <img src="./docs/content/assets/demos/olla-v1.0.x-demo.gif" height="" width="" /><br/>
    <small>Recorded with <a href="https://vhs.charm.sh/">VHS</a> - see <a href="./docs/vhs/demo.tape">demo tape</a></small><br/><br/>
    </div>
    <a href="https://thushan.github.io/olla/"><img src="https://img.shields.io/badge/📖_Documentation-0078D4?style=for-the-badge&logoColor=white" height="32" alt="Documentation"></a>
    &nbsp;
    <a href="https://github.com/thushan/olla/issues"><img src="https://img.shields.io/badge/🐛_Issues-D73502?style=for-the-badge&logoColor=white" height="32" alt="Issues"></a>
    &nbsp;
    <a href="https://github.com/thushan/olla/releases"><img src="https://img.shields.io/badge/🚀_Releases-6f42c1?style=for-the-badge&logoColor=white" height="32" alt="Releases"></a>
  </p>
</div>

> [!IMPORTANT]  
> Olla is currently **in active-development**. While it is usable, we are still finalising some features and optimisations. 
> Your feedback is invaluable! Open <a href="https://github.com/thushan/olla/issues">an issue</a> and let us know features you'd like to see in the future.

Olla is a high-performance, low-overhead, low-latency proxy and load balancer for managing LLM infrastructure. It intelligently routes LLM requests across local and remote inference nodes - including [Ollama](https://github.com/ollama/ollama), [LM Studio](https://lmstudio.ai/) and OpenAI-compatible endpoints like [vLLM](https://github.com/vllm-project/vllm). Olla provides model discovery and unified model catalogues within each provider, enabling seamless routing to available models on compatible endpoints.

You can choose between two proxy engines: **Sherpa** for simplicity and maintainability or **Olla** for maximum performance with advanced features like circuit breakers and connection pooling.

Single CLI application and config file is all you need to go Olla!

![Olla Usecase](assets/diagrams/usecases.excalidraw.png)

In the above example, we configure [Jetbrains Junie](https://www.jetbrains.com/junie/) to use Olla for its Ollama and LMStudio endpoints for local-ai inference with Junie.

## Key Features

- **🔄 Smart Load Balancing**: [Priority-based routing](https://thushan.github.io/olla/concepts/load-balancing/) with automatic failover and connection retry
- **🔍 Smart Model Unification**: [Per-provider unification + OpenAI-compatible cross-provider routing](https://thushan.github.io/olla/concepts/model-unification/)
- **⚡ Dual Proxy Engines**: [Sherpa (simple) and Olla (high-performance)](https://thushan.github.io/olla/concepts/proxy-engines/)
- **💊 Health Monitoring**: [Continuous endpoint health checks](https://thushan.github.io/olla/concepts/health-checking/) with circuit breakers and automatic recovery
- **🔁 Intelligent Retry**: Automatic retry on connection failures with immediate endpoint failover
- **🔧 Self-Healing**: Automatic model discovery refresh when endpoints recover
- **📊 Request Tracking**: Detailed response headers and [statistics](https://thushan.github.io/olla/api-reference/overview/#response-headers)
- **🛡️ Production Ready**: Rate limiting, request size limits, graceful shutdown
- **⚡ High Performance**: Sub-millisecond endpoint selection with lock-free atomic stats
- **🎯 LLM-Optimised**: Streaming-first design with optimised timeouts for long inference
- **⚙️ High Performance**: Designed to be very [lightweight & efficient](https://thushan.github.io/olla/configuration/practices/performance/), runs on less than 50Mb RAM.

### Supported Backends

Olla natively supports the following backend providers. Learn more about [Olla Integrations](https://thushan.github.io/olla/integrations/overview/).

* [Ollama](https://github.com/ollama/ollama) - native support for Ollama, including model unification. \
  Use: `/olla/ollama/`
* [LM Studio](https://lmstudio.ai/) - native support for LMStudio, including model unification. \
  Use: `/olla/lmstudio/` || `/olla/lm-studio/` || `/olla/lm_studio/`
* [vLLM](https://github.com/vllm-project/vllm) - native support for vllm, including model unification. \
  Use: `/olla/vllm/` \
  Models from vLLM will be available under `/olla/models` and `/olla/vllm/v1/models`
* [OpenAI](https://platform.openai.com/docs/overview) - You can use OpenAI API that provides a unified query API across all providers. \
  Use: `/olla/openai/`

### Supported Frontends

* [OpenWebUI](https://github.com/open-webui/open-webui) - You can route you [OpenWebUI Ollama endpoint to Olla](https://thushan.github.io/olla/integrations/frontend/openwebui/) and serve unified models easily.

Coming soon - but you can use the OpenAI compatibility in the interim:

* [LMDeploy](https://github.com/InternLM/lmdeploy)
* [Lemonade](https://github.com/lemonade-sdk/lemonade)

Learn more about [Olla's Profile System](https://thushan.github.io/olla/concepts/profile-system/).

## Platform Support

Olla runs on multiple platforms and architectures:

| Platform | AMD64 | ARM64 | Notes |
|----------|-------|-------|-------|
| Linux    | ✅ | ✅ | Full support including Raspberry Pi 4+ |
| macOS    | ✅ | ✅ | Intel and Apple Silicon (M1/M2/M3/M4) |
| Windows  | ✅ | ✅ | Windows 10/11 and Windows on ARM |
| Docker   | ✅ | ✅ | Multi-architecture images (amd64/arm64) |

## Quick Start

### Installation

```bash
# Download latest release (auto-detects your platform)
bash <(curl -s https://raw.githubusercontent.com/thushan/olla/main/install.sh)
```
```bash
# Docker (automatically pulls correct architecture)
docker run -t \
  --name olla \
  -p 40114:40114 \
  ghcr.io/thushan/olla:latest

# Or explicitly specify platform (e.g., for ARM64)
docker run --platform linux/arm64 -t \
  --name olla \
  -p 40114:40114 \
  ghcr.io/thushan/olla:latest
```
```bash
# Install via Go
go install github.com/thushan/olla@latest
```
```bash
# Build from source
git clone https://github.com/thushan/olla.git && cd olla && make build-release
# Run Olla
./bin/olla
```

When you have things running you can check everything's working with:

```bash
# Check health of Olla
curl http://localhost:40114/internal/health

# Check endpoints
curl http://localhost:40114/internal/status/endpoints

# Check models available
curl http://localhost:40114/internal/status/models
```

For detailed installation and deployment options, see [Getting Started Guide](https://thushan.github.io/olla/getting-started/quickstart/).

## Examples

We've also got ready-to-use Docker Compose setups for common scenarios:

### 🌐 **OpenWebUI Integration**

Complete setup with [OpenWebUI](https://github.com/open-webui/open-webui) + Olla load balancing multiple Ollama instances.

- **See**: [`examples/ollama-openwebui/`](examples/ollama-openwebui/)
- **Services**: OpenWebUI (web UI) + Olla (proxy/load balancer)  
- **Use Case**: Web interface with intelligent load balancing across multiple Ollama servers with Olla
- **Quick Start**: 
  ```bash
  cd examples/ollama-openwebui
  # Edit olla.yaml to configure your Ollama endpoints
  docker compose up -d
  # Access OpenWebUI at http://localhost:3000
  ```

You can learn more about [OpenWebUI Ollama with Olla](https://thushan.github.io/olla/integrations/frontend/openwebui/).

More examples coming soon:
- **Multi-Provider Setup**: Ollama + LM Studio + OpenAI-compatible endpoints
- **High-Availability**: Production deployment with failover
- **Kubernetes**: K8s manifests for container orchestration

## Basic Configuration

Modify the existing `config.yaml` or create a copy:

```yaml
server:
  host: "0.0.0.0"           # to allow external connections
  port: 40114
  ...

proxy:
  engine: "sherpa"          # or "olla" for high performance
  profile: "auto"           # or 'streaming' or 'standard'
  load_balancer: "priority" # or round-robin, least-connections
  ...

discovery:
  type: "static"
  static:
    endpoints:
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100         # Higher = preferred
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s

      - url: "https://ollama-42.acmecorp.com/"
        name: "work-ollama"
        type: "ollama"
        priority: 50          # Lower priority fallback
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

For comprehensive configuration options, see [Configuration Reference](https://thushan.github.io/olla/configuration/reference/).

### Start Olla

```bash
./olla                    # Uses config.yaml
# or
./olla -c custom.yaml     # Custom config
```

## Usage

### API Compatibility & Smart Routing

Olla is **OpenAI, LM Studio and Ollama compatible**. The magic is in the unified OpenAI-compatible endpoint:

```python
import openai

# One client, multiple providers seamlessly
client = openai.OpenAI(base_url="http://localhost:40114/olla/openai")

# This routes to Ollama automatically  
response = client.chat.completions.create(
    model="llama3.2:latest",
    messages=[{"role": "user", "content": "Hello!"}]
)

# This routes to LM Studio automatically
response = client.chat.completions.create(
    model="microsoft/phi-4-mini-reasoning", 
    messages=[{"role": "user", "content": "Hello!"}]
)

# Your code doesn't need to know or care where models are hosted!
```

### Provider Examples

**Ollama Provider**
```bash
# Chat via Ollama (OpenAI-compatible)
curl -X POST http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Ollama native API
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "prompt": "Hello!"
  }'

# List Ollama models (native format)
curl http://localhost:40114/olla/ollama/api/tags
```

**LM Studio Provider**
```bash
# Chat via LM Studio
curl -X POST http://localhost:40114/olla/lmstudio/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-model",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# List LM Studio models
curl http://localhost:40114/olla/lmstudio/v1/models
```

### Model Discovery & Smart Unification

Olla's magic lies in its intelligent model discovery and routing. It unifies models within each provider type while providing seamless cross-provider access via OpenAI-compatible APIs.

```bash
# Discover all models (per-provider unification)
curl http://localhost:40114/olla/models
# Example: 15 Ollama + 6 LM Studio = 21 total models

# The beauty: One API, multiple backends
curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -d '{"model": "llama3.2:latest", "messages": [...]}'
# ↳ Routes to Ollama automatically

curl -X POST http://localhost:40114/olla/openai/v1/chat/completions \
  -d '{"model": "microsoft/phi-4-mini-reasoning", "messages": [...]}'
# ↳ Routes to LM Studio automatically
```

**The Power:** Use one OpenAI client to seamlessly access models from Ollama, LM Studio, and other providers without knowing where they're hosted.

For detailed information about model discovery and unification, see [Model Discovery Guide](https://thushan.github.io/olla/concepts/model-unification/).

### Key Endpoints

| Type | Example | Description |
|------|---------|-------------|
| **🎯 Smart Routing** | `/olla/openai/v1/chat/completions` | OpenAI Compatible API, auto-routes to any provider based on model |
| **Ollama Routes** | `/olla/ollama/v1/chat/completions` | Explicit routing to Ollama endpoints only |
| **LM Studio Routes** | `/olla/lmstudio/v1/chat/completions` | Explicit routing to LM Studio endpoints only |
| **Unified Discovery** | `/olla/models` | All models across all providers (21 total) |
| **Provider Discovery** | `/olla/ollama/v1/models` <br/> `/olla/lmstudio/v1/models` | Models from specific provider type |
| **Health & Status** | `/internal/health` | Monitor system health |

For complete endpoint documentation, see [API Endpoints](https://thushan.github.io/olla/api-reference/overview/).

### Response Headers

Every request includes tracking information:

```
X-Olla-Endpoint: local-ollama     # Which backend handled it
X-Olla-Model: llama4              # Model used
X-Olla-Backend-Type: ollama       # Platform type
X-Olla-Request-ID: req_abc123     # For debugging
X-Olla-Response-Time: 1.234s      # Total time (trailer)
```

For API usage patterns and examples, see [API Endpoints Reference](https://thushan.github.io/olla/api-reference/overview/).

## Configuration

### ⚖️ Load Balancing Strategies

- **Least Connections**: Routes to endpoint with fewest active connections (recommended for businesses)
- **Priority**: Routes to highest priority healthy endpoint (recommended for home)
- **Round Robin**: Even distribution across all endpoints

For detailed strategy selection and configuration, see [Load Balancing Guide](https://thushan.github.io/olla/concepts/load-balancing/).

#### 📊 Least Connections (`least-connections`)
Routes to the endpoint with least active requests. Ideal for:
- **Mixed workloads**: Different request types with varying processing times
- **Dynamic balancing**: Automatically adapts to endpoint performance
- **Optimal resource utilisation**: Prevents any single endpoint from being overwhelmed

```yaml
load_balancer: "least-connections"
```

#### 🎯 Priority (`priority`)
Routes requests to the highest priority healthy endpoint. Perfect for:
- **Home setups**: Workstation (priority 100) → Laptop (priority 50)
- **Tiered infrastructure**: GPU servers → CPU servers → Cloud endpoints
- **Cost optimisation**: Local hardware → Expensive cloud APIs

```yaml
load_balancer: "priority"
```

#### 🔄 Round Robin (`round-robin`)
Distributes requests evenly across all healthy endpoints. Good for:
- **Equal hardware**: Multiple identical servers
- **Even load distribution**: When all endpoints have similar capacity
- **Simple load spreading**: No complex routing logic needed

```yaml
load_balancer: "round-robin"
```

### Proxy Engines

- **Sherpa**: Simple, maintainable (8KB buffers, shared transport)
- **Olla**: High-performance (64KB buffers, per-endpoint pools, circuit breakers)

For engine selection and performance tuning, see [Proxy Engine Guide](https://thushan.github.io/olla/concepts/proxy-engines/).

### Why Olla for LLMs?

Unlike generic proxies, Olla is purpose-built for LLM workloads:

- **Smart Model Routing**: One API endpoint, automatic routing to the right provider
- **Unified Client Experience**: Use OpenAI clients to access Ollama, LM Studio, and cloud APIs
- **Streaming-First**: Immediate response streaming without buffering delays
- **Long-Running Requests**: Optimised timeouts for extended LLM inference times  
- **Memory Efficient**: 64KB buffers optimised for token streaming (Olla engine)
- **Connection Pooling**: Persistent connections to backend endpoints reduce latency
- **Circuit Breakers**: Automatic failover prevents cascade delays during model loading

For detailed configuration options including Docker deployment and environment variables, see the [Configuration Reference](https://thushan.github.io/olla/configuration/reference/), [Load Balancing Guide](https://thushan.github.io/olla/concepts/load-balancing/), [Proxy Engine Guide](https://thushan.github.io/olla/concepts/proxy-engines/), and [Getting Started Guide](https://thushan.github.io/olla/getting-started/quickstart/).

## Example: Multi-Platform Setup

```yaml
discovery:
  type: "static"
  static:
    endpoints:
      # Local Ollama (highest priority)
      - url: "http://localhost:11434"
        name: "workstation"
        type: "ollama"
        priority: 100
        model_url: "/api/tags"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
        
      # LM Studio backup
      - url: "http://192.168.1.100:1234"
        name: "laptop"
        type: "lm-studio"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/"
        check_interval: 2s
        check_timeout: 1s
```

With this setup:
1. Requests go to your workstation first
2. If workstation fails, tries laptop
3. Models from each provider are discoverable and available for routing

## Documentation

Full documentation is available at **[https://thushan.github.io/olla/](https://thushan.github.io/olla/)**

### Quick Links

- **[Getting Started](https://thushan.github.io/olla/getting-started/quickstart/)** - Installation and quick setup
- **[Configuration Reference](https://thushan.github.io/olla/configuration/reference/)** - Complete configuration options
- **[API Reference](https://thushan.github.io/olla/api-reference/overview/)** - Full API documentation
- **[Concepts](https://thushan.github.io/olla/concepts/overview/)** - Core concepts and architecture
- **[Integrations](https://thushan.github.io/olla/integrations/overview/)** - Frontend and backend integrations
- **[Development](https://thushan.github.io/olla/development/overview/)** - Contributing and development guide

## Development

```bash
make build        # Build binary
make test         # Run tests
make ready        # Test + lint + format (run before commit)
make dev          # Development mode with auto-reload
```

## 🚨 Security Considerations

Olla is designed to sit behind a reverse proxy (nginx, Cloudflare, etc.) in production. 

The built-in security features are optimised for this deployment pattern:
- **Rate limiting**: Protects against request flooding
- **Request size limits**: Prevents resource exhaustion
- **Trusted proxy support**: Correctly handles client IPs behind load balancers
- **No authentication**: Relies on your reverse proxy for authentication

## 🤔 FAQ

**Q: Why use Olla instead of nginx or HAProxy?** \
A: Olla understands LLM-specific patterns like model routing, streaming responses, and health semantics. It also provides built-in model discovery and LLM-optimised timeouts.

**Q: Can I use Olla with other LLM providers?** \
A: Yes! Any OpenAI-compatible API works. Configure them as `type: "openai-compatible"` endpoints (such as vLLM, LocalAI, Together AI, etc.).

**Q: Does Olla support authentication?** \
A: Olla focuses on load balancing and lets your reverse proxy handle authentication. This follows the Unix philosophy of doing one thing well.

**Q: Which proxy engine should I use?** \
A: Use **Sherpa** for simple deployments with moderate traffic. Choose **Olla** for high-throughput production workloads that need connection pooling, circuit breakers, and maximum performance.

**Q: How does priority routing work with model availability?** \
A: Olla discovers models within each provider type and routes requests to compatible endpoints. Per-provider unification means Ollama requests only route to Ollama endpoints, LM Studio requests only route to LM Studio endpoints, etc. See the [Model Discovery Guide](https://thushan.github.io/olla/concepts/model-unification/) for details.

**Q: Can I run Olla in Kubernetes?** \
A: Absolutely! Olla is stateless and containerised. We'll add some examples soon - but if you'd like to share, PR away!

**Q: What is behind the name Olla?** \
A: Olla is the name of our llama (featured in the logo). It's pronounced like 'holla' and comes from a running joke about getting things working with Ollama. The fact it means 'pot' in Spanish is coincidental—though you can indeed cook up a lot when Olla is in the middle!

## 🤝 Contributing

We welcome contributions! Please open an issue first to discuss major changes.

## 🤖 AI Disclosure

This project has been built with the assistance of AI tools for documentation, test refinement, and code reviews.

We've utilised GitHub Copilot, Anthropic Claude, Jetbrains Junie and OpenAI ChatGPT for documentation, code reviews, test refinement and troubleshooting.

## 🙏 Acknowledgements

* [@pterm/pterm](https://github.com/pterm/pterm) - Terminal UI framework
* [@puzpuzpuz/xsync](https://github.com/puzpuzpuz/xsync/) - High-performance concurrent maps
* [@golangci/golangci-lint](https://github.com/golangci/golangci-lint) - Go linting
* [@dkorunic/betteralign](https://github.com/dkorunic/betteralign) - Struct alignment optimisation

## 📄 License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## 🎯 Roadmap

- [x] **Circuit breakers**: Advanced fault tolerance (Olla engine)
- [x] **Connection pooling**: Per-endpoint connection management (Olla engine)
- [x] **Object pooling**: Reduced GC pressure for high throughput (Olla engine)
- [X] **Model routing**: Route based on model requested
- [ ] **Authenticated Endpoints**: Support calling authenticated endpoints (bearer) like OpenAI/Groq/OpenRouter as endpoints
- [ ] **Auto endpoint discovery**: Add endpoints, let Olla determine the type
- [ ] **Model benchmarking**: Benchmark models across multiple endpoints easily
- [ ] **Metrics export**: Prometheus/OpenTelemetry integration
- [ ] **Dynamic configuration**: API-driven endpoint management
- [ ] **TLS termination**: Built-in SSL support
- [ ] **Olla Admin Panel**: View Olla metrics easily within the browser
- [ ] **Model caching**: Intelligent model preloading
- [ ] **Advanced Connection Management**: Authenticated endpoints (via SSH tunnels, OAuth, Tokens)
- [ ] **OpenRouter Support**: Support OpenRouter calls within Olla (divert to free models on OpenRouter etc)

Let us know what you want to see!

---

<div align="center">

**Made with ❤️ for the LLM community**

[🏠 Homepage](https://github.com/thushan/olla) • [📖 Documentation](https://thushan.github.io/olla/) • [🐛 Issues](https://github.com/thushan/olla/issues) • [🚀 Releases](https://github.com/thushan/olla/releases)


</div>
