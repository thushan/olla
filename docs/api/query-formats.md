# Query String Support for Model Response Formats

The `/olla/models` endpoint supports multiple output formats and filtering options through query parameters.

## Format Parameter

The `format` query parameter controls the response format:

- `/olla/models` - Default unified format (OpenAI-compatible with Olla extensions)
- `/olla/models?format=unified` - Explicit unified format (same as default)
- `/olla/models?format=openai` - Pure OpenAI compatibility (strips Olla extensions)
- `/olla/models?format=ollama` - Ollama's native response format
- `/olla/models?format=lmstudio` - LM Studio's native response format

### Response Format Examples

#### Unified Format (Default)
```json
{
  "object": "list",
  "data": [
    {
      "id": "phi/4:14.7b-q4km",
      "object": "model", 
      "created": 1735123456,
      "owned_by": "olla",
      "olla": {
        "family": "phi",
        "variant": "4",
        "parameter_size": "14.7b",
        "quantization": "q4km",
        "aliases": ["phi4:latest", "microsoft/phi-4"],
        "availability": [
          {
            "endpoint": "ollama-local",
            "url": "http://localhost:11434",
            "state": "loaded"
          }
        ],
        "capabilities": ["chat", "code"],
        "max_context_length": 131072,
        "prompt_template_id": "chatml"
      }
    }
  ]
}
```

#### OpenAI Format
```json
{
  "object": "list", 
  "data": [
    {
      "id": "phi/4:14.7b-q4km",
      "object": "model",
      "created": 1735123456,
      "owned_by": "olla"
    }
  ]
}
```

#### Ollama Format
```json
{
  "models": [
    {
      "name": "phi4:latest",
      "model": "phi4:latest", 
      "modified_at": "2025-01-02T14:30:00Z",
      "size": 9053116391,
      "digest": "ac896e5b8b34...",
      "details": {
        "family": "phi",
        "parameter_size": "14.7b",
        "quantization_level": "Q4_K_M"
      }
    }
  ]
}
```

#### LM Studio Format
```json
{
  "object": "list",
  "data": [
    {
      "id": "microsoft/phi-4",
      "object": "model",
      "type": "llm", 
      "publisher": "microsoft",
      "arch": "phi",
      "quantization": "Q4_K_M",
      "state": "loaded",
      "max_context_length": 131072
    }
  ]
}
```

## Filter Parameters

### Endpoint Filter
Filter models by specific endpoint:
- `/olla/models?endpoint=ollama-local` - Only models from the "ollama-local" endpoint
- `/olla/models?endpoint=lmstudio-local` - Only models from the "lmstudio-local" endpoint

### Availability Filter
Filter by model loading state:
- `/olla/models?available=true` - Only currently loaded/available models
- `/olla/models?available=false` - Only not-loaded models

### Include Unavailable Filter
Control whether to show models from unhealthy endpoints:
- `/olla/models` - Default: only shows models from healthy endpoints
- `/olla/models?include_unavailable=true` - Shows all models with availability status

When `include_unavailable=true` is used with `format=unified`, the response includes availability information:
```json
{
  "olla": {
    "availability": [
      {
        "endpoint": "ollama-local",
        "state": "loaded"      // Model loaded on healthy endpoint
      },
      {
        "endpoint": "ollama-backup", 
        "state": "unhealthy"   // Endpoint is down
      }
    ]
  }
}
```

### Family Filter
Filter by model family:
- `/olla/models?family=llama` - Only Llama family models
- `/olla/models?family=phi` - Only Phi family models

### Type Filter
Filter by model type:
- `/olla/models?type=llm` - Only language models
- `/olla/models?type=vlm` - Only vision/multimodal models
- `/olla/models?type=embeddings` - Only embedding models

### Legacy Capability Filter
The `capability` parameter is supported for backward compatibility and maps to types:
- `/olla/models?capability=vision` → `type=vlm`
- `/olla/models?capability=chat` → `type=llm`
- `/olla/models?capability=embeddings` → `type=embeddings`

## Combining Parameters

Multiple parameters can be combined:
```
/olla/models?format=ollama&family=llama&available=true
```

This returns only loaded Llama models in Ollama format.

## Error Handling

Invalid parameter values return a 400 Bad Request with helpful error messages:

```json
{
  "error": "invalid query parameter format=invalid: unsupported format. Supported formats: unified, openai, ollama, lmstudio"
}
```

## Implementation Notes

- **Platform-specific formats**: When using `format=ollama` or `format=lmstudio`, only models available on those platforms are included in the response
- **Native names**: Platform-specific formats use the native model names from that platform
- **Endpoint resolution**: The endpoint filter accepts both endpoint names and URLs
- **Normalization**: All string comparisons for filters are case-insensitive