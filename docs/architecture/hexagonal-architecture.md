# Hexagonal Architecture in Olla

Olla implements a clean hexagonal architecture (also known as ports and adapters) that provides a robust foundation for building a high-performance LLM proxy. This architectural pattern ensures our business logic remains independent of infrastructure concerns, making the system more testable, maintainable, and adaptable to changing requirements.

## Architecture Overview

The hexagonal architecture divides the application into three distinct layers:

1. **Core Domain** - Business logic and domain models
2. **Ports** - Interface definitions that define contracts
3. **Adapters** - Infrastructure implementations

This separation allows us to swap implementations without touching the core business logic—a critical capability when dealing with diverse LLM platforms and evolving infrastructure requirements.

## Core Domain (`/internal/core/domain/`)

The domain layer contains our pure business entities and rules, free from any infrastructure concerns. These models represent the fundamental concepts in our LLM proxy system.

### Key Domain Models

**Endpoint**
```go
type Endpoint struct {
    Name                string
    URL                 *url.URL
    Type                string
    Priority            int
    Status              EndpointStatus
    LastChecked         time.Time
    NextCheckTime       time.Time
    LastLatency         time.Duration
    CheckInterval       time.Duration
    CheckTimeout        time.Duration
    ConsecutiveFailures int
    BackoffMultiplier   int
    // URL strings for quick access
    URLString            string
    HealthCheckURLString string
    ModelURLString       string
}
```

The Endpoint model represents any inference endpoint—whether it's Ollama, LM Studio, or an OpenAI-compatible service. It tracks health status, check intervals, and supports priority-based routing. The model includes backoff logic for failed endpoints and pre-computed URL strings for performance.

**UnifiedModel**
```go
type UnifiedModel struct {
    ID               string                 // Canonical ID
    Family           string                 // Model family (e.g., phi, llama)
    Variant          string                 // Version/variant
    ParameterSize    string                 // Normalised size (e.g., 14.7b)
    Quantization     string                 // Normalised quantization
    Format           string                 // Model format (gguf, etc.)
    Aliases          []AliasEntry           // All known aliases with source
    SourceEndpoints  []SourceEndpoint       // Where model is available
    Capabilities     []string               // Inferred capabilities
    MaxContextLength *int64                 // Context window size
    DiskSize         int64                  // Total disk size
    Metadata         map[string]interface{} // Platform-specific extras
}
```

UnifiedModel provides a platform-agnostic representation of models. It tracks where each model is available across endpoints, maintains all naming aliases, and stores metadata from different platforms. This enables correct routing regardless of platform-specific naming conventions.

The domain layer also includes interfaces for platform-specific behaviour profiles and model capabilities, which are implemented by adapters to handle different LLM platforms' unique requirements.

## Ports (`/internal/core/ports/`)

Ports define the contracts between our domain and the outside world. They're pure interfaces that establish boundaries and dependencies.

### Primary Ports (Driving)

**ProxyService**
```go
type ProxyService interface {
    ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, 
                stats *RequestStats, rlog logger.StyledLogger) error
    ProxyRequestToEndpoints(ctx context.Context, w http.ResponseWriter, r *http.Request, 
                           endpoints []*domain.Endpoint, stats *RequestStats, 
                           rlog logger.StyledLogger) error
    GetStats(ctx context.Context) (ProxyStats, error)
    UpdateConfig(configuration ProxyConfiguration)
}
```

The ProxyService port defines how the application layer interacts with our proxy functionality. This abstraction allows us to swap between Sherpa and Olla engines seamlessly.

### Secondary Ports (Driven)

**DiscoveryService**
```go
type DiscoveryService interface {
    GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
    GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error)
    RefreshEndpoints(ctx context.Context) error
}
```

**ModelUnifier**
```go
type ModelUnifier interface {
    UnifyModels(ctx context.Context, models []*domain.ModelInfo, 
                endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error)
    ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error)
    GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error)
}
```

These ports define how our domain interacts with external systems. The implementations can vary—discovery might use static configuration, Kubernetes service discovery, or Consul—without affecting the core logic.

## Adapters (`/internal/adapter/`)

Adapters implement the port interfaces, bridging our domain with the real world. Each adapter is responsible for a specific infrastructure concern.

### Proxy Adapters

We provide two proxy engine implementations, each optimised for different use cases:

**Sherpa Proxy** (`/adapter/proxy/sherpa/`)
- Simple, maintainable implementation
- Basic connection pooling
- 8KB buffer size
- Ideal for moderate workloads

**Olla Proxy** (`/adapter/proxy/olla/`)
- High-performance implementation
- Per-endpoint connection pooling
- Circuit breakers for failure isolation
- Object pooling to reduce GC pressure
- 64KB buffers optimised for LLM streaming
- Lock-free atomic statistics

### Load Balancer Adapters

**Priority Balancer** (`/adapter/balancer/priority.go`)
```go
type PrioritySelector struct {
    stats ports.StatsCollector
}
```

The priority balancer routes requests based on endpoint priorities, falling back gracefully when higher-priority endpoints are unavailable. This is our recommended strategy for production deployments.

**Round Robin** (`/adapter/balancer/round_robin.go`)
Simple rotation through available endpoints, useful for evenly distributing load.

**Least Connections** (`/adapter/balancer/least_connections.go`)
Routes to the endpoint with the fewest active connections, helping prevent overload.

### Platform Profile Adapters

Each LLM platform has unique characteristics that we handle through profile adapters:

**OllamaProfile** (`/adapter/registry/profile/ollama.go`)
- 5-minute timeout for large model loading
- Model capability inference from naming patterns
- Resource requirement estimation based on model size

**LMStudioProfile** (`/adapter/registry/profile/lmstudio.go`)
- Custom header handling
- Compatibility mode for older versions

**OpenAICompatible** (`/adapter/registry/profile/openai_compatible.go`)
- Generic implementation for OpenAI API-compatible services
- Configurable retry logic

## Dependency Flow

The hexagonal architecture enforces a strict dependency rule: dependencies only point inward. Here's how it works in practice:

```
HTTP Handler → ProxyService (Port) → Sherpa/Olla (Adapter)
                                          ↓
                                   DiscoveryService (Port)
                                          ↓
                                   ConfigDiscovery (Adapter)
                                          ↓
                                   Domain Models
```

The domain models at the centre have no dependencies. Ports depend only on domain models. Adapters depend on ports and domain models. The application layer orchestrates everything but remains thin.

## Benefits in Practice

### Testability

Because our core logic doesn't depend on infrastructure, we can test it in isolation:

```go
func TestPriorityBalancer(t *testing.T) {
    mockStats := &MockStatsCollector{}
    balancer := NewPrioritySelector(mockStats)
    
    endpoints := []domain.Endpoint{
        {Priority: 1, Status: domain.StatusHealthy},
        {Priority: 2, Status: domain.StatusHealthy},
    }
    
    selected := balancer.Select(endpoints)
    assert.Equal(t, 1, selected.Priority)
}
```

### Flexibility

Swapping implementations is straightforward. Need to add Redis-based discovery? Create a new adapter implementing DiscoveryService:

```go
type RedisDiscovery struct {
    client *redis.Client
}

func (r *RedisDiscovery) GetHealthyEndpoints() []domain.Endpoint {
    // Redis-specific implementation
}
```

### Maintainability

Changes to external APIs only affect their specific adapters. When Ollama updates their API, we only modify the OllamaProfile adapter—the rest of the system remains untouched.

## Implementation Guidelines

When extending Olla, follow these principles:

1. **Domain First**: Start by modelling in the domain layer. What are the core concepts?

2. **Define Ports**: Create interfaces that express what you need, not how it's implemented.

3. **Implement Adapters**: Build concrete implementations that satisfy your ports.

4. **Keep It Simple**: Don't create abstractions you don't need. We have two proxy engines because they serve different use cases, not because we love complexity.

5. **Test at Boundaries**: Focus testing efforts on adapter implementations and domain logic. The ports themselves are just contracts.

## Real-World Example: Adding a New LLM Platform

Let's say we want to add support for a new platform called "FastLLM". Here's the process:

1. **Analyse Platform Characteristics**
   - What's unique about their API?
   - Any special requirements for timeouts or retries?
   - How do they name models?

2. **Create Platform Profile**
   ```go
   type FastLLMProfile struct {
       baseTimeout time.Duration
   }
   
   func (f *FastLLMProfile) GetTimeout(operation string) time.Duration {
       // FastLLM-specific logic
   }
   ```

3. **Register Profile**
   ```go
   profileFactory.Register("fastllm", NewFastLLMProfile)
   ```

4. **Update Model Unifier** (if needed)
   Add any platform-specific model name normalisation.

5. **Test End-to-End**
   The existing proxy engines and load balancers will work automatically with your new platform.

This architecture has proven robust in production, handling millions of LLM requests while maintaining clean boundaries between business logic and infrastructure concerns. The investment in proper architecture pays dividends through easier testing, simpler debugging, and faster feature development.