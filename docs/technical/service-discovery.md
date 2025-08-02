# Service Discovery and Model Unification

Service discovery in Olla encompasses both endpoint management and model inventory across heterogeneous LLM platforms. This dual responsibility requires sophisticated mechanisms to track available services while normalising model representations across different naming conventions.

## Architecture Overview

The service discovery system consists of two primary components:

1. **Endpoint Discovery**: Manages available inference endpoints
2. **Model Unification**: Normalises model representations across platforms

Both components work together to provide a unified view of available resources.

## Endpoint Discovery

### Configuration-Based Discovery

The primary discovery mechanism uses static configuration, suitable for most deployments:

```go
type ConfigDiscovery struct {
    endpoints    []domain.Endpoint
    mu          sync.RWMutex
    stats       ports.StatsCollector
    healthCheck ports.HealthChecker
}
```

Configuration supports multiple formats:

```yaml
discovery:
  endpoints:
    - name: "local-ollama"
      url: "http://localhost:11434"
      platform: "ollama"
      priority: 1
      tags:
        location: "local"
        gpu: "rtx4090"
        
    - name: "gpu-cluster-1"
      url: "http://10.0.1.10:11434"
      platform: "ollama"
      priority: 2
      tags:
        location: "datacenter"
        gpu: "a100"
        capacity: "high"
```

### Dynamic Endpoint Management

Endpoints can be managed at runtime through the discovery service:

```go
func (c *ConfigDiscovery) AddEndpoint(endpoint domain.Endpoint) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Validate endpoint
    if err := c.validateEndpoint(endpoint); err != nil {
        return err
    }
    
    // Check for duplicates
    for _, existing := range c.endpoints {
        if existing.ID == endpoint.ID {
            return ErrDuplicateEndpoint
        }
    }
    
    // Assign defaults
    if endpoint.Priority == 0 {
        endpoint.Priority = 10 // Low priority by default
    }
    
    c.endpoints = append(c.endpoints, endpoint)
    
    // Trigger immediate health check
    go c.healthCheck.CheckEndpoint(&endpoint)
    
    return nil
}
```

### Endpoint Filtering

The discovery service provides sophisticated filtering capabilities:

```go
func (c *ConfigDiscovery) GetEndpointsByCapability(capability string) []domain.Endpoint {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    var matched []domain.Endpoint
    
    for _, endpoint := range c.endpoints {
        // Check platform capabilities
        profile := c.profileRegistry.GetProfile(endpoint.Platform)
        if profile.SupportsCapability(capability) {
            matched = append(matched, endpoint)
        }
    }
    
    return matched
}

func (c *ConfigDiscovery) GetEndpointsByTags(tags map[string]string) []domain.Endpoint {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    var matched []domain.Endpoint
    
    for _, endpoint := range c.endpoints {
        if c.matchesTags(endpoint, tags) {
            matched = append(matched, endpoint)
        }
    }
    
    return matched
}
```

## Model Unification

Model unification solves a critical challenge: different platforms use different naming conventions for the same models. The unifier creates a canonical representation.

### The Challenge

Consider how the same model appears across platforms:

- Ollama: `llama3.2:3b-instruct-q4_K_M`
- LM Studio: `lmstudio-community/Meta-Llama-3.2-3B-Instruct-GGUF/Meta-Llama-3.2-3B-Instruct-Q4_K_M.gguf`
- OpenAI Compatible: `meta-llama-3.2-3b-instruct`

### Unified Model Structure

```go
type UnifiedModel struct {
    // Canonical name following pattern: family/variant:size-quant
    UnifiedName string
    
    // Platform-specific names mapped by endpoint
    Aliases map[string][]string
    
    // Available endpoints
    Endpoints []string
    
    // Aggregated state
    State ModelState
    
    // Combined metadata
    Metadata ModelMetadata
}

type ModelMetadata struct {
    Family          string
    Variant         string
    ParameterSize   string
    Quantization    string
    ContextLength   int
    Capabilities    []ModelCapability
    ResourceReqs    ResourceRequirements
}
```

### Model Unification Process

The unifier processes models from each endpoint, deduplicating based on digest and name:

```go
func (u *DefaultUnifier) UnifyModels(ctx context.Context, models []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error) {
    // Clean up stale models periodically
    if u.store.NeedsCleanup() {
        u.store.CleanupStaleModels(u.staleThreshold)
    }
    
    // Remove old models when endpoint updates its list
    oldModelIDs := u.store.GetEndpointModels(endpointURL)
    for _, modelID := range oldModelIDs {
        u.removeModelFromEndpoint(modelID, endpointURL)
    }
    
    // Process each model
    processedModels := make([]*domain.UnifiedModel, 0, len(models))
    for _, modelInfo := range models {
        unified := u.processModel(modelInfo, endpoint)
        if unified != nil {
            processedModels = append(processedModels, unified)
        }
    }
    
    return processedModels, nil
}
```

### Model Collection

Each platform requires specific API calls to enumerate models:

```go
func (u *DefaultUnifier) collectOllamaModels(endpoint *domain.Endpoint) ([]ModelInfo, error) {
    resp, err := http.Get(endpoint.URL + "/api/tags")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var response ollamaTagsResponse
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, err
    }
    
    models := make([]ModelInfo, 0, len(response.Models))
    for _, model := range response.Models {
        info := ModelInfo{
            Name:     model.Name,
            Digest:   model.Digest,
            Size:     model.Size,
            Modified: model.ModifiedAt,
        }
        
        // Extract metadata from name
        u.parseOllamaModelName(&info)
        
        models = append(models, info)
    }
    
    return models
}
```

### Model Deduplication

Models are deduplicated using digest and name matching:

```go
func (u *DefaultUnifier) processModel(modelInfo *domain.ModelInfo, endpoint *domain.Endpoint) *domain.UnifiedModel {
    model := u.convertModelInfoToModel(modelInfo)
    
    // Digest matching is most reliable - same binary content
    if model.Digest != "" {
        if existingModels, found := u.store.ResolveByDigest(model.Digest); found && len(existingModels) > 0 {
            existing := existingModels[0]
            u.mergeModel(existing, model, endpoint)
            u.stats.DigestMatches++
            return existing
        }
    }
    
    // Fall back to name matching if no digest
    if existing, found := u.store.ResolveByName(model.Name); found {
        if u.canMergeByName(existing, model) {
            u.mergeModel(existing, model, endpoint)
            u.stats.NameMatches++
            return existing
        }
    }
    
    // Create new unified model if no match
    unified := u.createUnifiedModel(model, endpoint)
    u.store.PutModel(unified)
    return unified
}
```

### Model Merging

When a matching model is found, the unifier merges information from multiple endpoints:

```go
func (u *DefaultUnifier) canMergeByName(existing *domain.UnifiedModel, newModel *Model) bool {
    // Different digest means different model even with same name
    if newModel.Digest != "" && existing.Metadata != nil {
        if existingDigest, ok := existing.Metadata["digest"].(string); ok {
            if existingDigest != "" && existingDigest != newModel.Digest {
                return false
            }
        }
    }
    return true
}
```

The merge process:
- Adds the endpoint to the model's source endpoints list
- Updates model state (loaded, available, etc.) per endpoint
- Preserves all original model names as aliases
- Combines metadata from different sources

### Model State Management

Unified models track aggregate state across endpoints:

```go
func (u *DefaultUnifier) determineModelState(endpoints []EndpointModel) domain.ModelState {
    loadedCount := 0
    availableCount := 0
    
    for _, em := range endpoints {
        switch em.State {
        case domain.ModelLoaded:
            loadedCount++
        case domain.ModelAvailable:
            availableCount++
        }
    }
    
    // Loaded if any endpoint has it loaded
    if loadedCount > 0 {
        return domain.ModelLoaded
    }
    
    // Available if any endpoint can load it
    if availableCount > 0 {
        return domain.ModelAvailable
    }
    
    return domain.ModelOffline
}
```

## Platform Profiles and Model Capabilities

Platform profiles define how each platform behaves and what capabilities its models support:

```go
type InferenceProfile interface {
    PlatformProfile
    GetModelCapabilities(model string) ModelCapabilities
    GetResourceRequirements(model string) ResourceRequirements
    ValidateModelRequest(model string, request interface{}) error
}
```

### Capability Detection

Different platforms expose capabilities differently:

```go
func (o *OllamaProfile) GetModelCapabilities(model string) ModelCapabilities {
    caps := ModelCapabilities{
        Chat: true,  // Ollama always supports chat
    }
    
    modelLower := strings.ToLower(model)
    
    // Vision models
    if strings.Contains(modelLower, "vision") || 
       strings.Contains(modelLower, "llava") ||
       strings.Contains(modelLower, "bakllava") {
        caps.Vision = true
    }
    
    // Code models
    if strings.Contains(modelLower, "code") ||
       strings.Contains(modelLower, "codellama") ||
       strings.Contains(modelLower, "deepseek-coder") ||
       strings.Contains(modelLower, "starcoder") {
        caps.Code = true
    }
    
    // Embedding models
    if strings.Contains(modelLower, "embed") ||
       strings.Contains(modelLower, "bge") ||
       strings.Contains(modelLower, "nomic-embed") {
        caps.Embeddings = true
        caps.Chat = false  // Embedding models don't chat
    }
    
    return caps
}
```

### Resource Estimation

Profiles estimate resource requirements based on model characteristics:

```go
func (o *OllamaProfile) GetResourceRequirements(model string) ResourceRequirements {
    size := extractParameterSize(model)
    quant := extractQuantization(model)
    
    // Base calculation
    baseMemoryGB := calculateBaseMemory(size, quant)
    
    // Add overhead for context
    contextOverhead := 2.0  // 2GB for context by default
    if strings.Contains(model, "32k") {
        contextOverhead = 4.0
    } else if strings.Contains(model, "128k") {
        contextOverhead = 8.0
    }
    
    return ResourceRequirements{
        MinMemoryGB:     baseMemoryGB + contextOverhead,
        RecommendedGPUs: calculateGPURequirement(baseMemoryGB),
        CPUCores:        4,  // Minimum for reasonable performance
        StorageGB:       baseMemoryGB * 1.2,  // 20% overhead for storage
    }
}
```

## Advanced Discovery Features

### Service Mesh Integration

For Kubernetes deployments, integrate with service mesh:

```go
type K8sDiscovery struct {
    client    kubernetes.Interface
    namespace string
    selector  labels.Selector
}

func (k *K8sDiscovery) DiscoverEndpoints() ([]domain.Endpoint, error) {
    services, err := k.client.CoreV1().Services(k.namespace).List(
        context.Background(),
        metav1.ListOptions{LabelSelector: k.selector.String()},
    )
    
    var endpoints []domain.Endpoint
    for _, svc := range services.Items {
        endpoint := k.serviceToEndpoint(svc)
        endpoints = append(endpoints, endpoint)
    }
    
    return endpoints, nil
}
```

### DNS-Based Discovery

Use DNS SRV records for discovery:

```go
type DNSDiscovery struct {
    domain   string
    resolver *net.Resolver
}

func (d *DNSDiscovery) DiscoverEndpoints() ([]domain.Endpoint, error) {
    _, srvs, err := d.resolver.LookupSRV(
        context.Background(),
        "ollama", "tcp", d.domain,
    )
    
    var endpoints []domain.Endpoint
    for _, srv := range srvs {
        endpoint := domain.Endpoint{
            Name:     srv.Target,
            URL:      fmt.Sprintf("http://%s:%d", srv.Target, srv.Port),
            Priority: int(srv.Priority),
        }
        endpoints = append(endpoints, endpoint)
    }
    
    return endpoints, nil
}
```

## Configuration Examples

### Multi-Platform Setup

```yaml
discovery:
  endpoints:
    # Local Ollama
    - name: "local-ollama"
      url: "http://localhost:11434"
      platform: "ollama"
      priority: 1
      
    # LM Studio
    - name: "lm-studio-workstation"
      url: "http://192.168.1.100:1234"
      platform: "lmstudio"
      priority: 2
      headers:
        Authorization: "Bearer ${LMSTUDIO_API_KEY}"
        
    # OpenAI Compatible
    - name: "groq-cloud"
      url: "https://api.groq.com/openai/v1"
      platform: "openai"
      priority: 3
      headers:
        Authorization: "Bearer ${GROQ_API_KEY}"

unification:
  refresh_interval: 5m
  cache_duration: 30m
  similarity_threshold: 0.7
```

### Model Aliasing

```yaml
model_aliases:
  # Map common names to canonical names
  "llama3": "llama3.2:8b-instruct-q4_K_M"
  "codellama": "codellama:13b-code-q4_K_M"
  "mistral": "mistral:7b-instruct-v0.3-q4_K_M"
```

## Monitoring and Debugging

### Key Metrics

```
# Discovery metrics
discovery.endpoints.total: 5
discovery.endpoints.healthy: 4
discovery.refresh.duration: 245ms
discovery.errors.total: 2

# Unification metrics
unification.models.total: 23
unification.models.unified: 18
unification.aliases.total: 45
unification.refresh.duration: 1.2s
```

### Debug Endpoints

```go
// GET /internal/discovery/endpoints
{
  "endpoints": [
    {
      "id": "local-ollama",
      "url": "http://localhost:11434",
      "status": "healthy",
      "models": 12,
      "last_check": "2024-01-20T10:30:00Z"
    }
  ]
}

// GET /internal/discovery/models
{
  "unified_models": [
    {
      "name": "llama3.2:8b-instruct-q4_K_M",
      "aliases": {
        "local-ollama": ["llama3.2:latest", "llama3.2:8b"],
        "lm-studio": ["Meta-Llama-3.2-8B-Instruct-Q4_K_M.gguf"]
      },
      "endpoints": ["local-ollama", "lm-studio"],
      "state": "loaded"
    }
  ]
}
```

## Best Practices

### 1. Configure Appropriate Refresh Intervals

```yaml
discovery:
  refresh_interval: 30s    # Endpoint discovery
  
unification:
  refresh_interval: 5m     # Model discovery (more expensive)
```

### 2. Use Tags for Flexible Routing

```yaml
endpoints:
  - name: "gpu-1"
    tags:
      capability: "vision"
      performance: "high"
      cost: "medium"
```

### 3. Handle Platform Quirks

```go
// Platform-specific normalisation
func normaliseLMStudioModel(name string) string {
    // Remove file extensions
    name = strings.TrimSuffix(name, ".gguf")
    
    // Extract meaningful parts
    parts := strings.Split(name, "/")
    if len(parts) > 1 {
        name = parts[len(parts)-1]
    }
    
    return name
}
```

### 4. Cache Discovery Results

```go
type CachedDiscovery struct {
    base     DiscoveryService
    cache    *cache.Cache
    ttl      time.Duration
}

func (c *CachedDiscovery) GetEndpoints() ([]domain.Endpoint, error) {
    if cached, found := c.cache.Get("endpoints"); found {
        return cached.([]domain.Endpoint), nil
    }
    
    endpoints, err := c.base.GetEndpoints()
    if err == nil {
        c.cache.Set("endpoints", endpoints, c.ttl)
    }
    
    return endpoints, err
}
```

Service discovery and model unification form the foundation of Olla's multi-platform support. By abstracting platform differences and providing a unified view of available resources, these systems enable seamless routing across heterogeneous LLM infrastructure.