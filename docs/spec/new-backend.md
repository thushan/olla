# Implementing a New Backend in Olla

This guide is designed for AI tools like Claude Code or OpenCode to help create new backends. As they are reproducible,
known integration points currently, it seems like the obvious choice for AI augmented backend development.

Verified on Opus 4.5 | 4.6 / Sonnet 4.5 | 4.6.

## Overview

Olla uses a **profile-based architecture**. Each backend needs:
1. A **YAML profile** defining API endpoints, characteristics, and capabilities
2. A **response parser** converting backend JSON to `ModelInfo`
3. A **model converter** converting unified models back to backend format
4. **Constants** for type-safe references

OpenAI-compatible backends can often reuse existing parsers, requiring only a YAML profile and constants.

---

## Quick Reference: Files to Create/Modify

### New Files
| File | Purpose |
|------|---------|
| `config/profiles/{backend}.yaml` | Profile configuration |
| `internal/adapter/registry/profile/{backend}.go` | Response type definitions |
| `internal/adapter/registry/profile/{backend}_parser.go` | Parser implementation |
| `internal/adapter/registry/profile/{backend}_parser_test.go` | Parser tests |
| `internal/adapter/converter/{backend}_converter.go` | Converter implementation |
| `internal/adapter/converter/{backend}_converter_test.go` | Converter tests |

### Modified Files
| File | Change |
|------|--------|
| `internal/core/constants/providers.go` | Add provider type constant |
| `internal/core/domain/profile.go` | Add profile constant |
| `internal/adapter/registry/profile/parsers.go` | Register parser |
| `internal/adapter/converter/factory.go` | Register converter |

---

## Reference Implementations

| Backend | Complexity | Good Example For | Test Coverage |
|---------|------------|------------------|---------------|
| `litellm` | Minimal | Reusing OpenAI parser (YAML-only, no custom code) | N/A (uses OpenAI parser) |
| `openai-compatible` | Low | Minimal implementation with OpenAI format | Complete |
| `llamacpp` | Medium | Single-model servers, GGUF format | Complete |
| `vllm` | Medium | Extended OpenAI format, permissions | Complete |
| `sglang` | Medium | Feature-rich backends | Partial (parser tests missing) |
| `lemonade` | High | Custom response formats | Complete |

All files live in: `config/profiles/`, `internal/adapter/registry/profile/`, `internal/adapter/converter/`

### Reference Implementation PRs

Use the `gh` command to review the implementations already merged:

* [llamacpp](https://github.com/thushan/olla/pull/73)
* [lemonade](https://github.com/thushan/olla/pull/70)
* [sglang](https://github.com/thushan/olla/pull/69)

---

## Step 1: Add Constants

**`internal/core/constants/providers.go`**

```go
const (
    ProviderTypeMyBackend    = "mybackend"
    ProviderDisplayMyBackend = "My Backend"
    ProviderPrefixMyBackend1 = "mybackend"
    ProviderPrefixMyBackend2 = "my-backend"  // Alternative alias
)
```

**`internal/core/domain/profile.go`**

```go
const (
    ProfileMyBackend = "mybackend"
)
```

---

## Step 2: Create Profile YAML

**`config/profiles/{backend}.yaml`**

### Required Sections

```yaml
name: "mybackend"                           # Must match constant
version: "1.0"
display_name: "My Backend"
description: "Description of this backend"

routing:
  prefixes:
    - mybackend
    - my-backend                            # Optional aliases

api:
  openai_compatible: true                   # Most backends are OpenAI-compatible
  paths:
    - /v1/models
    - /v1/chat/completions
    - /v1/completions
    - /v1/embeddings
    - /health
  model_discovery_path: /v1/models
  health_check_path: /v1/models             # Use /health if available

characteristics:
  timeout: 2m
  max_concurrent_requests: 50
  default_priority: 80                      # 0-100, higher = preferred
  streaming_support: true

request:
  model_field_paths:
    - "model"
  response_format: "mybackend"              # Parser identifier
  parsing_rules:
    chat_completions_path: "/v1/chat/completions"
    completions_path: "/v1/completions"
    model_field_name: "model"
    supports_streaming: true
```

### Optional Sections

```yaml
home: "https://github.com/example/mybackend"

detection:
  path_indicators: ["/v1/models", "/health"]
  default_ports: [8000]
  headers: ["X-MyBackend-Version"]

# Only these 5 keys are consumed by code; others are silently ignored
path_indices:
  health: 0
  models: 1
  completions: 2
  chat_completions: 3
  embeddings: 4

models:
  name_format: "{{.Name}}"
  capability_patterns:
    chat: ["*-Instruct*", "*-Chat*"]
    embeddings: ["*embed*"]
    vision: ["*vision*", "*llava*"]
    code: ["*code*", "*coder*"]
  context_patterns:
    - pattern: "*llama-3.1*"
      context: 131072
    - pattern: "*mistral*"
      context: 32768

resources:
  model_sizes:
    - patterns: ["*70b*", "*72b*"]
      min_memory_gb: 40
      recommended_memory_gb: 48
      estimated_load_time_ms: 60000
    - patterns: ["*7b*", "*8b*"]
      min_memory_gb: 6
      recommended_memory_gb: 8
      estimated_load_time_ms: 15000
  defaults:
    min_memory_gb: 4
    recommended_memory_gb: 8
    requires_gpu: false
    estimated_load_time_ms: 5000
  concurrency_limits:
    - min_memory_gb: 30
      max_concurrent: 2
    - min_memory_gb: 0
      max_concurrent: 10

features:
  custom_feature:
    enabled: true
    description: "Description of feature"

metrics:
  extraction:
    enabled: true
    source: response_body
    format: json
    paths:
      model: "$.model"
      input_tokens: "$.usage.prompt_tokens"
      output_tokens: "$.usage.completion_tokens"
    calculations:
      is_complete: 'len(finish_reason) > 0'
```

### Profile Loading and Precedence

Olla includes **built-in profiles** (defined in `internal/adapter/registry/profile/loader.go`) for common backends that work without configuration files. YAML profiles in `config/profiles/` **override** built-in profiles with the same name.

**Interface hierarchy** (for advanced use cases beyond YAML):
- `InferenceProfile` - Main interface (`internal/core/domain/inference_profile.go`)
- `PlatformProfile` - Embedded by `InferenceProfile`, defines platform-specific methods
- `ConfigurableProfile` - Implements `InferenceProfile`, used for YAML-based profiles

Most backends only need YAML profiles.

---

## Step 3: Create Response Types

**`internal/adapter/registry/profile/{backend}.go`**

Define Go structs matching your backend's `/v1/models` JSON response.

**Guidelines**:
- Response wrapper (typically `Object` + `Data` array), model struct with OpenAI-compatible base fields
- Use `json` tags for all fields, pointers for nullable fields (`*int64`, `*string`)
- Add `omitempty` for optional fields; follow OpenAI structure where possible

**Reference**: `llamacpp.go`, `lemonade.go`, `sglang.go`

---

## Step 4: Implement Parser

**`internal/adapter/registry/profile/{backend}_parser.go`**

Parses backend JSON into `[]*domain.ModelInfo`.

```go
type myBackendParser struct{}

func (p *myBackendParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
    // 1. Handle empty input - return empty slice, not nil
    // 2. Unmarshal JSON to your response types
    // 3. Convert each model to domain.ModelInfo
    // 4. Set Type field using constants.ProviderType*
    // 5. Populate ModelDetails only if you have metadata
    // 6. Skip invalid models (empty ID)
    // 7. Return slice of ModelInfo
}
```

**Key points**:
- Package uses json-iterator (aliased as `json`)
- Return empty slice `[]` for empty input, never `nil`
- Set `LastSeen` to `time.Now()`
- Only allocate `ModelDetails` if you have metadata to populate

**Reference**: `llamacpp_parser.go`, `lemonade_parser.go`, `vllm_parser.go`

### Register Parser

**`internal/adapter/registry/profile/parsers.go`** -- add case to `getParserForFormat`:

```go
case constants.ProviderTypeMyBackend:
    return &myBackendParser{}
```

---

## Step 5: Implement Converter

**`internal/adapter/converter/{backend}_converter.go`**

Converts unified models back to backend-specific format.

```go
type MyBackendConverter struct {
    *BaseConverter
}

func NewMyBackendConverter() ports.ModelResponseConverter {
    return &MyBackendConverter{
        BaseConverter: NewBaseConverter(constants.ProviderTypeMyBackend),
    }
}

func (c *MyBackendConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
    // 1. Call filterModels() to apply filters
    // 2. Convert each UnifiedModel to backend format
    // 3. Use BaseConverter.FindProviderAlias() to get native model name
    // 4. Fall back to model.Aliases[0] or model.ID if no alias found
    // 5. Return backend-specific response struct
}
```

**Key utilities**: `BaseConverter.FindProviderAlias()`, `ExtractOwnerFromModelID()`, `filterModels()`

**Reference**: `llamacpp_converter.go`, `lemonade_converter.go`, `sglang_converter.go`

### Register Converter

**`internal/adapter/converter/factory.go`** -- add to `NewConverterFactory`:

```go
factory.RegisterConverter(NewMyBackendConverter())
```

---

## Step 6: Write Tests

### Parser Tests (`{backend}_parser_test.go`)

Required cases: valid response with full metadata, models without optional fields, empty response, empty data array, models without ID (should skip), invalid JSON (should error), LastSeen timestamp is set.

### Converter Tests (`{backend}_converter_test.go`)

Required cases: correct conversion, filters apply correctly, fallback when no native alias, correct format name, empty model list.

---

## Shortcut: Reusing Existing Parsers

Set `response_format: "openai"` in YAML to skip creating a custom parser. See `config/profiles/litellm.yaml` for a complete YAML-only backend (no custom parser or converter).

If your backend matches vLLM, SGLang, or llama.cpp format exactly, reference their parser instead.

---

## Key Domain Types

### ModelInfo (Parser Output) -- `internal/core/domain/model.go`
- `Name` - Model identifier
- `Type` - Backend type (use `constants.ProviderType*`)
- `LastSeen` - Discovery timestamp
- `Details` - Optional `ModelDetails` metadata

### ModelDetails (Metadata) -- `internal/core/domain/model.go`
- `MaxContextLength`, `ParameterSize`, `QuantizationLevel` - Model specs
- `Family`, `Format`, `Publisher` - Model attributes
- `Checkpoint`, `Recipe` - Backend-specific (e.g., Lemonade)
- `Type`, `State`, `Digest` - Runtime metadata

### UnifiedModel (Converter Input) -- `internal/core/domain/unified_model.go`
- `ID`, `Family`, `Variant` - Model identity
- `ParameterSize`, `Quantization`, `Format`, `MaxContextLength` - Specs
- `Aliases` - All known names across backends
- `Capabilities` - Inferred features (chat, vision, etc.)
- `Metadata` - Backend-specific extras

---

## Common Pitfalls

1. **Empty response handling** - Return empty slice, not nil
2. **Skip invalid models** - Check `model.ID != ""` before processing
3. **ModelDetails allocation** - Only create if you have metadata to set
4. **Use constants** - Never hardcode provider names; use `constants.ProviderType*`
5. **Pointer fields** - Use pointers for nullable fields in response structs
6. **Filter models** - Always call `filterModels()` before converting
7. **Register parser** - Update `getParserForFormat()` in `parsers.go`
8. **Register converter** - Update `NewConverterFactory()` in `factory.go`
9. **Error messages** - Include backend name: `"failed to parse MyBackend response"`
10. **Type field** - Set `modelInfo.Type` to the provider constant (some existing parsers are inconsistent; best practice is `constants.ProviderType*`)

---

## Verification

```bash
make test
go test -v ./internal/adapter/registry/profile/... -run MyBackend
go test -v ./internal/adapter/converter/... -run MyBackend
make test-race
make ready-tools
make build-local
```

---

## Example Configuration

Add endpoint to `config.yaml`:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:8000"
        name: "my-backend"
        type: "mybackend"
        priority: 80
        model_url: "/v1/models"
        health_check_url: "/v1/models"
        check_interval: 10s
        check_timeout: 5s
```
