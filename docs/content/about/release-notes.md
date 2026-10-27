---
title: Release Notes - What's New and What Changed
description: Notable additions, fixes, and breaking changes in each Olla release. Use this page as the source of truth for upgrade migrations and behaviour changes.
keywords: olla release notes, changelog, breaking changes, upgrade, migration
---

# Release Notes

This page tracks notable additions, fixes, and breaking changes in Olla. Each entry
is written for an operator upgrading from a prior release: what changed, why, and the
exact migration step if a behaviour change affects you.

Breaking changes are tagged **Breaking**. Additions and fixes are tagged **Added** or
**Fixed**. Behaviour changes that need no migration are tagged **Changed**. Quoted field
names and config keys are the literal wire or YAML identifiers.

## v0.0.30

### Fixed: model discovery no longer sends backend credentials to `HTTP_PROXY` or across hosts

Model discovery still honoured `HTTP_PROXY`/`HTTPS_PROXY`, so with a proxy set in the environment, a plain-HTTP
endpoint's `auth:` token or API key went to that proxy on every discovery cycle. Discovery and the health checker also
followed redirects to other hosts, and Go only strips `Authorization` on those, so an `api_key` header or custom
`headers:` entry could be replayed at the redirect target. Both now connect directly and stop at a redirect that
leaves the configured host and port (a same-host `http` to `https` upgrade is still followed), so a cross-host redirect
counts as a failed discovery or health check. Loopback endpoints were never proxied, so only setups that relied on `HTTP_PROXY`
to let discovery reach a non-loopback backend are affected: that backend now needs to be reachable directly from the Olla host,
as chat traffic already required.

### Added: Text Embeddings Inference backend

Olla now routes to Hugging Face [Text Embeddings Inference](https://github.com/huggingface/text-embeddings-inference)
(TEI) servers. Set `type: "text-embeddings-inference"` on an endpoint; the provider routes are
`/olla/tei/` and `/olla/text-embeddings-inference/`, and requests are forwarded to TEI's own paths
(`/embed`, `/embed_all`, `/embed_sparse`, `/v1/embeddings`, `/rerank`, `/predict`, `/similarity`,
`/tokenize`, `/decode`, `/info`, `/health`, `/metrics`). TEI has no `/v1/models`, so the shipped
`text-embeddings-inference.yaml` profile discovers the single model each instance serves from `GET /info`
(`served_model_name`, falling back to `model_id`) and reads its kind from `model_type`, giving
embedding, reranker and classifier models their own types and capabilities. Mixed fleets work: a TEI
host is never sent chat requests, and model-less native requests are steered to a TEI instance serving
the matching kind. See the [TEI integration guide](../integrations/backend/text-embeddings-inference.md).

### Added: Text Embeddings Inference model kinds

TEI reranker and classifier models get their own types (`reranker`, `classifier`) with
`rerank` and `classification` capabilities (rerankers carry both, as TEI's `/predict` serves them too), and are never tagged `text-generation`. They show
up under `/olla/models?type=reranker` and `?type=classifier`. Model-less `/embed`, `/embed_all`,
`/embed_sparse`, `/similarity`, `/rerank` and `/predict` requests, whose bodies carry no `model`
field, are routed to an instance serving a model of the matching kind. A custom `models.yaml`
doesn't need `reranker` or `classifier` entries under `type_capabilities`: any `type_capabilities`
or `context_thresholds` key your file omits falls back to the built-in default.

### Changed: named-model requests skip capability filtering

When a request names a model, endpoint selection now depends only on which endpoints host that model.
Capability tags are heuristic, so a request for `llama3` could previously be narrowed away from its own
hosts because some other model carried (or lacked) a tag. Capability filtering now applies only to
requests that name no model (for example a model-less vision or `/rerank` request). When it does apply,
the capabilities must all be on a single model: an endpoint hosting one vision model and a different
tools model no longer satisfies a request needing both. A request needing `function_calling` no longer also
requires the `tools` tag, which many backends never set. If you depended on a named request being
steered by capability tags, name the model you want instead.

### Fixed: a model-less request is refused when the only capable backend is down

A request that names no model but needs a capability (for example `/embed`, `/rerank`, `/v1/embeddings` or a request with images) is steered to hosts with a model of that kind. When none of those hosts was healthy, Olla fell back to every compatible endpoint, so with the only embedder stopped, `/embed` went to the reranker and classifier instances and TEI answered `424`. Olla now answers `503` with `X-Olla-Routing-Reason: capability_unavailable` and `X-Olla-Routing-Strategy: capability` when models with the capability exist but none is on a healthy endpoint. Nothing to change on your side: if no model with the capability has ever been discovered, the unfiltered fallback still applies, and requests that name a model are unaffected.

### Fixed: profile paths now list what each backend serves

`/olla/proxy/v1/responses` and `/olla/proxy/v1/messages` were narrowed to the profiles that
declared them, evicting backends that serve them without saying so. The shipped profiles
now declare `/v1/responses` (vLLM, SGLang, LMDeploy, llama.cpp, LM Studio, vLLM-MLX, LiteLLM,
Ollama, OpenAI-compatible, Lemonade, Docker Model Runner),
`/v1/messages` (vLLM, SGLang, LMDeploy, llama.cpp, LM Studio, vLLM-MLX, Ollama),
`/v1/messages/count_tokens` (vLLM, SGLang, LMDeploy, llama.cpp, vLLM-MLX) and `/v1/rerank`
(SGLang, llama.cpp, vLLM-MLX). Ollama also declares `/api/ps` and `/api/version`, so a proxy call to
them goes to Ollama hosts only. Lemonade serves `/v1/responses` at `/api/v1/responses` and
`/v1/responses`, and Docker Model Runner registers it under `/v1` and `/engines/v1`; both are declared
at the unprefixed path so a standard `/olla/proxy/v1/responses` call keeps them as candidates.
Docker Model Runner also declares `/anthropic/v1/messages` and `/anthropic/v1/messages/count_tokens`,
which a direct proxy call used to miss in a mixed fleet.

The generic OpenAI-compatible profile stands in for servers Olla has no profile for, such as a remote
vLLM or SGLang typed `openai`, so it declares `/v1/responses`, `/v1/messages`,
`/v1/messages/count_tokens` and `/v1/rerank`: it stays a candidate for each of them in a fleet where
other profiles declare the path. Declaring a path is not Anthropic passthrough
(`anthropic_support` stays disabled), and a request that names a model is still routed by that
model. A custom profile in `config/profiles/` that serves these paths needs the same entries.

### Changed: one-segment profile paths no longer claim deeper request paths

TEI declares single-segment paths such as `/embed`, `/rerank`, `/health`, `/metrics` and `/tokenize`.
Request path inspection matched paths by suffix, so those would also have claimed deeper paths on
other backends (for example `/api/embed` or a `.../v1/rerank`). One-segment profile paths now match
only a request path of that single segment.

### Fixed: one client could flood the log with rejections

Olla wrote a WARN for every request it rejected, so a client hammering a rate limit produced millions of identical lines in minutes and pushed useful history out of the rotated log. Rate limit (`429`), request size (`413`/`431`), dashboard access (`403`) and proxy routing rejections are now sampled: the first in each 10 second window logs in full, and the rest become one summary such as `WARN Rate limit exceeded, repeats suppressed layer=rate_limit client_ip=203.0.113.7 suppressed=19994 window=10s`. The `Model routing rejected request` and `Large request blocked` WARNs are gone, since the sampled `Request rejected` line carries the same detail. Responses, and the rate limit and size limit counters, are unchanged, and a backend's own 4xx and 5xx responses are still logged per request.

### Fixed: model routing no longer depends on the Content-Type header

A request that named a model but didn't send `Content-Type: application/json` was routed as if it named
none, so it could land on any compatible host, including ones without the model (TEI answered 424 for
embeddings sent this way). `curl -d` sends form-urlencoded, `fetch()` with a string body sends
`text/plain`, and plenty of scripts send nothing. Olla now decides from the body: a body that opens with
`{` is read for its `model` and capabilities whatever the header says, and the request is forwarded
unchanged. Multipart, form-encoded and compressed bodies are still not read. Nothing to change on your side.

### Fixed: Ollama `/api/embed` and embedding models in a fleet with TEI

The built-in Ollama profile now lists `/api/embed`, so Ollama's native embeddings route is not
captured by TEI's `/embed`. Ollama embedding-only models are typed `embeddings` from the
`capabilities` that `/api/tags` reports (newer Ollama releases; older ones report none and stay
untyped), so `/v1/embeddings` still reaches them when a TEI instance is in the fleet.

Because `/api/embed` is now a recognised Ollama path, `/olla/ollama/api/embed` checks the requested
model like Ollama's other routes. A model pulled
without a tag lists as `all-minilm:latest`; a bare `all-minilm` resolves to it here, as it now does
on every other route (see the untagged-names fix below).

### Fixed: LM Studio model types and capabilities

LM Studio models carry `"object": "model"` beside their real `type` (`llm`, `vlm`,
`embeddings`), and the generic value used to mask the real one. LM Studio embedding
models were never tagged `embeddings` and `vlm` models never got `vision`. The real type
now wins, so those models gain the capabilities their type implies: `vision`,
`multimodal` and `image-understanding` for `vlm`, `embeddings`, `similarity` and
`vector-search` (without `text-generation`) for embeddings, and `chat` and `completion`
for `llm`. Model-less vision and embeddings requests are steered to those models.

### Fixed: crash under concurrent model discovery and requests

Olla could exit with `fatal error: concurrent map iteration and map write` (or `concurrent map write`) when a model listing, alias lookup or status request ran while discovery was merging the same model from a second endpoint, re-registering it, or removing an endpoint. The unifier was editing model records that request handlers were still reading. It now edits a private copy and publishes it, so readers always see a complete record. The unified-stats counters behind the status endpoints are also guarded now. Nothing needs changing in your config.

### Fixed: Ollama model names without a tag

A model pulled with `ollama pull all-minilm` is listed as `all-minilm:latest`, and Ollama accepts
`all-minilm` for it, but Olla returned a 404 `model_not_found` for the bare name on `/olla/proxy/`,
the provider prefixes, `/olla/anthropic/v1/messages` and `GET /olla/models/{id}`. When no endpoint
serves the exact name, Olla now retries once as `<name>:latest` against Ollama endpoints only, so a
vLLM `foo:latest` still never answers `foo`. The request goes to Ollama as you wrote it, and `foo`
and `foo:latest` share one stats row, and one sticky key when it is built from the model name (`prefix_hash` hashes the raw body, so it still differs). Nothing to change on your side.

## v0.0.29

### Breaking: userinfo URLs now fail startup

An endpoint URL with embedded `user:pass@host` is rejected at config load. Previously
such a URL loaded silently and the credentials flowed into every status/dashboard JSON
surface as the literal URL string. The endorsed credential path is the `auth:` block,
which is held as `json:"-"` fields on the internal endpoint record and never reaches the
status layer.

**Before:**

```yaml
- url: "http://user:pass@host:8080"
  name: "protected-backend"
  type: "openai-compatible"
```

**After:**

```yaml
- url: "http://host:8080"
  name: "protected-backend"
  type: "openai-compatible"
  auth:
    type: basic
    username: user
    password: pass
```

For secrets kept out of config, use the `username_file` / `password_file` variants (see
[Endpoint Authentication](../configuration/endpoint-auth.md)).

The boot error rewrites the rejected URL into an `auth:` block with the credentials
replaced by placeholders, so the fix is a copy-paste away without the error ever echoing
your real credentials. For a full `user:pass@host` URL this is an exactly equivalent
basic block; for a username-only `user@host` URL there is no equivalent basic
configuration (basic auth requires a password), so the error offers migration
alternatives instead - a basic block with a password placeholder, or a bearer block if
the username was really a token.

### Breaking: zero-traffic status semantics changed

`/internal/status` no longer reports `"status": "critical"` on a fresh boot with all
endpoints healthy. With no proxy traffic, the system status derives from endpoint health
alone, `success_rate` reports `"N/A"`, and a new always-present boolean
`system.has_traffic` lets clients branch on the no-traffic state without parsing the
success-rate string. Previously a healthy fresh boot fell through a `< 90.0` success-rate
threshold and reported `critical`, which coupled with the dashboard produced a misleading
red status on first start.

### Breaking: endpoint `id` derivation changed

The `id` values surfaced on `/internal/status`, `/internal/status/endpoints`, and the
per-model `endpoint_ids` on `/internal/status/models` are now derived from the sanitised
URL (scheme+host+port+path) with positional disambiguation for siblings that share a
sanitised form. Credential rotation no longer changes the ID, because userinfo, query,
and fragment are stripped before hashing. **Bookmarked dashboard deep-links to a specific
endpoint row will change once after upgrading**, then stay stable for endpoints with a
distinct name or a distinct sanitised URL. IDs may change again if a sibling sharing the
same sanitised URL is later added or removed, because the shared `-N` suffix is positional
and gets renumbered (and the degenerate case of two endpoints sharing both name and
sanitised URL is not immune either). The IDs are base36 FNV-1a, identical across all three
status payloads for the same endpoint from the same repository snapshot.

### Breaking: endpoint `url` in status responses is now sanitised

The `url` field on `/internal/status` and `/internal/status/endpoints` now has userinfo,
query string, and fragment stripped before it is surfaced. (`/internal/status/models`
exposes no URL at all - only endpoint display names and `endpoint_ids`.) Previously this
field echoed the raw configured URL, which could include embedded credentials. Clients
that compared this field against the raw config value for identity should switch to the
`id` field, which is designed for that purpose.

### Added: weak ETags on status JSON

The three status JSON routes now emit a **weak** `ETag` of the form `W/"<base36>"` over
their stable fields. `If-None-Match` uses weak comparison, so clients that echo the
`ETag` verbatim are unaffected. Clients doing strong-only comparison should allow weak
matches. See [Conditional requests and compression](../api-reference/system.md#conditional-requests-and-compression)
for the full contract.

### Fixed: proxy access logging restored

Successful proxy requests log at `Info` again in the access log. A regression had
demoted them to `Debug`, which silently broke the "log all requests" expectation the
security practices document promises. Only `/internal/` GET/HEAD polling that returns
`2xx` or `304` stays at `Debug` so an open dashboard tab does not flood the log; 4xx,
5xx, and any non-GET/HEAD method under `/internal/` continue to log at `Info`.

### Added: unbuilt-dashboard warning at startup

A binary built without `make build-web` (e.g. via `go install` or a plain `go build`)
now logs a clear startup line and serves `503` at `/internal/ui/` with a body naming the
fix. Previously such a binary served a silent placeholder. See
[Development Setup: building the dashboard](../development/setup.md#optional-bun-135-for-the-admin-dashboard).

### Added: admin dashboard at `/internal/ui/`

A read-only, single-page dashboard is now embedded in the binary and served from the
same listener as the proxy: Overview (fleet status, success rate, latency, a live
requests-per-second sparkline), Endpoints (per-endpoint health, priority, latency,
model count), and Models (inventory grouped by family, with hosting endpoints). It
polls the existing `/internal/status*` JSON every 5-15 seconds with `ETag`/`304`
caching - no WebSocket, no push, and nothing in it changes state.

There is no authentication. Access is controlled entirely by the new `dashboard.access_policy`
config block (`allowed_cidrs` + `allowed_hosts`), which defaults to loopback-only. The
published Docker image ships with `allowed_cidrs` pre-widened to the RFC1918 ranges and
the listener bound to `0.0.0.0`, so `docker run -p 40114:40114 ghcr.io/thushan/olla:latest`
followed by opening `/internal/ui/` works with no config mount - and also means the
dashboard, and the container, are reachable from anyone else on the same LAN through that
published port. The access policy gates `/internal/ui/` only: the `/internal/status*`,
`/internal/health`, `/internal/metrics` and `/version` JSON it reads stay as reachable as
they already were, `allowed_cidrs` or not. Unifying that gap under one policy is tracked
in issue #214.

See [Admin Dashboard](../configuration/dashboard.md) for the full security model and
configuration reference.

### Added: native `GET /internal/metrics`

Prometheus-format metrics are now exposed directly, built from the same data as
`/internal/status` and `/internal/stats/models` - system status, per-endpoint health,
security counters, model usage and routing. No external exporter is required for core
proxy monitoring. Thanks to **@Puupuls** for the contribution. See
[System API](../api-reference/system.md) for the metric series.

### Fixed: circuit-breaker-open no longer blocks failover

A request that lands on an endpoint whose circuit breaker is open now fails over to the
next available endpoint instead of the request failing outright. The endpoint is removed
from that request's candidate list only; its persisted health is left untouched, because
an open breaker already reflects accumulated failure state and demoting health on top of
it is the proxy engine's job for genuine connectivity failures, not the retry path's.
Exhaustion errors now distinguish circuit-breaker-open counts from connection-failure
counts instead of conflating the two under one message.

### Breaking: strict routing now fails fast on an unroutable alias

A request for a `model_aliases` entry whose target model exists on no endpoint used to be
logged as rejected but still proxied to a compatible backend anyway, returning `200` from
the wrong model. It now returns `404`/`503` with `routing_action: rejected`, matching how
unknown models are already handled. One side effect: `/olla/proxy/` with zero healthy
endpoints now returns `503` instead of `502` - update any monitor keyed on that specific
status code.

### Breaking: `owned_by` in converted model listings may return the raw matched segment

The organisation-extraction logic used by the vLLM, vLLM-MLX, SGLang, llama.cpp, LMDeploy
and Lemonade converters is now shared. Alongside the existing `org/model` slash split, it
also recognises a hyphen-separated leading segment against a known-organisation list and
returns that segment verbatim - so a model ID like `Qwen2.5-7B` now yields `owned_by:
"Qwen2.5"` rather than falling through to the converter's generic default. Anything
parsing `owned_by` for exact organisation matching should treat it as a best-effort label,
not a canonical identifier.

### Fixed: `config/models.yaml` customisations are no longer silently discarded

The shipped `config/models.yaml` used `\d` inside double-quoted YAML scalars, which is not
a valid YAML escape - the file has never parsed, on any install. The failure was swallowed
and Olla silently fell back to embedded defaults, so any customisation (e.g. capability
tagging via `name_patterns`) had no effect and no diagnostic. The regex patterns are fixed
to single-quoted scalars, config loading now happens eagerly at boot rather than lazily on
first request, and a found-but-unparseable candidate logs a `WARN` with the path and YAML
error instead of failing silently.

### Added: `--validate-config` flag

Run `olla --validate-config` to check the configuration and provider profiles without
starting the server - a clear pass/warn/fail report with exit codes, useful in CI or
before a restart.

### Fixed: `logging.level` in config is now honoured

The `logging.level` config field was parsed but never applied to the runtime logger.
Precedence is `OLLA_LOGGING_LEVEL` env var, then config file, then default; an invalid
value now warns and falls back instead of being silently ignored.
