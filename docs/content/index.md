---
title: Olla - High-Performance LLM Proxy and Load Balancer
description: Olla is a high-performance proxy, model unifier and load balancer for Ollama, LM Studio, vLLM and OpenAI-compatible LLM backends. Unified model catalogues, intelligent routing, and automatic failover.
keywords: llm proxy, ollama proxy, lm studio proxy, vllm proxy, model unification, load balancer, ai infrastructure
---

<div align="center">
  <img src="assets/images/banner.png" alt="Olla - LLM Proxy & Load Balancer" style="max-width: 100%; height: auto;">
  <p>
    <a href="https://github.com/thushan/olla/blob/master/LICENSE"><img src="https://img.shields.io/github/license/thushan/olla" alt="License"></a>
    <a href="https://golang.org/"><img src="https://img.shields.io/github/go-mod/go-version/thushan/olla" alt="Go"></a>
    <a href="https://github.com/thushan/olla/actions/workflows/ci.yml"><img src="https://github.com/thushan/olla/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/thushan/olla"><img src="https://goreportcard.com/badge/github.com/thushan/olla" alt="Go Report Card"></a>
    <a href="https://github.com/thushan/olla/releases/latest"><img src="https://img.shields.io/github/release/thushan/olla" alt="Latest Release"></a> <br />
    <a href="https://ollama.com"><img src="https://img.shields.io/badge/Ollama-native-lightgreen.svg" alt="Ollama: Native Support"></a> 
    <a href="https://lmstudio.ai/"><img src="https://img.shields.io/badge/LM Studio-native-lightgreen.svg" alt="LM Studio: Native Support"></a> 
    <a href="https://github.com/vllm-project/vllm"><img src="https://img.shields.io/badge/vLLM-native-lightgreen.svg" alt="vLLM: Native Support"></a> 
    <a href="https://github.com/lemonade-sdk/lemonade"><img src="https://img.shields.io/badge/Lemonade-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
    <a href="https://github.com/InternLM/lmdeploy"><img src="https://img.shields.io/badge/LM Deploy-openai-lightblue.svg" alt="Lemonade AI: OpenAI Compatible"></a> 
  </P>
</div>

Olla is a high-performance, low-overhead, low-latency proxy, model unifier and load balancer for managing LLM infrastructure. 

It intelligently routes LLM requests across local and remote inference nodes - including [Ollama](https://github.com/ollama/ollama), [LM Studio](https://lmstudio.ai/) and OpenAI-compatible endpoints like [vLLM](https://github.com/vllm-project/vllm). Olla provides model discovery and unified model catalogues within each provider, enabling seamless routing to available models on compatible endpoints.

## Key Features

- **Unified Model Registry**: Unifies models registered across instances (of the same type - Eg. Ollama or LMStudio)
- **Dual Proxy Engines**: Choose between Sherpa (simple, maintainable) and Olla (high-performance with advanced features)
- **Intelligent Load Balancing**: Priority-based, round-robin, and least-connections strategies
- **Health Monitoring**: Circuit breakers and automatic failover
- **High Performance**: Connection pooling, object pooling, and lock-free statistics
- **Security**: Built-in rate limiting and request validation
- **Observability**: Comprehensive metrics and request tracing

## Core Concepts

Understand these key concepts to get the most from Olla:

- **[Proxy Engines](concepts/proxy-engines.md)** - Choose between Sherpa (simple) or Olla (high-performance) engines
- **[Load Balancing](concepts/load-balancing.md)** - Distribute requests across multiple endpoints with priority, round-robin, or least-connections
- **[Model Unification](concepts/model-unification.md)** - Single catalogue of models across all your backends
- **[Health Checking](concepts/health-checking.md)** - Automatic endpoint monitoring and intelligent failover
- **[Profile System](concepts/profile-system.md)** - Customise backend behaviour without writing code

---

## Quick Start

Get up and running with Olla in minutes:

=== "Using Docker"
    ```bash
    # If you have ollama or lmstudio locally
    docker run -t -p 40114:40114 ghcr.io/thushan/olla:latest
    ```

=== "Using Go"

    ```bash
    go install github.com/thushan/olla@latest
    olla
    ```

=== "From Source"

    ```bash
    git clone https://github.com/thushan/olla.git
    cd olla
    make build-release
    ./olla
    ```

## Response Headers

Olla provides detailed response headers for observability:

| Header | Description |
|--------|-------------|
| `X-Olla-Endpoint` | Backend endpoint name |
| `X-Olla-Model` | Model used for the request |
| `X-Olla-Backend-Type` | Backend type (ollama/openai/lmstudio) |
| `X-Olla-Request-ID` | Unique request identifier |
| `X-Olla-Response-Time` | Total processing time |

## Why Olla?

- **Production Ready**: Built for high-throughput production environments
- **Flexible**: Works with any OpenAI-compatible endpoint
- **Observable**: Rich metrics and tracing out of the box
- **Reliable**: Circuit breakers and automatic failover
- **Fast**: Optimised for minimal latency and maximum throughput

## Next Steps

- [Installation Guide](getting-started/installation.md) - Get Olla installed
- [Quick Start](getting-started/quickstart.md) - Basic setup and configuration
- [Architecture Overview](development/architecture.md) - Understand how Olla works
- [Configuration Reference](configuration/reference.md) - Complete configuration options

## Community

- 🐛 [Report Issues](https://github.com/thushan/olla/issues)
- 💡 [Feature Requests](https://github.com/thushan/olla/discussions)
- 📖 [Documentation](https://thushan.github.io/olla/)
- ⭐ [Star on GitHub](https://github.com/thushan/olla)