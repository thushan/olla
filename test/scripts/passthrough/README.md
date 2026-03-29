# Passthrough Behaviour Test Script

Validates that Olla's Anthropic Messages API translator correctly selects **passthrough** mode for backends with native Anthropic support, and falls back to **translation** mode for those without.

## What It Tests

The script auto-discovers available backends and models, then runs a test matrix covering:

- **Non-streaming requests** - Verifies `X-Olla-Mode` header matches expected mode
- **Streaming requests** - Validates SSE event types and Content-Type headers
- **OpenAI baseline** - Confirms the standard proxy path still works alongside translation
- **Edge cases** - Non-existent model (expects 4xx), system parameter, multi-turn conversations
- **Translator stats** - Fetches and displays passthrough/translation rate metrics

## Mode Selection

Backends are categorised by whether they natively support the Anthropic Messages API:

| Mode | Backend Types | Behaviour |
|------|--------------|-----------|
| **Passthrough** | `vllm`, `vllm-mlx`, `lm-studio`, `ollama`, `llamacpp`, `lemonade` | Request forwarded directly in Anthropic format |
| **Translation** | `openai-compatible`, `litellm` | Anthropic -> OpenAI -> backend -> OpenAI -> Anthropic |

## Prerequisites

- Python 3.8+
- Olla running with at least one healthy backend
- Dependencies installed (`pip install -r ../requirements.txt`)

## Usage

```bash
# Run full test suite
python test-passthrough.py

# Custom Olla URL
python test-passthrough.py --url http://localhost:8080

# Skip streaming tests (faster)
python test-passthrough.py --skip-streaming

# Skip edge case tests
python test-passthrough.py --skip-edge-cases

# Show full response bodies
python test-passthrough.py --verbose

# Combine flags
python test-passthrough.py --url http://myhost:40114 --timeout 60 --skip-streaming
```

## CLI Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| `--url` | `http://localhost:40114` | Olla base URL |
| `--timeout` | `30` | Request timeout in seconds |
| `--skip-edge-cases` | off | Skip edge case tests |
| `--skip-streaming` | off | Skip streaming tests |
| `--verbose` | off | Show full response bodies |

## Example Output

```
========================================================================
  Olla Anthropic Passthrough Test
  Validates passthrough vs translation mode selection
========================================================================

Checking Olla availability...
[OK] Olla is reachable

Discovering endpoints...
[OK] Found 2 endpoint(s)
Discovering models...

Configuration Summary
------------------------------------------------------------------------
  Backend              Type               Status     Models Mode
  my-vllm              vllm               healthy    3      passthrough
  my-openai            openai-compatible  healthy    2      translation

Test Matrix
========================================================================

Backend: my-vllm  type=vllm  model=llama3:latest
  Expected mode: passthrough
  Non-streaming: [PASS]
  Streaming:     .....[PASS]  (5 events)
  OpenAI check:  [PASS]

Edge Cases
========================================================================
  Non-existent model: [PASS]  HTTP 404
  System parameter: [PASS]  HTTP 200
  Multi-turn conversation: [PASS]  HTTP 200

Translator Stats
========================================================================
  anthropic
    Requests: 4 total  |  3 passthrough  |  1 translation
    Passthrough rate: 75.00%  |  Streaming: 1  |  Non-streaming: 3

========================================================================
  Results Summary
========================================================================

  Total: 6  |  Passed: 6  |  Failed: 0

  All tests passed.
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All tests passed |
| `1` | One or more tests failed, or Olla unreachable |
| `130` | User interrupted (Ctrl+C) |
