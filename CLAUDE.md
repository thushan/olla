# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Olla is a high-performance proxy and load balancer for LLM infrastructure, written in Go. It intelligently routes requests across local and remote inference nodes (Ollama, LM Studio, OpenAI-compatible endpoints). The project provides two proxy engines: Sherpa (simple, maintainable) and Olla (high-performance with advanced features).

## Common Development Commands

### Build & Run
```bash
# Build the binary
make build

# Build release version with optimizations
make build-release

# Run with default config
make run

# Run with debug logging
make run-debug

# Development mode (watches for changes)
make dev
```

### Testing
```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with race detector
make test-race

# Generate coverage report
make test-cover

# Generate HTML coverage report
make test-cover-html

# Run a single test
go test -v ./internal/adapter/balancer -run TestPriorityBalancer

# Run proxy engine tests
go test -v ./internal/adapter/proxy -run TestAllProxies
go test -v ./internal/adapter/proxy -run TestSherpa
go test -v ./internal/adapter/proxy -run TestOlla

# Run benchmarks
make bench

# Run specific benchmark
make bench-balancer

# Run proxy benchmarks
make bench-proxy
```

### Code Quality
```bash
# Format code
make fmt

# Run linter
make lint

# Align struct fields for better memory layout
make align

# Run full CI pipeline locally
make ci
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

## Key Implementation Details

### Proxy Engines
- **Sherpa**: Simple, maintainable proxy for moderate traffic
- **Olla**: High-performance proxy with:
  - Per-endpoint connection pooling
  - Circuit breakers for failure isolation  
  - Object pooling (buffers, contexts, errors)
  - 64KB default buffer size (vs 8KB in Sherpa)
  - Optimized for streaming LLM responses

### Load Balancing
The **priority balancer** is the recommended strategy. It routes requests based on endpoint priorities and falls back to lower priority endpoints when higher ones are unavailable.

### Health Checking
- Continuous monitoring with configurable intervals
- Circuit breaker pattern for failing endpoints
- Automatic recovery when endpoints become healthy

### Rate Limiting
- Per-IP and global rate limits
- Burst handling support
- Configurable via `config.yaml` or environment variables

### Configuration
Primary configuration is in `config.yaml`. Key sections:
- `server`: Host, port, timeouts
- `proxy`: 
  - `engine`: Choose "sherpa" or "olla"
  - `load_balancer`: Strategy selection
  - `max_idle_conns`: Connection pool size (Olla only)
  - `max_conns_per_host`: Per-host limit (Olla only)
  - `idle_conn_timeout`: Connection timeout (Olla only)
- `discovery`: Endpoint definitions with priorities
- `security`: Rate limits, request size limits
- `logging`: Level and format settings

## Testing Strategy

1. **Unit Tests**: Test individual components in isolation
2. **Integration Tests**: Test full request flow through the proxy
3. **Benchmark Tests**: 
   - Performance of critical paths
   - Proxy engine comparisons
   - Connection pooling efficiency
   - Circuit breaker behavior
4. **Security Tests**: Validate rate limiting and size restrictions (see `/test/scripts/security/`)
5. **Shared Proxy Tests**: Common test suite for both proxy engines ensuring compatibility

## Important Notes

- The project uses Go 1.24 with module support
- All internal packages are under `/internal/` (not importable by external projects)
- Public utilities are in `/pkg/`
- Always run `make lint` before committing
- The main entry point is `main.go` which sets up logging, profiling, and graceful shutdown
- Health endpoint: `GET /internal/health`
- Status endpoint: `GET /internal/status`

## Response Headers

Olla adds the following headers to all proxied responses:

- `X-Olla-Endpoint`: The name of the endpoint that handled the request (e.g., "ollama-local")
- `X-Olla-Model`: The actual model used (only present if a specific model was requested)
- `X-Served-By`: Standard compatibility header in format "olla/{endpoint-name}"

These headers help with debugging routing decisions and are set before copying upstream headers to prevent override.

## Environment Variables

Key environment variables override config values:
- `OLLA_HOST`: Server host
- `OLLA_PORT`: Server port
- `OLLA_CONFIG`: Path to config file
- `OLLA_LOG_LEVEL`: Logging level (debug, info, warn, error)
- `OLLA_PROXY_ENGINE`: Proxy engine ("sherpa" or "olla")
- `OLLA_PROXY_MAX_IDLE_CONNS`: Max idle connections (Olla engine)
- `OLLA_PROXY_MAX_CONNS_PER_HOST`: Max connections per host (Olla engine)

## Performance Considerations

- **Proxy Engine Selection**:
  - Use Sherpa for simple deployments with moderate traffic
  - Use Olla for high-throughput production workloads
- **Connection Pooling**: 
  - Generic pool implementation in `/pkg/pool/lite_pool.go`
  - Olla engine maintains per-endpoint connection pools
  - Default: 100 idle connections, 50 per host
- **Statistics**: 
  - Lock-free atomic operations for minimal overhead
  - Automatic cleanup of stale endpoint data
- **Memory Optimization**:
  - Olla uses object pooling to reduce GC pressure
  - Larger buffers (64KB) optimized for streaming
- **Circuit Breakers**: Olla engine prevents cascade failures
- Use priority balancer for best performance with multiple endpoints