---
title: Model Routing - Intelligent Request Routing in Olla
description: Understanding Olla's model routing strategies for handling requests when models aren't available on all endpoints.
keywords: model routing, routing strategy, fallback behavior, request routing, model availability
---

# Model Routing

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

Controls what happens when the requested model isn't available:

- **`compatible_only`**: Route to endpoints of compatible type (e.g., Ollama to Ollama)
- **`all`**: Route to any healthy endpoint regardless of type
- **`none`**: Never fall back, always reject if model not found

## Response Headers

Routing decisions are exposed via HTTP headers for observability:

```http
X-Olla-Routing-Strategy: strict
X-Olla-Routing-Decision: routed
X-Olla-Routing-Reason: model_found
```

## Status Codes

Different routing decisions result in appropriate HTTP status codes:

- **200 OK**: Request successfully routed
- **404 Not Found**: Model doesn't exist on any endpoint
- **503 Service Unavailable**: Model exists but no healthy endpoints available

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
5. **Use compatible_only fallback** to maintain API compatibility