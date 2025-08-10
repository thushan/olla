---
title: "Olla Integrations Overview - Supported Backends & Frontends"
description: "Complete overview of Olla's supported integrations. Backend support for Ollama, LM Studio, vLLM, OpenAI compatibility, and frontend integration with OpenWebUI."
keywords: ["olla integrations", "backend support", "ollama integration", "lm studio", "vllm", "openai compatibility", "openwebui"]
---

# Integrations

Olla supports various backends (endpoints) and front-ends integrations powered by Olla [Profiles](../concepts/profile-system.md).

## Backend Endpoints

Olla natively supports:

* [Ollama](./backend/ollama.md) - native support for [Ollama](https://github.com/ollama/ollama), including model unification.
* [LM Studio](./backend/lmstudio.md) - native support for [LM Studio](https://lmstudio.ai/), including model unification.
* [vLLM](./backend/vllm.md) - native support for [vLLM](https://github.com/vllm-project/vllm), including model unification.

Other backends that support OpenAI APIs can be integrated too:

* [OpenAI Compatibility](https://platform.openai.com/docs/overview) - Provides a unified query API across all OpenAI backends.

## Frontend Support

* [OpenWebUI](./frontend/openwebui.md) - native support for [OpenWebUI](https://github.com/open-webui/open-webui).

## Profiles

[Profiles](../concepts/profile-system.md) provide an easy way to customise the behaviours of existing supported integrations (instead of writing Go code, compiling etc).

* You can customise existing behaviours
    * [Remove prefixes](../concepts/profile-system.md#routing-prefixes) you don't use
    * Add prefixes you would like to use instead    
* You can extend existing functionality
    * Add paths not supported to proxy through
    * Change the [model capability detection](../concepts/profile-system.md#capability-detection) patterns

You can also [create a custom profile](../concepts/profile-system.md#creating-custom-profiles) to add new capabilities or backend support until native support is added.