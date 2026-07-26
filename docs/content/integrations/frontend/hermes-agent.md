---
title: Hermes Agent + Olla Integration (Sticky Sessions)
description: Configure Nous Research's Hermes Agent to route through Olla as a custom OpenAI-compatible provider, and use a community plugin to send X-Olla-Session-ID so multi-turn conversations pin to the same backend for KV-cache reuse.
keywords: Hermes Agent, Nous Research, Olla, sticky sessions, X-Olla-Session-ID, custom provider, OpenAI-compatible, KV cache, load balancing, local LLM
---

# Hermes Agent Integration with Olla

[Hermes Agent](https://github.com/NousResearch/hermes-agent) is Nous Research's agent harness. It talks to any OpenAI-compatible endpoint through its custom provider mechanism, so pointing it at Olla is a two-field change.

**Minimal Hermes config** (`~/.hermes/config.yaml`):

```yaml
custom_providers:
  - name: olla
    base_url: http://localhost:40114/olla/openai/v1
    key_env: OLLA_API_KEY
    api_mode: chat_completions   # default; auto-detection is the fallback

model:
  default: qwen3.6:27b
  provider: custom:olla
```

That much gives you load balancing, health checks, failover and a unified model list. The rest of this page covers the part that needs extra work: getting **sticky sessions** to work.

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://github.com/NousResearch/hermes-agent">Hermes Agent</a> (Nous Research)</td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Frontend UI / Terminal Agent Harness</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>Custom provider, OpenAI-compatible (<code>api_mode: chat_completions</code>)</td>
    </tr>
    <tr>
        <th>Olla Endpoint</th>
        <td><code>http://localhost:40114/olla/openai/v1</code></td>
    </tr>
    <tr>
        <th>Sticky Sessions</th>
        <td>Needs a community plugin to send <code>X-Olla-Session-ID</code> (see below)</td>
    </tr>
</table>

## Why this page exists

Olla's [sticky sessions](../../concepts/sticky-sessions.md) pin the turns of a conversation to the same backend so the backend can reuse its KV-cache instead of re-ingesting the whole prompt each turn. The most reliable key source is `session_header`, which reads the `X-Olla-Session-ID` request header.

Hermes can already attach custom headers to a custom provider. Both `model.default_headers` (every request) and a per-provider `extra_headers` map (scoped to one endpoint) are supported natively:

```yaml
model:
  providers:
    olla:
      base_url: http://localhost:40114/olla/openai/v1
      extra_headers:
        X-Client-Name: hermes-agent
```

Those values are **static**, which is the catch. A fixed `X-Olla-Session-ID` would give every conversation the same key, pinning all of your traffic to a single backend permanently. That is worse than leaving the header off, where Olla at least falls back to `prefix_hash`.

What sticky sessions need is a value that changes per conversation, and Hermes has no built-in way to set one. Filling that gap needs plugin middleware.

This recipe was raised in [issue #195](https://github.com/thushan/olla/issues/195), by a user running Olla as the router for a local inference cluster.

## The community plugin

!!! warning "Not maintained by Olla"
    [`shared-goals/hermes-custom-header-plugin`](https://github.com/shared-goals/hermes-custom-header-plugin) is a third-party plugin. It is not written, maintained, reviewed or supported by the Olla project. Read the source before installing it, and raise plugin problems on the plugin's own issue tracker rather than ours.

    Olla implements nothing specific to Hermes. Anything that can send `X-Olla-Session-ID` gets the same behaviour. If Hermes gains native custom-header support, you can opt for that (and we'll update the docs too!).

The plugin registers [`llm_request` middleware](https://github.com/NousResearch/hermes-agent/blob/main/docs/middleware/README.md), a documented Hermes extension point that runs just before each provider call and can rewrite the request. Middleware callbacks receive runtime context including `session_id` and `model`, which is what makes a per-conversation value possible at all.

For provider identities you explicitly name, the plugin adds one header whose value is the current Hermes session ID and the effective model joined with a colon:

```text
20260715_210001_a1b2c3:coder
```

Olla hashes whatever arrives in that header with 64-bit FNV-1a, so the exact format does not matter to Olla. What matters is that the value is stable across the turns of one conversation and differs between conversations, which it is.

Note that Olla already scopes session keys by model internally, so the `:<model>` suffix is redundant from Olla's side. It is harmless.

## Setup

### 1. Enable sticky sessions in Olla

Sticky sessions are opt-in. In your `config.yaml`:

```yaml
proxy:
  engine: olla
  load_balancer: least-connections
  sticky_sessions:
    enabled: true
    idle_ttl_seconds: 600
    max_sessions: 10000
    key_sources:
      - "session_header"      # X-Olla-Session-ID; what the plugin sends
      - "prefix_hash"         # fallback for requests without the header
      - "auth_header"
```

`session_header` must be in `key_sources` and should be first. Confirm the feature is live:

```bash
curl http://localhost:40114/internal/stats/sticky
```

A response of `{"enabled":false}` means the config did not take effect.

### 2. Point Hermes at Olla

In `~/.hermes/config.yaml`, define Olla as a named custom provider. Model selection stays in Hermes as normal; the plugin does not select or rewrite models.

```yaml
custom_providers:
  - name: olla
    base_url: http://localhost:40114/olla/openai/v1
    key_env: OLLA_API_KEY
    api_mode: chat_completions

model:
  default: qwen2.5-coder:7b
  provider: custom:olla
```

The model must be an ID that Olla actually serves:

```bash
curl http://localhost:40114/olla/openai/v1/models | jq '.data[].id'
```

Switch models mid-session with `/model custom:olla:<model-id>`.

Olla does not validate API keys by default, so `key_env` can point at any placeholder value in `~/.hermes/.env`.

### 3. Install and configure the plugin

Check the plugin's own README for its current Hermes version requirement before installing.

```bash
hermes plugins install shared-goals/hermes-custom-header-plugin --enable
```

Then add the plugin entry alongside the provider. This block is the plugin's own schema, not Hermes':

```yaml
plugins:
  entries:
    hermes-custom-header-plugin:
      providers:
        custom:olla:
          header: X-Olla-Session-ID
```

The key is the canonical `custom:<name>` identity, regardless of whether `model.provider` is written as `olla` or `custom:olla`.

### 4. Restart

```bash
hermes gateway restart
hermes gateway status
```

Long-running Hermes processes load plugins and configuration at startup, so a restart is required.

## Verifying it works

Olla reports every affinity decision in response headers:

| Header | Values | Meaning |
|---|---|---|
| `X-Olla-Sticky-Session` | `hit` / `miss` / `repin` / `disabled` | Outcome of the affinity lookup |
| `X-Olla-Sticky-Key-Source` | `session_header` / `prefix_hash` / ... | Which key source was used |
| `X-Olla-Endpoint` | backend name | Which backend served the request |

Run a conversation while your endpoints stay healthy and watch the sequence:

1. First turn of a new session: expect `miss`, then note the `X-Olla-Endpoint` value.
2. Second turn in the same session on the same model: expect `hit` on the same endpoint.
3. Switch model within that session (`/model custom:olla:<other-model>`): expect a `miss`, because the key covers session and model, then hits after it.
4. Start a fresh Hermes session: expect another `miss`.

The counters should move with it:

```bash
curl http://localhost:40114/internal/stats/sticky | jq
```

If `X-Olla-Sticky-Key-Source` reads `prefix_hash` or `auth_header` rather than `session_header`, the header is not reaching Olla.

## Troubleshooting

### Key source is not `session_header`

Work backwards along the request path:

- Confirm the plugin is enabled: `hermes plugins list`.
- Confirm the plugin key matches the provider exactly. `custom:olla` is the identity, `olla` alone will not match.
- Confirm Hermes was restarted after the config change.
- Confirm the request is going through the provider you configured, not a probe or a discovery call. The plugin only covers the main conversation request path.
- Check the Hermes logs. Middleware in Hermes is **fail-open**: if the plugin raises, Hermes logs a warning and continues with the request unchanged. The turn still succeeds, it just arrives at Olla without the header, so a silent loss of affinity is the expected failure mode rather than an error you would notice.

### Header is stripped in transit

Any reverse proxy between Hermes and Olla must forward `X-Olla-Session-ID` untouched. nginx and Traefik pass unknown headers through by default, but header allowlists, header-rewriting middleware and some corporate gateways do not.

### Sticky sessions report `disabled`

No key source produced a value. Check `proxy.sticky_sessions.enabled` is `true` and that `key_sources` is not empty.

### Every turn is a `miss`

Either the session value is changing per request, or the pinned backend is going non-routable and forcing a repin. Check `/internal/status/endpoints` for endpoints flapping between healthy and unhealthy, and compare `X-Olla-Sticky-Session: repin` counts against plain misses.

### Affinity works but responses are not faster

Sticky sessions improve the *chance* of a KV-cache hit; they do not guarantee one. Backend cache eviction, a backend restart, TTL expiry, failover to another endpoint, or a prompt whose prefix changed all defeat cache reuse while affinity itself is working correctly. See [when not to use sticky sessions](../../concepts/sticky-sessions.md#when-not-to-use-sticky-sessions).

## Related documentation

- [Sticky Sessions](../../concepts/sticky-sessions.md): key sources, lifecycle, eviction and configuration reference
- [Load Balancing](../../concepts/load-balancing.md): the strategies sticky sessions wrap
- [Health Checking](../../concepts/health-checking.md): health states and what "routable" means
- [OpenAI API Reference](../../api-reference/openai.md): the endpoint Hermes talks to
- [OpenCode Integration](opencode.md): another OpenAI-compatible harness, with native header support

**External**:

- [Hermes Agent](https://github.com/NousResearch/hermes-agent)
- [hermes-custom-header-plugin](https://github.com/shared-goals/hermes-custom-header-plugin) (third-party)
- [Issue #195](https://github.com/thushan/olla/issues/195): background on why this page exists
