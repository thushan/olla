# CLAUDE.md

## Overview
Olla is a high-performance proxy and load balancer for LLM infrastructure, written in Go. It intelligently routes requests across local and remote inference nodes (Ollama, LM Studio, LiteLLM, vLLM, SGLang, Llamacpp, Lemonade, Anthropic, and OpenAI-compatible endpoints).

The project provides two proxy engines: Sherpa (simple, maintainable) and Olla (high-performance with advanced features).

Full documentation available at: https://thushan.github.io/olla/

## Commands
```bash
make ready           # Run before commit (test-short + test-race + fmt + lint + align)
make ready-tools     # Check code with tools only (fmt + lint + align)
make test            # Run all tests
make test-race       # Run tests with race detection
make test-stress     # Run comprehensive stress tests
make bench           # Run all benchmarks
make bench-balancer  # Run balancer benchmarks
make build           # Build optimised binary with version info
make build-local     # Build binary to ./build/ (fast, for testing)
make run             # Run with version info
make run-debug       # Run with debug logging
make ci              # Run full CI pipeline locally
make help            # Show all available targets
```

## Project Structure
```
olla/
├── main.go                    # Entry point, initialises services
├── config.yaml               # Default configuration
├── config/
│   ├── profiles/            # Provider-specific profiles
│   │   ├── ollama.yaml     # Ollama configuration
│   │   ├── llamacpp.yaml   # llama.cpp configuration
│   │   ├── lmstudio.yaml   # LM Studio configuration
│   │   ├── lemonade.yaml   # Lemonade SDK configuration
│   │   ├── litellm.yaml    # LiteLLM gateway configuration
│   │   ├── vllm.yaml       # vLLM configuration
│   │   ├── sglang.yaml     # SGLang configuration
│   │   ├── anthropic.yaml  # Anthropic Claude API configuration
│   │   └── openai.yaml     # OpenAI-compatible generic profile
│   ├── models.yaml         # Model configurations
│   └── config.local.yaml   # Local configuration overrides (user, not committed to git)
├── internal/
│   ├── core/               # Domain layer (business logic)
│   │   ├── domain/         # Core entities
│   │   ├── ports/          # Interface definitions
│   │   └── constants/      # Application constants
│   ├── adapter/            # Infrastructure layer
│   │   ├── balancer/       # Load balancing strategies
│   │   ├── converter/      # Model format converters
│   │   ├── discovery/      # Service discovery
│   │   ├── factory/        # Factory patterns
│   │   ├── filter/         # Request/response filtering
│   │   ├── health/         # Health checking & circuit breakers
│   │   ├── inspector/      # Request inspection
│   │   ├── metrics/        # Metrics collection
│   │   ├── proxy/          # Proxy implementations
│   │   │   ├── sherpa/     # Simple, maintainable proxy
│   │   │   ├── olla/       # High-performance proxy
│   │   │   └── core/       # Shared proxy components
│   │   ├── registry/       # Model & profile registries
│   │   ├── security/       # Security features (rate/size limits)
│   │   ├── stats/          # Statistics collection
│   │   ├── translator/     # API translation layer (OpenAI ↔ Provider)
│   │   └── unifier/        # Model unification
│   ├── app/                # Application layer
│   │   ├── handlers/       # HTTP handlers
│   │   │   ├── server.go              # HTTP server setup
│   │   │   ├── server_routes.go       # Route registration
│   │   │   ├── handler_proxy.go       # Main proxy handler
│   │   │   ├── handler_provider_*.go  # Provider-specific handlers
│   │   │   ├── handler_translation.go # Translation handler
│   │   │   ├── handler_status*.go     # Status endpoints
│   │   │   ├── handler_health.go      # Health endpoints
│   │   │   └── handler_version.go     # Version information
│   │   ├── middleware/     # HTTP middleware
│   │   └── services/       # Application services
│   ├── config/             # Configuration management
│   ├── env/                # Environment handling
│   ├── integration/        # Integration tests
│   ├── logger/             # Logging framework
│   ├── router/             # Routing logic
│   ├── util/               # Utilities
│   └── version/            # Version management
├── pkg/                    # Reusable packages
│   ├── container/         # Dependency injection
│   ├── eventbus/          # Event bus (pub/sub)
│   ├── format/            # Formatting utilities
│   ├── nerdstats/         # Process statistics
│   ├── pool/              # Object pooling
│   └── profiler/          # Profiling support
└── test/
    └── scripts/           # Test scripts
        ├── anthropic/     # Anthropic API tests
        ├── cases/         # Test cases
        ├── inspector/     # Inspector tests
        ├── load/          # Load testing
        ├── logic/         # Logic & routing tests
        ├── security/      # Security tests
        └── streaming/     # Streaming tests
```

## Key Files
- `main.go` - Application entry point
- `config.yaml` - Main configuration
- `internal/app/handlers/server_routes.go` - Route registration & API setup
- `internal/app/handlers/handler_proxy.go` - Request routing logic
- `internal/adapter/proxy/sherpa/service.go` - Sherpa proxy implementation
- `internal/adapter/proxy/olla/service.go` - Olla proxy implementation
- `internal/adapter/translator/` - API translation layer (OpenAI ↔ Provider formats)
- `internal/version/version.go` - Version information embedded at build time
- `/test/scripts/logic/test-model-routing.sh` - Test routing & headers

## API Endpoints

### Internal Endpoints
- `/internal/health` - Health check endpoint
- `/internal/status` - Endpoint status
- `/internal/status/endpoints` - Endpoints status details
- `/internal/status/models` - Models status details
- `/internal/stats/models` - Model statistics
- `/internal/process` - Process statistics
- `/version` - Version information

### Unified Model Endpoints
- `/olla/models` - Unified models listing with filtering
- `/olla/models/{id}` - Get unified model by ID or alias

### Proxy Endpoints
- `/olla/proxy/` - Olla API proxy endpoint (POST)
- `/olla/proxy/v1/models` - OpenAI-compatible models listing (GET)

### Translator Endpoints
Dynamically registered based on configured translators (e.g., Anthropic Messages API)

## Response Headers
- `X-Olla-Endpoint`: Backend name
- `X-Olla-Model`: Model used
- `X-Olla-Backend-Type`: ollama/openai/lmstudio/vllm/litellm/sglang/llamacpp/lemonade/anthropic
- `X-Olla-Request-ID`: Request ID
- `X-Olla-Response-Time`: Total processing time

## Testing

### Testing Strategy
1. **Unit Tests**: Components in isolation
2. **Integration Tests**: Full request flow through proxy engines
3. **Benchmark Tests**: Performance comparison (balancers, proxy engines, repositories)
4. **Security Tests**: Rate limiting and size restrictions (see `/test/scripts/security/`)
5. **Stress Tests**: Comprehensive testing under load
6. **Script Tests**: End-to-end scenarios in `/test/scripts/`

### Testing Commands
```bash
# Core test commands
make test              # Run all tests
make test-race         # Run with race detection
make test-stress       # Run stress tests
make test-cover-html   # Generate coverage HTML report

# Benchmark commands
make bench             # Run all benchmarks
make bench-balancer    # Run balancer benchmarks
make bench-repo        # Run repository benchmarks

# Specific test patterns
go test -v ./internal/adapter/proxy -run TestAllProxies
go test -v ./internal/adapter/proxy -run TestSherpa
go test -v ./internal/adapter/proxy -run TestOlla
```

Always run `make ready` before committing changes.

## Architecture Notes

### Hexagonal Architecture
- **Domain Layer** (`internal/core`): Business logic, entities, and interfaces
- **Infrastructure Layer** (`internal/adapter`): Implementations (proxies, balancers, registries)
- **Application Layer** (`internal/app`): HTTP handlers, middleware, and services

### Key Components
- **Translator Layer**: Enables API format translation (e.g., OpenAI ↔ Anthropic)
- **Proxy Engines**: Choose Sherpa (simple) or Olla (high-performance)
- **Load Balancing**: Priority-based recommended for production
- **Version Management**: Build-time version injection via `internal/version`

### Development Guidelines
- Go 1.24+
- Australian English for comments and documentation
- Comment on **why** rather than **what**
- Always run `make ready` before committing
- Use `make help` to see all available commands