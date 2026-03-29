# Comprehensive Integration Test Script

Validates all major API endpoints and functionality against a running Olla instance. Gives confidence there are no major regressions before release by auto-discovering backends and models to dynamically build the test matrix.

## What It Tests

The script runs nine phases covering every endpoint category:

| Phase | Area | Tests |
|-------|------|-------|
| 1 | **Health Check** | Olla reachability gate |
| 2 | **Internal/Monitoring** | `/internal/health`, `/internal/status`, endpoints, models, stats, process, `/version` |
| 3 | **Unified Models** | `/olla/models` listing, `/olla/models/{id}` lookup |
| 4 | **Proxy (OpenAI)** | Model list, non-streaming chat, streaming SSE, response headers |
| 5 | **Anthropic Translator** | Model list, non-streaming/streaming messages, token counting, passthrough mode |
| 6 | **Passthrough/Translation** | Mode validation per backend type, SSE event types, response structure, translator stats |
| 7 | **Provider Routes** | Per-discovered-backend `/olla/{provider}/v1/models` |
| 8 | **Response Headers** | Version structure, `X-Olla-Request-ID`, `X-Olla-Response-Time` |
| 9 | **Error Handling** | Non-existent model, invalid body, missing model field |
| 10 | **Summary** | Pass/fail totals grouped by phase |

## Prerequisites

- Python 3.8+
- Olla running with at least one healthy backend
- Dependencies installed (`pip install -r ../requirements.txt`)

## Usage

```bash
# Run full test suite
python test-integration.py

# Custom Olla URL
python test-integration.py --url http://localhost:8080

# Skip streaming tests (faster)
python test-integration.py --skip-streaming

# Skip Anthropic translator tests
python test-integration.py --skip-anthropic

# Skip provider-specific route tests
python test-integration.py --skip-providers

# Show response bodies
python test-integration.py --verbose

# Combine flags
python test-integration.py --url http://myhost:40114 --timeout 60 --skip-streaming
```

## CLI Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| `--url` | `http://localhost:40114` | Olla base URL |
| `--timeout` | `30` | Request timeout in seconds |
| `--skip-streaming` | off | Skip streaming tests |
| `--skip-anthropic` | off | Skip Anthropic translator tests |
| `--skip-providers` | off | Skip provider-specific route tests |
| `--verbose` | off | Show response bodies |

## Example Output

```
========================================================================
  Olla Comprehensive Integration Test
  Validates all major API endpoints and functionality
========================================================================

Phase 1: Health Check
------------------------------------------------------------------------
  [OK] Olla is reachable

Discovering endpoints and models...
  [OK] Found 3 endpoint(s)
  [OK] Found 5 model(s)

Phase 2: Internal/Monitoring Endpoints
------------------------------------------------------------------------
  [PASS] Health endpoint (/internal/health)
  [PASS] Status endpoint (/internal/status)
  [PASS] Endpoints status (/internal/status/endpoints)
  ...

========================================================================
  Results Summary
========================================================================
  8/8  Internal/Monitoring
  2/2  Unified Models
  4/4  Proxy (OpenAI)
  5/5  Anthropic Translator
  3/3  Provider Routes
  2/2  Response Headers
  3/3  Error Handling

  Total: 27  |  Passed: 27  |  Failed: 0

  All tests passed.
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All tests passed |
| `1` | One or more tests failed, or Olla unreachable |
| `130` | User interrupted (Ctrl+C) |
