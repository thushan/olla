---
title: Model Aliases - Cross-Backend Model Name Mapping
description: Define virtual model names that map to platform-specific model names across Ollama, LM Studio, llamacpp and other backends.
keywords: model aliases, model mapping, model name rewriting, cross-platform models, virtual model names
---

# Model Aliases

> :memo: **Configuration**
> ```yaml
> model_aliases:
>   gpt-oss-120b:
>     - gpt-oss:120b              # Ollama format
>     - gpt-oss-120b-MLX          # LM Studio MLX format
>     - gguf_gpt_oss_120b.gguf    # llamacpp GGUF filename
> ```
> **Key Points**:
>
> - Aliases are defined at the top level of `config.yaml`
> - Each alias maps a single virtual name to one or more actual model names
> - The request body's `"model"` field is automatically rewritten for the selected backend
> - Aliases take priority over standard model routing when both match

## Overview

When running multiple LLM backends (Ollama, LM Studio, llamacpp, vLLM, etc.), the same underlying model often has different names on each platform. For example, `Llama 3.1 8B` might be known as:

- `llama3.1:8b` on Ollama
- `llama-3.1-8b-instruct` on LM Studio
- `Meta-Llama-3.1-8B-Instruct.gguf` on llamacpp

Without aliases, a client request for `llama3.1:8b` would only match the Ollama endpoint — even though the other backends have the exact same model.

**Model aliases** let you define a single virtual model name that maps to all of these variants, so any backend that has the model can serve the request.

## How It Works

When a request arrives with a model name that matches a configured alias:

```text
Client request: "model": "my-llama"
         │
         ▼
┌─────────────────────┐
│  Alias Resolution   │  my-llama → [llama3.1:8b, llama-3.1-8b-instruct]
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Endpoint Discovery │  Find endpoints serving any of those model names
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Load Balancing     │  Select best endpoint (priority, health, etc.)
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Model Rewrite      │  Rewrite "model" field → "llama3.1:8b" (for Ollama)
└─────────┬───────────┘
          │
          ▼
      Backend
```

1. **Alias resolution** — Olla checks whether the requested model name is a configured alias and looks up the list of actual model names.
2. **Endpoint discovery** — For each actual model name, Olla queries the model registry to find endpoints that serve it. This builds an endpoint → actual model name mapping.
3. **Load balancing** — The matched endpoints are filtered through the normal load balancing pipeline (priority, health checks, etc.).
4. **Model rewrite** — Before the request is sent to the selected backend, Olla rewrites the `"model"` field in the JSON request body to the actual model name that backend expects.

## Configuration

Aliases are defined under the `model_aliases` key in `config.yaml`:

```yaml
model_aliases:
  # Alias name → list of actual model names across backends
  my-llama:
    - "llama3.1:8b"                           # Ollama
    - llama-3.1-8b-instruct                   # LM Studio
    - Meta-Llama-3.1-8B-Instruct.gguf         # llamacpp

  my-codegen:
    - "qwen2.5-coder:7b"                      # Ollama
    - qwen2.5-coder-7b-instruct               # LM Studio
```

Clients can then use the alias name in their requests:

```bash
curl http://localhost:40114/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "my-llama",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

Olla will route to whichever backend has one of the listed models and rewrite `"my-llama"` to the correct name for that backend.

## Self-Referencing Aliases

An alias name can also appear in its own list of actual model names. This is useful when the alias name is itself a real model name on one of the backends:

```yaml
model_aliases:
  gpt-oss-120b:
    - "gpt-oss:120b"         # Ollama knows it as gpt-oss:120b
    - gpt-oss-120b            # LM Studio knows it as gpt-oss-120b (same as alias)
```

In this case:

- An Ollama endpoint serving `gpt-oss:120b` will be included, and the request body will be rewritten to `"gpt-oss:120b"`.
- An LM Studio endpoint serving `gpt-oss-120b` will also be included, and the request body keeps `"gpt-oss-120b"` (no unnecessary rewrite since it already matches).

## Alias Priority

When a model name matches both a configured alias **and** a real model known to the registry, the alias takes priority. This ensures consistent cross-backend routing.

If the alias resolves to zero endpoints (none of the actual model names are available), Olla falls back to standard model routing using the alias name as a regular model name.

## Interaction with Other Features

### Model Routing

Alias resolution runs **before** the standard model routing pipeline (strict, optimistic, or discovery modes). Once alias endpoints are resolved, they go through the same load balancing and health filtering as any other request.

### Model Unification

Aliases are separate from model unification. Unification merges model catalogues within a single provider type (e.g. multiple Ollama instances). Aliases map across provider types (e.g. Ollama ↔ LM Studio ↔ llamacpp).

### Proxy Engines

Both the Olla and Sherpa proxy engines support model alias rewriting. The rewrite happens transparently before the request is forwarded to the backend.

## Example Scenario

Consider a home lab with three backends:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://workstation:11434"
        name: "ollama-rtx4090"
        type: "ollama"
        priority: 100
      - url: "http://macbook:1234"
        name: "lmstudio-m2"
        type: "lm-studio"
        priority: 75
      - url: "http://server:8080"
        name: "llamacpp-a100"
        type: "llamacpp"
        priority: 50

model_aliases:
  llama3:
    - "llama3.1:8b"
    - llama-3.1-8b-instruct
    - Meta-Llama-3.1-8B-Instruct.gguf
```

A request for `"model": "llama3"` will:

1. Resolve to all three endpoints (each has the model under a different name)
2. Prefer `ollama-rtx4090` (highest priority)
3. Rewrite the model name to `llama3.1:8b` if routed to Ollama, `llama-3.1-8b-instruct` if routed to LM Studio, etc.
4. Fall back to the next endpoint if the primary is unhealthy

## Troubleshooting

### Alias Not Resolving

**Issue**: Requests with an alias name return 404 or route incorrectly.

**Possible Causes**:

- Actual model names in the alias don't match what backends report
- Model discovery hasn't run yet

**Solutions**:

1. Check discovered models: `curl http://localhost:40114/olla/models`
2. Verify model names match exactly (including tags like `:latest`)
3. Wait for model discovery to complete or trigger a refresh

### Wrong Model Name Sent to Backend

**Issue**: Backend receives the alias name instead of the actual model name.

**Possible Causes**:

- Actual model name in the alias list doesn't exactly match the model name reported by the backend
- Request body is not JSON

**Solutions**:

1. Compare alias model names against discovered models: `curl http://localhost:40114/olla/models`
2. Ensure requests use `Content-Type: application/json`

### Alias Overriding a Real Model

**Issue**: An alias is intercepting requests meant for a real model with the same name.

**This is by design** — aliases always take priority. If you need to reach the real model directly, either:

- Remove the alias, or
- Include the real model name in the alias list (self-referencing) so it stays in the candidate pool
