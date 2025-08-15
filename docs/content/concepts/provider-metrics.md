---
title: "Provider Metrics - Real-time LLM Performance Monitoring"
description: "How Olla captures and exposes real-time metrics from LLM providers including token usage, generation speed, and latency measurements."
keywords: ["provider metrics", "llm metrics", "token usage", "performance monitoring", "ollama metrics", "model performance"]
---

# Provider Metrics

!!! info "Part of the Profile System"
    Provider metrics are configured as part of the [Profile System](profile-system.md). Each provider profile can define its own metrics extraction configuration to capture platform-specific performance data.

Olla automatically extracts and exposes detailed performance metrics from LLM provider responses, giving you real-time insights into model performance, token usage, and processing times.

## Overview

Provider metrics extraction is a **profile-based feature** that captures performance data from the final response chunks of LLM providers. Each provider profile (`config/profiles/*.yaml`) can define how to extract metrics from its specific response format. This data includes:

- Token generation statistics
- Processing latencies
- Model-specific metrics
- Provider-specific performance data

## Supported Providers

### Ollama
Ollama provides comprehensive metrics in its response format:

```json
{
  "model": "llama3.2",
  "created_at": "2025-08-15T10:00:00Z",
  "done": true,
  "total_duration": 5000000000,
  "load_duration": 500000000,
  "prompt_eval_count": 50,
  "prompt_eval_duration": 100000000,
  "eval_count": 200,
  "eval_duration": 4400000000
}
```

### OpenAI-Compatible
OpenAI and compatible providers return usage information:

```json
{
  "usage": {
    "prompt_tokens": 50,
    "completion_tokens": 200,
    "total_tokens": 250
  }
}
```

### LM Studio
LM Studio provides timing and token metrics:

```json
{
  "usage": {
    "prompt_tokens": 50,
    "completion_tokens": 200,
    "total_tokens": 250
  },
  "timings": {
    "prompt_n": 50,
    "prompt_ms": 100,
    "predicted_n": 200,
    "predicted_ms": 4400
  }
}
```

### vLLM
vLLM exposes detailed metrics via Prometheus endpoint:

```
vllm:prompt_tokens_total{model="llama3.2"} 50
vllm:generation_tokens_total{model="llama3.2"} 200
vllm:time_to_first_token_seconds{model="llama3.2"} 0.1
vllm:time_per_output_token_seconds{model="llama3.2"} 0.022
```

## Extracted Metrics

The following metrics are extracted when available:

| Metric | Description | Unit |
|--------|-------------|------|
| `total_duration_ms` | Total end-to-end processing time | milliseconds |
| `load_duration_ms` | Model loading time (if applicable) | milliseconds |
| `prompt_eval_duration_ms` | Time to process prompt | milliseconds |
| `eval_duration_ms` | Time to generate response | milliseconds |
| `prompt_tokens` | Number of tokens in the prompt | count |
| `completion_tokens` | Number of tokens generated | count |
| `total_tokens` | Total tokens processed | count |
| `tokens_per_second` | Generation speed | tokens/sec |
| `time_to_first_token_ms` | Latency to first token | milliseconds |
| `time_per_token_ms` | Average time per token | milliseconds |
| `finish_reason` | Why generation stopped | string |
| `model` | Model identifier used | string |

## Configuration

!!! tip "Profile-Based Configuration"
    Metrics extraction is configured within each provider's profile file (e.g., `config/profiles/ollama.yaml`). This allows each provider to define its own extraction logic based on its specific response format. See the [Profile System documentation](profile-system.md) for more details on profile configuration.

### Configuration Structure
Provider metrics are configured in the profile YAML files under the `metrics.extraction` section:

```yaml
metrics:
  extraction:
    enabled: true|false        # Enable/disable metrics extraction
    source: "response_body"     # Where to extract from (response_body or response_headers)
    format: "json"              # Format of the source data
    
    paths:                      # JSONPath expressions to extract raw values
      <field_name>: <jsonpath>
    
    calculations:               # Mathematical expressions using extracted values
      <metric_name>: <expression>
```

### Key Components

1. **`paths`**: Maps field names to JSON path expressions for extracting values from the provider's response
   - Supports both JSONPath notation (`$.field.subfield`) and gjson notation (`field.subfield`)
   - JSONPath prefixes are automatically normalized: `$.` is trimmed, `$` becomes empty string
2. **`calculations`**: Defines derived metrics using mathematical expressions that reference extracted fields
3. **Expression variables**: Any field defined in `paths` can be used as a variable in `calculations`
4. **Pre-compilation**: Expressions are compiled at startup for performance

### Profile Configuration
Each provider profile can define how to extract metrics using the `metrics.extraction` configuration:

```yaml
# config/profiles/ollama.yaml
name: ollama
display_name: "Ollama"
description: "Local Ollama instance for running GGUF models"

# Metrics extraction configuration
metrics:
  extraction:
    enabled: true
    source: "response_body"  # Where to extract from
    format: "json"           # Expected format
    
    # JSONPath expressions for extracting values from provider response
    # Note: Both JSONPath ($.field) and gjson (field) notation are supported
    paths:
      model: "$.model"                    # JSONPath notation (normalized to "model")
      is_complete: "done"                 # gjson notation (used as-is)
      # Token counts
      input_tokens: "$.prompt_eval_count"
      output_tokens: "eval_count"         # Both formats work identically
      # Timing data (in nanoseconds from Ollama)
      total_duration_ns: "$.total_duration"
      load_duration_ns: "$.load_duration"
      prompt_duration_ns: "$.prompt_eval_duration"
      eval_duration_ns: "$.eval_duration"
    
    # Mathematical expressions to calculate derived metrics
    calculations:
      tokens_per_second: "output_tokens / (eval_duration_ns / 1000000000)"
      ttft_ms: "prompt_duration_ns / 1000000"
      total_ms: "total_duration_ns / 1000000"
      model_load_ms: "load_duration_ns / 1000000"
```

### OpenAI Profile Example
```yaml
# config/profiles/openai.yaml
name: openai
display_name: "OpenAI Compatible"

metrics:
  extraction:
    enabled: true
    source: "response_body"
    format: "json"
    
    paths:
      # OpenAI standard usage format
      input_tokens: "$.usage.prompt_tokens"
      output_tokens: "$.usage.completion_tokens"
      total_tokens: "$.usage.total_tokens"
      model: "$.model"
      finish_reason: "$.choices[0].finish_reason"
```

### LM Studio Profile Example
```yaml
# config/profiles/lmstudio.yaml
name: lmstudio
display_name: "LM Studio"

metrics:
  extraction:
    enabled: true
    source: "response_body"
    format: "json"
    
    paths:
      # Usage data
      input_tokens: "$.usage.prompt_tokens"
      output_tokens: "$.usage.completion_tokens"
      total_tokens: "$.usage.total_tokens"
      # Timing data specific to LM Studio
      prompt_n: "$.timings.prompt_n"
      prompt_ms: "$.timings.prompt_ms"
      predicted_n: "$.timings.predicted_n"
      predicted_ms: "$.timings.predicted_ms"
    
    calculations:
      tokens_per_second: "predicted_n / (predicted_ms / 1000)"
      time_per_token_ms: "predicted_ms / predicted_n"
```

## Accessing Metrics

### Via Response Headers
Provider metrics are included in detailed debug logs when available:

```
2025/08/15 10:00:00 DEBUG Sherpa proxy metrics
  endpoint=local-ollama
  latency_ms=5000
  provider_total_ms=5000
  provider_prompt_eval_ms=100
  provider_eval_ms=4400
  provider_prompt_tokens=50
  provider_completion_tokens=200
  provider_tokens_per_second=45.45
```

### Via Status Endpoint
Aggregated metrics are available through the status endpoint:

```bash
curl http://localhost:40114/internal/status
```

Response includes provider metrics when available:
```json
{
  "proxy": {
    "endpoints": {
      "local-ollama": {
        "requests": 100,
        "avg_latency_ms": 5000,
        "avg_tokens_per_second": 45.5,
        "avg_prompt_tokens": 50,
        "avg_completion_tokens": 200
      }
    }
  }
}
```

## Performance Considerations

### Extraction Implementation
Olla uses high-performance libraries for metrics extraction:
- **[gjson](https://github.com/tidwall/gjson)**: For JSON path parsing (7.6x faster than encoding/json)
- **[expr](https://github.com/expr-lang/expr)**: For pre-compiled mathematical expressions

**JSONPath Normalization**: Olla automatically normalizes JSONPath-style prefixes for gjson compatibility:
- `$.foo.bar` → `foo.bar` (leading `$.` is trimmed)
- `$` → `` (root selector is converted to empty string)
- `foo.bar` → `foo.bar` (already normalized paths are unchanged)

This means you can use either JSONPath notation (`$.model`) or gjson notation (`model`) in your configurations - both work identically.

### Extraction Overhead
- Metrics extraction runs with a 10ms timeout to prevent blocking
- Extraction is best-effort - failures don't affect request processing
- Expressions are pre-compiled at startup, not runtime
- Zero-allocation design for high-throughput scenarios
- Performance: ~10µs per extraction operation

### Memory Usage
- **Olla**: Only captures last chunk on EOF (13x reduction in allocations)
- **Sherpa**: Ring buffer implementation (8KB max) for bounded memory
- Typical overhead: ~2KB per extraction
- Automatic cleanup after extraction

## Monitoring Best Practices

### Key Metrics to Track

1. **Token Generation Speed**
   - `tokens_per_second` - Overall generation performance
   - `time_per_token_ms` - Consistency of generation

2. **Latencies**
   - `prompt_eval_duration_ms` - Prompt processing time
   - `eval_duration_ms` - Generation time
   - `time_to_first_token_ms` - Initial response latency

3. **Resource Usage**
   - `prompt_tokens` - Input size
   - `completion_tokens` - Output size
   - `total_tokens` - Total processing load

### Alerting Thresholds

```yaml
# Example alerting configuration
alerts:
  - name: slow_generation
    condition: tokens_per_second < 10
    severity: warning
    
  - name: high_prompt_latency
    condition: prompt_eval_duration_ms > 5000
    severity: warning
    
  - name: excessive_tokens
    condition: total_tokens > 8000
    severity: info
```

## Troubleshooting

### Metrics Not Appearing
1. Check provider supports metrics in responses
2. Verify profile configuration includes `metrics.extraction` section
3. Enable debug logging to see extraction attempts
4. Ensure response format matches expected structure

### Incorrect Calculations
1. Verify JSONPath expressions match actual response structure
2. Check mathematical expressions for division by zero
3. Ensure units are correctly converted (nanoseconds to milliseconds)

### Performance Impact
1. Monitor extraction timeout occurrences in logs
2. Check for excessive memory usage from large responses
3. Consider disabling for extremely high-throughput scenarios

## Examples

### Comparing Model Performance
```bash
# Request to model A
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -d '{"model": "llama3.2", "prompt": "Hello"}'
  
# Check logs for metrics
# provider_tokens_per_second=45.5

# Request to model B  
curl -X POST http://localhost:40114/olla/ollama/api/generate \
  -d '{"model": "mistral", "prompt": "Hello"}'
  
# Check logs for metrics
# provider_tokens_per_second=38.2
```

### Tracking Token Usage
```bash
# Monitor token consumption across requests
tail -f olla.log | grep "provider_total_tokens"
```

### Performance Dashboard
Use the extracted metrics with monitoring tools:

```python
# prometheus_exporter.py
from prometheus_client import Histogram, Counter
import json
import requests

# Define Prometheus metrics
token_usage = Histogram('olla_token_usage', 'Token usage per request', 
                        ['model', 'endpoint'])
generation_speed = Histogram('olla_tokens_per_second', 'Token generation speed',
                            ['model', 'endpoint'])

def collect_metrics():
    # Get status from Olla
    response = requests.get('http://localhost:40114/internal/status')
    data = response.json()
    
    # Export metrics to Prometheus
    for endpoint, stats in data['proxy']['endpoints'].items():
        if 'avg_tokens_per_second' in stats:
            generation_speed.labels(
                model=stats.get('primary_model', 'unknown'),
                endpoint=endpoint
            ).observe(stats['avg_tokens_per_second'])
```

## Adding Metrics to Custom Profiles

Since provider metrics are part of the profile system, you can easily add metrics extraction to any custom provider profile:

1. **Create your profile** in `config/profiles/your-provider.yaml`
2. **Add the metrics section** following the structure shown above
3. **Define JSONPath expressions** in `paths` to extract values from your provider's response
4. **Add calculations** for any derived metrics using the extracted values
5. **Test with debug logging** to verify metrics are extracted correctly

Example for a custom provider:
```yaml
name: my-custom-llm
metrics:
  extraction:
    enabled: true
    source: "response_body"
    format: "json"
    paths:
      request_id: "$.id"
      tokens_used: "$.usage.tokens"
      time_ms: "$.timing.total_ms"
    calculations:
      tokens_per_second: "tokens_used / (time_ms / 1000)"
```

## Related Documentation

- **[Profile System](profile-system.md)** - Complete guide to the profile system and how metrics fit within it
- [Monitoring Guide](../configuration/practices/monitoring.md) - General monitoring setup and best practices
- [API Reference](../api-reference/overview.md) - Response headers and status endpoint documentation