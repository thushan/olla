---
title: Testing Guide - Olla Testing Patterns and Strategies
description: Comprehensive testing guide for Olla. Learn testing patterns, strategies, and best practices for unit and integration tests.
keywords: olla testing, go testing, unit tests, integration tests, test patterns
---

# Testing Guide

This guide covers testing patterns and strategies used in Olla.

## Testing Philosophy

- **Test behaviour, not implementation**
- **Fast, reliable, and isolated tests**
- **Comprehensive coverage of critical paths**
- **Shared test suites for common interfaces**

## Test Organisation

```
.
├── internal/
│   └── */                  # Unit tests alongside code
│       └── *_test.go
├── test/
│   ├── integration/        # Integration tests
│   ├── scripts/           # Test scripts
│   └── fixtures/          # Test data
└── benchmarks/            # Performance benchmarks
```

## Unit Testing

### Basic Test Structure

```go
func TestEndpointHealth(t *testing.T) {
    // Arrange
    endpoint := &Endpoint{
        URL: "http://localhost:8080",
        Health: StatusHealthy,
    }
    
    // Act
    result := endpoint.IsHealthy()
    
    // Assert
    if !result {
        t.Errorf("expected healthy endpoint, got unhealthy")
    }
}
```

### Table-Driven Tests

```go
func TestEndpointSelection(t *testing.T) {
    tests := []struct {
        name      string
        endpoints []*Endpoint
        expected  string
        wantErr   bool
    }{
        {
            name: "selects highest priority",
            endpoints: []*Endpoint{
                {URL: "http://a", Priority: 50},
                {URL: "http://b", Priority: 100},
            },
            expected: "http://b",
            wantErr:  false,
        },
        {
            name:      "returns error when no endpoints",
            endpoints: []*Endpoint{},
            expected:  "",
            wantErr:   true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            selector := NewPrioritySelector()
            result, err := selector.Select(tt.endpoints)
            
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            
            if result != nil && result.URL != tt.expected {
                t.Errorf("got %v, want %v", result.URL, tt.expected)
            }
        })
    }
}
```

### Testing Concurrent Code

```go
func TestConcurrentStats(t *testing.T) {
    stats := NewStats()
    
    var wg sync.WaitGroup
    workers := 100
    iterations := 1000
    
    wg.Add(workers)
    for i := 0; i < workers; i++ {
        go func() {
            defer wg.Done()
            for j := 0; j < iterations; j++ {
                stats.IncrementRequests()
            }
        }()
    }
    
    wg.Wait()
    
    expected := workers * iterations
    if got := stats.GetRequestCount(); got != expected {
        t.Errorf("expected %d requests, got %d", expected, got)
    }
}
```

### Testing with Context

```go
func TestRequestWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    service := NewService()
    
    // Simulate slow operation
    err := service.SlowOperation(ctx)
    
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected timeout error, got %v", err)
    }
}
```

## Integration Testing

### Shared Test Suites

Both proxy engines share test suites:

```go
// internal/adapter/proxy/shared_test.go
func runProxyTests(t *testing.T, factory ProxyFactory) {
    t.Run("forwards request", func(t *testing.T) {
        testForwardsRequest(t, factory)
    })
    
    t.Run("handles streaming", func(t *testing.T) {
        testHandlesStreaming(t, factory)
    })
    
    t.Run("circuit breaker", func(t *testing.T) {
        testCircuitBreaker(t, factory)
    })
}

// internal/adapter/proxy/sherpa/service_test.go
func TestSherpaProxy(t *testing.T) {
    factory := NewSherpaFactory()
    runProxyTests(t, factory)
}

// internal/adapter/proxy/olla/service_test.go
func TestOllaProxy(t *testing.T) {
    factory := NewOllaFactory()
    runProxyTests(t, factory)
}
```

### Test Servers

```go
func TestProxyForwarding(t *testing.T) {
    // Create test backend
    backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("test response"))
    }))
    defer backend.Close()
    
    // Configure proxy
    proxy := NewProxy([]Endpoint{{URL: backend.URL}})
    
    // Create test request
    req := httptest.NewRequest("GET", "/test", nil)
    rec := httptest.NewRecorder()
    
    // Test
    err := proxy.ProxyRequest(context.Background(), rec, req)
    if err != nil {
        t.Fatalf("proxy failed: %v", err)
    }
    
    // Verify
    if rec.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", rec.Code)
    }
    
    if body := rec.Body.String(); body != "test response" {
        t.Errorf("expected 'test response', got %s", body)
    }
}
```

### Testing Streaming

```go
func TestStreamingResponse(t *testing.T) {
    // Create streaming backend
    backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        flusher := w.(http.Flusher)
        
        for i := 0; i < 3; i++ {
            fmt.Fprintf(w, "chunk %d\n", i)
            flusher.Flush()
            time.Sleep(10 * time.Millisecond)
        }
    }))
    defer backend.Close()
    
    proxy := NewProxy([]Endpoint{{URL: backend.URL}})
    
    req := httptest.NewRequest("GET", "/stream", nil)
    rec := httptest.NewRecorder()
    
    err := proxy.ProxyRequest(context.Background(), rec, req)
    if err != nil {
        t.Fatalf("streaming failed: %v", err)
    }
    
    expected := "chunk 0\nchunk 1\nchunk 2\n"
    if got := rec.Body.String(); got != expected {
        t.Errorf("expected %q, got %q", expected, got)
    }
}
```

## Mocking

### Interface Mocking

```go
type MockDiscoveryService struct {
    endpoints []*Endpoint
    err       error
}

func (m *MockDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*Endpoint, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.endpoints, nil
}

func TestProxyWithMockDiscovery(t *testing.T) {
    mock := &MockDiscoveryService{
        endpoints: []*Endpoint{
            {URL: "http://test", Health: StatusHealthy},
        },
    }
    
    proxy := NewProxy(mock)
    
    // Test with mock
    endpoints, err := proxy.GetAvailableEndpoints(context.Background())
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if len(endpoints) != 1 {
        t.Errorf("expected 1 endpoint, got %d", len(endpoints))
    }
}
```

### HTTP Client Mocking

```go
type MockHTTPClient struct {
    response *http.Response
    err      error
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.response, nil
}

func TestHealthCheckWithMock(t *testing.T) {
    client := &MockHTTPClient{
        response: &http.Response{
            StatusCode: http.StatusOK,
            Body:       io.NopCloser(strings.NewReader("healthy")),
        },
    }
    
    checker := NewHealthChecker(client)
    healthy := checker.Check("http://test")
    
    if !healthy {
        t.Error("expected healthy, got unhealthy")
    }
}
```

## Test Helpers

### Common Test Utilities

```go
// test/helpers/endpoints.go
func GenerateEndpoints(count int) []*Endpoint {
    endpoints := make([]*Endpoint, count)
    for i := 0; i < count; i++ {
        endpoints[i] = &Endpoint{
            URL:      fmt.Sprintf("http://endpoint-%d", i),
            Priority: rand.Intn(100),
            Health:   StatusHealthy,
        }
    }
    return endpoints
}

// test/helpers/context.go
func ContextWithTimeout(t *testing.T, timeout time.Duration) context.Context {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    t.Cleanup(cancel)
    return ctx
}
```

### Test Fixtures

```go
// test/fixtures/config.go
func LoadTestConfig(t *testing.T) *Config {
    data, err := os.ReadFile("testdata/config.yaml")
    if err != nil {
        t.Fatalf("failed to load test config: %v", err)
    }
    
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        t.Fatalf("failed to parse test config: %v", err)
    }
    
    return &cfg
}
```

## Race Detection

Always test concurrent code with race detection:

```bash
go test -race ./...
```

Example race-safe test:

```go
func TestRaceSafety(t *testing.T) {
    service := NewService()
    
    // Run concurrent operations
    go service.Write("data1")
    go service.Write("data2")
    go service.Read()
    
    // Give goroutines time to execute
    time.Sleep(100 * time.Millisecond)
}
```

## Coverage

### Generate Coverage Report

```bash
# Run with coverage
go test -cover -coverprofile=coverage.out ./...

# View in terminal
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

### Coverage Goals

- **Critical paths**: 90%+ coverage
- **Business logic**: 80%+ coverage
- **Overall**: 70%+ coverage

## Test Performance

### Keep Tests Fast

```go
// Bad - slow test
func TestSlowOperation(t *testing.T) {
    time.Sleep(5 * time.Second) // Don't do this
    // ...
}

// Good - use time manipulation
func TestWithMockTime(t *testing.T) {
    clock := &MockClock{now: time.Now()}
    service := NewServiceWithClock(clock)
    
    // Advance time instantly
    clock.Advance(5 * time.Second)
    
    // Test timeout behaviour
    if !service.IsTimedOut() {
        t.Error("expected timeout")
    }
}
```

### Parallel Tests

```go
func TestParallel(t *testing.T) {
    t.Parallel() // Run in parallel with other tests
    
    // Test code here
}

func TestSubtests(t *testing.T) {
    t.Run("subtest1", func(t *testing.T) {
        t.Parallel() // Subtests can also be parallel
        // Test code
    })
    
    t.Run("subtest2", func(t *testing.T) {
        t.Parallel()
        // Test code
    })
}
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Run tests
        run: make test
      
      - name: Race detection
        run: go test -race ./...
      
      - name: Coverage
        run: |
          go test -cover -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out
```

## Best Practices

### 1. Test Independence

Tests should not depend on each other:

```go
// Bad - depends on order
var sharedState int

func TestFirst(t *testing.T) {
    sharedState = 42
}

func TestSecond(t *testing.T) {
    if sharedState != 42 { // Fails if TestFirst didn't run
        t.Error("unexpected state")
    }
}

// Good - independent
func TestIndependent(t *testing.T) {
    state := setupState()
    defer cleanupState(state)
    
    // Test with local state
}
```

### 2. Clear Test Names

```go
// Bad
func TestProxy1(t *testing.T) {}
func TestProxy2(t *testing.T) {}

// Good
func TestProxy_ForwardsRequest_Success(t *testing.T) {}
func TestProxy_CircuitBreaker_OpensAfterFailures(t *testing.T) {}
```

### 3. Cleanup Resources

```go
func TestWithCleanup(t *testing.T) {
    resource := acquireResource()
    t.Cleanup(func() {
        releaseResource(resource)
    })
    
    // Test code
}
```

## Next Steps

- Review [Benchmarking Guide](benchmarking.md) for performance testing
- Check [Contributing Guide](contributing.md) for submission standards
- See example tests in the codebase