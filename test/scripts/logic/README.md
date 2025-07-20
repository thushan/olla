# Model Routing Test Scripts

This directory contains scripts for testing Olla's model routing logic.

## test-model-routing.sh

Tests model routing with different models and request types to verify that:
- Requests are routed to endpoints with the appropriate models
- Capability-based routing works correctly (vision, function calling, embeddings, etc.)
- Model discovery and listing works properly

### Features

- **Automatic Model Discovery** - Fetches available models from Olla's `/olla/models` endpoint
- **Dynamic Testing** - Tests only models that are actually available
- **Additional Model Support** - Can test extra models specified as command-line arguments
- **Smart Capability Detection** - Tests appropriate capabilities based on model names

### Usage

```bash
# Test all discovered models
./test-model-routing.sh

# Test discovered models plus additional ones
./test-model-routing.sh gpt-4-turbo claude-3-sonnet

# With custom Olla URL
TARGET_URL=http://localhost:8080 ./test-model-routing.sh

# With verbose output (shows request bodies on failure)
VERBOSE=true ./test-model-routing.sh

# Show help
./test-model-routing.sh --help
```

### What it tests

1. **Model Listing** - Verifies the `/olla/models` endpoint
2. **Simple Chat** - Basic chat completions for all models
3. **Vision Requests** - Image understanding (for vision-capable models)
4. **Function Calling** - Tool/function requests (for capable models)
5. **Code Generation** - Code-specific requests (for code models)
6. **Embeddings** - Embedding generation (for embedding models)

### Models tested

The script tests routing for various model types:
- General chat models (llama3.1, phi4, mistral)
- Vision models (llava, gpt-4-vision)
- Code models (codellama, deepseek-coder)
- Embedding models (nomic-embed-text)
- Multi-capability models (gpt-4, claude-3-opus)

### Output

The script provides:
- Real-time test results with success/failure indicators
- HTTP response codes and response times
- Detailed error messages when routing fails:
  - JSON error messages from the API
  - HTTP status code explanations (404, 502, 503, etc.)
  - Connection timeout notifications
  - Raw response body for non-JSON errors
- Summary statistics with success rate
- Common troubleshooting tips for failures

### Error Reporting

When a request fails, the script shows:
- HTTP status code and response time
- Parsed error message from JSON response
- Helpful interpretation of common HTTP codes:
  - `404` - Model not found or no compatible endpoint
  - `502` - Backend endpoint may be down
  - `503` - All endpoints may be unhealthy
- Raw response body if not JSON (truncated to 200 chars)
- Request body details when `VERBOSE=true`

### Requirements

- `curl` command-line tool
- Olla running and accessible
- At least one endpoint configured with models