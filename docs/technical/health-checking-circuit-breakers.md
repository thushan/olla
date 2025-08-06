# Health Checking and Circuit Breakers

Olla implements a health checking system that continuously monitors endpoint availability and a circuit breaker pattern to prevent cascade failures when endpoints become unhealthy.

## Health Checking Architecture

The health checking system runs as a background service, periodically checking all configured endpoints and updating their status based on the results.

### Core Components

```go
type HTTPHealthChecker struct {
    repository   domain.EndpointRepository
    healthClient *HealthClient
    ticker       *time.Ticker
    stopCh       chan struct{}
    logger       logger.StyledLogger
    isRunning    atomic.Bool
}
```

The health checker:
- Runs checks every 30 seconds by default
- Limits concurrent checks to 5 endpoints at a time
- Uses exponential backoff for failing endpoints
- Updates endpoint status in the repository

### Health States

Endpoints can be in one of these states (defined as strings):

```go
const (
    StatusStringHealthy   = "healthy"
    StatusStringBusy      = "busy"
    StatusStringOffline   = "offline"
    StatusStringWarming   = "warming"
    StatusStringUnhealthy = "unhealthy"
    StatusStringUnknown   = "unknown"
)
```

### Health Check Process

The health checker performs HTTP requests to each endpoint's health check URL:

```go
func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint *domain.Endpoint) {
    result, err := c.healthClient.Check(ctx, endpoint)
    
    oldStatus := endpoint.Status
    newStatus := result.Status
    
    // Update endpoint state
    endpointCopy := *endpoint
    endpointCopy.Status = newStatus
    endpointCopy.LastChecked = time.Now()
    endpointCopy.LastLatency = result.Latency
    
    // Calculate backoff for next check
    isSuccess := result.Status == domain.StatusHealthy
    nextInterval, newMultiplier := calculateBackoff(&endpointCopy, isSuccess)
    
    if !isSuccess {
        endpointCopy.ConsecutiveFailures++
        endpointCopy.BackoffMultiplier = newMultiplier
    } else {
        endpointCopy.ConsecutiveFailures = 0
        endpointCopy.BackoffMultiplier = 1
    }
    
    endpointCopy.NextCheckTime = time.Now().Add(nextInterval)
    c.repository.UpdateEndpoint(ctx, &endpointCopy)
}
```

### Concurrent Health Checks

To avoid overwhelming the system, health checks run with controlled concurrency:

```go
func (c *HTTPHealthChecker) performHealthChecks(ctx context.Context) {
    // Limit concurrency to DefaultConcurrentChecks (5)
    semaphore := make(chan struct{}, DefaultConcurrentChecks)
    
    for _, endpoint := range endpointsToCheck {
        wg.Add(1)
        go func(ep *domain.Endpoint) {
            defer wg.Done()
            
            select {
            case semaphore <- struct{}{}:
                defer func() { <-semaphore }()
                c.checkEndpointSafely(ctx, ep)
            case <-ctx.Done():
                return
            }
        }(endpoint)
    }
    
    wg.Wait()
}
```

## Circuit Breakers

The circuit breaker implementation protects against cascade failures by temporarily blocking requests to failing endpoints.

### Implementation

```go
type CircuitBreaker struct {
    endpoints        *xsync.Map[string, *circuitState]
    failureThreshold int  // Default: 5 failures
    timeout          time.Duration  // Default: 30 seconds
}

type circuitState struct {
    failures    int64
    lastFailure int64
    lastAttempt int64
    isOpen      int32
}
```

### Circuit Breaker States

The circuit breaker can be in one of these states:
- **Closed** (0): Normal operation, requests pass through
- **Open** (1): Failing, requests are blocked
- **Half-Open**: After timeout, one request is allowed through to test recovery

### Operation

```go
func (cb *CircuitBreaker) IsOpen(endpointURL string) bool {
    state, ok := cb.endpoints.Load(endpointURL)
    if !ok {
        return false
    }
    
    // Check if circuit should auto-recover
    if atomic.LoadInt32(&state.isOpen) == 1 {
        lastFailure := atomic.LoadInt64(&state.lastFailure)
        if time.Unix(0, lastFailure).Add(cb.timeout).Before(time.Now()) {
            // Allow one request through (half-open state)
            if atomic.CompareAndSwapInt64(&state.lastAttempt, 0, now) {
                return false
            }
        }
        return true
    }
    
    return false
}
```

### Recording Results

```go
func (cb *CircuitBreaker) RecordFailure(endpointURL string) {
    state := cb.loadOrCreateState(endpointURL)
    
    failures := atomic.AddInt64(&state.failures, 1)
    atomic.StoreInt64(&state.lastFailure, time.Now().UnixNano())
    
    if failures >= int64(cb.failureThreshold) {
        atomic.StoreInt32(&state.isOpen, 1)
    }
}

func (cb *CircuitBreaker) RecordSuccess(endpointURL string) {
    state, ok := cb.endpoints.Load(endpointURL)
    if !ok {
        return
    }
    
    atomic.StoreInt64(&state.failures, 0)
    atomic.StoreInt32(&state.isOpen, 0)
}
```

## Backoff Strategy

Failed endpoints are checked less frequently using exponential backoff:

```go
func calculateBackoff(endpoint *domain.Endpoint, isSuccess bool) (time.Duration, int) {
    if isSuccess {
        return endpoint.CheckInterval, 1
    }
    
    // Exponential backoff with max limit
    multiplier := endpoint.BackoffMultiplier
    if multiplier < 1 {
        multiplier = 1
    }
    
    if endpoint.ConsecutiveFailures > 0 {
        multiplier = multiplier * 2
        if multiplier > 32 {  // Max backoff multiplier
            multiplier = 32
        }
    }
    
    backoffDuration := endpoint.CheckInterval * time.Duration(multiplier)
    if backoffDuration > 5*time.Minute {
        backoffDuration = 5*time.Minute
    }
    
    return backoffDuration, multiplier
}
```

## Configuration

Health checking is configured through endpoint settings:

```go
type Endpoint struct {
    CheckInterval       time.Duration  // How often to check
    CheckTimeout        time.Duration  // Timeout for each check
    ConsecutiveFailures int           // Current failure count
    BackoffMultiplier   int           // Current backoff multiplier
    NextCheckTime       time.Time     // When to check next
}
```

Default values:
- Check interval: 30 seconds
- Check timeout: 5 seconds
- Concurrent checks: 5
- Circuit breaker threshold: 5 failures
- Circuit breaker timeout: 30 seconds

## Status Transitions

Endpoints transition between states based on health check results:

1. **Unknown → Healthy**: Initial successful check
2. **Unknown → Unhealthy**: Initial failed check
3. **Healthy → Unhealthy**: After consecutive failures
4. **Unhealthy → Healthy**: Successful check after failures
5. **Any → Offline**: Administratively disabled

## Logging

The health checker provides detailed logging for troubleshooting:

- Status changes are logged at INFO level
- Failures are logged at WARN level with details
- Ongoing issues are logged every 5 consecutive failures
- Circuit breaker state changes are tracked

Example log output:
```
INFO  Endpoint recovered: gpu-1 status=healthy was=unhealthy latency=234ms next_check_in=30s
WARN  Endpoint status changed: gpu-2 status=unhealthy was=healthy consecutive_failures=3
```

## Usage

The health checker starts automatically when Olla starts:

```go
healthChecker := health.NewHTTPHealthCheckerWithDefaults(repository, logger)
err := healthChecker.StartChecking(ctx)
```

Manual health checks can be triggered:
```go
err := healthChecker.RunHealthCheck(ctx, false)
```

## Limitations

- Health checks are HTTP-only (GET requests)
- No custom health check logic per platform
- Circuit breaker state is in-memory only (resets on restart)
- No distributed state for multi-instance deployments