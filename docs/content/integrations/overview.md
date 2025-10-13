---
title: "Olla Integrations Overview - Supported Backends & Frontends"
description: "Complete overview of Olla's supported integrations. Backend support for Ollama, LM Studio, vLLM, OpenAI compatibility, and frontend integration with OpenWebUI."
keywords: ["olla integrations", "backend support", "ollama integration", "lm studio", "vllm", "openai compatibility", "openwebui"]
---

# Integrations

Olla supports various backends (endpoints) and front-ends integrations powered by Olla [Profiles](../concepts/profile-system.md).

## Backend Endpoints

Olla natively supports the following backends:

| Backend | Type | Description |
|---------|------|-------------|
| [Ollama](./backend/ollama.md) | `ollama` | Native support for [Ollama](https://github.com/ollama/ollama), including model unification |
| [LM Studio](./backend/lmstudio.md) | `lm-studio` | Native support for [LM Studio](https://lmstudio.ai/), including model unification |
| [llama.cpp](./backend/llamacpp.md) | `llamacpp` | Native support for [llama.cpp](https://github.com/ggml-org/llama.cpp) lightweight C++ inference server with GGUF models, including slot management, code infill, and CPU-first design for edge deployment |
| [vLLM](./backend/vllm.md) | `vllm` | Native support for [vLLM](https://github.com/vllm-project/vllm), including model unification |
| [SGLang](./backend/sglang.md) | `sglang` | Native support for [SGLang](https://github.com/sgl-project/sglang) with RadixAttention and Frontend Language, including model unification and vision support |
| [Lemonade SDK](./backend/lemonade.md) | `lemonade` | Native support for [Lemonade SDK](https://lemonade-server.ai/), AMD's local inference solution with Ryzen AI optimisation, including model unification |
| [LiteLLM](./backend/litellm.md) | `litellm` | Native support for [LiteLLM](https://github.com/BerriAI/litellm), providing unified gateway to 100+ LLM providers |
| [OpenAI Compatible](https://platform.openai.com/docs/overview) | `openai` | Generic support for any OpenAI-compatible API |

You can use the `type` in [Endpoint Configurations](/olla/configuration/overview/#endpoint-configuration) when adding new endpoints.

## Frontend Support

### OpenWebUI

Native support for [OpenWebUI](https://github.com/open-webui/open-webui) with Olla via:

* [OpenWebUI with Ollama](./frontend/openwebui.md)
* [OpenWebUI with OpenAI](./frontend/openwebui-openai.md)

## Profiles

[Profiles](../concepts/profile-system.md) provide an easy way to customise the behaviours of existing supported integrations (instead of writing Go code, compiling etc).

* You can customise existing behaviours
    * [Remove prefixes](../concepts/profile-system.md#routing-prefixes) you don't use
    * Add prefixes you would like to use instead    
* You can extend existing functionality
    * Add paths not supported to proxy through
    * Change the [model capability detection](../concepts/profile-system.md#capability-detection) patterns

You can also [create a custom profile](../concepts/profile-system.md#creating-custom-profiles) to add new capabilities or backend support until native support is added.