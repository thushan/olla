# Logic Test Scripts

This directory contains scripts for testing Olla's routing logic, model discovery, and provider-specific endpoints.

## Scripts Overview

| Script | Purpose | Key Features |
|--------|---------|--------------|
| `test-model-routing.sh` | Tests model routing and capability detection | Auto-discovery, capability-based routing |
| `test-provider-routing.sh` | Tests provider-specific routing namespaces | Provider isolation, header validation |
| `test-provider-models.sh` | Tests provider-specific model listing endpoints | Format validation, provider filtering |

## test-model-routing.sh

Tests model routing with different models and request types to verify that requests are routed to endpoints with appropriate models and capabilities.

### Features
- **Automatic Model Discovery** - Fetches available models from Olla's `/olla/models` endpoint
- **Dynamic Testing** - Tests only models that are actually available
- **Capability Detection** - Tests appropriate capabilities based on model names
- **Response Header Tracking** - Shows which endpoint handled each request

### Usage
```bash
# Test all discovered models
./test-model-routing.sh

# Test discovered models plus additional ones
./test-model-routing.sh gpt-4-turbo claude-3-sonnet

# With custom Olla URL
TARGET_URL=http://localhost:8080 ./test-model-routing.sh

# With verbose output
VERBOSE=true ./test-model-routing.sh
```

### What it tests
1. **Simple Chat** - Basic chat completions for all models
2. **Vision Requests** - Image understanding (for vision-capable models)
3. **Function Calling** - Tool/function requests (for capable models)
4. **Code Generation** - Code-specific requests (for code models)
5. **Embeddings** - Embedding generation (for embedding models)

### Response Headers Displayed
- `X-Olla-Endpoint` - The endpoint that handled the request
- `X-Olla-Model` - The model used (if specified in request)
- `X-Olla-Backend-Type` - Backend platform (ollama, openai, lmstudio)
- `X-Olla-Request-ID` - Unique request ID for correlation
- `X-Olla-Response-Time` - Total time including streaming

## test-provider-routing.sh

Tests the provider-specific routing implementation to ensure requests are correctly routed based on provider namespaces.

### Features
- **Provider Namespace Testing** - Validates `/olla/ollama/*`, `/olla/lmstudio/*`, etc.
- **Path Stripping Validation** - Ensures provider prefixes are correctly removed
- **Error Response Testing** - Validates appropriate error codes for invalid routes

### Usage
```bash
# Run all provider routing tests
./test-provider-routing.sh

# With custom Olla URL
TARGET_URL=http://localhost:8080 ./test-provider-routing.sh
```

### What it tests
1. **Valid Provider Routes** - Ollama, LM Studio, OpenAI, vLLM namespaces
2. **Invalid Routes** - Unknown providers, malformed paths
3. **Path Processing** - Correct stripping of provider prefixes

## test-provider-models.sh

Tests the provider-specific model listing endpoints to ensure they return models in the correct format for each provider.

### Features
- **Format Validation** - Verifies each endpoint returns the expected format
- **Provider Filtering** - Ensures only models from the specific provider are returned
- **Multi-Format Support** - Tests providers that support multiple formats

### Usage
```bash
# Run all provider model endpoint tests
./test-provider-models.sh

# With verbose output
VERBOSE=true ./test-provider-models.sh
```

### Endpoints Tested

#### Ollama
- `/olla/ollama/api/tags` - Native Ollama format
- `/olla/ollama/v1/models` - OpenAI-compatible format

#### LM Studio
- `/olla/lmstudio/v1/models` - OpenAI format
- `/olla/lmstudio/api/v1/models` - OpenAI format (alt path)
- `/olla/lmstudio/api/v0/models` - LM Studio enhanced format

#### OpenAI-Compatible & vLLM
- `/olla/openai/v1/models` - OpenAI format
- `/olla/vllm/v1/models` - OpenAI format

#### Unified Models
- `/olla/models` - All providers (unified format)
- `/olla/models?format=unified` - Explicit unified format (default)
- `/olla/models?format=openai` - OpenAI format (all models)
- `/olla/models?format=ollama` - Ollama format (Ollama models only)
- `/olla/models?format=lmstudio` - LM Studio format (LM Studio models only)

## Common Environment Variables

All scripts support these environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `TARGET_URL` | Olla proxy URL | `http://localhost:40114` |
| `VERBOSE` | Show detailed debug output | `false` |

## Error Reporting

The scripts provide detailed error information:
- HTTP status codes with explanations
- Parsed error messages from JSON responses
- Connection timeout notifications
- Raw response bodies for debugging

### Common Error Codes
- `404` - Model/endpoint not found
- `502` - Backend endpoint down
- `503` - All endpoints unhealthy

## Requirements

- `curl` command-line tool
- Olla running and accessible
- At least one configured endpoint

## Running All Tests

To run all logic tests in sequence:

```bash
# Run all tests
for script in test-*.sh; do
    echo "Running $script..."
    ./$script || echo "Failed: $script"
    echo
done
```

## Exit Codes

All scripts follow standard conventions:
- `0` - All tests passed
- `1` - One or more tests failed

This makes them suitable for CI/CD pipelines.