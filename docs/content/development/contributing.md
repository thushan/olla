---
title: Contributing to Olla - Guidelines and Process
description: Learn how to contribute to Olla. Code standards, pull request process, and community guidelines.
keywords: olla contributing, pull requests, code standards, open source contribution
---

# Contributing to Olla

Thank you for considering contributing to Olla! This guide will help you get started.

## Code of Conduct

Be respectful and constructive. We're all here to build something great together.

## Getting Started

### 1. Fork and Clone

```bash
# Fork on GitHub, then:
git clone https://github.com/YOUR_USERNAME/olla.git
cd olla
git remote add upstream https://github.com/thushan/olla.git
```

### 2. Create a Branch

```bash
# Update main branch
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/your-feature-name
```

### 3. Make Changes

Follow the coding standards and ensure tests pass:

```bash
# Make your changes
vim internal/...

# Run checks
make ready
```

### 4. Commit

Write clear commit messages:

```bash
git add .
git commit -m "feat: add support for new endpoint type

- Implement profile for X provider
- Add converter for X API format
- Include tests and documentation"
```

## Commit Message Format

Follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, etc)
- `refactor`: Code restructuring
- `perf`: Performance improvement
- `test`: Adding tests
- `chore`: Maintenance tasks

## Code Standards

### Go Style

Follow standard Go conventions:

```go
// Good - exported types have comments
// Endpoint represents a backend LLM service
type Endpoint struct {
    URL      string
    Priority int
}

// Good - error handling
resp, err := client.Do(req)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}
defer resp.Body.Close()
```

### Package Structure

```go
// Good - small, focused packages
package balancer   // Just load balancing logic

// Bad - kitchen sink packages
package utils      // Too generic
```

### Error Handling

```go
// Define specific errors
var (
    ErrEndpointNotFound = errors.New("endpoint not found")
    ErrCircuitOpen      = errors.New("circuit breaker open")
)

// Wrap errors with context
if err := doSomething(); err != nil {
    return fmt.Errorf("failed to process: %w", err)
}
```

### Testing

Every feature needs tests:

```go
func TestEndpointSelection(t *testing.T) {
    // Arrange
    endpoints := []*Endpoint{
        {URL: "http://a", Priority: 100},
        {URL: "http://b", Priority: 50},
    }
    
    // Act
    selected := selector.Select(endpoints)
    
    // Assert
    if selected.URL != "http://a" {
        t.Errorf("expected highest priority, got %s", selected.URL)
    }
}
```

### Comments

Write comments for "why", not "what":

```go
// Bad - states the obvious
// Increment counter
counter++

// Good - explains reasoning
// Use exponential backoff to avoid overwhelming
// the endpoint during recovery
delay := time.Second * time.Duration(math.Pow(2, float64(attempts)))
```

## Pull Request Process

### 1. Before Submitting

- [ ] Tests pass: `make test`
- [ ] Linting passes: `make lint`
- [ ] Code formatted: `make fmt`
- [ ] Documentation updated
- [ ] Benchmarks run (if performance-critical)

### 2. PR Description

Use the template:

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex parts
- [ ] Documentation updated
- [ ] No new warnings
```

### 3. Review Process

1. Automated checks run
2. Maintainer review
3. Address feedback
4. Merge when approved

## Testing Requirements

### Unit Tests

Required for all new code:

```bash
# Run unit tests
go test ./internal/...

# With coverage
go test -cover ./internal/...
```

### Integration Tests

For new features:

```bash
# Run integration tests
go test ./test/...
```

### Benchmarks

For performance-critical code:

```bash
# Run benchmarks
go test -bench . ./internal/adapter/proxy/...
```

## Documentation

### Code Documentation

```go
// ProxyService handles request forwarding to backend endpoints.
// It implements circuit breaking, retry logic, and connection pooling.
type ProxyService interface {
    // ProxyRequest forwards an HTTP request to a backend endpoint.
    // Returns an error if no healthy endpoints are available.
    ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}
```

### User Documentation

Update relevant docs in `docs/content/`:

- API changes: `api/reference.md`
- New features: Appropriate concept guide
- Configuration: `configuration/reference.md`

## Adding New Features

### New Endpoint Type

1. Create profile in `config/profiles/`
2. Implement converter in `internal/adapter/converter/`
3. Add tests in `internal/adapter/converter/`
4. Update documentation

### New Load Balancer

1. Implement `EndpointSelector` interface
2. Add to balancer factory
3. Write comprehensive tests
4. Document behaviour

### New Statistics

1. Update `stats.Collector`
2. Use atomic operations
3. Expose via status endpoint
4. Add to documentation

## Performance Guidelines

### Benchmarking

Always benchmark performance-critical code:

```go
func BenchmarkEndpointSelection(b *testing.B) {
    endpoints := generateEndpoints(100)
    selector := NewPrioritySelector()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        selector.Select(endpoints)
    }
}
```

### Memory Allocation

Minimise allocations:

```go
// Bad - allocates on each call
func process() []byte {
    return make([]byte, 1024)
}

// Good - reuses buffer
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 1024)
    },
}

func process() []byte {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf)
    return buf
}
```

## Common Contributions

### Bug Fixes

1. Create failing test
2. Fix the bug
3. Ensure test passes
4. Submit PR with test

### Documentation

1. Identify gap or error
2. Make correction
3. Verify accuracy
4. Submit PR

### Performance Improvements

1. Benchmark current performance
2. Implement improvement
3. Benchmark new performance
4. Include results in PR

## Troubleshooting {#troubleshooting}

### Build Issues

If you encounter build problems:

```bash
# Clean and rebuild
make clean
make build

# Check Go version
go version  # Should be 1.24+

# Update dependencies
go mod tidy
go mod download
```

### Test Failures

For failing tests:

```bash
# Run with verbose output
go test -v ./internal/...

# Run specific test
go test -v ./internal/adapter/proxy/ -run TestProxyService

# Check for race conditions
go test -race ./internal/...
```

### Development Environment

Common setup issues:

1. **Path Issues**: Ensure `$GOPATH/bin` is in your `$PATH`
2. **Module Issues**: Run `go mod tidy` to fix dependency issues
3. **Version Issues**: Use Go 1.24+ as specified in `go.mod`

### Performance Problems

If benchmarks are slow:

```bash
# Profile CPU usage
go test -bench=. -cpuprofile=cpu.prof ./internal/adapter/proxy/

# Profile memory usage
go test -bench=. -memprofile=mem.prof ./internal/adapter/proxy/
```

## Getting Help

### Discord

Join our Discord for discussions: [Coming Soon]

### GitHub Issues

- Check existing issues first
- Use issue templates
- Provide reproduction steps

### Pull Request Help

Tag maintainers for review:
- @thushan - Project lead

## Recognition

Contributors are recognised in:
- CONTRIBUTORS.md file
- Release notes
- Project documentation

## Legal

By contributing, you agree that your contributions will be licensed under the same license as the project (Apache 2.0).

## Next Steps

1. Set up your [Development Environment](setup.md)
2. Review [Technical Patterns](patterns.md)
3. Check [Testing Guide](testing.md)
4. Start with a [good first issue](https://github.com/thushan/olla/labels/good%20first%20issue)