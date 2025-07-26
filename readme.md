<div align="center">
    <img src="assets/images/banner.png" width="480" height="249" alt="Olla - Smart LLM Load Balancer & Proxy" />
  <p>
    <a href="https://github.com/thushan/olla/blob/master/LICENSE"><img src="https://img.shields.io/github/license/thushan/olla" alt="License"></a>
    <a href="https://github.com/thushan/olla/actions/workflows/ci.yml"><img src="https://github.com/thushan/olla/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/thushan/olla"><img src="https://goreportcard.com/badge/github.com/thushan/olla" alt="Go Report Card"></a>
    <a href="https://github.com/thushan/olla/releases/latest"><img src="https://img.shields.io/github/release/thushan/olla" alt="Latest Release"></a>
  </p>
</div>

> [!IMPORTANT]  
> Olla is currently **in active-development**. While it is usable, we are still finalising some features and optimisations. Your feedback is invaluable!

Olla is a high-performance, low-overhead, low-latency proxy and load balancer for managing LLM infrastructure. It intelligently routes LLM requests across local and remote inference nodes—including [Ollama](https://github.com/ollama/ollama), [LM Studio](https://lmstudio.ai/) and OpenAI-compatible endpoints like [vLLM](https://github.com/vllm-project/vllm). Choose between two proxy engines: **Sherpa** for simplicity and maintainability or **Olla** for maximum performance with advanced features like circuit breakers and connection pooling.

![Olla Usecase](assets/diagrams/usecases.excalidraw.png)

Whether you're running workloads on laptops, workstations or clustered servers, Olla dynamically selects the best endpoint using configurable priorities, health checks and load-aware strategies. With built-in failover, adaptive routing, and continuous monitoring, it ensures resilience, performance and minimal operational complexity. 

Single CLI application and config file is all you need to go Olla!

## ✨ Features

### 🎯 **Smart Load Balancing**
- **Priority-based routing**: Configure node priorities (workstation first, laptop second) with automatic failover
- **Multiple strategies**: Round-robin, least connections, and priority-aware load balancing
- **Health-aware routing**: Only routes to healthy endpoints, with automatic recovery detection
- **Circuit breakers**: Automatic failure isolation prevents cascade failures (Olla engine)
- **Model-aware routing**: 🚧 Coming soon - route to endpoints with specific models

### 🛡️ **In-built Security**
- **Rate limiting**: Per-IP and global request limits with burst handling
- **Request size limits**: Protect against oversized requests and headers
- **Trusted proxy support**: Works seamlessly behind reverse proxies and CDNs

### 🔍 **Comprehensive Monitoring**
- **Health checking**: Continuous endpoint monitoring with configurable intervals
- **Model discovery**: Automatic detection of available models across endpoints
- **Detailed metrics**: Request stats, response times, and performance insights

### ⚙️ **Easy Configuration**
- **Single YAML file**: All settings in one place with sensible defaults
- **Environment variables**: Override any setting for different deployments
- **Container-ready**: Pre-configured Docker images with example configurations

### 🔌 **Universal Compatibility**
- **Ollama**: Full support for Ollama endpoints
- **LM Studio**: Seamless integration with LM Studio
- **OpenAI-compatible**: Works with any OpenAI-compatible API

## ⚡ **High Performance**
- **Dual proxy engines**: Choose Sherpa for simplicity or Olla for maximum performance
- **Connection pooling**: Per-endpoint connection pools reduce latency (Olla engine)
- **Object pooling**: Reduced GC pressure with pooled buffers and contexts (Olla engine)
- **Low latency**: Sub-millisecond endpoint selection with lock-free atomic stats
- **Memory optimised**: Efficient buffer pooling with 64KB default buffers for streaming
- **Concurrent**: Handles thousands of simultaneous streaming connections
- **Resource efficient**: Stable memory usage with automatic cleanup of stale data

## 🔧 **Optimised for LLMs**
- **Streaming-first**: Immediate response streaming without buffering delays
- **Long-running requests**: Optimised timeouts for extended LLM inference times
- **Connection pooling**: Persistent connections to backend endpoints reduce latency
- **Circuit breakers**: Automatic failover prevents cascade delays

## 🚀 Quick Start

### Docker (Recommended)

Olla comes with pre-configured Docker images for quick deployment:

```bash
# Run with default configuration
docker run -d \
  --name olla \
  -p 40114:40114 \
  ghcr.io/thushan/olla:latest

# Or with custom configuration
docker run -d \
  --name olla \
  -p 40114:40114 \
  -v "$(pwd)/config/config.local.yaml:/config/config.yaml:ro" \
  -e OLLA_CONFIG_FILE=/config/config.yaml \
  ghcr.io/thushan/olla:latest
```

You can then check it's running with:

```bash
curl http://localhost:40114/internal/health
```

### Docker Compose

A `docker-compose.yaml` is included in the repository:

```bash
# Run with the included docker-compose.yaml
docker-compose up -d

# Or create your own override for local development
cp docker-compose.yaml docker-compose.local.yaml
vim docker-compose.local.yaml
docker-compose -f docker-compose.local.yaml up
```

The included `docker-compose.yaml` mounts your local config for easy customization:

```yaml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    ports:
      - "40114:40114"
    volumes:
      # Mount your local config (optional)
      - ./config/config.local.yaml:/config/config.yaml:ro
    environment:
      - OLLA_CONFIG_FILE=/config/config.yaml
```

### Local Binary

You can also run Olla directly on your machine without Docker. 

```bash
bash <(curl -s https://raw.githubusercontent.com/thushan/olla/main/install.sh)
```

It will download the latest version & extract it to its own folder for you.

If you prefer running Olla directly on your machine, download a pre-built binary.

1. Download the latest release from [Releases](https://github.com/thushan/olla/releases)
2. Extract the archive
3. Navigate to the directory and run:

```bash
./olla
```

Alternatively, you can install it via go:

```bash
go install github.com/thushan/olla@latest
```

### Build from Source

```bash
# Clone the repository
git clone https://github.com/thushan/olla.git
cd olla

# Install dependencies and build
make deps
make build

# Run with default config
./bin/olla

# Or run with your local config
cp config/config.local.yaml.example config/config.local.yaml
vim config/config.local.yaml
OLLA_CONFIG_FILE=config/config.local.yaml ./bin/olla
```

## 📝 Configuration

Olla uses a layered configuration approach: defaults → YAML config → environment variables (highest priority).

### Basic Configuration

The main configuration file is `config/config.yaml`. For local development, create a `config/config.local.yaml` (which is git-ignored):

```yaml
server:
  host: "0.0.0.0"
  port: 40114
  read_timeout: 30s
  write_timeout: 0s  # Important: Leave as 0s for streaming LLM responses

proxy:
  engine: "olla"            # or "sherpa" for simpler deployments
  load_balancer: "priority"  # or "round_robin", "least_connections"
  response_timeout: 900s     # 15 minutes for long LLM responses
  stream_buffer_size: 65536  # 64KB for Olla, 8KB for Sherpa
  
  # Olla engine specific (connection pooling)
  max_idle_conns: 100        # Maximum idle connections
  max_conns_per_host: 50     # Maximum connections per host
  idle_conn_timeout: 90s     # Idle connection timeout

discovery:
  static:
    endpoints:
      - url: "http://192.168.1.100:11434"
        name: "workstation-ollama"
        type: "ollama"
        priority: 100          # Higher number = higher priority
        health_check_url: "/"
        model_url: "/api/tags"
        check_interval: 5s
        check_timeout: 2s
      
      - url: "http://192.168.1.101:1234"
        name: "laptop-lmstudio"
        type: "lm-studio"
        priority: 50           # Lower priority fallback
        health_check_url: "/"
        model_url: "/v1/models"
        check_interval: 10s
        check_timeout: 3s
```

### Endpoint Configuration Guide

Each endpoint in your configuration supports the following options:

```yaml
endpoints:
  - url: "http://192.168.1.100:11434"    # Required: Base URL of the LLM service
    name: "workstation-ollama"            # Required: Unique name for this endpoint
    type: "ollama"                        # Required: ollama, lm-studio, or openai-compatible
    priority: 100                         # Optional: Higher = preferred (default: 50)
    health_check_url: "/"                 # Optional: Health check path (default: "/")
    model_url: "/api/tags"                # Optional: Model discovery endpoint
    check_interval: 5s                    # Optional: Health check frequency (default: 30s)
    check_timeout: 2s                     # Optional: Health check timeout (default: 5s)
    enabled: true                         # Optional: Enable/disable endpoint (default: true)
```

#### Endpoint Types

**Ollama Endpoints**
```yaml
- url: "http://localhost:11434"
  name: "local-ollama"
  type: "ollama"
  priority: 100
  health_check_url: "/"
  model_url: "/api/tags"          # Ollama's model list endpoint
```

**LM Studio Endpoints**
```yaml
- url: "http://localhost:1234"
  name: "local-lmstudio"
  type: "lm-studio"
  priority: 90
  health_check_url: "/v1/models"  # LM Studio health endpoint
  model_url: "/v1/models"         # OpenAI-compatible model list
```

**OpenAI-Compatible Endpoints** (vLLM, LocalAI, etc.)
```yaml
- url: "http://gpu-server:8000"
  name: "vllm-server"
  type: "openai-compatible"
  priority: 80
  health_check_url: "/health"
  model_url: "/v1/models"
```

**Cloud Provider Endpoints**
```yaml
- url: "https://api.openai.com"
  name: "openai-cloud"
  type: "openai-compatible"
  priority: 10                    # Low priority due to cost
  enabled: false                  # Disabled by default
  check_interval: 60s             # Less frequent checks for cloud
```

#### Priority Strategy Examples

**Home Lab Setup** - Local hardware preferred over cloud:
```yaml
endpoints:
  - url: "http://desktop:11434"
    name: "desktop-gpu"
    type: "ollama"
    priority: 100                 # Always use desktop GPU first
  
  - url: "http://laptop:11434"
    name: "laptop-cpu"
    type: "ollama"
    priority: 50                  # Fallback to laptop
  
  - url: "https://api.together.ai"
    name: "together-ai"
    type: "openai-compatible"
    priority: 10                  # Only use cloud if local unavailable
```

**Enterprise Setup** - Tiered by capability:
```yaml
endpoints:
  - url: "http://gpu-cluster:8000"
    name: "a100-cluster"
    type: "openai-compatible"
    priority: 100                 # High-end GPU cluster
  
  - url: "http://gpu-pool:8000"
    name: "rtx4090-pool"
    type: "openai-compatible"
    priority: 75                  # Mid-tier GPUs
  
  - url: "http://cpu-farm:8000"
    name: "cpu-inference"
    type: "openai-compatible"
    priority: 25                  # CPU-only as last resort
```

#### Health Check Configuration

Fine-tune health checks based on your infrastructure:

```yaml
# Fast local network - aggressive health checks
- url: "http://localhost:11434"
  check_interval: 2s              # Check every 2 seconds
  check_timeout: 1s               # 1 second timeout
  
# Remote/cloud endpoints - conservative checks  
- url: "https://api.example.com"
  check_interval: 30s             # Check every 30 seconds
  check_timeout: 10s              # 10 second timeout
```

### Environment Variables

Common settings you'll want to override:

```bash
# Server settings
export OLLA_SERVER_HOST="0.0.0.0"
export OLLA_SERVER_PORT="40114"

# Proxy settings
export OLLA_PROXY_ENGINE="olla"              # or "sherpa"
export OLLA_PROXY_LOAD_BALANCER="priority"
export OLLA_PROXY_RESPONSE_TIMEOUT="15m"
export OLLA_PROXY_MAX_IDLE_CONNS="100"       # Olla engine only
export OLLA_PROXY_MAX_CONNS_PER_HOST="50"    # Olla engine only

# Rate limiting (optional but recommended)
export OLLA_SERVER_GLOBAL_RATE_LIMIT="1000"    # requests per minute
export OLLA_SERVER_PER_IP_RATE_LIMIT="100"     # requests per minute per IP
export OLLA_SERVER_TRUST_PROXY_HEADERS="true"  # if behind reverse proxy

# Security (optional)
export OLLA_SERVER_MAX_BODY_SIZE="50MB"
export OLLA_SERVER_MAX_HEADER_SIZE="1MB"

# Logging
export OLLA_LOGGING_LEVEL="info"  # debug, info, warn, error
export OLLA_LOGGING_FORMAT="json" # json or text
```

## ⚖️ Load Balancing Strategies

Choose the right strategy for your setup:

### 🎯 Priority (`priority`) - **Recommended**
Routes requests to the highest priority healthy endpoint. Perfect for:
- **Home setups**: Workstation (priority 100) → Laptop (priority 50)
- **Tiered infrastructure**: GPU servers → CPU servers → Cloud endpoints
- **Cost optimization**: Local hardware → Expensive cloud APIs

```yaml
load_balancer: "priority"
```

### 🔄 Round Robin (`round_robin`)
Distributes requests evenly across all healthy endpoints. Good for:
- **Equal hardware**: Multiple identical servers
- **Even load distribution**: When all endpoints have similar capacity
- **Simple load spreading**: No complex routing logic needed

```yaml
load_balancer: "round_robin"
```

### 📊 Least Connections (`least_connections`)
Routes to the endpoint with fewest active requests. Ideal for:
- **Mixed workloads**: Different request types with varying processing times
- **Dynamic balancing**: Automatically adapts to endpoint performance
- **Optimal resource utilisation**: Prevents any single endpoint from being overwhelmed

```yaml
load_balancer: "least_conn"
```

## 🔗 Usage

Once Olla is running, point your LLM clients to it instead of directly to Ollama:

### OpenAI-compatible Clients

```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:40114/olla/v1",  # Point to Olla
    api_key="dummy"  # Ollama doesn't need auth
)

response = client.chat.completions.create(
    model="llama3.2",
    messages=[{"role": "user", "content": "Hello from Olla!"}]
)
```

### Curl

```bash
# Chat completions
curl -X POST http://localhost:40114/olla/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Ollama generate API
curl -X POST http://localhost:40114/olla/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "prompt": "Hello!"
  }'
```

### Direct Model Switching

Olla automatically routes requests to endpoints that have the requested model:

```bash
# This will route to whichever endpoint has 'codellama'
curl -X POST http://localhost:40114/olla/v1/chat/completions \
  -d '{"model": "codellama", "messages": [...]}'
```

## 📊 Monitoring

### Health and Status

```bash
# Quick health check
curl http://localhost:40114/internal/health

# Detailed status including endpoint health
curl http://localhost:40114/internal/status | jq

# Application version and build info
curl http://localhost:40114/version | jq
```

### Process Statistics

```bash
# Runtime stats (memory, GC, goroutines)
curl http://localhost:40114/internal/process | jq
```

### Example Status Response

```json
{
  "endpoints": [
    {
      "name": "workstation-ollama",
      "url": "http://192.168.1.100:11434",
      "status": "healthy",
      "priority": 100,
      "last_latency": "45ms",
      "consecutive_failures": 0
    }
  ],
  "proxy": {
    "load_balancer": "priority",
    "total_requests": 1247,
    "successful_requests": 1245,
    "failed_requests": 2,
    "avg_latency_ms": 156
  },
  "total_endpoints": 2,
  "healthy_endpoints": 1,
  "routable_endpoints": 1
}
```

## 🛠️ Development

### Local Development Setup

#### Configuration

Olla uses a clean configuration structure that separates shipped configs from local development:

```
config/
├── config.yaml              # Main configuration (shipped)
├── config.local.yaml        # Your local overrides (git-ignored)
├── models.yaml             # Model mappings
├── docker.yaml             # Docker-specific config
└── profiles/               # Platform profiles
    ├── ollama.yaml
    ├── lmstudio.yaml
    └── openai.yaml
```

**Setting up local development:**

```bash
# Copy the example local config
cp config/config.local.yaml.example config/config.local.yaml

# Edit with your endpoints
vim config/config.local.yaml

# Run with your local config
OLLA_CONFIG_FILE=config/config.local.yaml go run main.go
```

#### Building

```bash
# Install dependencies
make deps

# Run tests
make test

# Run with race detection
make test-race

# Build optimised binary to ./bin/
make build

# Build just the binary to ./build/ (fast, for testing)
make build-local
./build/olla --version

# Test with your local config
OLLA_CONFIG_FILE=config/config.local.yaml ./build/olla

# Run with debug logging
make run-debug

# See all available targets
make help
```

### Release Building

#### Local Release Testing

```bash
# Build full release (binaries + archives) to ./dist/
make build-snapshot

# Check goreleaser configuration
make goreleaser-check

# What gets packaged:
# - config/*.yaml (except *.local.yaml)
# - config/profiles/*.yaml
# - LICENSE, README.md
# - Empty directories: logs/, tmp/, data/
```

#### Docker Testing

```bash
# Build Docker images locally
make docker-build

# Run Docker image with your local config
make docker-run

# Or use docker-compose
docker-compose up

# Create a local override (git-ignored)
cp docker-compose.yaml docker-compose.local.yaml
vim docker-compose.local.yaml
docker-compose -f docker-compose.local.yaml up
```

#### Clean Build Directories

```bash
# Remove all build artifacts
make clean
# Removes: bin/, build/, dist/, logs/, coverage files
```

### Configuration Management

**Important:** Local configuration files (`*.local.yaml`) are git-ignored and never shipped in releases. This prevents accidentally publishing your personal endpoints or development settings.

```yaml
# config/config.local.yaml (example)
server:
  host: "localhost"
  port: 40114
  
logging:
  level: "debug"
  format: "pretty"

discovery:
  static:
    endpoints:
      # Your local development endpoints
      - url: "http://localhost:11434"
        name: "local-ollama"
        type: "ollama"
        priority: 100
```

### Project Structure

```
├── config/                 # Configuration files
│   ├── config.yaml        # Main configuration (shipped)
│   ├── *.local.yaml       # Local overrides (git-ignored)
│   └── profiles/          # Platform profiles
├── internal/               # Private application code (Go convention)
│   ├── adapter/            # External integrations and infrastructure
│   │   ├── balancer/       # Load balancing strategies (priority, round-robin, least-conn)
│   │   ├── discovery/      # Service discovery and endpoint management
│   │   ├── health/         # Health checking with circuit breakers
│   │   ├── proxy/          # Dual proxy engines (Sherpa: simple, Olla: high-performance)
│   │   ├── registry/       # Model registry and platform profiles
│   │   ├── security/       # Rate limiting and request validation
│   │   └── stats/          # Atomic statistics collection
│   ├── app/                # Application assembly and HTTP handlers
│   ├── config/             # Configuration management
│   ├── core/               # Business logic and domain models
│   │   ├── constants/      # Application constants
│   │   ├── domain/         # Domain entities and interfaces
│   │   └── ports/          # Interface definitions (ports & adapters pattern)
│   ├── env/                # Environment variable utilities
│   ├── integration/        # Integration test helpers
│   ├── logger/             # Structured logging with themes
│   ├── router/             # HTTP route registry and middleware
│   ├── util/               # Common utilities (networking, terminal detection)
│   └── version/            # Version information and build metadata
├── pkg/                    # Public packages (reusable components)
│   ├── format/             # Formatting utilities (bytes, duration)
│   ├── nerdstats/          # Runtime statistics and performance metrics
│   └── pool/               # Generic connection pooling (lite_pool)
├── test/                   # Test suites and testing utilities
│   └── scripts/            # Test automation scripts
│       ├── load/           # Load testing scripts
│       └── security/       # Security validation tests
└── theme/                  # Terminal theming and color schemes
```

## 🔧 Advanced Configuration

### Rate Limiting

Protect your endpoints from abuse:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000      # Total requests across all IPs
    per_ip_requests_per_minute: 100       # Per-IP limit
    burst_size: 50                        # Allow bursts up to this size
    health_requests_per_minute: 1000      # Separate limit for health checks
    trust_proxy_headers: true             # For X-Forwarded-For support
    trusted_proxy_cidrs:                  # Which proxies to trust
      - "127.0.0.0/8"
      - "10.0.0.0/8"
      - "172.16.0.0/12"
      - "192.168.0.0/16"
```

### Request Size Limits

Prevent resource exhaustion:

```yaml
server:
  request_limits:
    max_body_size: 50MB      # Maximum request body size
    max_header_size: 1MB     # Maximum total header size
```

### Model Discovery

Automatically discover available models:

```yaml
discovery:
  model_discovery:
    enabled: true
    interval: 5m              # How often to check for new models
    timeout: 30s              # Timeout for discovery requests
    concurrent_workers: 5     # Parallel discovery workers
    retry_attempts: 3
    retry_backoff: 1s
```

## 🚨 Security Considerations

Olla is designed to sit behind a reverse proxy (nginx, Cloudflare, etc.) in production. The built-in security features are optimised for this deployment pattern:

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
A: Model routing infrastructure is in place but not yet active. When implemented, Olla will filter endpoints by model availability before applying your load balancing strategy.

**Q: Can I run Olla in Kubernetes?** \
A: Absolutely! Olla is stateless and containerised. We'll add some examples soon - but if you'd like to share, PR away!

**Q: What is behind the name Olla?** \
A: Olla is the name of our llama (featured in the logo). It's pronounced like 'holla' and comes from a running joke about getting things working with Ollama. The fact it means 'pot' in Spanish is coincidental—though you can indeed cook up a lot when Olla is in the middle!

## 🤝 Contributing

We welcome contributions! Please open an issue first to discuss major changes.

## 🤖 AI Disclosure

This project has been built with the assistance of AI tools for documentation, test refinement, and code reviews.

We've utilised GitHub Copilot, Anthropic Claude, and OpenAI ChatGPT for documentation, code reviews, test refinement, and troubleshooting.

## 🙏 Acknowledgements

* [@pterm/pterm](https://github.com/pterm/pterm) - Terminal UI framework
* [@puzpuzpuz/xsync](https://github.com/puzpuzpuz/xsync/) - High-performance concurrent maps
* [@golangci/golangci-lint](https://github.com/golangci/golangci-lint) - Go linting
* [@dkorunic/betteralign](https://github.com/dkorunic/betteralign) - Struct alignment optimization

## 📄 License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## 🎯 Roadmap

- [x] **Circuit breakers**: Advanced fault tolerance (Olla engine)
- [x] **Connection pooling**: Per-endpoint connection management (Olla engine)
- [x] **Object pooling**: Reduced GC pressure for high throughput (Olla engine)
- [ ] **Auto endpoint discovery**: Add endpoints, let Olla determine the type
- [ ] **Model routing**: Route based on model requested (infrastructure ready)
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

[🏠 Homepage](https://github.com/thushan/olla) • [📖 Documentation](https://github.com/thushan/olla#readme) • [🐛 Issues](https://github.com/thushan/olla/issues) • [🚀 Releases](https://github.com/thushan/olla/releases)

</div>