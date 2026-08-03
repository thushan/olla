---
title: Admin Dashboard - Read-Only Operator Overview
description: The embedded read-only admin dashboard for Olla. Enable and disable it, widen access for LAN or Docker, and the security model you must understand before exposing it.
keywords: olla dashboard, admin ui, fleet overview, dashboard security, access_policy, allowed_cidrs, allowed_hosts, gate_internal_api
---

# Admin Dashboard

The admin dashboard is a read-only, single-page overview of an Olla fleet, served at
`GET /internal/ui/` on the same HTTP listener as the proxy. The static assets are embedded
into the `olla` binary at build time, so there is no separate web server to run, no port
to open, and no Node toolchain required to serve it at runtime.

It exists as a glanceable alternative to `curl`-ing `/internal/status*` and reading JSON
when you want to answer questions like "is my fleet healthy", "which backend is slow", and
"which models are loaded where" without SSH-ing in.

## What it is

- **Read-only.** No config editing, no endpoint enable or disable, no restart, no cache
  clearing. If a view changes state, it is not in the dashboard.
- **Polled, not realtime.** The page fetches the same `/internal/status*` JSON the CLI
  exposes. There is no WebSocket, no Server-Sent Events channel, no push. Each request is a
  conditional `GET` using `ETag`/`If-None-Match`, so an unchanged snapshot returns a `304`
  with no body; the status endpoints also gzip their responses when the client accepts it.
- **Same listener as the proxy.** It shares `server.host` and `server.port`. There is no
  second port to configure and no separate TLS posture.
- **Three panels: Overview, Endpoints, Models.** Aggregate fleet health, per-endpoint
  status and latency, and the discovered model inventory grouped by family.

## What it is not

- **No authentication.** No login, no token, no session. This is consistent with every
  other `/internal/*` endpoint in Olla today. See [Security model](#security-model) for
  what that means in practice.
- **No circuit-breaker column.** The breaker trips only on health-probe failures, not on
  live proxy traffic, so exposing it in the dashboard would under-report real failures.
- **No per-model traffic figures.** The proxy engines do not currently record per-model
  request counts, so any such column would read as always zero. The Models panel shows
  discovery data only, plus any configured aliases.
- **No mutation surface, no log streaming, no chat playground, no benchmark UI.** Those
  are separate efforts.

## Reaching it

With the shipped default config, start Olla and open:

```
http://localhost:40114/internal/ui/
```

The trailing slash matters - the dashboard is a single-page app mounted at that exact
prefix, and its own assets are requested relative to it. `localhost` works because it is
in the default `access_policy.allowed_hosts`; `http://127.0.0.1:40114/internal/ui/` also
works without any config change, because an IP-literal Host is always accepted (see
[Security model](#security-model)). A hostname you have not added to `allowed_hosts` -
even one that resolves to an allowed IP - is rejected.

## The three panels

| Panel | What it shows | Backed by | Poll interval |
|-------|----------------|-----------|----------------|
| Overview | Aggregate fleet status, success rate, average latency, total traffic, active connections, total requests, total failures, security violations, uptime, proxy engine/balancer in use, and a live requests-per-second sparkline | `GET /internal/status` | 5s |
| Endpoints | Per-endpoint name, type, status, priority, success rate, average/min/max proxy latency, request count, active connections, model count, last and next health check, last model sync, sanitised URL | `GET /internal/status/endpoints` | 5s |
| Models | Discovered model inventory grouped by family: name, aliases, parameter size, quantisation, size on disk, which endpoints host each model, last seen | `GET /internal/status/models` | 15s |

Clicking a model's host in the Models panel jumps to and highlights that endpoint's row in
the Endpoints panel. The active panel, and any endpoint jumped to, is reflected in the URL
hash, so a refresh or a shared link restores the same view. The dashboard follows the
browser's light/dark preference by default, with a toggle to force light, dark, or back to
automatic.

The Endpoints panel does **not** show a circuit-breaker column, and the Models panel does
**not** show per-model request, success-rate, or latency columns. Both are deliberate
omissions; see [What it is not](#what-it-is-not).

The dashboard reads a handful of additive fields the status handlers did not previously
expose (`start_time` for live uptime, per-endpoint `active_connections`, min/max/average
proxy latency as machine-readable numbers, absolute RFC3339 timestamps alongside the
existing relative ones, and a sanitised `url`). No existing field on those responses was
renamed, retyped, or removed.

Every panel polls through one shared scheduler, not an independent timer per panel:

- Each interval is jittered so multiple open dashboard tabs don't all poll in lockstep.
- When the browser tab is hidden, polling backs off, then fires an immediate refresh the
  moment the tab becomes visible again.
- A panel is flagged stale in the UI if its last successful poll is older than a multiple
  of its configured interval, so a network hiccup is visible rather than silently showing
  old data.

### Browser support

The frontend is built with Vite and Svelte 5 targeting current evergreen browsers. It has
not been tested against older or non-evergreen browsers - if you need to support one,
treat that as unverified territory rather than assuming it works.

### Building the frontend

The SPA is built from `web/dashboard/` and emitted into
`internal/app/handlers/dashboard/dist/`, which is embedded via `go:embed`. Building the
SPA requires Bun 1.1+:

```bash
make install-web   # bun install --frozen-lockfile
make build-web     # bun run build, then populate the embed directory
```

`make build`, `make run`, and the release targets run `build-web` automatically. A plain
`go build ./...`, `make ready`, or `make test` does **not** require Bun: a committed
`.gitkeep` sentinel keeps the embed non-empty, and a binary built without the SPA step
serves an explanatory `503` at `/internal/ui/` rather than failing to compile.

## Enable and disable

The dashboard is controlled by the top-level `dashboard` config section. The shipped
default enables it with a loopback-only access policy.

```yaml
dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "127.0.0.0/8"
      - "::1/128"
    allowed_hosts:
      - "localhost"
  gate_internal_api: false
```

`enabled: false` is a genuine off switch. When false, no `/internal/ui/*` routes are
registered on the mux at all - the binary still contains the embedded assets, but they
are never served, and a request to `/internal/ui/` returns the default-mux `404`. The
mount point is not discoverable by a scanner when disabled.

```yaml
dashboard:
  enabled: false
```

A binary built without `make build-web` having run still starts and serves the proxy
normally; `/internal/ui/` then returns `503` with a body naming the missing build step,
so the absence is loud rather than a silent blank page.

### Configuration reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dashboard.enabled` | bool | `true` | Whether `/internal/ui/*` routes are registered at all |
| `dashboard.access_policy.allowed_cidrs` | []string | `["127.0.0.0/8", "::1/128"]` | CIDR allowlist matched against the TCP source address. Required, non-empty, whenever `enabled` is true |
| `dashboard.access_policy.allowed_hosts` | []string | `["localhost"]` | Hostname allowlist matched against the request `Host` header (port stripped, case-insensitive). May be empty: any Host that parses as an IP literal is always accepted, so you only need to list non-IP hostnames you intend to browse by |
| `dashboard.gate_internal_api` | bool | `false` | **Reserved, currently inert.** Intended for a future release that extends the same `access_policy` to the rest of `/internal/*` and `/version`. Setting it `true` has no effect today - Olla logs a startup warning and otherwise ignores it. It ships now so the later change needs no config migration |

`access_policy.allowed_cidrs` is the only required field when the dashboard is enabled.
Startup fails loudly if it is empty or if a CIDR does not parse - the dashboard never
silently widens its own gate. `allowed_hosts` may legitimately be empty.

!!! warning "Nest fields under `access_policy`, not directly under `dashboard`"

    Olla parses config with a plain YAML unmarshal, not a strict one, so an unknown or
    misplaced key is silently dropped rather than rejected at startup. If you write
    `allowed_cidrs` directly under `dashboard:` instead of under `dashboard.access_policy:`,
    Olla will not complain - it will simply fall back to the loopback-only default and you
    will not find out until access from where you expected is refused. Always nest
    `allowed_cidrs` and `allowed_hosts` under `access_policy`, exactly as shown above.

## Widening access

The default policy lets the dashboard load only from a browser on the same machine. The
two common reasons to widen it are LAN access and Docker.

### LAN deployment

To browse the dashboard from another host on the LAN, add that host's CIDR and any
non-IP hostname you will type into the browser.

```yaml
dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "127.0.0.0/8"
      - "::1/128"
      - "10.0.1.0/24"
    allowed_hosts:
      - "olla.internal.example.net"
  gate_internal_api: false
```

Browsing `http://10.0.1.5:40114/internal/ui/` from a host in `10.0.1.0/24` works without
listing `10.0.1.5` in `allowed_hosts` - any Host that parses as an IP literal is accepted
automatically. Browsing `http://olla.internal.example.net:40114/internal/ui/` requires
the hostname entry shown above, because that name does not parse as an IP.

### Docker deployment

In Olla's typical containerised deployment a client connecting to the published port
arrives from the Docker bridge network's gateway address, not from loopback. On a default
Linux install that gateway is `172.17.0.1`; on Compose or a custom network it varies.
Confirm your own with `docker network inspect bridge` (or your Compose network name).

```yaml
dashboard:
  enabled: true
  access_policy:
    allowed_cidrs:
      - "172.17.0.0/16"    # docker0 default bridge subnet - confirm with
                           # `docker network inspect bridge`, this varies by host
    allowed_hosts: []
  gate_internal_api: false
```

A worked Docker Compose snippet, with the host-side CIDR matching the bridge gateway:

```yaml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    ports:
      - "40114:40114"
    volumes:
      - ./olla.local.yaml:/app/config/config.local.yaml:ro
    # The published port means browser traffic to the host arrives at the
    # container from the bridge gateway (commonly 172.17.0.1). Loopback-only
    # would 403 on first run inside Docker.
```

The loopback-only default 403s out of the box under Docker, because the request reaches
Olla from the bridge gateway, not from `127.0.0.1`. The predictable but dangerous
response to an unexplained 403 is `allowed_cidrs: ["0.0.0.0/0"]`, which converts a
conservative default into the worst possible configuration in one edit. The
self-diagnosing 403 (see below) exists precisely to head this off: it tells you the
source IP and Host Olla actually saw, so you can add the bridge subnet, not the world.

## Security model

This dashboard has **no authentication**. That is a deliberate, inherited decision
consistent with every other `/internal/*` endpoint in Olla. Because there is no auth,
network-layer restriction is the only control, and it is applied per request - Olla's
proxy listener may legitimately bind `0.0.0.0` (the shipped `config.yaml` does exactly
that), so "only listen on 127.0.0.1" is not an available control. The dashboard shares
that listener.

### The gate only covers `/internal/ui/`

The dashboard's `access_policy` gates `/internal/ui/*` and nothing else. The JSON
endpoints the dashboard reads - `/internal/status`, `/internal/status/endpoints`,
`/internal/status/models`, `/internal/health`, `/internal/metrics`, `/version` - stay
exactly as reachable as they already were. An attacker interested in fleet topology has
no reason to load the SPA: they `curl /internal/status/endpoints` directly. Gating the
dashboard's UI alone does not gate the data it renders.

`gate_internal_api` exists in config to one day close that gap by extending the same
`access_policy` to the rest of `/internal/*` and `/version`. It is **currently inert**:
setting it `true` parses cleanly, logs a startup warning, and changes no behaviour. It is
off by default, and defaulting it on would silently break deployments that scrape
`/internal/metrics` from Prometheus or poll `/internal/health` from hosts the policy would
reject. Wiring it is tracked for a later release.

### The two checks

Both are enforced per request, inside the middleware that wraps the dashboard handler.
Both must pass; failure returns `403 Forbidden`.

1. **Client IP must be within `access_policy.allowed_cidrs`.** The IP is read from
   `r.RemoteAddr` - the TCP-layer source address - via `net.SplitHostPort`. **`X-Forwarded-For`
   and `X-Real-IP` are never consulted for this decision, under any configuration.** There
   is no `trusted_proxies` mechanism for the dashboard. The rate limiter's
   `trust_proxy_headers` setting does not apply here.
2. **`Host` header must either parse as an IP literal, or match `access_policy.allowed_hosts`**,
   after stripping any trailing `:port`, normalising IPv6 brackets, and comparing
   case-insensitively. Any Host that parses as an IP address - `127.0.0.1`, `[::1]`,
   `192.168.1.10`, etc. - is accepted unconditionally, without needing to appear in
   `allowed_hosts`. This is what makes browsing `http://192.168.1.10:40114/internal/ui/`
   work without an extra entry. DNS rebinding requires the browser to have resolved a
   *hostname*, so an IP-literal Host is by definition the address the browser actually
   dialled and cannot be the vehicle for a rebinding attack. A non-IP Host (a LAN mDNS
   name, or `attacker-domain.example`) is rejected unless explicitly listed.

### Self-diagnosing 403

A request that fails either check does not get a bare, unexplained `403`. The body states
which check failed and names the client IP and Host the request actually presented:

```
403 forbidden: ip not in allowed range (ip=172.17.0.1, host=172.17.0.1:40114)
403 forbidden: host not accepted (ip=10.0.1.5, host=olla.corp.example:40114)
```

A matching `Warn`-level log line is emitted with the same detail. This is deliberate.
There is no authentication secret to protect by staying silent; the policy configuration
is operational information the operator is entitled to see reflected back; and a bare
unexplained 403 is exactly what turns the Docker first-run problem into "just set it to
`0.0.0.0/0`" instead of "add the bridge CIDR".

### What this does not protect against

Stated plainly:

- **Anyone who can reach an address in `allowed_cidrs` with an accepted Host can read the
  dashboard, with no further gate.** If you widen the CIDR to `10.0.0.0/24`, then anyone
  on that LAN segment can load `/internal/ui/`. Widening `allowed_cidrs` is an explicit
  convenience-for-exposure trade; do not make it casually.
- **The rest of `/internal/*` and `/version` are reachable by anyone who can reach the
  listener**, regardless of the dashboard's own policy. `gate_internal_api` does nothing
  about that today.
- **The dashboard's access policy is not authentication**, and it does not protect against
  anything the reverse-proxy bypass below already allows.

### Reverse proxy defeats the loopback check entirely

If Caddy, Traefik, nginx, or similar terminates TLS on the same host and reverse-proxies
to Olla over `127.0.0.1`, then `r.RemoteAddr` is loopback for every request the proxy
forwards - including requests that originated from the public internet. Because this
feature correctly refuses to trust `X-Forwarded-For` without an explicit, separate trust
mechanism (which the dashboard deliberately does not provide), **there is no fix for this
within the dashboard's own policy.** An operator running Olla behind a reverse proxy on
the same host must understand that the loopback check provides no protection in that
topology; the effective security boundary is whatever the reverse proxy itself enforces.

Olla does not see through the proxy and will not pretend to. If you need dashboard access
from beyond a reverse proxy, the supported topology is to put the access control
(authentication, network ACLs) at the proxy and treat Olla's own check as belt-and-braces.

### CORS interaction

CORS wraps the entire mux, outermost. If you enable CORS with a permissive configuration
(`allowed_origins: ["*"]`), then `/internal/*` JSON becomes readable cross-origin from
any site an operator's browser visits - a page at `evil.example` can `fetch()`
`/internal/status/endpoints` from a victim's browser and read the response, no DNS
rebinding required, because the browser already considers it a legitimate cross-origin
request that Olla's own CORS policy said to allow.

The dashboard's access policy is applied to `/internal/ui/*` regardless of what CORS
decided - it is not bypassed by an `Access-Control-Allow-Origin` header having been
attached - and it refuses to soften this. But a permissive CORS policy plus an ungated
`/internal/*` is a self-inflicted configuration, not a gap in this feature. Do not pair
them.

## Verifying your policy

After changing `dashboard.*`, restart Olla. The fastest functional check from the same
host:

```bash
curl -i http://127.0.0.1:40114/internal/ui/
```

A `200` confirms the dashboard is mounted and the loopback policy admits the request. A
`403` from a host you expected to be allowed is self-diagnosing: the response body names
the failed check and echoes the IP and Host Olla actually saw, so you can see which
`allowed_cidrs` or `allowed_hosts` entry is missing without re-reading your YAML. A `404`
means the route is not registered at all - either `dashboard.enabled: false` or, if you
built without `make build-web`, the SPA is not present (in which case a `503` is served
instead).
