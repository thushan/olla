---
title: "Olla Profile System - Configure Backend Integrations"
description: "Complete guide to Olla's profile system for configuring LLM backend integrations. Create custom profiles, configure routing, API filtering, and model discovery."
keywords: ["olla profiles", "backend configuration", "profile system", "custom profiles", "api routing", "model discovery", "llm integration"]
---

# Profile Configuration

> :memo: **Default Configuration**
> ```yaml
> # Built-in profiles (auto-loaded)
> # Ollama: config/profiles/ollama.yaml
> # LM Studio: config/profiles/lmstudio.yaml
> # vLLM: config/profiles/vllm.yaml
> # OpenAI: config/profiles/openai.yaml
> 
> endpoints:
>   - type: "ollama"      # Uses ollama profile
>   - type: "lm-studio"   # Uses lmstudio profile
>   - type: "vllm"        # Uses vllm profile
> ```
> **Key Features**:
> 
> - Profiles are auto-loaded from `config/profiles/`
> - Custom profiles override built-in ones by name
> - Selected via endpoint `type` field
> 
> **Custom Profiles**: Place YAML files in `config/profiles/` to add or override

Profiles are the core of Olla's backend integration system. They define how Olla communicates with different LLM platforms, what APIs are exposed, how requests are routed and how responses are parsed.


## Overview

Each backend type (Ollama, LM Studio, vLLM, OpenAI Compatibility) has a profile that controls:

- **URL Routing** - Which URL prefixes map to this backend
- **API Filtering** - Which API paths are allowed through the proxy
- **Model Discovery** - How to find and parse available models
- **Request Handling** - How to parse and route requests
- **Native Intercepts** - Special handlers for platform-specific endpoints
- **Resource Management** - Memory requirements and concurrency limits
- **Capability Detection** - Identifying model features (vision, embeddings, code)

## Profile Loading

Profiles are loaded during Olla startup in a specific order:

1. **Native profiles** are shipped with Olla in the `config/profiles` directory
2. **Custom profiles** found in `config/profiles/*.yaml` override or extend built-ins
3. **Profile selection** happens based on the `type` field in endpoint configuration

!!! info "Profile Overrides"
    A custom profile with the same `name` as a built-in will completely replace the built-in profile.

    For example, if you want to extend vLLM, creating a copy of `vllm.yaml` as `vllm-custom.yaml` will override the other profile.

## Core Concepts

### Basic Meta

Basic meta data about an LLM Backend is provided in these fields.

For example, the `vllm.yaml` file contains:

```yaml
name: ollama
version: "1.0"
display_name: "Ollama"
description: "Local Ollama instance for running GGUF models"
```

- `name` - Name of this particular profile, any subsequent profiles can specify the same name and be overridden.
- `version` - Version of the Profile format, we're currently in `1.0`
- `display_name` - A nicer way to display information about the profile
- `description` - A short description for the interface

### Routing Prefixes

The `routing.prefixes` section defines URL paths that route to this backend.

```yaml
routing:
  prefixes:
    - ollama      # Routes /olla/ollama/* to this backend
    - ma          # Routes /olla/ma/* to this backend
```

**How it works:**

- Each prefix creates a URL namespace under `/olla/`
- Requests to `/olla/{prefix}/*` are routed to endpoints with matching profile
- Multiple prefixes allow flexibility (e.g., `lmstudio`, `lm-studio`, `lm_studio`)
- The prefix is stripped from requests before forwarding to the backend (Eg. `/olla/ma/v1/chat` => `/v1/chat` sent to backend)

For convenience, some profiles (like `lmstudio.yaml`) specify multiple variations of its name:

```yaml
routing:
  prefixes:
    - lmstudio
    - lm-studio
    - lm_studio
```

You can observe the various prefixes URIs being created at Olla's startup for the above:

```
ROUTE                             | METHOD | DESCRIPTION
/olla/lmstudio/v1/models          | GET    | lmstudio models (OpenAI format)
/olla/lmstudio/api/v1/models      | GET    | lmstudio models (OpenAI format alt path)
/olla/lmstudio/api/v0/models      | GET    | lmstudio enhanced models
/olla/lmstudio/                   |        | lmstudio proxy
/olla/lm-studio/v1/models         | GET    | lm-studio models (OpenAI format)
/olla/lm-studio/api/v1/models     | GET    | lm-studio models (OpenAI format alt path)
/olla/lm-studio/api/v0/models     | GET    | lm-studio enhanced models
/olla/lm-studio/                  |        | lm-studio proxy
/olla/lm_studio/v1/models         | GET    | lm_studio models (OpenAI format)
/olla/lm_studio/api/v1/models     | GET    | lm_studio models (OpenAI format alt path)
/olla/lm_studio/api/v0/models     | GET    | lm_studio enhanced models
/olla/lm_studio/                  |        | lm_studio proxy
...
```
{#prefix-cull}
To reduce the number, you can comment ones you don't wish to use:

```yaml
routing:
  prefixes:
    - lmstudio
    #- lm-studio
    #- lm_studio
```

### API Path Filtering

The `api.paths` section acts as an allowlist - only these paths can be proxied to the backend.

```yaml
api:
  paths:
    - /                    # 0: Health check
    - /api/generate        # 1: Text completion
    - /api/chat            # 2: Chat completion
    - /v1/models           # 3: OpenAI models list
    - /v1/chat/completions # 4: OpenAI chat
```

**How it works:**

- Only paths in this list are forwarded to the backend
- Unlisted paths return 404 or 501 (Not Implemented)
- Paths are exact matches (no wildcards)
- Order matters for `path_indices` references

**Security benefit:**

- Prevents access to administrative or dangerous endpoints
- Limits attack surface to known-safe operations
- Blocks model management operations (pull/push/delete)
- Blocks unsupported or unknown endpoints to general users

### Native Intercepts

Some profiles have native handlers that intercept specific endpoints instead of proxying them.

**Built-in intercepts:**

| Profile | Endpoint | Purpose |
|---------|----------|---------|
| ollama | `/api/tags` | Aggregates models across instances |
| ollama | `/v1/models` | Converts to OpenAI format |
| lmstudio | `/v1/models` | Handles LM Studio format |
| all | `/api/pull` | Blocks model management |

Generally, any time a backend returns models, Olla intercepts the call and returns a unified representation of all models in instances of the backend.

**How it works:**

1. Request arrives at `/olla/ollama/api/tags`
2. Olla checks if a native handler exists
3. If yes, handler aggregates data from all healthy endpoints
4. If no, request is proxied to a single endpoint

**Why intercepts exist:**

- **Model aggregation** - Combine model lists from multiple instances
- **Format conversion** - Transform between Ollama/OpenAI formats
- **Safety** - Block dangerous operations (model deletion)
- **Optimisation** - Cache responses across instances

### Model Discovery

The `api.model_discovery_path` defines how Olla finds available models.

```yaml
api:
  model_discovery_path: /api/tags  # Ollama
  # or
  model_discovery_path: /v1/models # OpenAI-compatible
```

**Discovery process:**

1. Health check verifies endpoint is online
2. GET request to `{endpoint_url}{model_discovery_path}`
3. Response parsed according to `request.response_format`
4. Models stored in registry with endpoint attribution
5. Unified view created across same-type endpoints

### Response Parsing

The `request.response_format` determines how model responses are parsed.

```yaml
request:
  response_format: "ollama"  # or "openai", "lmstudio", "vllm"
```

**Format mappings:**

| Format | Models Field Path | Parser |
|--------|------------------|--------|
| ollama | `models` | Ollama JSON structure |
| openai | `data` | OpenAI models array |
| lmstudio | `data` | OpenAI-compatible |
| vllm | `data` | OpenAI-compatible |

### Capability Detection

The `models.capability_patterns` section uses glob patterns to detect model features.

```yaml
models:
  capability_patterns:
    vision:
      - "*llava*"        # Matches llava, llava-13b, etc.
      - "*vision*"       # Matches gpt-4-vision
    embeddings:
      - "*embed*"        # Matches embed, embedding, text-embed
      - "nomic-embed-text" # Exact match
    code:
      - "*code*"         # Matches codellama, deepseek-coder
      - "qwen*coder*"    # Matches qwen-coder, qwencoder-7b
```

**Pattern matching:**

- Uses Go filepath.Match glob patterns
- `*` matches any characters
- Case-sensitive matching
- First matching pattern wins

### Context Window Detection

The `models.context_patterns` maps model names to context sizes.

```yaml
models:
  context_patterns:
    - pattern: "*llama-3.1*"
      context: 131072    # 128K context
    - pattern: "*-32k*"
      context: 32768     # 32K context
    - pattern: "*-16k*"
      context: 16384     # 16K context
    - pattern: "llama3*"
      context: 8192      # Default 8K
```

**Matching order:**

- Patterns evaluated top to bottom
- First match sets context size
- No match uses platform default

## Complete Profile Structure

```yaml
# Profile identification
name: ollama              # Unique identifier (required)
version: "1.0"            # Profile version
display_name: "Ollama"    # Human-readable name
description: "Local Ollama instance"

# URL routing
routing:
  prefixes:               # URL prefixes for this backend
    - ollama
    - ai

# API configuration
api:
  openai_compatible: true        # Supports OpenAI API format
  paths:                          # Allowed API paths (allowlist)
    - /
    - /api/generate
    - /api/chat
    - /api/tags
    - /v1/models
    - /v1/chat/completions
  model_discovery_path: /api/tags # Where to find models
  health_check_path: /            # Health check endpoint
  metrics_path: /metrics          # Prometheus metrics (vLLM)

# Platform characteristics
characteristics:
  timeout: 5m                     # Request timeout
  max_concurrent_requests: 10    # Concurrent request limit
  default_priority: 100           # Default endpoint priority
  streaming_support: true         # Supports streaming responses

# Request/response handling
request:
  model_field_paths:              # Where to find model name in request
    - "model"
    - "model_name"
  response_format: "ollama"       # Response parser type
  parsing_rules:
    chat_completions_path: "/api/chat"
    completions_path: "/api/generate"
    model_field_name: "model"
    supports_streaming: true

# Model handling
models:
  name_format: "{{.Name}}"        # Model name template
  capability_patterns:            # Feature detection patterns
    vision:
      - "*llava*"
    embeddings:
      - "*embed*"
    code:
      - "*code*"
  context_patterns:               # Context size detection
    - pattern: "*-32k*"
      context: 32768

# Auto-detection hints
detection:
  user_agent_patterns:            # User-Agent headers
    - "ollama/"
  headers:                        # Response headers
    - "X-Ollama-Version"
  path_indicators:                # Unique API paths
    - "/api/tags"
  default_ports:                  # Common ports
    - 11434

# Resource management
resources:
  model_sizes:                    # Memory requirements by model size
    - patterns: ["70b", "72b"]
      min_memory_gb: 40
      recommended_memory_gb: 48
      min_gpu_memory_gb: 40
      estimated_load_time_ms: 300000
  
  quantization:                   # Quantisation memory multipliers
    multipliers:
      q4: 0.5
      q5: 0.625
      q8: 0.875
  
  defaults:                       # Default requirements
    min_memory_gb: 4
    recommended_memory_gb: 8
    requires_gpu: false
    estimated_load_time_ms: 5000
  
  concurrency_limits:             # Dynamic concurrency
    - min_memory_gb: 30
      max_concurrent: 1
    - min_memory_gb: 0
      max_concurrent: 8
  
  timeout_scaling:                # Dynamic timeout adjustment
    base_timeout_seconds: 30
    load_time_buffer: true

# Path indices (optional)
path_indices:                     # Map names to path array indices
  health: 0                       # paths[0] = /
  completions: 1                  # paths[1] = /api/generate
  chat_completions: 2             # paths[2] = /api/chat

# Platform-specific features (optional)
features:
  metrics:                        # vLLM Prometheus metrics
    enabled: true
    prefix: "vllm:"
  tokenization:                   # vLLM tokenisation API
    enabled: true
    endpoints:
      - /tokenize
      - /detokenize
```

## Creating Custom Profiles

### Basic Custom Profile

To support a new LLM platform, create `config/profiles/myplatform.yaml`:

```yaml
name: myplatform
version: "1.0"
display_name: "My Platform"

routing:
  prefixes:
    - myplatform
    - mp

api:
  openai_compatible: false
  paths:
    - /health
    - /models
    - /generate
  model_discovery_path: /models
  health_check_path: /health

characteristics:
  timeout: 2m
  max_concurrent_requests: 5
  streaming_support: true
```

### Extending Existing Profiles

To modify an existing profile, create a file with the same `name`:

```yaml
# config/profiles/ollama.yaml
name: ollama  # Same name replaces built-in
version: "1.1"

routing:
  prefixes:
    - ollama
    - ai        # Add custom prefix
    - llm       # Add another prefix

characteristics:
  timeout: 10m  # Increase timeout for large models
```

### Adding New Endpoints

To allow additional API paths:

```yaml
api:
  paths:
    - /                    # Existing
    - /api/generate        # Existing
    - /api/chat            # Existing
    - /api/experimental    # New endpoint
    - /v2/chat             # New version
```

!!! warning "Security Risk"
    Only add paths you understand and trust. Unknown endpoints could expose dangerous operations.

## Troubleshooting

### Profile Not Loading

**Issue**: Custom profile not being used

**Diagnosis:**
```bash
# Check Olla logs during startup
docker logs olla | grep "Loading profile"

# Verify profile is valid YAML
yamllint config/profiles/myprofile.yaml
```

**Common causes:**

- Invalid YAML syntax
- Missing required field (`name`)
- File not in `config/profiles/` directory
- Permission issues reading file

### Routes Not Working

**Issue**: URLs return 404 despite profile configuration

**Diagnosis:**
```bash
# Check registered routes
curl http://localhost:40114/internal/status | jq .routes

# Verify prefix registration
docker logs olla | grep "Registering routes for provider"
```

**Common causes:**

- Profile missing `routing.prefixes`
- Endpoint `type` doesn't match profile `name`
- Path not in `api.paths` allowlist

### Models Not Discovered

**Issue**: No models appearing from backend

**Diagnosis:**
```bash
# Test discovery endpoint directly
curl http://backend:11434/api/tags

# Check discovery in Olla
curl http://localhost:40114/internal/status/models
```

**Common causes:**

- Wrong `model_discovery_path`
- Incorrect `response_format`
- Backend returning unexpected JSON structure
- Health check failing

### Native Intercepts Not Working

**Issue**: Requests being proxied instead of intercepted

**Diagnosis:**
```bash
# Check response headers
curl -I http://localhost:40114/olla/ollama/api/tags

# Look for X-Olla-Endpoint header (should be absent for intercepts)
```

**Common causes:**

- Profile not recognised as native type
- Handler not registered for specific path
- Custom profile overriding built-in

## Best Practices

### 1. Minimal Path Exposure

Only include paths your application needs:

```yaml
api:
  paths:
    - /v1/chat/completions  # Chat only
    - /v1/models            # Model discovery
    # Don't include admin or management endpoints
```

### 2. Appropriate Timeouts

Set timeouts based on model characteristics:

```yaml
characteristics:
  timeout: 10m  # Large models need more time

resources:
  timeout_scaling:
    base_timeout_seconds: 180
    load_time_buffer: true  # Add model load time
```

### 3. Accurate Capability Detection

Use specific patterns for capability detection:

```yaml
models:
  capability_patterns:
    vision:
      - "*llava*"           # Ollama vision models
      - "gpt-4-vision*"     # OpenAI vision
      - "claude-3*"         # Anthropic vision
```

### 4. Resource Limits

Configure realistic resource requirements:

```yaml
resources:
  concurrency_limits:
    - min_memory_gb: 0
      max_concurrent: 1  # LM Studio limitation
```

### 5. Version Your Profiles

Track your custom profile changes:

```yaml
name: myplatform
version: "1.2"  # Increment for changes
# Changelog:
# 1.2 - Added /v2/chat endpoint
# 1.1 - Increased timeout to 10m
# 1.0 - Initial version
```

Don't update versions in natively supported profiles however.

## Profile Reference

### Built-in Profiles

| Profile | Type Value | Prefixes | Native Intercepts |
|---------|------------|----------|-------------------|
| ollama | `ollama` | `ollama` | `/api/tags`, `/v1/models` |
| lmstudio | `lm-studio` | `lmstudio`, `lm-studio`, `lm_studio` | `/v1/models` |
| vllm | `vllm` | `vllm` | None |
| openai-compatible | `openai` | `openai`, `openai-compatible` | None |

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique profile identifier |
| `api.paths` | array | Allowed API paths |

### Optional Fields

All other fields have sensible defaults and are optional.

## Next Steps

- [Configuration Reference](../configuration/reference.md) - Complete configuration options
- [Security Considerations](../configuration/practices/security.md) - Profile security