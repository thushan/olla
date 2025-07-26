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

## Architecture
The project follows **Hexagonal Architecture** (Ports & Adapters):

- `/internal/core/` - Business logic and domain models
  - `domain/` - Core entities (Endpoint, LoadBalancer, etc.)
  - `ports/` - Interface definitions
- `/internal/adapter/` - Infrastructure implementations
  - `balancer/` - Load balancing strategies (priority, round-robin, least connections)
  - `proxy/` - Dual HTTP proxy engines:
    - **Sherpa**: Clean, simple implementation for moderate loads
    - **Olla**: High-performance with circuit breakers, connection pooling, object pooling
  - `health/` - Health checking with circuit breakers
  - `discovery/` - Service discovery
  - `security/` - Rate limiting and validation
  - `stats/` - Atomic statistics collection with lock-free operations
- `/internal/app/` - Application assembly and HTTP handlers

## Key Files
- `config.yaml` - Main configuration
- `handler_proxy.go` - Request routing logic
- `proxy_sherpa.go` / `proxy_olla.go` - Proxy implementations
- `/test/scripts/logic/test-model-routing.sh` - Test routing & headers

## Response Headers
- `X-Olla-Endpoint`: Backend name
- `X-Olla-Model`: Model used
- `X-Olla-Backend-Type`: ollama/openai/lmstudio
- `X-Olla-Request-ID`: Request ID
- `X-Olla-Response-Time`: Total time (trailer)

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