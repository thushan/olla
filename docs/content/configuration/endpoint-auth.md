---
title: Endpoint Authentication - Configure Auth for Backend Endpoints
description: Configure bearer, API key, and basic authentication for Olla backend endpoints. Includes env interpolation, file-based secrets, Docker/Kubernetes examples, and recipes for vLLM, llama.cpp, and LiteLLM.
keywords: olla auth, endpoint authentication, bearer token, api key, basic auth, vllm auth, llamacpp auth, litellm auth
---

# Endpoint Authentication

Olla can attach outbound authentication headers to requests forwarded to a backend endpoint. This is
for authenticating **Olla to the backend**. It has no bearing on how clients authenticate to Olla.

## When to Use It

Most local inference servers (Ollama, llama.cpp without `--api-key`) run without authentication.
You need `auth:` when:

- A backend is started with an API key flag (e.g. `vllm --api-key`, `llama-server --api-key`)
- A backend sits behind a reverse proxy that requires credentials
- A LiteLLM gateway has a master key configured

## Supported Types

### `bearer`

Sends `Authorization: Bearer <token>`.

```yaml
discovery:
  static:
    endpoints:
      - url: "http://gpu-server:8000"
        name: "vllm-gpu"
        type: "vllm"
        auth:
          type: bearer
          token: "sk-my-secret-token"
```

### `api_key`

Sends a custom header (default `X-Api-Key`). Use `header:` to override. The raw credential
value is written to the header with no scheme prefix -- use `bearer` if the backend expects
`Authorization: Bearer <token>`.

```yaml
      - url: "http://analytics-llm:9000"
        name: "analytics-gw"
        type: "openai-compatible"
        auth:
          type: api_key
          key: "${ANALYTICS_API_KEY}"
          header: "X-Api-Key"   # optional, this is the default
```

### `basic`

Sends `Authorization: Basic <base64(user:pass)>`.

```yaml
      - url: "http://internal-llm:8080"
        name: "llamacpp-basic"
        type: "llamacpp"
        auth:
          type: basic
          username: "admin"
          password: "s3cr3t"
```

## Environment Variable Interpolation

Hardcoding credentials in config files is an antipattern. Use `${VAR}` placeholders instead:

```yaml
auth:
  type: bearer
  token: "${VLLM_API_KEY}"
```

Olla expands these at startup using `ExpandStrict`. **If the variable is unset and has no default,
the process exits with a clear error**. This prevents silent misconfiguration.

### Default Values

Use `${VAR:-default}` for optional credentials or fallback values:

```yaml
auth:
  type: api_key
  key: "${CUSTOM_API_KEY:-changeme}"
```

!!! warning "Defaults in production"
    `:-default` is useful for development. In production, prefer requiring the variable explicitly
    so a missing secret surfaces as a startup failure rather than silently using a fallback.

## File-Based Secrets (`_file` suffix)

Each credential field has a `_file` sibling that reads the value from a file path. This is the
standard pattern for Docker Secrets and Kubernetes mounted secrets, where a volume provides a
file containing a single secret value.

```yaml
auth:
  type: bearer
  token_file: "/run/secrets/vllm_api_key"
```

The file contents are trimmed of leading/trailing whitespace. Setting both the inline field and
the `_file` field is a fatal startup error.

### Available `_file` Fields

| Auth type | Inline field | File field |
|-----------|-------------|------------|
| `bearer` | `token` | `token_file` |
| `api_key` | `key` | `key_file` |
| `basic` | `username` | `username_file` |
| `basic` | `password` | `password_file` |

### Docker Compose Example

```yaml
# docker-compose.yml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    secrets:
      - vllm_api_key
    volumes:
      - ./config.local.yaml:/app/config/config.local.yaml

secrets:
  vllm_api_key:
    file: ./secrets/vllm_api_key.txt
```

```yaml
# config.local.yaml
discovery:
  static:
    endpoints:
      - url: "http://vllm:8000"
        name: "vllm"
        type: "vllm"
        auth:
          type: bearer
          token_file: "/run/secrets/vllm_api_key"
```

### Kubernetes Secret Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: olla-backend-creds
stringData:
  vllm-token: "sk-my-token"
---
# In your Deployment, mount as a volume or env var:
env:
  - name: VLLM_API_KEY
    valueFrom:
      secretKeyRef:
        name: olla-backend-creds
        key: vllm-token
```

Then reference it from config:

```yaml
auth:
  type: bearer
  token: "${VLLM_API_KEY}"
```

## The `headers:` Escape Hatch

For backends that need authentication headers that don't fit bearer/api_key/basic, use the
`headers:` map directly. Headers set here are copied verbatim on every forwarded request.

```yaml
      - url: "http://custom-llm:9000"
        name: "custom"
        type: "openai-compatible"
        headers:
          X-Custom-Auth: "token abc123"
          X-Tenant-ID: "acme"
```

`headers:` and `auth:` can coexist. The `auth:` block sets the `Authorization` (or configured)
header; `headers:` sets everything else.

## Order of Precedence

When a forwarded request is assembled, headers are applied in this order:

1. **Client request headers** are stripped of hop-by-hop headers
2. **`headers:` map** values are set verbatim
3. **`auth:`** sets the credential header (overrides any `headers:` entry for the same name)

The `auth:` block intentionally wins over `headers:` for the credential header. This prevents
an operator from accidentally overriding a resolved secret with a static `headers:` entry.

## Fatal Startup Behaviour

Auth validation runs before the HTTP server starts. The process exits immediately on:

- Unknown `auth.type` (must be `bearer`, `api_key`, or `basic`)
- Both inline field and `_file` sibling set simultaneously
- Neither inline nor `_file` set for a required field
- `${VAR}` placeholder where `VAR` is unset and no `:-default` is provided
- File in `_file` field that does not exist or cannot be read

This fail-fast behaviour is intentional: a proxy that silently starts without credentials and
forwards unauthenticated requests to a protected backend is harder to debug than a startup error.

## Recipes

### vLLM with `--api-key`

Start vLLM:

```bash
vllm serve meta-llama/Llama-3.1-8B-Instruct --api-key sk-my-key
```

Olla config:

```yaml
      - url: "http://vllm-host:8000"
        name: "vllm-gpu"
        type: "vllm"
        auth:
          type: bearer
          token: "${VLLM_API_KEY}"
```

### llama.cpp with `--api-key`

Start llama-server:

```bash
llama-server -m model.gguf --api-key sk-my-key
```

Olla config:

```yaml
      - url: "http://llamacpp-host:8080"
        name: "llamacpp"
        type: "llamacpp"
        auth:
          type: bearer
          token: "${LLAMACPP_API_KEY}"
```

### LiteLLM with Master Key

Start LiteLLM proxy:

```bash
litellm --config litellm_config.yaml --master_key sk-master
```

Olla config:

```yaml
      - url: "http://litellm:4000"
        name: "litellm-gw"
        type: "litellm"
        auth:
          type: bearer
          token: "${LITELLM_MASTER_KEY}"
```

!!! note "LiteLLM API key format"
    LiteLLM accepts the master key as a standard `Authorization: Bearer` header or as `x-goog-api-key`
    depending on the version and configuration. Use `api_key` auth with `header: x-goog-api-key` if
    bearer does not work for your deployment.

## See Also

- [Configuration Reference](reference.md): complete `auth:` field list
- [Security Best Practices](practices/security.md): production hardening
- [Experimental Remote Backends](endpoint-auth-remote.md): cloud API recipes
