---
title: "Provider Metrics - Real-time LLM Performance Monitoring"
description: "How Olla captures and exposes real-time metrics from LLM providers including token usage, generation speed, and latency measurements."
keywords: ["provider metrics", "llm metrics", "token usage", "performance monitoring", "ollama metrics", "model performance"]
---

# Provider Metrics

Olla automatically extracts and exposes detailed performance metrics from LLM provider responses, giving you real-time insights into model performance, token usage, and processing times.

## Overview

Provider metrics extraction is a best-effort feature that captures performance data from the final response chunks of LLM providers. This data includes:

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

### Profile Configuration
Each provider profile can define how to extract metrics:

```yaml
# config/profiles/ollama.yaml
profile:
  name: ollama
  type: inference
  provider: ollama
  
  metrics_extraction:
    # JSONPath expressions for extracting metrics
    response_paths:
      total_duration: "$.total_duration"
      load_duration: "$.load_duration"
      prompt_eval_count: "$.prompt_eval_count"
      prompt_eval_duration: "$.prompt_eval_duration"
      eval_count: "$.eval_count"
      eval_duration: "$.eval_duration"
      model: "$.model"
      done_reason: "$.done_reason"
    
    # Mathematical transformations
    calculations:
      total_duration_ms: "total_duration / 1000000"
      load_duration_ms: "load_duration / 1000000"
      prompt_eval_duration_ms: "prompt_eval_duration / 1000000"
      eval_duration_ms: "eval_duration / 1000000"
      tokens_per_second: "eval_count / (eval_duration / 1000000000)"
      prompt_tokens_per_second: "prompt_eval_count / (prompt_eval_duration / 1000000000)"
      time_per_token_ms: "(eval_duration / 1000000) / eval_count"
```

### OpenAI Profile Example
```yaml
# config/profiles/openai.yaml
profile:
  name: openai
  type: inference
  provider: openai
  
  metrics_extraction:
    response_paths:
      prompt_tokens: "$.usage.prompt_tokens"
      completion_tokens: "$.usage.completion_tokens"
      total_tokens: "$.usage.total_tokens"
      model: "$.model"
      finish_reason: "$.choices[0].finish_reason"
```

### LM Studio Profile Example
```yaml
# config/profiles/lmstudio.yaml
profile:
  name: lmstudio
  type: inference
  provider: lmstudio
  
  metrics_extraction:
    response_paths:
      prompt_tokens: "$.usage.prompt_tokens"
      completion_tokens: "$.usage.completion_tokens"
      total_tokens: "$.usage.total_tokens"
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

### Extraction Overhead
- Metrics extraction runs with a 10ms timeout to prevent blocking
- Extraction is best-effort - failures don't affect request processing
- Cached JSONPath expressions for efficient repeated extractions
- Zero-allocation design for high-throughput scenarios

### Memory Usage
- Last chunk buffering: Only the final response chunk is kept for extraction
- Typical overhead: < 10KB per request
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
2. Verify profile configuration includes `metrics_extraction`
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

## Related Documentation

- [Monitoring Guide](../configuration/practices/monitoring.md) - General monitoring setup
- [Profile System](profile-system.md) - Profile configuration details
- [API Reference](../api-reference/overview.md) - Response headers documentation