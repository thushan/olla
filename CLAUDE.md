# CLAUDE.md

## Overview
Olla is a high-performance proxy and load balancer for LLM infrastructure, written in Go. It intelligently routes requests across local and remote inference nodes (Ollama, LM Studio, OpenAI-compatible endpoints). 

The project provides two proxy engines: Sherpa (simple, maintainable) and Olla (high-performance with advanced features).

## Commands
```bash
make ready        # Run before commit (test + lint + fmt)
make dev          # Development mode (auto-reload)
make test         # Run all tests
make bench        # Run benchmarks
```

## Project Structure
```
olla/
├── main.go                    # Entry point, initialises services
├── config.yaml               # Default configuration
├── config/
│   ├── profiles/            # Provider-specific profiles
│   │   ├── ollama.yaml     # Ollama configuration
│   │   ├── lmstudio.yaml   # LM Studio configuration
│   │   ├── openai.yaml     # OpenAI-compatible configuration
│   │   └── vllm.yaml       # vLLM configuration
│   └── models.yaml         # Model configurations
├── internal/
│   ├── core/               # Domain layer (business logic)
│   │   ├── domain/         # Core entities
│   │   │   ├── endpoint.go         # Endpoint management
│   │   │   ├── model.go            # Model registry
│   │   │   ├── unified_model.go    # Unified model format
│   │   │   └── routing.go          # Request routing logic
│   │   ├── ports/          # Interface definitions
│   │   └── constants/      # Application constants
│   ├── adapter/            # Infrastructure layer
│   │   ├── balancer/       # Load balancing strategies
│   │   │   ├── priority.go         # Priority-based selection
│   │   │   ├── round_robin.go      # Round-robin selection
│   │   │   └── least_connections.go # Least connections selection
│   │   ├── proxy/          # Proxy implementations
│   │   │   ├── sherpa/     # Simple, maintainable proxy
│   │   │   ├── olla/       # High-performance proxy
│   │   │   └── core/       # Shared proxy components
│   │   ├── health/         # Health checking
│   │   │   ├── checker.go          # Health check coordinator
│   │   │   └── circuit_breaker.go  # Circuit breaker implementation
│   │   ├── discovery/      # Service discovery
│   │   │   └── service.go          # Model discovery service
│   │   ├── registry/       # Model & profile registries
│   │   │   ├── profile/            # Provider profiles
│   │   │   └── unified_memory_registry.go # Unified model registry
│   │   ├── unifier/        # Model unification
│   │   ├── converter/      # Model format converters
│   │   ├── inspector/      # Request inspection
│   │   ├── security/       # Security features
│   │   │   ├── request_rate_limit.go  # Rate limiting
│   │   │   └── request_size_limit.go  # Size limiting
│   │   └── stats/          # Statistics collection
│   │       ├── collector.go        # Main stats collector
│   │       └── model_collector.go  # Model-specific stats
│   └── app/                # Application layer
│       ├── app.go          # Service manager
│       └── handlers/       # HTTP handlers
│           ├── handler_proxy.go    # Main proxy handler
│           ├── handler_status.go   # Status endpoints
│           └── handler_health.go   # Health endpoints
├── pkg/                    # Reusable packages
│   ├── pool/              # Object pooling
│   └── nerdstats/         # Process statistics
└── test/
    └── scripts/           # Test scripts
        ├── logic/         # Logic & routing tests
        ├── security/      # Security tests
        └── streaming/     # Streaming tests
```

## Key Files
- `main.go` - Application entry point
- `config.yaml` - Main configuration
- `internal/app/handlers/handler_proxy.go` - Request routing logic
- `internal/adapter/proxy/sherpa/service.go` - Sherpa proxy
- `internal/adapter/proxy/olla/service.go` - Olla proxy
- `/test/scripts/logic/test-model-routing.sh` - Test routing & headers

## Response Headers
- `X-Olla-Endpoint`: Backend name
- `X-Olla-Model`: Model used
- `X-Olla-Backend-Type`: ollama/openai/lmstudio/vllm
- `X-Olla-Request-ID`: Request ID
- `X-Olla-Response-Time`: Total processing time

## Testing
- Unit tests: Components in isolation
- Integration: Full request flow
- Benchmarks: Performance comparison
- Always run `make ready` before commit

### Testing Strategy

1. **Unit Tests**: Test individual components in isolation
2. **Integration Tests**: Test full request flow through the proxy
3. **Benchmark Tests**:
  - Performance of critical paths
  - Proxy engine comparisons
  - Connection pooling efficiency
  - Circuit breaker behavior
4. **Security Tests**: Validate rate limiting and size restrictions (see `/test/scripts/security/`)
5. **Shared Proxy Tests**: Common test suite for both proxy engines ensuring compatibility

### Testing Commands

```
# Run proxy engine tests
go test -v ./internal/adapter/proxy -run TestAllProxies
go test -v ./internal/adapter/proxy -run TestSherpa
go test -v ./internal/adapter/proxy -run TestOlla
```

## Notes
- Go 1.24+
- Endpoints: `/internal/health`, `/internal/status`
- Proxy prefix: `/olla/`
- Priority balancer recommended for production
- Australian English for comments and documentation, comment on why rather than what.
- Use `make ready` before committing changes to ensure code quality