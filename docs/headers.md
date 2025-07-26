# Olla Response Headers

Olla adds custom response headers to provide transparency about request routing and performance.

## Headers

### X-Olla-Endpoint
The friendly name of the backend endpoint that handled the request.

Example: `X-Olla-Endpoint: ollama-local`

### X-Olla-Model
The actual model used to process the request (if specified).

Example: `X-Olla-Model: llama3.2:3b`

### X-Olla-Backend-Type
The platform type of the backend endpoint.

Values: `ollama`, `openai`, `lmstudio`

Example: `X-Olla-Backend-Type: ollama`

### X-Olla-Request-ID
Unique identifier for the request, useful for correlation in logs.

Example: `X-Olla-Request-ID: req_abc123def456`

### X-Olla-Response-Time
Total response time including streaming (sent as HTTP trailer).

Example: `X-Olla-Response-Time: 1234ms`

### X-Served-By
Standard proxy identification header.

Example: `X-Served-By: olla/ollama-local`

## Usage

These headers help with:
- **Debugging**: Know which backend handled your request
- **Monitoring**: Track performance and routing patterns
- **Load balancing**: Verify requests are distributed correctly
- **Model tracking**: Confirm the correct model was used

## Test Script

The `test-model-routing.sh` script displays all these headers and provides an endpoint usage summary:

```bash
./test/scripts/logic/test-model-routing.sh
```

Output includes:
- Individual request routing details
- Endpoint usage statistics
- Performance metrics
