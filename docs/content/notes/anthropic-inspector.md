---
title: "Anthropic Inspector - Request/Response Debugging"
description: "Debug Anthropic API translation, compare model responses, and troubleshoot request transformations with detailed JSONL logging."
keywords: ["anthropic inspector", "debugging", "model comparison", "request logging", "response analysis", "jsonl"]
---

# Anthropic Inspector

The Anthropic Inspector is a debugging tool that captures and logs complete request/response flows through Olla's 
Anthropic translator. Use it to debug model behaviour, compare different models, understand request transformations
and troubleshoot integration issues.

!!! danger "DEVELOPMENT USE ONLY - DO NOT ENABLE IN PROD"

    Anthropic Inspector is purely for development purposes and is not designed to be used in production.


**Key Features**:

- Complete request/response logging in JSONL format
- Multi-stage inspection (Anthropic to OpenAI to Anthropic)
- Session-based organisation for comparing requests
- Configurable redaction for sensitive data
- Built-in analysis capabilities with `jq`
- Minimal performance impact

## Security Warning

**DO NOT ENABLE IN PRODUCTION**

The inspector logs full request/response bodies including user messages, system prompts, tool definitions, model responses, and potentially sensitive data.

Only enable in development and testing environments where you control all data.

## When to Use

Good use cases:

- Compare Model responses to identical prompts
- Debug Anthropic to OpenAI conversion issues
- Record sessions during feature development
- Track request/response timing per model

Avoid using for:

- Production deployments (security risk, PII data leaks)
- Environments with sensitive user data
- High-throughput systems (adds latency overhead; unoptimised code)

## Configuration

Enable the inspector in your `config.yaml`:

```yaml
translators:
  anthropic:
    enabled: true
    max_message_size: 10485760

    inspector:
      enabled: true
      output_dir: "logs/inspector"
      session_header: "X-Session-ID"
```

## Usage

Include the session header in your requests:

```bash
curl -X POST http://localhost:40114/olla/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "X-Session-ID: debug-qwen-factorial" \
  -d '{"model":"qwen/qwen3-coder","max_tokens":4096,"messages":[...]}'
```

## Output Format

Each session creates a JSONL file with request/response entries:

```jsonl
{"type":"request","ts":"2025-10-21T10:30:00Z","model":"qwen/qwen3-coder","body":{...}}
{"type":"response","ts":"2025-10-21T10:30:03Z","body":{...}}
```

## Analysing Output

Extract all requests:

```bash
cat logs/inspector/2025-10-21/debug-session.jsonl | jq 'select(.type == "request") | .body'
```

Extract all responses:

```bash
cat logs/inspector/2025-10-21/debug-session.jsonl | jq 'select(.type == "response") | .body.content[0].text'
```

Compare responses from different models:

```bash
jq 'select(.type == "response") | .body.content[0].text' logs/inspector/2025-10-21/compare-qwen.jsonl > qwen.txt
jq 'select(.type == "response") | .body.content[0].text' logs/inspector/2025-10-21/compare-glm.jsonl > glm.txt
diff -u qwen.txt glm.txt
```

## Security Considerations

The inspector logs everything. Protect logs:

```bash
chmod 700 logs/inspector/
chown olla:olla logs/inspector/
```

Manual redaction if needed:

```bash
jq 'if .type == "request" then .body.messages = [] else . end' session.jsonl > redacted.jsonl
```

