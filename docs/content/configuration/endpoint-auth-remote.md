---
title: Remote Backend Auth (Experimental) - Cloud API Recipes
description: Experimental recipes for using Olla with remote cloud APIs like Ollama Cloud, OpenRouter, and Groq. Understand the limitations and caveats before use.
keywords: olla remote, cloud api, ollama cloud, openrouter, groq, experimental
---

# Remote Backend Auth (Experimental)

!!! warning "Not officially supported"
    Olla is designed for **local, self-hosted inference backends**. Remote cloud APIs are not
    a first-class use case. The recipes below work today for users who want to experiment, but
    we make no guarantees about continued compatibility, and issues specific to cloud providers
    will not be prioritised.

    If you want to use hosted APIs, consider LiteLLM as an intermediary. It handles the
    provider-specific quirks, and Olla then talks to LiteLLM as a local OpenAI-compatible endpoint.

## Why Cloud APIs Are Not First-Class

Cloud inference APIs have operational characteristics that Olla does not currently handle:

- **Rate limit headers** (`x-ratelimit-*`, `retry-after`): Olla does not parse or propagate
  provider-specific rate limit signalling beyond honouring 429 for health state.
- **Path-prefix base URLs**: Some APIs require a base path in the URL
  (e.g. `https://api.openrouter.ai/api/v1`). The `preserve_path: true` flag helps but
  health check probes may still fail.
- **Cold-start latency**: Serverless-backed providers can have high first-token latency that
  exceeds Olla's default health check timeouts.
- **Model namespacing**: Many cloud APIs use `provider/model-name` format. Olla's model
  discovery and unification are tuned for local naming conventions.
- **No local health check**: Cloud APIs do not expose a `/health` endpoint. Health checks
  against `/v1/models` incur real API calls and may consume quota.

## What We Don't Promise

- Health check accuracy for cloud endpoints
- Correct model listing or unification across local and remote endpoints
- Retry or backoff behaviour that respects provider-specific rate limiting
- Compatibility with provider authentication changes

## Recipes

These configurations work at the time of writing. Treat them as starting points, not
production-tested deployments.

### Ollama Cloud

Ollama Cloud (`https://ollama.com`) accepts bearer authentication. Set your API key from
[ollama.com/settings/keys](https://ollama.com/settings/keys).

```yaml
discovery:
  static:
    endpoints:
      - url: "https://ollama.com"
        name: "ollama-cloud"
        type: "ollama"
        priority: 10          # lower than local instances
        check_interval: 60s   # avoid hammering cloud health checks
        check_timeout: 10s
        auth:
          type: bearer
          token: "${OLLAMA_CLOUD_API_KEY}"
```

**Known limitations:**

- The Ollama Cloud API surface may differ from local Ollama. Model names include the namespace
  (e.g. `hf.co/bartowski/Llama-3.2-3B-Instruct-GGUF`).
- Health check hits `/`, which works on the Ollama Cloud base URL.

### OpenRouter

OpenRouter exposes an OpenAI-compatible API at `https://openrouter.ai/api/v1`. The `/api/v1`
prefix path means you need `preserve_path: true` to prevent Olla from stripping it.

```yaml
discovery:
  static:
    endpoints:
      - url: "https://openrouter.ai/api/v1"
        name: "openrouter"
        type: "openai-compatible"
        priority: 10
        preserve_path: true   # required: prevents stripping the /api/v1 prefix
        check_interval: 120s
        check_timeout: 15s
        auth:
          type: bearer
          token: "${OPENROUTER_API_KEY}"
```

**Known limitations:**

- Health checks probe `/v1/models` which incurs an API call. Set `check_interval` high
  to avoid burning quota.
- OpenRouter requires an `HTTP-Referer` header for attribution on some tiers. Use `headers:`
  to set it:

  ```yaml
        headers:
          HTTP-Referer: "https://your-app.example.com"
          X-Title: "Your App Name"
  ```

- Model names include the provider prefix (e.g. `openai/gpt-4o`, `anthropic/claude-3-5-sonnet`).
  These will not unify with local model names.

### Groq

Groq provides a fast OpenAI-compatible inference API.

```yaml
discovery:
  static:
    endpoints:
      - url: "https://api.groq.com/openai/v1"
        name: "groq"
        type: "openai-compatible"
        priority: 10
        preserve_path: true
        check_interval: 120s
        check_timeout: 10s
        auth:
          type: bearer
          token: "${GROQ_API_KEY}"
```

**Known limitations:**

- Same health check cost caveat as OpenRouter.
- Groq's rate limits are aggressive on the free tier. A misconfigured health interval can
  exhaust rate limits before any inference requests are made.

## Mixing Local and Remote

You can combine local and remote endpoints. Set priorities so local endpoints are strongly
preferred and remote endpoints act as overflow:

```yaml
discovery:
  static:
    endpoints:
      # Local, always preferred
      - url: "http://localhost:8000"
        name: "local-vllm"
        type: "vllm"
        priority: 100

      # Remote fallback
      - url: "https://api.groq.com/openai/v1"
        name: "groq-fallback"
        type: "openai-compatible"
        priority: 5
        preserve_path: true
        check_interval: 120s
        auth:
          type: bearer
          token: "${GROQ_API_KEY}"
```

With `load_balancer: priority`, requests only reach the remote endpoint when all local
endpoints are unhealthy.

## Community Contributions

If you build cloud-specific profile YAML files or improve health check behaviour for cloud
APIs, PRs are welcome. See [Contributing](../development/contributing.md).

## See Also

- [Endpoint Authentication](endpoint-auth.md): auth configuration reference
- [Configuration Overview](overview.md): general configuration
- [LiteLLM Integration](../integrations/backend/litellm.md): recommended cloud API gateway
