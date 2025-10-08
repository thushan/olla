# Inference Profiles

This directory contains configuration files for different LLM inference platforms supported by Olla.

## Overview

Each YAML file in this directory defines how Olla interacts with a specific inference platform. 

These profiles control:

- API paths and endpoints
- Request/response formats
- Platform-specific behaviours
- Model discovery and parsing

## Built-in Profiles

- `ollama.yaml` - Ollama local inference server
- `lmstudio.yaml` - LM Studio local inference server
- `lemonade.yaml` - Lemonade SDK local LLM serving platform with AMD Ryzen AI support
- `vllm.yaml` - vLLM high-performance inference server
- `sglang.yaml` - SGLang high-performance inference server
- `openai.yaml` - OpenAI-compatible API generic profile (Ollama, LocalAI, etc.)

## Adding a New Platform

To add support for a new inference platform:

1. Create a new YAML file named `{platform}.yaml`
2. Use the template below as a starting point
3. Restart Olla to load the new profile

## Profile Template

```yaml
# Platform name and metadata
name: myplatform
version: "1.0"
display_name: "My Platform"
description: "Description of the platform"

# API configuration
api:
  openai_compatible: false  # Set to true if it follows OpenAI API
  paths:
    - /health             # List all supported API paths
    - /api/models
    - /api/completions
  model_discovery_path: /api/models
  health_check_path: /health

# Platform characteristics
characteristics:
  timeout: 2m                    # Request timeout
  max_concurrent_requests: 5     # Concurrent request limit
  default_priority: 50           # Default endpoint priority (0-100)
  streaming_support: true        # Supports streaming responses

# Auto-detection hints
detection:
  user_agent_patterns:           # User-Agent patterns
    - "myplatform/"
  headers:                       # Response headers
    - "X-MyPlatform-Version"
  path_indicators:               # Paths that indicate this platform
    - "/api/models"
  default_ports:                 # Default ports
    - 8080

# Model configuration
models:
  name_format: "{{.Name}}"       # How model names are formatted
  capability_patterns:           # Patterns to detect model capabilities
    chat:
      - "*chat*"
    embeddings:
      - "*embed*"

# Request handling
request:
  model_field_paths:             # Where to find model name in requests
    - "model"
    - "model_name"
  response_format: "openai"      # Response parser: ollama, lmstudio, openai
  parsing_rules:
    chat_completions_path: "/api/chat"
    completions_path: "/api/completions"
    model_field_name: "model"
    supports_streaming: true

# Path indices (which path serves which purpose)
path_indices:
  health: 0
  models: 1
  completions: 2
```

## Customising Existing Profiles

To customise an existing profile:

1. Copy the existing profile file (e.g., `ollama.yaml`)
2. Modify the settings as needed
3. Your custom profile will override the built-in defaults

## Configuration Reference

### API Section
- `openai_compatible`: Whether the platform follows OpenAI's API structure
- `paths`: All API paths this platform supports
- `model_discovery_path`: Path to discover available models
- `health_check_path`: Path for health checks

### Characteristics Section
- `timeout`: Maximum time to wait for responses
- `max_concurrent_requests`: How many requests can be processed simultaneously
- `default_priority`: Priority for load balancing (higher = preferred)
- `streaming_support`: Whether the platform supports streaming responses

### Detection Section
Used for auto-detecting platform types:
- `user_agent_patterns`: Patterns in User-Agent headers
- `headers`: Response headers that indicate this platform
- `path_indicators`: API paths unique to this platform
- `default_ports`: Common ports for this platform

### Models Section
- `name_format`: Template for model name formatting
- `capability_patterns`: Patterns to detect model capabilities (vision, embeddings, etc.)

### Request Section
- `model_field_paths`: JSON paths to find model name in requests
- `response_format`: Which parser to use (ollama, lmstudio, vllm, sglang, lemonade, openai)
- `parsing_rules`: How to parse different request types

## Examples

### Adding vLLM Support

> [!NOTE]  
> We now have official support for vLLM, but this is just an example of how to add a new profile.

Create `vllm.yaml`:

```yaml
name: vllm
version: "1.0"
display_name: "vLLM"
description: "High-performance LLM serving"

api:
  openai_compatible: true
  paths:
    - /v1/models
    - /v1/chat/completions
    - /v1/completions
  model_discovery_path: /v1/models
  health_check_path: /v1/models

characteristics:
  timeout: 2m
  max_concurrent_requests: 100  # vLLM handles high concurrency well
  default_priority: 80
  streaming_support: true

# ... rest of configuration
```

### Adding LocalAI Support

Create `localai.yaml`:

```yaml
name: localai
version: "1.0"
display_name: "LocalAI"
description: "Local OpenAI-compatible API"

api:
  openai_compatible: true
  paths:
    - /v1/models
    - /v1/chat/completions
    - /v1/completions
    - /v1/images/generations
    - /v1/audio/transcriptions
  # ... rest of configuration
```

## Validation

Olla validates profiles on startup. 

Common validation errors:

- Missing required fields (name, api.paths)
- Invalid path indices
- Unknown response formats

Check Olla logs if your profile isn't loading correctly.