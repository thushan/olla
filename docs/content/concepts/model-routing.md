---
title: Model Routing - Intelligent Request Routing in Olla
description: Understanding Olla's model routing strategies for handling requests when models aren't available on all endpoints.
keywords: model routing, routing strategy, fallback behavior, request routing, model availability
---

# Model Routing

> :memo: **Default Configuration**
> ```yaml
> model_registry:
>   routing_strategy:
>     type: "strict"  # strict, optimistic, or discovery
>     options:
>       fallback_behavior: "compatible_only"  # compatible_only, all, or none
>       discovery_timeout: 2s
>       discovery_refresh_on_miss: false
> ```
> **Routing Strategies**:
> 
> - `strict` _(default)_ - Only routes to endpoints with the model
> - `optimistic` - Routes with configurable fallback behavior
> - `discovery` - Refreshes model catalog before routing
> 
> **Fallback Behaviors**:
> 
> - `compatible_only` _(default)_ - Rejects if model not found
> - `all` - Routes to any healthy endpoint
> - `none` - Always rejects if model not found

Olla implements intelligent model routing strategies to handle scenarios where requested models aren't available on all endpoints.

## Overview

When a request specifies a model (e.g., `phi3.5:latest`), Olla needs to determine which endpoints can handle that request. Not all endpoints have all models, and endpoints can become unhealthy during operation.

## Routing Strategies

### Strict Mode (Default)

Only routes requests to endpoints known to have the model.

**Characteristics:**
- High reliability - requests only go to endpoints with the model
- Fails fast when model unavailable
- Returns 404 when model not found anywhere
- Returns 503 when model only on unhealthy endpoints

**Use Case:** Production environments where predictability is critical.

```yaml
model_registry:
  routing_strategy:
    type: strict
```

### Optimistic Mode

Attempts to route to any healthy endpoint when the model isn't found.

**Characteristics:**
- Higher availability - tries all healthy endpoints
- May route to endpoints without the model
- Configurable fallback behavior
- Best effort approach

**Use Case:** Development environments or when models might be pulled on-demand.

```yaml
model_registry:
  routing_strategy:
    type: optimistic
    options:
      fallback_behavior: compatible_only  # or "all", "none"
```

### Discovery Mode

Refreshes model discovery before making routing decisions.

**Characteristics:**
- Most accurate model availability
- Adds latency for discovery refresh
- Configurable timeout
- Falls back to strict behavior after refresh

**Use Case:** When models are frequently added/removed from endpoints.

```yaml
model_registry:
  routing_strategy:
    type: discovery
    options:
      discovery_refresh_on_miss: true
      discovery_timeout: 2s
```

## Fallback Behavior

Controls what happens when the requested model isn't available on any healthy endpoint:

- **`compatible_only`**: Reject the request with 404 - prevents routing to endpoints that don't have the model
- **`all`**: Route to any healthy endpoint even if they don't have the model
- **`none`**: Never fall back, always reject with 404 if model not found

## Response Headers

Routing decisions are exposed via HTTP headers for observability:

```http
X-Olla-Routing-Strategy: strict
X-Olla-Routing-Decision: routed
X-Olla-Routing-Reason: model_found
```

## Status Codes and Routing Decisions

Different scenarios result in specific HTTP status codes and routing behaviors:

### Strict Mode Behavior

| Scenario | Status Code | Routing Decision | Description |
|----------|-------------|------------------|-------------|
| Model found on healthy endpoint | 200 OK | `routed` | Normal routing to endpoint with model |
| Model not found anywhere | 404 Not Found | `rejected` | Model doesn't exist in the system |
| Model exists but only on unhealthy endpoints | 503 Service Unavailable | `rejected` | Model unavailable due to endpoint health |

### Optimistic Mode Behavior

| Scenario | Fallback | Status Code | Routing Decision | Description |
|----------|----------|-------------|------------------|-------------|
| Model found on healthy endpoint | Any | 200 OK | `routed` | Normal routing to endpoint with model |
| Model not found | `none` | 404 Not Found | `rejected` | Model doesn't exist, no fallback |
| Model not found | `compatible_only` | 404 Not Found | `rejected` | Model doesn't exist, no fallback |
| Model not found | `all` | 200 OK | `fallback` | Routes to any healthy endpoint |
| Model on unhealthy endpoint only | `none` | 503 Service Unavailable | `rejected` | Model unavailable, no fallback |
| Model on unhealthy endpoint only | `compatible_only` | 503 Service Unavailable | `rejected` | Model unavailable, no fallback |
| Model on unhealthy endpoint only | `all` | 200 OK | `fallback` | Routes to any healthy endpoint |

### Discovery Mode Behavior

| Scenario | Status Code | Routing Decision | Description |
|----------|-------------|------------------|-------------|
| Model found after refresh | 200 OK | `routed` | Discovery found the model |
| Model not found after refresh | Depends on fallback | `rejected` or `fallback` | Follows fallback behavior settings |
| Discovery timeout | Depends on fallback | `rejected` or `fallback` | Falls back to cached data |

## Routing Reasons

The `X-Olla-Routing-Reason` header provides detailed information about routing decisions:

| Reason | Status | Description |
|--------|--------|-------------|
| `model_found` | 200 | Model found on healthy endpoints |
| `model_not_found` | 404 | Model doesn't exist in the system |
| `model_not_found_fallback` | 200 | Model not found but falling back to all endpoints |
| `model_unavailable_no_fallback` | 503 | Model exists but unavailable, no fallback |
| `model_unavailable_compatible_only` | 503 | Model exists but unavailable, compatible_only prevents fallback |
| `all_healthy_fallback` | 200 | Using all healthy endpoints as fallback |
| `discovery_failed` | Varies | Discovery refresh failed, using cached data |

## Configuration Example

Complete routing configuration:

```yaml
# Strict mode for production (default)
model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: strict
    
# Optimistic with compatible fallback
model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: optimistic
    options:
      fallback_behavior: compatible_only
      
# Discovery with timeout
model_registry:
  type: "memory"
  enable_unifier: true
  routing_strategy:
    type: discovery
    options:
      discovery_refresh_on_miss: true
      discovery_timeout: 2s
      fallback_behavior: none
```

## Metrics and Monitoring

Routing decisions are tracked in metrics:

- Total routing decisions by strategy
- Success/failure rates per strategy
- Fallback usage statistics
- Discovery refresh latency

Access metrics via `/internal/status` endpoint.

## Best Practices

1. **Use strict mode in production** for predictable behavior
2. **Enable discovery mode** when models change frequently
3. **Monitor routing headers** to understand request flow
4. **Set appropriate timeouts** for discovery mode
5. **Choose fallback behavior carefully**:
   - `none` or `compatible_only` for APIs that need model accuracy
   - `all` only when any endpoint can handle unknown models

## Troubleshooting

### Getting 404 for Known Models

**Issue**: Requests return 404 even though the model exists

**Possible Causes**:
- Model only exists on unhealthy endpoints
- Using `compatible_only` or `none` fallback when model isn't discovered yet
- Model discovery hasn't run yet

**Solutions**:
1. Check endpoint health: `curl http://localhost:40114/internal/status/endpoints`
2. Verify model discovery: `curl http://localhost:40114/internal/status/models`
3. Try discovery mode with refresh: 
   ```yaml
   routing_strategy:
     type: discovery
     options:
       discovery_refresh_on_miss: true
   ```

### Getting 503 vs 404

**Understanding the Difference**:
- **404**: Model doesn't exist anywhere in the system
- **503**: Model exists but no healthy endpoints have it

**How to Debug**:
```bash
# Check all models in the system
curl http://localhost:40114/olla/models

# Check endpoint health
curl http://localhost:40114/internal/status/endpoints

# Look at routing headers
curl -I http://localhost:40114/olla/ollama/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "test-model", "messages": []}'
```

### Unexpected Fallback Behavior

**Issue**: Requests going to endpoints without the model

**Check Your Configuration**:
```yaml
# This allows fallback to any endpoint
fallback_behavior: "all"

# These prevent fallback
fallback_behavior: "none"        # Returns 404 for unknown models
fallback_behavior: "compatible_only"  # Returns 404 for unknown models
```