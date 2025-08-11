---
title: Developer Guide - Olla Development Overview
description: Overview of Olla's architecture, key patterns, and development workflow. Start here to understand the codebase.
keywords: olla development, golang patterns, hexagonal architecture, developer guide
---

# Developer Guide

Welcome to Olla development. This guide provides an overview of the architecture and key patterns. For detailed information, see the specific guides linked throughout.


!!! note "Developed on Linux & macOS"
    Primary development has been done on Linux and macOS, you can develop on Windows but you may hit UAC prompts for port usage and occasional pauses at startup.

## Quick Start

```bash
# Clone and setup
git clone https://github.com/thushan/olla.git
cd olla
make deps

# Development workflow
make dev      # Build with hot-reload
make test     # Run tests
make ready    # Pre-commit checks
```

See [Development Setup](setup.md) for detailed environment configuration.

## Architecture

Olla follows **hexagonal architecture** (ports & adapters) with three distinct layers:

```
internal/
├── core/           # Domain layer - business logic, zero external dependencies
├── adapter/        # Infrastructure - implementations of core interfaces
└── app/            # Application layer - HTTP handlers, orchestration
```

**Key principle**: Dependencies point inward. Core has no dependencies on outer layers.

See [Architecture](architecture.md) for component deep-dive.

## Core Patterns

### Service Management

Services use dependency injection with topological sorting:

```go
type ManagedService interface {
    Name() string
    Dependencies() []string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Concurrency

Heavy use of lock-free patterns for performance:

- **Atomic operations** for statistics and state
- **xsync** library for concurrent maps and counters
- **Worker pools** for controlled concurrency

```go
// Example: Lock-free stats from stats/collector.go
type Collector struct {
    totalRequests *xsync.Counter
    endpoints     *xsync.Map[string, *endpointData]
}
```

### Memory Optimisation

Object pooling reduces GC pressure:

```go
type Service struct {
    bufferPool *pool.Pool[*[]byte]
}
```

See [Technical Patterns](patterns.md) for comprehensive pattern documentation.

## Project Structure

```
.
├── cmd/            # Application entry points
├── internal/       # Private application code
│   ├── core/      # Business logic
│   ├── adapter/   # External integrations
│   └── app/       # Application layer
├── pkg/           # Public packages
├── config/        # Configuration files
├── test/          # Integration tests
└── docs/          # Documentation
```

## Key Components

### Proxy Engines

Two implementations with different trade-offs:

| Engine | Description | Use Case |
|--------|-------------|----------|
| **Sherpa** | Simple, shared HTTP transport | Development, moderate load |
| **Olla** | Per-endpoint connection pools | Production, high throughput |

See [Proxy Engines](../concepts/proxy-engines.md) for detailed comparison.

### Load Balancing

Three strategies available:

- **Priority**: Routes to highest priority endpoint
- **Round-Robin**: Distributes requests evenly
- **Least-Connections**: Routes to least busy endpoint

### Health Checking

Automatic health monitoring with circuit breakers:

- Configurable check intervals
- Exponential backoff on failures
- Circuit breaker pattern (3 failures = open)

## Development Workflow

### Code Style

```bash
# Format code
make fmt

# Run linters
make lint

# All checks (run before commit)
make ready
```

### Testing

```bash
# Unit tests
make test

# With race detection
go test -race ./...

# Benchmarks
make bench
```

See [Testing Guide](testing.md) for testing patterns.

### Common Tasks

#### Adding a New Endpoint Type

1. Create profile in `config/profiles/`
2. Implement converter in `internal/adapter/converter/`
3. Add to profile registry
4. Write tests

#### Modifying Proxy Behaviour

1. Check both `sherpa` and `olla` implementations
2. Update shared test suite in `internal/adapter/proxy/`
3. Run benchmarks to verify performance

#### Adding Statistics

1. Update `stats.Collector` with new metrics
2. Use atomic operations or xsync
3. Expose via status endpoints

## Key Libraries

- **[puzpuzpuz/xsync](https://github.com/puzpuzpuz/xsync)**: Lock-free data structures
- **[docker/go-units](https://github.com/docker/go-units)**: Human-readable formatting
- **Standard library**: Extensive use of `context`, `sync/atomic`, `net/http`

## Performance Considerations

- **Pre-allocate slices**: `make([]T, 0, capacity)`
- **Use object pools**: Reduce allocations in hot paths
- **Atomic operations**: Prefer over mutexes for counters
- **Context propagation**: Always pass context through call chain

## Common Pitfalls

### Context Handling

```go
// Bad - new context loses request metadata
ctx := context.Background()

// Good - propagate request context
func (s *Service) Process(ctx context.Context) error
```

### Resource Cleanup

```go
// Always close response bodies
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### Concurrent Map Access

```go
// Bad - race condition
regularMap[key] = value

// Good - use xsync
concurrentMap.Store(key, value)
```

## Debugging

### Request Tracing

Every request gets a unique ID for tracing:

```bash
# Check logs for request flow
grep "request_id=abc123" olla.log
```

### Performance Profiling

```bash
# CPU profile
go tool pprof http://localhost:40114/debug/pprof/profile

# Memory profile
go tool pprof http://localhost:40114/debug/pprof/heap
```

## Getting Help

- Check existing tests for examples
- Review [Technical Patterns](patterns.md) for detailed patterns
- See [Contributing Guide](contributing.md) for submission process
- Ask in [GitHub Issues](https://github.com/thushan/olla/issues)

## Next Steps

- [Development Setup](setup.md) - Configure your environment
- [Architecture](architecture.md) - System design and implementation
- [Technical Patterns](patterns.md) - Deep dive into patterns
- [Circuit Breaker](circuit-breaker.md) - Resilience patterns
- [Testing Guide](testing.md) - Testing strategies
- [Contributing](contributing.md) - Contribution process
- [Benchmarking](benchmarking.md) - Performance testing