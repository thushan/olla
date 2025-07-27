# How to Add a New Provider to Olla

## Simple Process (No Code Changes Required!)

With the dynamic route registration system, adding a new provider is incredibly simple - just create a YAML profile and add endpoints to your config:

### 1. Create a Profile YAML File

Create `config/profiles/vllm.yaml`:

```yaml
name: vllm
version: "1.0"
display_name: "vLLM"
description: "vLLM high-performance inference server"

# Routing configuration - URL prefixes that map to this provider
routing:
  prefixes:
    - vllm

# API compatibility
api:
  openai_compatible: true
  paths:
    - /v1/models
    - /v1/chat/completions
    - /v1/completions
  model_discovery_path: /v1/models
  health_check_path: /v1/models

# Platform characteristics
characteristics:
  timeout: 2m
  max_concurrent_requests: 100
  default_priority: 80
  streaming_support: true
```

### 2. Add Endpoints to config.yaml

```yaml
discovery:
  static:
    endpoints:
      - name: vllm-1
        url: http://localhost:8000
        type: vllm
        priority: 80
```

### 3. Run Olla

```bash
./olla
```

That's it! No code changes or recompilation needed. The system will automatically:
- Register routes for `/olla/vllm/`
- Create model discovery endpoints at `/olla/vllm/v1/models` 
- Set up the proxy handler for all other vLLM API calls
- Handle model aggregation and format conversion

## How It Works

The dynamic route registration system automatically reads all profile YAML files and creates routes for each provider:

1. **Profile Discovery**: On startup, Olla scans the `config/profiles` directory for YAML files
2. **Route Generation**: For each profile with routing prefixes, it automatically creates:
   - Model discovery endpoints (both native and OpenAI formats if supported)
   - Catch-all proxy routes for API passthrough
   - Special endpoints for provider-specific features (like Ollama's `/api/show`)
3. **Generic Handlers**: Uses a single generic handler that works for any provider, eliminating code duplication

## Supported URL Variations

The system automatically handles URL variations. For example, LM Studio can be accessed via:
- `/olla/lmstudio/`
- `/olla/lm-studio/`
- `/olla/lm_studio/`

Just add all variations to the `routing.prefixes` list in your profile.

## Example: Adding Groq

1. Create `config/profiles/groq.yaml`:
```yaml
name: groq
version: "1.0"
display_name: "Groq"
description: "Groq LPU inference"

routing:
  prefixes:
    - groq

api:
  openai_compatible: true
  model_discovery_path: /openai/v1/models
  health_check_path: /openai/v1/models
```

2. Add to config.yaml:
```yaml
endpoints:
  - name: groq-cloud
    url: https://api.groq.com
    type: groq
    priority: 90
```

3. Run Olla - done!

## Benefits

- **No Code Changes**: Add providers without touching Go code
- **No Recompilation**: Changes take effect on restart
- **Consistent API**: All providers follow the same pattern
- **Easy Testing**: Test new providers immediately
- **Reduced Maintenance**: One handler for all providers

## Advanced Features

For providers that need special handling, you can still add custom code, but the basic functionality works out of the box for any OpenAI-compatible or standard API.