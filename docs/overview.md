# Olla Overview

## What is Olla?

Olla is a smart proxy that sits between your applications and your AI models. Think of it as a traffic controller for LLM requests - it knows which AI servers are available, which ones are healthy, and automatically routes your requests to the best one.

## The Problem Olla Solves

Running local AI models presents unique challenges:

1. **Multiple Endpoints**: You might have Ollama on your workstation, LM Studio on your laptop, and maybe a cloud endpoint for backup
2. **Reliability**: What happens when your primary AI server crashes or becomes overloaded?
3. **Model Discovery**: Different servers have different models - how do you know what's available?
4. **Performance**: How do you avoid overwhelming a single server when you have multiple available?

Olla solves all of these by providing a single endpoint that intelligently manages multiple backends.

## How It Works

```
Your App → Olla → [Workstation Ollama]
                → [Laptop LM Studio]
                → [Cloud API]
```

1. **Single Entry Point**: Your applications connect to Olla (e.g., `http://localhost:40114`)
2. **Automatic Routing**: Olla checks which backends are healthy and have the model you requested
3. **Smart Failover**: If your primary server fails, Olla automatically routes to the next available
4. **Full Transparency**: Response headers tell you exactly which backend handled your request

## Key Benefits

### For Developers
- **One API endpoint** instead of managing multiple
- **Automatic failover** without changing code
- **OpenAI-compatible** - works with existing clients
- **Request tracking** via response headers

### For Operations
- **Health monitoring** of all AI endpoints
- **Load distribution** across available resources
- **Rate limiting** to protect infrastructure
- **Detailed metrics** for capacity planning

### For End Users
- **Better reliability** - requests succeed even if primary server fails
- **Faster responses** - load balancing prevents overload
- **Model availability** - access to all models across all endpoints

## Common Use Cases

### Home Lab Setup
You're running AI models on various machines at home:
- Gaming PC with Ollama (primary)
- Old laptop with LM Studio (backup)
- Olla ensures you can always access your models

### Small Team
Your team shares a few AI servers:
- Olla distributes load fairly
- Rate limiting prevents one user from monopolizing resources
- Everyone gets reliable access

### Development Environment
Testing across different model providers:
- Switch between Ollama, LM Studio, and cloud APIs
- Compare model performance
- No code changes needed

## Response Headers

Every request through Olla includes headers showing:
- Which backend handled it (`X-Olla-Endpoint`)
- What model was used (`X-Olla-Model`)
- How long it took (`X-Olla-Response-Time`)
- Request ID for debugging (`X-Olla-Request-ID`)

This transparency helps you understand and optimise your AI infrastructure.

## Next Steps

- [User Guide](user-guide.md) - Get started with installation and configuration
- [Technical Overview](technical.md) - Deep dive into architecture
- [API Reference](api/) - Detailed endpoint documentation