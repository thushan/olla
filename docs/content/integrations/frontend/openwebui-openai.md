---
title: OpenWebUI + Olla (OpenAI API) Integration
description: Configure OpenWebUI to talk to multiple OpenAI‑compatible backends via Olla’s /olla/openai/v1 proxy. Load‑balancing, failover, streaming, and model unification for vLLM, SGLang, and other OpenAI‑compatible servers.
keywords: OpenWebUI, Olla, OpenAI API, vLLM, SGLang, LM Studio, load balancing, model unification
---

# OpenWebUI Integration with OpenAI

OpenWebUI can speak to any OpenAI‑compatible endpoint. Olla sits in front as a smart proxy, exposing a single **OpenAI API base** that merges multiple backends (e.g. vLLM, SGLang) and handles load‑balancing + failover.

**Set in OpenWebUI:**

```bash
export OPENAI_API_BASE_URL="http://localhost:40114/olla/openai/v1"
```

**What you get via Olla**

* One stable OpenAI base URL for all backends
* Priority/least‑connections load‑balancing and health checks
* Streaming passthrough
* Unified `/v1/models` across providers

## Overview

<table>
    <tr>
        <th>Project</th>
        <td><a href="https://github.com/open-webui/open-webui">github.com/open-webui/open-webui</a></td>
    </tr>
    <tr>
        <th>Integration Type</th>
        <td>Frontend UI</td>
    </tr>
    <tr>
        <th>Connection Method</th>
        <td>Open API Compatibility</td>
    </tr>
    <tr>
        <th>
          Features Supported <br/>
          <small>(via Olla)</small>
        </th>
        <td>
            <ul>
                <li>Chat Interface</li>
                <li>Model Selection</li>
                <li>Streaming Responses</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Configuration</th>
        <td>
            Set <code>OPENAI_API_BASE_URL</code> to Olla OpenAI endpoint <br/>
            ```
            export OPENAI_API_BASE_URL="http://localhost:40114/olla/openai/v1"  
            ```
        </td>
    </tr>
    <tr>
        <th>Example</th>
        <td>
            You can find an example of integration in <code>examples/ollama-openwebui</code> for Ollama as a full example, just remember to change to <code>OPENAI_API_BASE_URL</code>.
        </td>
    </tr>
</table>

## Architecture

```
┌─────────────┐     ┌───────── Olla (40114) ──────┐      ┌─────────────────────┐
│  OpenWebUI  │ ───▶│ /olla/openai/v1 (proxy)    │ ───▶ │ vLLM :8000 (/v1/*)  │
│  (3000)     │     │  • LB + failover            │      └─────────────────────┘
└─────────────┘     │  • health checks            │      ┌─────────────────────┐
                    │  • model unification (/v1)  │ ───▶ │ SGLang :30000 (/v1) │
                    └─────────────────────────────┘      └─────────────────────┘
```



# Quick Start (Docker Compose)

Create **`compose.yaml`**:

```yaml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    container_name: olla
    restart: unless-stopped
    ports:
      - "40114:40114"
    volumes:
      - ./olla.yaml:/app/config.yaml:ro
      - ./logs:/app/logs
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:40114/internal/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

  openwebui:
    image: ghcr.io/open-webui/open-webui:main
    container_name: openwebui
    restart: unless-stopped
    ports:
      - "3000:8080"
    volumes:
      - openwebui_data:/app/backend/data
    environment:
      - OPENAI_API_BASE_URL=http://olla:40114/olla/openai/v1
      - WEBUI_NAME=Olla + OpenWebUI
      - WEBUI_URL=http://localhost:3000
    depends_on:
      olla:
        condition: service_healthy

volumes:
  openwebui_data:
    driver: local
```

Create **`olla.yaml`** (static discovery example):

```yaml
server:
  host: 0.0.0.0
  port: 40114

proxy:
  engine: sherpa           # or: olla (lower overhead), test both
  load_balancer: priority  # or: least-connections

# Service discovery of OpenAI-compatible backends
# (Each backend must expose /v1/*; Olla will translate as needed.)
discovery:
  type: static
  static:
    endpoints:
      - url: http://192.168.1.100:8000
        name: gpu-vllm
        type: vllm
        priority: 100

      - url: http://192.168.1.101:30000
        name: gpu-sglang
        type: sglang
        priority: 50

# Optional timeouts & streaming profile
# proxy:
#   response_timeout: 1800s
#   read_timeout: 600s
#   profile: streaming
```

Bring it up:

```bash
docker compose up -d
```

OpenWebUI: [http://localhost:3000](http://localhost:3000)



# Verifying via cURL

List unified models:

```bash
curl http://localhost:40114/olla/openai/v1/models | jq
```

Simple completion (non‑streaming):

```bash
curl -s http://localhost:40114/olla/openai/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-oss-120b",  
    "messages": [{"role":"user","content":"Hello from Olla"}]
  }' | jq
```

Streaming (SSE):

```bash
curl -N http://localhost:40114/olla/openai/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-oss-120b",
    "stream": true,
    "messages": [{"role":"user","content":"Stream test"}]
  }'
```

Inspect Olla headers (which backend served the call):

```bash
curl -sI http://localhost:40114/internal/status/endpoints | sed -n '1,20p'
# Look for: X-Olla-Endpoint, X-Olla-Backend-Type, X-Olla-Response-Time
```



# OpenWebUI Configuration Notes

* **Env var:** `OPENAI_API_BASE_URL` must point to Olla’s **/olla/openai/v1**.
* **Model picker:** OpenWebUI’s model list is sourced from `/v1/models` (via Olla). If empty, see Troubleshooting.
* **API keys:** If OpenWebUI prompts for an OpenAI key but your backends don’t require one, leave blank.



# Multiple Backends (vLLM, SGLang, LM Studio)

Add as many OpenAI‑compatible servers as you like. Priorities control routing.

```yaml
discovery:
  static:
    endpoints:
      - url: http://vllm-a:8000
        name: vllm-a
        type: vllm
        priority: 100

      - url: http://sglang-b:30000
        name: sglang-b
        type: sglang
        priority: 80

      - url: http://lmstudio-c:1234
        name: lmstudio-c
        type: openai-compatible     # generic OpenAI-compatible server
        priority: 60
```

> Tip: Use `least-connections` when all nodes are similar; use `priority` to prefer local/cheaper nodes.



# Authentication (Front‑door keying via Nginx)

Olla doesn’t issue/validate API keys (yet). To expose Olla publicly, front it with Nginx to enforce simple static keys.

**`/etc/nginx/conf.d/olla.conf`**

```nginx
map $http_authorization $api_key_valid {
    default 0;
    ~*"Bearer (sk-thushan-XXXXXXXX|sk-yolo-YYYYYYYY)" 1;
}

server {
    listen 80;
    server_name ai.example.com;

    location /api/ {
        if ($api_key_valid = 0) { return 401; }
        proxy_pass http://localhost:40114;
        proxy_set_header Host $host;
        proxy_http_version 1.1;
    }
}
```

Then point external users to `http://ai.example.com/api/olla/openai/v1` and give them a matching `Authorization: Bearer ...`.

> For more robust auth (rate limits, per‑key quotas, logs), put an API gateway (Traefik/Envoy/Kong) ahead of Olla.



# Monitoring & Health

**Olla health:**

```bash
curl http://localhost:40114/internal/health
```

**Endpoint status:**

```bash
curl http://localhost:40114/internal/status/endpoints | jq
```

**Unified models:**

```bash
curl http://localhost:40114/olla/openai/v1/models | jq
```

**Logs:**

```bash
docker logs -f olla

docker logs -f openwebui
```



# Troubleshooting

**Models not appearing in OpenWebUI**

1. Olla up?

```bash
curl http://localhost:40114/internal/health
```

2. Backends discovered?

```bash
curl http://localhost:40114/internal/status/endpoints | jq
```

3. Models resolvable?

```bash
curl http://localhost:40114/olla/openai/v1/models | jq
```

4. OpenWebUI sees correct base?

```bash
docker exec openwebui env | grep OPENAI_API_BASE_URL
```

**Connection refused from OpenWebUI → Olla**

* Verify compose service names and ports
* From container: `docker exec openwebui curl -sS olla:40114/internal/health`

**Slow responses**

* Switch to `proxy.engine: olla` or `profile: streaming`
* Use `least-connections` for fairer distribution
* Increase `proxy.response_timeout` for long generations

**Docker networking (Linux)**

* To hit host services: `http://172.17.0.1:<port>`
* Remote nodes: use actual LAN IPs



# Standalone (no compose)

Run Olla locally:

```bash
olla --config ./olla.yaml
```

Run OpenWebUI:

```bash
docker run -d --name openwebui \
  -p 3000:8080 \
  -v openwebui_data:/app/backend/data \
  -e OPENAI_API_BASE_URL=http://host.docker.internal:40114/olla/openai/v1 \
  ghcr.io/open-webui/open-webui:main
```