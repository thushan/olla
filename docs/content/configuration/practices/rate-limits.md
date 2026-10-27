---
title: "Olla Rate Limit Best Practices - Sizing for Claude Code and Agents"
description: "How to size Olla's rate limits for Claude Code, parallel agents and shared networks. Typical request rates, what a 429 means, when to raise per-IP versus global limits, and how to check you are hitting them."
keywords: ["olla rate limits", "claude code rate limit", "429 too many requests", "per ip rate limit", "global rate limit", "anthropic rate_limit_error"]
---

# Rate Limit Best Practices

Olla limits requests per client IP and across all clients. The limits apply to the proxy routes and to the
translator routes, `/olla/anthropic/v1/messages`, `/olla/anthropic/v1/messages/count_tokens` and
`/olla/anthropic/v1/models`. Status, health and stats endpoints are not limited. The settings are in
[Rate Limits](../reference.md#rate-limiting).

## How Claude Code behaves

Measured against Olla with a real Ollama backend (Claude Code 2.1.302, headless, all sessions from one IP, limits open):

| Case | Requests | Peak per 1s | Peak per 10s | Peak per 60s |
|------|----------|-------------|--------------|--------------|
| One session, a normal coding task | 2 to 3 | 2 | 2 | 3 |
| Three parallel sessions (stand-ins for parallel subagents) | 11 | 4 | 4 | 10 |
| Four parallel sessions starting together | 5 | 4 | 5 | 6 |

- Claude Code sent only `POST /olla/anthropic/v1/messages`. Each model turn, including a retry, is one request.
  The headless runs made no `count_tokens` or `models` calls; allow a few per interactive session.
- Parallel sessions start together, so the burst matters more than the sustained rate: four requests arrived in the first second.
- Each turn carries a large prompt, so on local hardware a session issues a request every 10 to 30 seconds.
  A fast backend answers sooner and a session loops faster. Treat these numbers as a floor, and expect a single
  session on fast hardware to approach 30 requests a minute.

The defaults (100 per IP per minute, burst 50, 1000 global) sit about nine times above the measured three-session peak
per minute and twelve times above its burst, so a single developer or a few parallel agents never notice them.

## What a 429 means

Olla answers a limited request with `429` and a `Retry-After` header in seconds. On the Anthropic routes the body is
Anthropic's error shape, which Claude Code recognises and retries with backoff:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"Too Many Requests"}}
```

Occasional 429s that clear on retry are the limiter working. Sustained 429s mean the limit is too low for your traffic.

## Per-IP or global

- **Raise `per_ip_requests_per_minute`** (and usually `burst_size`) when many requests come from one address: several
  sessions or agents on one machine, a team behind one NAT or VPN exit, CI runners sharing an egress IP, or a reverse
  proxy that hides client addresses. For the proxy case see `trust_proxy_headers` so each client is counted separately.
- **Raise `global_requests_per_minute`** when the total across all clients is what you hit, for example many users
  each within their own limit. Set it from what your backends can serve, not from client behaviour.

```yaml
server:
  rate_limits:
    per_ip_requests_per_minute: 600   # a team behind one NAT, or many parallel agents
    burst_size: 100                   # parallel sessions start together
    global_requests_per_minute: 6000
```

## Turning limiting off

Set both limits to `0` to disable rate limiting entirely. Do this only on a trusted network. The size limits are separate
(`server.request_limits`) and still apply.

```yaml
server:
  rate_limits:
    global_requests_per_minute: 0
    per_ip_requests_per_minute: 0
```

## Checking whether you are hitting limits

- `/internal/status` reports `security.violations.rate_limits` (a running count) and `security.blocked_ips`.
  The Prometheus metric is `olla_security_rate_limit_violations_total`.
- The log carries a `Rate limit exceeded` warning with `client_ip`, `path`, `limit` and `retry_after`. It is sampled per
  client, so a flood produces a summary rather than a line per request.
- The `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `Retry-After` response headers show the limit a client was held to.
