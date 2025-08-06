# Streaming Behavior Test Scripts

These scripts test Olla's streaming behavior for LLM responses, ensuring tokens are delivered in real-time rather than using standard HTTP delivery.

## Model Selection

All scripts now auto-select phi models (phi4:latest, phi3.5:latest, or phi3:latest) by default for consistent testing. Use the `--select-model` or `--select-models` flag to manually choose a different model.

## Overview

Olla supports three proxy profiles:
- **`auto`** (default) - Automatically detects based on content type
- **`streaming`** - Always streams responses
- **`standard`** - Normal HTTP delivery without forced flushing

These scripts validate that the proxy correctly streams text responses and buffers binary content.

## Scripts

### test-streaming-detection.py
Detects whether Olla is streaming or buffering responses by analyzing chunk timing patterns.

**Features:**
- Auto-selects phi4:latest model if available
- Tests multiple scenarios (long responses, quick responses, stream:false, images)
- Provides visual feedback with colored dots for each chunk
- Comprehensive analysis of streaming behavior

**Usage:**
```bash
# Quick test
python test-streaming-detection.py --quick

# Comprehensive test (default)
python test-streaming-detection.py

# Test with specific model
python test-streaming-detection.py --model llama2:latest

# Custom timeout
python test-streaming-detection.py --timeout 60
```

**What to expect:**
- For text with `stream: true`: Should show STREAMING mode
- For text with `stream: false`: Should show STANDARD mode
- For images/binary content: Should show STANDARD mode

### test-streaming-latency.py
Measures streaming latency and quality by analyzing chunk arrival patterns.

**Features:**
- Simulates real-world usage with various prompts
- Measures time to first token (TTFT)
- Analyzes inter-chunk delays
- Classifies streaming quality (smooth, choppy, batched)
- Loads questions from customizable file

**Usage:**
```bash
# Test with default 5 questions (auto-selects phi model)
python test-streaming-latency.py

# Force manual model selection
python test-streaming-latency.py --select-model

# Test with specific model
python test-streaming-latency.py --model llama2:latest

# Test with more questions
python test-streaming-latency.py --count 10

# Use custom questions file
python test-streaming-latency.py --questions my-questions.txt
```

**Quality Classifications:**
- **Smooth**: Regular chunks with <200ms average delay
- **Choppy**: Irregular chunks or >200ms average delay
- **Batched**: Very fast chunks (<10ms) suggesting buffering
- **Single chunk**: Only one chunk received (definitely standard mode)

### test-streaming-responses.py
Validates streaming across different API formats (OpenAI, Ollama, LM Studio).

**Features:**
- Tests multiple provider endpoints
- Measures tokens per second
- Detects batched vs true streaming
- Analyzes chunk arrival patterns
- Supports sampling mode for quick tests

**Usage:**
```bash
# Test all models and providers
python test-streaming-responses.py

# Test specific providers
python test-streaming-responses.py --providers openai ollama

# Quick sample test
python test-streaming-responses.py --sample

# Test specific models
python test-streaming-responses.py --models phi4:latest llama2:latest

# Show detailed analysis
python test-streaming-responses.py --analyze
```

## Understanding Results

### Visual Indicators
- 🟢 Green dot: First chunk received
- 🔵 Cyan dots: Subsequent chunks
- ✅ Green checkmark: Test passed
- ❌ Red X: Test failed
- ⚠️ Yellow warning: Potential issue

### Key Metrics

**Time to First Token (TTFT)**:
- < 1s: Excellent
- 1-3s: Good
- 3-5s: Acceptable (model may be loading)
- > 5s: Poor (likely buffering)

**Inter-chunk Delays**:
- < 50ms: Smooth streaming
- 50-200ms: Acceptable streaming
- 200ms-1s: Choppy streaming
- > 1s: Likely buffering between chunks

**Chunk Count**:
- 1-2 chunks: Definitely standard mode
- 3-10 chunks: Possibly standard mode or very short response
- 10+ chunks: Likely streaming

## Configuration

### Proxy Profile Configuration
Set in your Olla config.yaml:
```yaml
proxy:
  profile: auto  # or 'streaming' or 'standard'
  stream_buffer_size: 8192  # Affects chunk granularity
```

### Stream Buffer Size Impact
- Smaller (4KB): More frequent flushes, lower latency
- Default (8KB): Balanced performance
- Larger (64KB): Better throughput, higher latency

## Troubleshooting

### "All tests show standard mode"
1. Check proxy profile setting in config
2. Verify middleware implements http.Flusher
3. Ensure backend (Ollama) is actually streaming

### "Choppy streaming detected"
1. May indicate network latency
2. Backend might be slow generating tokens
3. Try reducing stream_buffer_size

### "Circuit breaker open"
1. Backend endpoint is failing
2. Wait for circuit breaker timeout (30s)
3. Check backend health

## Expected Behavior

### Text Generation (stream: true)
- Should see multiple chunks arriving over time
- Green dot followed by cyan dots
- STREAMING mode detected

### Text Generation (stream: false)
- Single response after full generation
- BUFFERED mode detected (this is correct)

### Image Generation
- Always standard mode regardless of stream setting
- BUFFERED mode detected (this is correct)

## Integration with CI/CD

These scripts exit with proper codes for CI integration:
- 0: All tests passed
- 1: Some tests failed
- 130: User interrupted

Example GitHub Actions usage:
```yaml
- name: Test Streaming Behavior
  run: |
    python test/scripts/streaming/test-streaming-detection.py
    python test/scripts/streaming/test-streaming-latency.py --count 3
```