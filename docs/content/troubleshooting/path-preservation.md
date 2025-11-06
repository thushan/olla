---
title: Path Preservation Troubleshooting - Olla Proxy
description: Troubleshooting guide for path preservation issues with endpoint base paths in Olla. Diagnose and resolve URL routing problems.
keywords: olla troubleshooting, path preservation, preserve_path, base path, URL routing, endpoint paths, docker model runner
---

# Path Preservation Troubleshooting

This guide helps diagnose and resolve issues related to path preservation when proxying requests through Olla.

## Understanding Path Preservation

Path preservation controls how Olla handles base paths in endpoint URLs when forwarding requests.

### Default Behaviour (preserve_path: false)

By default, Olla **replaces** the base path from the endpoint URL:

```
Endpoint URL: http://localhost:8080/api/v1
Client request: POST /v1/chat/completions
Proxied to: http://localhost:8080/v1/chat/completions
```

The base path `/api/v1` is stripped and replaced with the request path.

### Path Preservation (preserve_path: true)

With path preservation enabled, Olla **appends** the request path to the full endpoint URL:

```
Endpoint URL: http://localhost:8080/api/v1
Client request: POST /v1/chat/completions
Proxied to: http://localhost:8080/api/v1/v1/chat/completions
```

The base path `/api/v1` is preserved, and the request path is appended.

## Common Issues and Solutions

### Issue 1: 404 Not Found Errors

**Symptoms:**

- Requests return 404 errors
- Endpoint is healthy but can't find the requested resource
- Works directly against the endpoint but not through Olla

**Diagnosis:**

Check if the endpoint has a base path that needs preservation:

```bash
# Test directly against endpoint
curl http://endpoint:8080/api/v1/models

# Test through Olla (might fail)
curl http://localhost:40114/v1/models
```

**Solution:**

Enable path preservation for the endpoint:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://endpoint:8080/api/v1"
        name: "api-endpoint"
        type: "openai"
        preserve_path: true  # Enable path preservation
```

### Issue 2: Duplicate Path Segments

**Symptoms:**

- URLs have duplicate path segments (e.g., `/v1/v1/chat/completions`)
- Requests fail with 404 or invalid path errors
- Path appears twice in the final URL

**Diagnosis:**

Check if path preservation is incorrectly enabled:

```bash
# Enable debug logging to see the actual request
OLLA_LOGGING_LEVEL=debug olla

# Look for log entries showing the proxied URL
```

**Solution:**

Disable path preservation if the endpoint doesn't have a base path:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://endpoint:8080"  # No base path
        name: "simple-endpoint"
        type: "openai"
        preserve_path: false  # Default - no preservation needed
```

### Issue 3: Docker Model Runner Not Working

**Symptoms:**

- Docker Model Runner endpoints return errors
- Models aren't accessible through Olla
- Health checks pass but requests fail

**Diagnosis:**

Docker Model Runner typically uses base paths like `/api/models/{model_name}`:

```bash
# Docker Model Runner structure
http://docker-runner:8080/api/models/llama/v1/chat/completions
```

**Solution:**

Enable path preservation for Docker Model Runner:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://docker-runner:8080/api/models/llama"
        name: "docker-llama"
        type: "openai"
        preserve_path: true  # Required for Docker Model Runner
        health_check_url: "/api/models/llama/health"
```

### Issue 4: API Gateway Path Routing

**Symptoms:**

- Requests fail when endpoint is behind an API gateway
- Gateway returns routing errors
- Direct endpoint access works but gateway access fails

**Diagnosis:**

API gateways often use path-based routing:

```
Gateway: https://api.company.com/llm/prod/v1
Routes to: http://internal-llm:8080/v1
```

**Solution:**

Enable path preservation for gateway endpoints:

```yaml
discovery:
  static:
    endpoints:
      - url: "https://api.company.com/llm/prod/v1"
        name: "gateway-prod"
        type: "openai"
        preserve_path: true  # Keep gateway routing path
```

## Debugging Path Issues

### Step 1: Enable Debug Logging

```yaml
logging:
  level: "debug"
  format: "text"  # Easier to read for debugging
```

Or via environment variable:

```bash
OLLA_LOGGING_LEVEL=debug olla
```

### Step 2: Enable Request Logging

```yaml
server:
  request_logging: true  # Log all HTTP requests
```

### Step 3: Check Response Headers

Olla adds diagnostic headers to responses:

```bash
curl -v http://localhost:40114/v1/chat/completions

# Look for headers:
# X-Olla-Endpoint: endpoint-name
# X-Olla-Backend-Type: openai
# X-Olla-Request-ID: unique-id
```

### Step 4: Test Direct vs Proxied

Compare direct endpoint access with proxied access:

```bash
# Direct to endpoint
curl -X POST http://endpoint:8080/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3","messages":[{"role":"user","content":"test"}]}'

# Through Olla
curl -X POST http://localhost:40114/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3","messages":[{"role":"user","content":"test"}]}'
```

## Testing Path Preservation

### Test Configuration

Create a test configuration to verify path behaviour:

```yaml
# test-paths.yaml
server:
  host: "localhost"
  port: 40114
  request_logging: true

proxy:
  engine: "sherpa"

discovery:
  static:
    endpoints:
      # Test WITH preservation
      - url: "http://localhost:8081/base/path"
        name: "with-preservation"
        type: "openai"
        preserve_path: true
        priority: 100

      # Test WITHOUT preservation
      - url: "http://localhost:8082/base/path"
        name: "without-preservation"
        type: "openai"
        preserve_path: false
        priority: 50

logging:
  level: "debug"
```

### Test Script

```bash
#!/bin/bash
# test-path-preservation.sh

echo "Testing path preservation..."

# Start mock servers (using nc or python)
# Server 1: expects /base/path/v1/chat/completions
python3 -m http.server 8081 &
SERVER1_PID=$!

# Server 2: expects /v1/chat/completions
python3 -m http.server 8082 &
SERVER2_PID=$!

# Start Olla
olla -c test-paths.yaml &
OLLA_PID=$!

sleep 2

# Test requests
echo "Testing WITH preservation (should see /base/path/v1/...):"
curl -v http://localhost:40114/v1/chat/completions 2>&1 | grep "GET\|POST"

echo "Testing WITHOUT preservation (should see /v1/...):"
curl -v http://localhost:40114/v1/chat/completions 2>&1 | grep "GET\|POST"

# Cleanup
kill $SERVER1_PID $SERVER2_PID $OLLA_PID
```

## Decision Tree

Use this decision tree to determine if you need path preservation:

```
Does your endpoint URL have a base path after the host/port?
├─ No (e.g., http://localhost:8080)
│  └─ preserve_path: false (default)
│
└─ Yes (e.g., http://localhost:8080/api/v1)
   │
   └─ Is the base path required for routing?
      ├─ No (legacy/optional path)
      │  └─ preserve_path: false
      │
      └─ Yes (required for routing)
         │
         └─ Is it Docker Model Runner or similar?
            ├─ Yes
            │  └─ preserve_path: true
            │
            └─ Is it behind an API gateway?
               ├─ Yes
               │  └─ preserve_path: true
               │
               └─ Is it a microservice with path routing?
                  ├─ Yes
                  │  └─ preserve_path: true
                  │
                  └─ No
                     └─ preserve_path: false
```

## Best Practices

### 1. Document Path Requirements

Always document why path preservation is enabled:

```yaml
endpoints:
  - url: "http://service:8080/api/models/llama"
    name: "docker-llama"
    type: "openai"
    preserve_path: true  # Required: Docker Model Runner uses path-based model routing
```

### 2. Test Both Configurations

When unsure, test both configurations:

```yaml
# Try without preservation first (default)
preserve_path: false

# If requests fail with 404, try with preservation
preserve_path: true
```

### 3. Use Health Checks

Configure health checks to match the path structure:

```yaml
endpoints:
  - url: "http://service:8080/api/v1"
    preserve_path: true
    health_check_url: "/api/v1/health"  # Match the base path
```

### 4. Group Similar Endpoints

Group endpoints with similar path requirements:

```yaml
endpoints:
  # All Docker Model Runner endpoints
  - url: "http://docker:8080/api/models/llama"
    preserve_path: true
  - url: "http://docker:8080/api/models/mistral"
    preserve_path: true

  # All standard endpoints
  - url: "http://ollama1:11434"
    preserve_path: false
  - url: "http://ollama2:11434"
    preserve_path: false
```

## Getting Help

If you're still experiencing issues:

1. **Check Logs**: Enable debug logging and request logging
2. **Test Directly**: Verify the endpoint works without Olla
3. **Review Configuration**: Double-check the `preserve_path` setting
4. **Open an Issue**: Include configuration, logs, and example requests

### Information to Include

When reporting path preservation issues:

```yaml
# Your endpoint configuration
endpoints:
  - url: "YOUR_ENDPOINT_URL"
    preserve_path: true/false

# Example request that fails
curl YOUR_REQUEST

# Error message or response
HTTP/1.1 404 Not Found

# Debug logs showing the proxied URL
[DEBUG] Proxying to: http://...
```

## Related Documentation

- [Configuration Reference](../configuration/reference.md#static-endpoints) - Endpoint configuration options
- [Configuration Examples](../configuration/examples.md#path-preservation-examples) - Path preservation examples
- [Concepts: Proxy Engines](../concepts/proxy-engines.md) - How requests are proxied