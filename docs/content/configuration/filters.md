---
title: Filters - Model and Profile Filtering in Olla
description: Control models and profiles in Olla with powerful glob pattern filters. Include/exclude models by name, type, or size. Profile filtering for provider control.
keywords: olla filters, model filtering, profile filtering, glob patterns, exclude models, include models, endpoint filters, wildcard patterns
---

# Filters

Olla provides a powerful filtering system that allows you to control which models and profiles are available in your deployment. Filters use glob patterns with wildcard support, making it easy to include or exclude resources based on naming patterns.

## Core Concepts

### Filter Configuration

Filters are configured using `include` and `exclude` lists:

```yaml
filter:
  include:   # Only items matching these patterns are allowed
    - "llama*"
    - "mistral*"
  exclude:   # Items matching these patterns are rejected
    - "*test*"
    - "*debug*"
```

### Pattern Matching

Olla supports glob-style patterns with the `*` wildcard:

| Pattern | Matches | Examples |
|---------|---------|----------|
| `*` | Everything | All models/profiles |
| `llama*` | Starts with "llama" | `llama3-8b`, `llama2-70b` |
| `*-7b` | Ends with "-7b" | `mistral-7b`, `qwen-7b` |
| `*embed*` | Contains "embed" | `nomic-embed-text`, `text-embedding-ada` |
| `deepseek-*` | Starts with "deepseek-" | `deepseek-coder`, `deepseek-r1` |

### Precedence Rules

1. **Exclude takes precedence over include** - If an item matches both include and exclude patterns, it will be excluded
2. **No filter means allow all** - If no filter is specified, all items are allowed
3. **Empty include means exclude all** - An empty include list with no exclude list blocks everything

## Where Filters Can Be Applied

Filters can be applied at multiple levels in your Olla configuration:

### 1. Profile Filtering

Control which inference profiles are loaded at startup. [Learn more →](reference.md#profile-filtering)

```yaml
proxy:
  profile_filter:
    include:
      - "ollama"
      - "openai*"
    exclude:
      - "*test*"
```

### 2. Endpoint Model Filtering

Filter models at the endpoint level during discovery. [Learn more →](reference.md#endpoint-model-filtering)

```yaml
discovery:
  static:
    endpoints:
      - name: ollama-prod
        url: http://localhost:11434
        model_filter:
          exclude:
            - "*embed*"    # No embedding models
            - "*test*"     # No test models
            - "nomic-*"    # No nomic models
```

### 3. Global Model Filtering (Planned)

Filter models globally across all endpoints. This feature is planned for a future release.

## Common Use Cases

### Production Deployment

Exclude test and experimental models:

```yaml
model_filter:
  exclude:
    - "*test*"
    - "*debug*"
    - "*experimental*"
    - "*uncensored*"
```

### Specialized Services

#### Embedding Service
Only allow embedding models:

```yaml
model_filter:
  include:
    - "*embed*"
    - "bge-*"
    - "e5-*"
```

#### Chat Service
Only allow conversational models:

```yaml
model_filter:
  include:
    - "*chat*"
    - "*instruct*"
  exclude:
    - "*embed*"
    - "*code*"
```

#### Code Generation Service
Only allow code-focused models:

```yaml
model_filter:
  include:
    - "*code*"
    - "deepseek-coder*"
    - "codellama*"
```

### Model Size Restrictions

#### Small Models Only (≤13B)
```yaml
model_filter:
  include:
    - "*-3b*"
    - "*-7b*"
    - "*-8b*"
    - "*-13b*"
```

#### Large Models Only (≥34B)
```yaml
model_filter:
  include:
    - "*-34b*"
    - "*-70b*"
    - "*-72b*"
```

### Provider-Specific Filtering

#### Only OpenAI-Compatible Models
```yaml
profile_filter:
  include:
    - "openai*"
    - "vllm"
    - "litellm"
```

#### Local Models Only
```yaml
profile_filter:
  include:
    - "ollama"
    - "lm-studio"
    - "llamacpp"
```

## Performance Considerations

- **Filter evaluation is cached** - Pattern matching results are cached for performance
- **Discovery-time filtering** - Models are filtered during discovery, not at request time
- **Minimal overhead** - The filter system adds negligible latency to model discovery

## Debugging Filters

To see which models are being filtered:

1. Enable debug logging:
```yaml
logging:
  level: debug
```

2. Check the logs during model discovery:
```
INFO  Applied model filter endpoint=ollama original_count=20 filtered_count=15
```

3. Use the status endpoint to verify active models:
```bash
curl http://localhost:8080/internal/status | jq '.models[].name'
```

## Best Practices

1. **Be specific with patterns** - Avoid overly broad patterns that might accidentally exclude needed models
2. **Test filters in development** - Verify your filters work as expected before deploying to production
3. **Document your filters** - Add comments explaining why certain patterns are included/excluded
4. **Use exclude for security** - Explicitly exclude sensitive or inappropriate models
5. **Consider maintenance** - Design patterns that won't break when new models are added

## Examples

### Complete Configuration Example

```yaml
# Exclude embedding models from a general-purpose endpoint
discovery:
  static:
    endpoints:
      - name: chat-endpoint
        url: http://localhost:11434
        type: ollama
        model_filter:
          include:
            - "llama*"      # Llama family
            - "mistral*"    # Mistral family
            - "qwen*"       # Qwen family
          exclude:
            - "*embed*"     # No embeddings
            - "*-uncensored" # No uncensored variants
            
      - name: embedding-endpoint
        url: http://localhost:11435
        type: ollama
        model_filter:
          include:
            - "*embed*"     # Only embeddings
            - "bge-*"       # BGE models
            - "e5-*"        # E5 models

# Only load production-ready profiles
proxy:
  profile_filter:
    exclude:
      - "*test*"
      - "*debug*"
```

### Migration from Unfiltered Setup

If you're adding filters to an existing deployment:

1. **Audit current models**: List all models currently in use
2. **Design inclusive patterns**: Start with broad includes
3. **Add specific excludes**: Gradually add exclusions
4. **Test thoroughly**: Verify critical models aren't filtered
5. **Deploy gradually**: Roll out to staging before production

## Related Documentation

- [Configuration Reference](reference.md) - Complete configuration options
- [Configuration Examples](examples.md) - Practical configuration examples
- [Profile System](../concepts/profile-system.md) - Understanding inference profiles