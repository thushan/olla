---
title: Benchmarking Guide - Performance Testing for Olla
description: Comprehensive guide to benchmarking Olla. Learn how to measure, analyse, and optimise performance.
keywords: olla benchmarking, go benchmarks, performance testing, optimisation
---

# Benchmarking Guide

This guide covers performance testing and optimisation techniques for Olla.

## Quick Start

```bash
# Run all benchmarks
make bench

# Run specific benchmarks
go test -bench=BenchmarkProxy ./internal/adapter/proxy/

# With memory profiling
go test -bench=. -benchmem ./...

# Run for specific duration
go test -bench=. -benchtime=10s ./...
```

## Writing Benchmarks

### Basic Benchmark Structure

```go
func BenchmarkEndpointSelection(b *testing.B) {
    // Setup - not timed
    endpoints := generateEndpoints(100)
    selector := NewPrioritySelector()
    
    // Reset timer after setup
    b.ResetTimer()
    
    // Report allocations
    b.ReportAllocs()
    
    // Run benchmark
    for i := 0; i < b.N; i++ {
        selector.Select(endpoints)
    }
}
```

### Sub-benchmarks

```go
func BenchmarkBalancers(b *testing.B) {
    endpoints := generateEndpoints(100)
    
    b.Run("Priority", func(b *testing.B) {
        selector := NewPrioritySelector()
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            selector.Select(endpoints)
        }
    })
    
    b.Run("RoundRobin", func(b *testing.B) {
        selector := NewRoundRobinSelector()
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            selector.Select(endpoints)
        }
    })
}
```

### Table-Driven Benchmarks

```go
func BenchmarkPayloadSizes(b *testing.B) {
    sizes := []struct {
        name string
        size int
    }{
        {"1KB", 1024},
        {"10KB", 10 * 1024},
        {"100KB", 100 * 1024},
        {"1MB", 1024 * 1024},
    }
    
    for _, tc := range sizes {
        b.Run(tc.name, func(b *testing.B) {
            data := generatePayload(tc.size)
            b.ResetTimer()
            b.SetBytes(int64(tc.size))
            
            for i := 0; i < b.N; i++ {
                processPayload(data)
            }
        })
    }
}
```

## Key Benchmarks

### Proxy Engine Comparison

Compare Sherpa vs Olla performance:

```bash
# Run proxy benchmarks
go test -bench=BenchmarkProxyComparison -benchmem \
    ./internal/adapter/proxy/

# Example output:
# BenchmarkProxyComparison/Sherpa-8    10000    115623 ns/op    4096 B/op    42 allocs/op
# BenchmarkProxyComparison/Olla-8      12000     98456 ns/op    3072 B/op    35 allocs/op
```

### Load Balancer Performance

```bash
# Test balancer strategies
go test -bench=BenchmarkBalancer -benchmem \
    ./internal/adapter/balancer/
```

### Concurrent Performance

```go
func BenchmarkConcurrentStats(b *testing.B) {
    stats := NewStats()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            stats.IncrementRequests()
            stats.RecordLatency(100 * time.Millisecond)
        }
    })
}
```

## Memory Profiling

### Allocation Analysis

```bash
# Generate memory profile
go test -bench=. -memprofile=mem.prof ./...

# Analyse allocations
go tool pprof -alloc_space mem.prof

# Show top allocators
(pprof) top10

# View specific function
(pprof) list functionName
```

### Escape Analysis

```bash
# Show escape analysis
go build -gcflags="-m -m" ./...

# Example output:
# ./internal/adapter/stats/collector.go:42: moved to heap: data
# ./internal/adapter/stats/collector.go:43: inlining call to atomic.AddInt64
```

## CPU Profiling

### Generate CPU Profile

```bash
# Profile for 30 seconds
go test -bench=. -cpuprofile=cpu.prof -benchtime=30s ./...

# Analyse profile
go tool pprof cpu.prof

# Interactive commands:
(pprof) top           # Show top functions
(pprof) web           # Open in browser
(pprof) list function # Show source
```

### Profile Running Service

```go
import _ "net/http/pprof"

// In main.go
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

```bash
# Profile live service
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

## Optimisation Techniques

### 1. Reduce Allocations

```go
// Bad - allocates on each call
func processRequest() []byte {
    return make([]byte, 1024)
}

// Good - uses pool
var bufferPool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 1024)
        return &buf
    },
}

func processRequest() []byte {
    bufPtr := bufferPool.Get().(*[]byte)
    defer bufferPool.Put(bufPtr)
    return *bufPtr
}
```

### 2. Minimise Interface Conversions

```go
// Bad - interface conversion overhead
func process(v interface{}) {
    str := v.(string)
    // use str
}

// Good - concrete type
func process(str string) {
    // use str directly
}
```

### 3. Pre-allocate Slices

```go
// Bad - grows dynamically
var results []Result
for _, item := range items {
    results = append(results, process(item))
}

// Good - pre-allocated
results := make([]Result, 0, len(items))
for _, item := range items {
    results = append(results, process(item))
}
```

### 4. Use Atomic Operations

```go
// Bad - mutex for counter
type Counter struct {
    mu    sync.Mutex
    value int64
}

func (c *Counter) Inc() {
    c.mu.Lock()
    c.value++
    c.mu.Unlock()
}

// Good - atomic operation
type Counter struct {
    value int64
}

func (c *Counter) Inc() {
    atomic.AddInt64(&c.value, 1)
}
```

## Benchmark Targets

### Performance Goals

| Component | Target | Measurement |
|-----------|--------|-------------|
| **Request Latency** | < 5ms overhead | p99 latency |
| **Throughput** | > 10K req/s | Single core |
| **Memory** | < 100MB | Under load |
| **Allocations** | < 50/request | Steady state |

### Critical Path Benchmarks

Focus on these hot paths:

1. **Endpoint Selection**: < 100ns
2. **Health Checking**: < 1ms
3. **Stats Collection**: < 50ns overhead
4. **Request Forwarding**: < 1ms overhead

## Continuous Benchmarking

### GitHub Actions

```yaml
name: Benchmarks
on:
  pull_request:
    paths:
      - 'internal/**'
      - 'pkg/**'

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Run benchmarks
        run: |
          go test -bench=. -benchmem -count=3 \
            -benchtime=10s ./... | tee new.txt
      
      - name: Compare with main
        run: |
          git checkout main
          go test -bench=. -benchmem -count=3 \
            -benchtime=10s ./... | tee old.txt
          
          # Install benchstat
          go install golang.org/x/perf/cmd/benchstat@latest
          
          # Compare results
          benchstat old.txt new.txt
```

### Local Comparison

```bash
# Benchmark current branch
git stash
go test -bench=. -count=5 ./... > new.txt

# Benchmark main branch
git checkout main
go test -bench=. -count=5 ./... > old.txt

# Compare
benchstat old.txt new.txt
```

## Analysis Tools

### benchstat

Statistical comparison of benchmarks:

```bash
# Install
go install golang.org/x/perf/cmd/benchstat@latest

# Compare
benchstat old.txt new.txt

# Example output:
# name         old time/op  new time/op  delta
# Proxy-8      105µs ± 2%   95µs ± 1%   -9.52%  (p=0.000 n=10+10)
```

### pprof Web UI

```bash
# Start web interface
go tool pprof -http=:8080 cpu.prof

# Opens browser with:
# - Flame graph
# - Top functions
# - Source view
# - Call graph
```

### trace Tool

```bash
# Generate trace
go test -trace=trace.out -bench=.

# View trace
go tool trace trace.out
```

## Best Practices

### 1. Benchmark Hygiene

```go
func BenchmarkFunction(b *testing.B) {
    // Setup outside timer
    data := prepareData()
    
    // Reset after setup
    b.ResetTimer()
    
    // Report metrics
    b.ReportAllocs()
    b.SetBytes(int64(len(data)))
    
    // Prevent compiler optimisations
    var result int
    for i := 0; i < b.N; i++ {
        result = function(data)
    }
    _ = result
}
```

### 2. Realistic Workloads

```go
// Bad - unrealistic
func BenchmarkEmpty(b *testing.B) {
    for i := 0; i < b.N; i++ {
        processEmpty("")
    }
}

// Good - representative data
func BenchmarkRealistic(b *testing.B) {
    requests := loadRealRequests()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        processRequest(requests[i%len(requests)])
    }
}
```

### 3. Stable Environment

- Close unnecessary applications
- Disable CPU frequency scaling
- Use consistent hardware
- Run multiple iterations

```bash
# Disable CPU scaling (Linux)
sudo cpupower frequency-set --governor performance

# Run stable benchmark
go test -bench=. -count=10 -benchtime=10s
```

## Common Pitfalls

### Compiler Optimisations

```go
// Bad - result discarded, may be optimised away
func BenchmarkBad(b *testing.B) {
    for i := 0; i < b.N; i++ {
        expensiveOperation()
    }
}

// Good - use result
func BenchmarkGood(b *testing.B) {
    var result int
    for i := 0; i < b.N; i++ {
        result = expensiveOperation()
    }
    _ = result
}
```

### Timer Pollution

```go
// Bad - includes setup in timing
func BenchmarkBad(b *testing.B) {
    for i := 0; i < b.N; i++ {
        data := generateData() // Timed!
        processData(data)
    }
}

// Good - setup outside loop
func BenchmarkGood(b *testing.B) {
    data := generateData()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        processData(data)
    }
}
```

## Next Steps

- Review [Testing Guide](testing.md) for test patterns
- See [Technical Patterns](patterns.md) for optimisation techniques
- Check current benchmarks in `internal/adapter/proxy/`
- Run `make bench` to establish baseline