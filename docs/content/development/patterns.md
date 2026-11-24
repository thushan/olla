---
title: Technical Patterns - Advanced Go Patterns in Olla
description: Deep dive into the technical patterns and Go idioms used throughout Olla's codebase. Learn about concurrency, memory optimisation, and architectural patterns.
keywords: go patterns, concurrency patterns, memory optimisation, xsync, atomic operations, object pooling
---

# Technical Patterns

This document details the advanced Go patterns and techniques used throughout Olla's codebase. Understanding these patterns is crucial for maintaining consistency and performance.

## Concurrency Patterns

### Lock-Free Data Structures with xsync

Olla heavily leverages `github.com/puzpuzpuz/xsync/v4` for lock-free concurrent data structures:

```go
// Thread-safe map without locks
type EndpointRegistry struct {
    endpoints *xsync.Map[string, *Endpoint]
}

// Concurrent access without explicit synchronisation
func (r *EndpointRegistry) UpdateEndpoint(url string, endpoint *Endpoint) {
    r.endpoints.Store(url, endpoint)
}

func (r *EndpointRegistry) GetEndpoint(url string) (*Endpoint, bool) {
    return r.endpoints.Load(url)
}
```

**Why xsync over sync.Map:**

- Type-safe with generics
- Better performance for read-heavy workloads
- More predictable memory usage
- Range operations don't block writers

### Atomic Operations for Statistics

All statistics collection uses atomic operations to avoid lock contention:

```go
type ModelStats struct {
    requestCount    atomic.Int64
    totalDuration   atomic.Int64
    errorCount      atomic.Int64
    bytesProcessed  atomic.Int64
}

func (s *ModelStats) RecordRequest(duration time.Duration, bytes int64) {
    s.requestCount.Add(1)
    s.totalDuration.Add(int64(duration))
    s.bytesProcessed.Add(bytes)
}

// Lock-free read
func (s *ModelStats) GetAverageLatency() time.Duration {
    count := s.requestCount.Load()
    if count == 0 {
        return 0
    }
    total := s.totalDuration.Load()
    return time.Duration(total / count)
}
```

### Circuit Breaker State Machine

There are two circuit breaker implementations. Neither uses `atomic.Int64` wrapper types; both use raw `int64` fields with `sync/atomic` package calls.

**Health-checker CB** (`internal/adapter/health/circuit_breaker.go`): keyed by endpoint URL in an `xsync.Map`; threshold = 3; state encoded as `int32` (`isOpen`).

```go
// internal/adapter/health/circuit_breaker.go
type CircuitBreaker struct {
    endpoints        *xsync.Map[string, *circuitState]
    failureThreshold int
    timeout          time.Duration
}

type circuitState struct {
    failures    int64 // atomic
    lastFailure int64 // atomic nanoseconds
    lastAttempt int64 // atomic nanoseconds (half-open sentinel)
    isOpen      int32 // atomic: 0=closed, 1=open
}
```

**Olla-proxy CB** (`internal/adapter/proxy/olla/service.go`): one `circuitBreaker` per endpoint, stored in `xsync.Map`; threshold = 5; three-state (`int64`: 0=closed, 1=open, 2=half-open).

```go
// internal/adapter/proxy/olla/service.go
type circuitBreaker struct {
    failures    int64 // atomic
    lastFailure int64 // atomic nanoseconds
    state       int64 // atomic: 0=closed, 1=open, 2=half-open
    threshold   int64
}
```

One success closes the circuit in both implementations. HTTP 5xx responses do NOT trip either circuit breaker; only transport errors do (issue #144).

### Worker Pool Pattern

Generic worker pool for controlled concurrency:

```go
type WorkerPool[T any] struct {
    workers   int
    taskQueue chan T
    processor func(T)
    wg        sync.WaitGroup
    stop      chan struct{}
}

func NewWorkerPool[T any](workers int, processor func(T)) *WorkerPool[T] {
    wp := &WorkerPool[T]{
        workers:   workers,
        taskQueue: make(chan T, workers*10), // Buffered queue
        processor: processor,
        stop:      make(chan struct{}),
    }
    wp.start()
    return wp
}

func (wp *WorkerPool[T]) start() {
    for range wp.workers {
        wp.wg.Go(wp.worker)
    }
}

func (wp *WorkerPool[T]) worker() {
    for {
        select {
        case task := <-wp.taskQueue:
            wp.processor(task)
        case <-wp.stop:
            return
        }
    }
}

func (wp *WorkerPool[T]) Submit(task T) {
    select {
    case wp.taskQueue <- task:
        // Task queued
    default:
        // Queue full, handle backpressure
    }
}
```

## Memory Optimisation Patterns

### Generic Object Pool

Type-safe object pooling with generics. The real implementation is in `pkg/pool/lite_pool.go`:

```go
// pkg/pool/lite_pool.go
type Pool[T any] struct {
    pool sync.Pool
    new  func() T
    // No separate reset field; types implement Resettable instead
}

// NewLitePool is the sole constructor; it takes only a constructor function.
// If T implements Reset(), Put() calls it automatically.
func NewLitePool[T any](newFn func() T) (*Pool[T], error)

// Usage: types that need zeroing implement Resettable:
type requestContext struct { ... }
func (r *requestContext) Reset() { r.requestID = ""; r.startTime = time.Time{} }

pool, err := pool.NewLitePool(func() *requestContext {
    return &requestContext{}
})
ctx := pool.Get()
defer pool.Put(ctx) // Reset() called automatically
```

### Connection Pool Management

Per-endpoint connection pools with automatic cleanup. The real implementation in `internal/adapter/proxy/olla/service.go` uses raw `int64` fields with `sync/atomic` calls (not `atomic.Int64` wrapper types), and `xsync.Map` with `LoadOrStore`:

```go
// internal/adapter/proxy/olla/service.go (simplified illustrative form)
type connectionPool struct {
    transport *http.Transport
    lastUsed  int64 // atomic nanoseconds
    healthy   int64 // atomic: 0=unhealthy, 1=healthy
}

// Pools are stored in:  xsync.Map[string, *connectionPool]
// Created lazily with:  endpointPools.LoadOrStore(endpoint, newPool)
// Cleaned up by a background goroutine every 5 minutes.
```

### Buffer Reuse Pattern

Efficient buffer management for streaming:

```go
type StreamProcessor struct {
    bufferPool *Pool[*bytes.Buffer]
    chunkPool  *Pool[[]byte]
}

func (sp *StreamProcessor) ProcessStream(r io.Reader, w io.Writer) error {
    // Get buffer from pool
    chunk := sp.chunkPool.Get()
    defer sp.chunkPool.Put(chunk)
    
    buffer := sp.bufferPool.Get()
    defer sp.bufferPool.Put(buffer)
    
    // Stream processing
    for {
        n, err := r.Read(chunk)
        if n > 0 {
            buffer.Write(chunk[:n])
            
            // Process when buffer reaches threshold
            if buffer.Len() >= 8192 {
                if _, err := w.Write(buffer.Bytes()); err != nil {
                    return err
                }
                buffer.Reset()
            }
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
    }
    
    // Flush remaining
    if buffer.Len() > 0 {
        _, err := w.Write(buffer.Bytes())
        return err
    }
    return nil
}
```

## Service Lifecycle Patterns

### Dependency Injection with Service Manager

Topological sorting for dependency resolution:

```go
type ServiceManager struct {
    services    map[string]ManagedService
    depGraph    map[string][]string
    startOrder  []string
}

func (sm *ServiceManager) ResolveDependencies() error {
    // Kahn's algorithm for topological sort
    inDegree := make(map[string]int)
    for name := range sm.services {
        inDegree[name] = 0
    }
    
    for _, deps := range sm.depGraph {
        for _, dep := range deps {
            inDegree[dep]++
        }
    }
    
    queue := []string{}
    for name, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, name)
        }
    }
    
    var sorted []string
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]
        sorted = append(sorted, current)
        
        for _, neighbor := range sm.depGraph[current] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }
    
    if len(sorted) != len(sm.services) {
        return errors.New("circular dependency detected")
    }
    
    sm.startOrder = sorted
    return nil
}
```

### Two-Phase Service Initialisation

Prevents circular dependencies:

```go
// Phase 1: Create all services
func createServices(cfg *Config) map[string]any {
    services := make(map[string]any)
    
    // Create with nil dependencies
    services["stats"] = NewStatsService(nil)
    services["security"] = NewSecurityService(nil)
    services["proxy"] = NewProxyService(nil)
    
    return services
}

// Phase 2: Wire dependencies
func wireServices(services map[string]any) {
    stats := services["stats"].(*StatsService)
    security := services["security"].(*SecurityService)
    proxy := services["proxy"].(*ProxyService)
    
    // Now wire them together
    security.SetStatsService(stats)
    proxy.SetSecurityService(security)
    proxy.SetStatsService(stats)
}
```

### Graceful Shutdown Pattern

Coordinated shutdown with cleanup:

```go
type Service struct {
    shutdownCh chan struct{}
    shutdownWg sync.WaitGroup
}

func (s *Service) Start(ctx context.Context) error {
    // Start background workers
    s.shutdownWg.Add(3)
    go s.healthChecker(ctx)
    go s.metricsCollector(ctx)
    go s.connectionCleaner(ctx)
    
    return nil
}

func (s *Service) Stop(ctx context.Context) error {
    // Signal shutdown
    close(s.shutdownCh)
    
    // Wait with timeout
    done := make(chan struct{})
    go func() {
        s.shutdownWg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Service) healthChecker(ctx context.Context) {
    defer s.shutdownWg.Done()
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            s.performHealthCheck()
        case <-s.shutdownCh:
            return
        case <-ctx.Done():
            return
        }
    }
}
```

## Event System Patterns

### Generic Event Bus

Type-safe event publishing and subscription:

```go
type Event[T any] struct {
    Type      string
    Timestamp time.Time
    Data      T
}

type EventBus[T any] struct {
    subscribers *xsync.Map[string, []chan Event[T]]
    workerPool  *WorkerPool[Event[T]]
}

func (eb *EventBus[T]) Subscribe(eventType string) <-chan Event[T] {
    ch := make(chan Event[T], 100)
    
    subs, _ := eb.subscribers.LoadOrStore(eventType, []chan Event[T]{})
    subs = append(subs, ch)
    eb.subscribers.Store(eventType, subs)
    
    return ch
}

func (eb *EventBus[T]) Publish(eventType string, data T) {
    event := Event[T]{
        Type:      eventType,
        Timestamp: time.Now(),
        Data:      data,
    }
    
    if subs, ok := eb.subscribers.Load(eventType); ok {
        for _, ch := range subs {
            select {
            case ch <- event:
                // Sent
            default:
                // Channel full, drop event
            }
        }
    }
}
```

## Request Context Patterns

### Request Metadata Propagation

Context-based request tracking. Olla's own keys are typed constants in `internal/core/constants/context.go`, read and written through accessors such as `GetModel` and `WithRoutePrefix`. Add new keys there rather than using string literals; staticcheck SA1029 rejects them. The sample below is a standalone illustration of the same shape:

```go
type contextKey string

const (
    requestIDKey     contextKey = "request-id"
    endpointKey      contextKey = "endpoint"
    modelKey         contextKey = "model"
    startTimeKey     contextKey = "start-time"
)

func WithRequestMetadata(ctx context.Context, r *http.Request) context.Context {
    // Generate request ID
    requestID := generateRequestID()
    ctx = context.WithValue(ctx, requestIDKey, requestID)
    
    // Add start time
    ctx = context.WithValue(ctx, startTimeKey, time.Now())
    
    // Extract model from request
    if model := extractModel(r); model != "" {
        ctx = context.WithValue(ctx, modelKey, model)
    }
    
    return ctx
}

func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}

func GetElapsedTime(ctx context.Context) time.Duration {
    if start, ok := ctx.Value(startTimeKey).(time.Time); ok {
        return time.Since(start)
    }
    return 0
}
```

### Structured Logging with Context

Context-aware logging throughout request lifecycle:

```go
type Logger struct {
    base *slog.Logger
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
    attrs := []slog.Attr{}
    
    if requestID := GetRequestID(ctx); requestID != "" {
        attrs = append(attrs, slog.String("request_id", requestID))
    }
    
    if model := GetModel(ctx); model != "" {
        attrs = append(attrs, slog.String("model", model))
    }
    
    if endpoint := GetEndpoint(ctx); endpoint != "" {
        attrs = append(attrs, slog.String("endpoint", endpoint))
    }
    
    return &Logger{
        base: l.base.With(attrs...),
    }
}
```

## Performance Patterns

### Zero-Allocation String Building

Efficient string concatenation:

```go
// String builder pool
var stringBuilderPool = sync.Pool{
    New: func() any {
        return &strings.Builder{}
    },
}

func BuildPath(segments ...string) string {
    sb := stringBuilderPool.Get().(*strings.Builder)
    defer func() {
        sb.Reset()
        stringBuilderPool.Put(sb)
    }()
    
    for i, segment := range segments {
        if i > 0 {
            sb.WriteByte('/')
        }
        sb.WriteString(segment)
    }
    
    return sb.String()
}
```

### Lazy Initialisation

Compute-once pattern for expensive operations:

```go
type LazyValue[T any] struct {
    once  sync.Once
    value T
    err   error
    init  func() (T, error)
}

func NewLazy[T any](init func() (T, error)) *LazyValue[T] {
    return &LazyValue[T]{init: init}
}

func (l *LazyValue[T]) Get() (T, error) {
    l.once.Do(func() {
        l.value, l.err = l.init()
    })
    return l.value, l.err
}

// Usage
var profileConfig = NewLazy(func() (*ProfileConfig, error) {
    return loadProfileFromDisk("ollama.yaml")
})
```

### Batch Processing

Aggregate operations for efficiency:

```go
type BatchProcessor[T any] struct {
    items    []T
    capacity int
    mu       sync.Mutex
    process  func([]T) error
    ticker   *time.Ticker
}

func (bp *BatchProcessor[T]) Add(item T) {
    bp.mu.Lock()
    bp.items = append(bp.items, item)
    
    if len(bp.items) >= bp.capacity {
        items := bp.items
        bp.items = make([]T, 0, bp.capacity)
        bp.mu.Unlock()
        
        go bp.process(items)
    } else {
        bp.mu.Unlock()
    }
}

func (bp *BatchProcessor[T]) flush() {
    bp.mu.Lock()
    if len(bp.items) > 0 {
        items := bp.items
        bp.items = make([]T, 0, bp.capacity)
        bp.mu.Unlock()
        
        bp.process(items)
    } else {
        bp.mu.Unlock()
    }
}
```

## Error Handling Patterns

### Typed Errors with Context

Domain-specific error types:

```go
type ErrorCode string

const (
    ErrEndpointNotFound ErrorCode = "ENDPOINT_NOT_FOUND"
    ErrModelUnavailable ErrorCode = "MODEL_UNAVAILABLE"
    ErrRateLimited      ErrorCode = "RATE_LIMITED"
)

type AppError struct {
    Code      ErrorCode
    Message   string
    Details   map[string]any
    Cause     error
    Timestamp time.Time
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
    return e.Cause
}

func NewAppError(code ErrorCode, message string) *AppError {
    return &AppError{
        Code:      code,
        Message:   message,
        Details:   make(map[string]any),
        Timestamp: time.Now(),
    }
}

func (e *AppError) WithDetail(key string, value any) *AppError {
    e.Details[key] = value
    return e
}
```

### Error Recovery Pattern

Graceful degradation with fallbacks:

```go
type Resilient struct {
    primary   func() (any, error)
    fallback  func() (any, error)
    retries   int
    backoff   time.Duration
}

func (r *Resilient) Execute() (any, error) {
    var lastErr error
    
    // Try primary with retries
    for i := 0; i < r.retries; i++ {
        result, err := r.primary()
        if err == nil {
            return result, nil
        }
        lastErr = err
        
        if i < r.retries-1 {
            time.Sleep(r.backoff * time.Duration(i+1))
        }
    }
    
    // Try fallback
    if r.fallback != nil {
        result, err := r.fallback()
        if err == nil {
            return result, nil
        }
        // Wrap both errors
        return nil, fmt.Errorf("primary failed: %w, fallback failed: %v", lastErr, err)
    }
    
    return nil, lastErr
}
```

## Best Practices Summary

### Do's

1. **Use atomic operations** for counters and flags
2. **Leverage xsync** for concurrent maps and counters
3. **Pool objects** that are frequently allocated
4. **Propagate context** through all function calls
5. **Use structured logging** with request context
6. **Implement circuit breakers** for external calls
7. **Handle panics** in goroutines
8. **Clean up resources** with defer

### Don'ts

1. **Don't use mutexes** when atomics suffice
2. **Don't create goroutines** without lifecycle management
3. **Don't ignore context cancellation**
4. **Don't allocate** in hot paths
5. **Don't use global variables** for state
6. **Don't panic** in library code
7. **Don't ignore errors** even in deferred functions

## Next Steps

- [Architecture Details](architecture.md) - System architecture
- [Proxy Engines](../concepts/proxy-engines.md) - Proxy implementations
- [Testing Guide](testing.md) - Testing patterns
- [Contributing](contributing.md) - Contribution guidelines