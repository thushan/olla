---
title: Technical Patterns - Advanced Go Patterns in Olla
description: Deep dive into the technical patterns and Go idioms used throughout Olla's codebase. Learn about concurrency, memory optimisation, and architectural patterns.
keywords: go patterns, concurrency patterns, memory optimisation, xsync, atomic operations, object pooling
---

# Technical Patterns

This document details the advanced Go patterns and techniques used throughout Olla's codebase. Understanding these patterns is crucial for maintaining consistency and performance.

## Concurrency Patterns

### Lock-Free Data Structures with xsync

Olla heavily leverages `github.com/puzpuzpuz/xsync/v3` for lock-free concurrent data structures:

```go
// Thread-safe map without locks
type EndpointRegistry struct {
    endpoints *xsync.MapOf[string, *Endpoint]
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

The circuit breaker uses atomic operations for lock-free state transitions:

```go
const (
    circuitClosed   int64 = 0
    circuitOpen     int64 = 1
    circuitHalfOpen int64 = 2
)

type CircuitBreaker struct {
    state           atomic.Int64
    failures        atomic.Int64
    lastFailureTime atomic.Int64
    threshold       int64
    timeout         time.Duration
}

func (cb *CircuitBreaker) RecordFailure() {
    failures := cb.failures.Add(1)
    cb.lastFailureTime.Store(time.Now().UnixNano())
    
    if failures >= cb.threshold {
        // Atomic state transition
        cb.state.CompareAndSwap(circuitClosed, circuitOpen)
    }
}

func (cb *CircuitBreaker) CanPass() bool {
    state := cb.state.Load()
    
    switch state {
    case circuitClosed:
        return true
    case circuitOpen:
        // Check if timeout expired
        lastFailure := time.Unix(0, cb.lastFailureTime.Load())
        if time.Since(lastFailure) > cb.timeout {
            // Try to transition to half-open
            if cb.state.CompareAndSwap(circuitOpen, circuitHalfOpen) {
                cb.failures.Store(0)
            }
            return true
        }
        return false
    case circuitHalfOpen:
        return true
    }
    return false
}
```

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
    for i := 0; i < wp.workers; i++ {
        wp.wg.Add(1)
        go wp.worker()
    }
}

func (wp *WorkerPool[T]) worker() {
    defer wp.wg.Done()
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

Type-safe object pooling with generics:

```go
type Pool[T any] struct {
    pool sync.Pool
    new  func() T
    reset func(*T)
}

func NewPool[T any](newFn func() T, resetFn func(*T)) *Pool[T] {
    return &Pool[T]{
        pool: sync.Pool{
            New: func() interface{} {
                return newFn()
            },
        },
        new:   newFn,
        reset: resetFn,
    }
}

func (p *Pool[T]) Get() T {
    v := p.pool.Get()
    if v == nil {
        return p.new()
    }
    return v.(T)
}

func (p *Pool[T]) Put(v T) {
    if p.reset != nil {
        p.reset(&v)
    }
    p.pool.Put(v)
}

// Usage example
var bufferPool = NewPool(
    func() []byte { return make([]byte, 0, 32*1024) },
    func(b *[]byte) { *b = (*b)[:0] }, // Reset slice
)
```

### Connection Pool Management

Per-endpoint connection pools with automatic cleanup:

```go
type ConnectionPool struct {
    transport   *http.Transport
    lastUsed    atomic.Int64
    activeConns atomic.Int64
}

type PoolManager struct {
    pools        *xsync.MapOf[string, *ConnectionPool]
    cleanupTimer *time.Timer
}

func (pm *PoolManager) GetPool(endpoint string) *ConnectionPool {
    pool, _ := pm.pools.LoadOrCompute(endpoint, func() *ConnectionPool {
        return &ConnectionPool{
            transport: &http.Transport{
                MaxIdleConns:        10,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
                DisableCompression:  true, // Better for local networks
            },
        }
    })
    
    pool.lastUsed.Store(time.Now().UnixNano())
    return pool
}

func (pm *PoolManager) cleanupStale() {
    threshold := time.Now().Add(-5 * time.Minute).UnixNano()
    
    pm.pools.Range(func(endpoint string, pool *ConnectionPool) bool {
        if pool.lastUsed.Load() < threshold && pool.activeConns.Load() == 0 {
            pool.transport.CloseIdleConnections()
            pm.pools.Delete(endpoint)
        }
        return true
    })
}
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
func createServices(cfg *Config) map[string]interface{} {
    services := make(map[string]interface{})
    
    // Create with nil dependencies
    services["stats"] = NewStatsService(nil)
    services["security"] = NewSecurityService(nil)
    services["proxy"] = NewProxyService(nil)
    
    return services
}

// Phase 2: Wire dependencies
func wireServices(services map[string]interface{}) {
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
    subscribers *xsync.MapOf[string, []chan Event[T]]
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

Context-based request tracking:

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
    New: func() interface{} {
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
    Details   map[string]interface{}
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
        Details:   make(map[string]interface{}),
        Timestamp: time.Now(),
    }
}

func (e *AppError) WithDetail(key string, value interface{}) *AppError {
    e.Details[key] = value
    return e
}
```

### Error Recovery Pattern

Graceful degradation with fallbacks:

```go
type Resilient struct {
    primary   func() (interface{}, error)
    fallback  func() (interface{}, error)
    retries   int
    backoff   time.Duration
}

func (r *Resilient) Execute() (interface{}, error) {
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