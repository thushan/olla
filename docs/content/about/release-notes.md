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

### Upgrading: behaviour changes to check

These v0.0.30 changes alter what existing setups see. Each links to its entry below.

- **SGLang and LMDeploy pass Anthropic requests through.** `/olla/anthropic/v1/messages` now forwards to them natively
  (SGLang v0.5.9+, LMDeploy v0.13.0+) instead of translating. LMDeploy requests carrying `tools` are still translated.
  On older builds, copy the profile to `config/profiles/zz-sglang.yaml` or `zz-lmdeploy.yaml` (the `zz-` prefix is needed
  because profiles load in lexical order and the last one with a given `name` wins) and set `api.anthropic_support.enabled: false`.
  See [Profile Loading](../concepts/profile-system.md#profile-loading) and [Changed: SGLang and LMDeploy](#changed-sglang-and-lmdeploy-use-native-anthropic-passthrough-by-default).
- **Profiles declare the paths they serve.** The shipped profiles now list `/v1/responses`, `/v1/messages`, `/v1/rerank` and
  Ollama's `/api/ps` and `/api/version`, and a request on a declared path only goes to backends whose profile declares it.
  A custom profile that serves these paths needs the same entries. See [profile paths](#fixed-profile-paths-now-list-what-each-backend-serves).
- **Anthropic translation uses each backend's own chat path.** Docker Model Runner gets `/engines/v1/chat/completions` and
  Lemonade `/api/v1/chat/completions`. If no backend in the fleet serves a chat path, the request gets a 404.
  See [chat path](#fixed-anthropic-translation-sends-each-backend-its-own-chat-path).
- **Ollama vision and tool models are typed differently.** A model that reports vision is `vlm` with the `vision` capability,
  and `tools` maps to `function_calling`. `?type=llm` no longer lists vision models; `?capability=chat` still does.
  Model-less requests with images or tools go to hosts with a model that declares them. See [Ollama vision](#fixed-ollama-vision-models-list-as-vlm-and-tool-support-is-recorded).
- **`/olla/ollama/api/ps` merges every Ollama host.** Entries carry `olla_endpoint`, a partial answer sets `X-Olla-Partial`, and the route is rate limited.
  The `/olla/ollama/api/list` stub is gone, so those requests now reach the backend, and `/api/blobs` returns 501.
  See [`/api/ps`](#fixed-ollaollamaapips-covers-every-ollama-endpoint-apilist-removed).
- **"Request completed" log lines carry `http_status`, and a backend 5xx logs at WARN** instead of INFO. Check any log-based alerting or
  level filters. See [the log change](#fixed-a-backend-5xx-no-longer-looks-like-a-normal-completion-in-the-log).
- **Discovery `retry_attempts` and `retry_backoff` are now honoured.** A fetch that failed once may now be retried, and a negative
  `retry_backoff` fails startup. See [discovery retries](#fixed-model-discovery-now-retries-failed-fetches).
- **Name patterns tag rerankers and embedders** on backends that report no kind, so those models drop `text-generation` and `chat`.
  A custom `models.yaml` with its own `name_patterns` needs the two new entries. See [name patterns](#added-rerankers-and-embedders-recognised-by-name-on-backends-that-report-no-kind).
- **`per_ip_requests_per_minute: 0` no longer turns rate limiting off.** If you set `per_ip_requests_per_minute: 0` (or `OLLA_SERVER_PER_IP_RATE_LIMIT=0`) to turn rate limiting off,
  the global limit (default 1000/min, burst 50) now applies; set `global_requests_per_minute: 0` as well to disable both.
  See [rate limit tiers](#fixed-the-global-rate-limit-applies-when-the-per-ip-limit-is-0).
- **Anthropic and Claude Code traffic is now rate limited.** `/olla/anthropic/v1/messages`, `/messages/count_tokens` and
  `/models` take the same limits as the proxy routes (default 100 requests per minute per IP, burst 50, 1000 globally),
  and share each client's per-IP bucket with them. Claude Code stays well inside these, including a few parallel sessions,
  but many sessions behind one IP (a team behind NAT, CI runners, parallel agents) may now see 429s: raise
  `server.rate_limits.per_ip_requests_per_minute` and `burst_size`. See [Anthropic rate limits](#fixed-anthropic-routes-are-now-rate-limited).
- **A request is never retried once part of the response has reached the client.** A GET that failed mid-stream used to be replayed on the next
  backend, so the client got the first backend's partial output followed by the second's full response. It now ends with the error. POST, PATCH and DELETE
  were already protected. See [retry policy](#added-proxyretry-controls-failover).
- **`X-RateLimit-*` headers are omitted when no rate limit applies.** With both limits at `0` (and for an unlimited health probe) responses used to carry `X-RateLimit-Limit: 0` and `X-RateLimit-Remaining: 0`; they now carry none. See [rate limit headers](#fixed-x-ratelimit-headers-are-omitted-when-rate-limiting-is-off).
- **Additive:** `olla.type` in `/olla/models` and a top-level `type` on `/olla/models/{id}`. See [model kind](#added-model-kind-in-the-models-api).

### Added: `proxy.retry` controls failover

Failover on a connection failure was hardcoded: every eligible endpoint once, and any method replayed. A POST whose backend read the
body and then dropped the connection was therefore sent again to the next endpoint, so a prompt could run twice. The new `proxy.retry`
block (`enabled`, `max_attempts`, `non_idempotent`) makes this configurable, with `OLLA_PROXY_RETRY_*` environment overrides. The
defaults are today's behaviour, so nothing changes unless you set it: `non_idempotent: false` stops POST, PATCH and DELETE failing over after
a connection failure (GET still does), `max_attempts: N` caps attempts, `enabled: false` makes every request a single attempt. A failed
connection still marks the endpoint unhealthy in every case, a negative `max_attempts` fails startup, and both engines and the Anthropic routes follow the policy.
Timeouts and HTTP error statuses do not fail over. The deprecated `proxy.max_retries` and
`proxy.retry_backoff` stay accepted, do nothing, and log one warning at startup when set to a non-zero value. They are not mapped onto the new block.
A request that fails after part of its response was sent is no longer retried (see the Upgrading note above). See [Retry Behaviour](../configuration/reference.md#retry-behaviour). Reported by @matthewjhunter.

### Fixed: Anthropic routes are now rate limited

`/olla/anthropic/v1/messages`, `/messages/count_tokens` and `/models` were registered as plain routes, so the rate limit
never reached them, although the API reference said the standard limits applied. They are the main entry point for
Claude Code. Translator routes now register through `RegisterTranslatorRoute` and sit behind the same rate limit as the proxy
routes, and a translator added later is covered without extra wiring. A limited request gets `429` with `Retry-After` and
Anthropic's `rate_limit_error` body, and an oversized one gets `413` with `request_too_large`, both carrying a `request-id`,
so Claude Code retries and reports them properly. The size limit already applied to these routes; it now answers in the same
Anthropic shape. Measured against a real Ollama, one Claude Code session peaked at 2 requests a minute and three parallel
sessions at 9 a minute, with a burst of 4 when four sessions started together, so the existing defaults (100 per minute per IP, burst 50) need no change; many sessions
behind one IP should raise `per_ip_requests_per_minute`. See [Rate Limit Best Practices](../configuration/practices/rate-limits.md).
Reported by @matthewjhunter.

### Fixed: chunked bodies on Anthropic routes are capped at the translator limit

A chunked request (no `Content-Length`) to `/olla/anthropic/v1/*` was read into memory up to `server.request_limits.max_body_size`
(100MB by default) before the translator's own `max_message_size` (10MB) rejected it, so a few concurrent oversized chunked
requests could hold gigabytes. The read now stops at the smaller of the two limits and answers `413` with `request_too_large`.
For `/messages` that is the same status as before; the rejection now happens before the handler reads the body, so it is counted
as a size-limit violation rather than in the translator metrics, and the message text is "Request body too large". A chunked body
between the two limits on `/messages/count_tokens` (previously a `400`) or `/models` (previously served) now also gets the `413`.
Requests with a `Content-Length`, and all proxy routes, behave as before.

### Fixed: `X-RateLimit-*` headers are omitted when rate limiting is off

Every response through a rate-limited route carried `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `X-RateLimit-Reset`, even with both limits at `0`, so a `413` or a plain `200` showed `X-RateLimit-Limit: 0, X-RateLimit-Remaining: 0`, which a client reads as fully throttled. The headers are now sent only while a limit applies.

### Added: dashboard herd columns, column picker, issues tooltip and filter

The Overview herd table now shows In flight, Requests, Type and Models next to the status, response rate and latency columns. A Columns menu adds Priority,
Probe latency (the last health-check probe, not proxied requests), Traffic and Failures (estimated from requests and response rate); the choice is saved in the
browser. Hover or focus a status pill to see why an endpoint is amber or red. A search box and an All/Unhealthy toggle filter the herd table and the Endpoints panel.
Under 600px wide only Endpoint, Status, Resp. rate and Latency show. Frontend only; no API or config change. See [Admin Dashboard](../configuration/dashboard.md).

### Added: unknown keys in `config.yaml` are reported

A key in the main config that Olla does not read (a typo, or one that was documented but never existed) used to be dropped silently. Startup now logs one WARN line listing every such key by its full path and line,
for example `proxy.engnie (line 12)`, and `olla --validate-config` lists them under "Ignored keys" without changing the
result or exit code. Loading still never fails on them, and `models.yaml` and profiles are not checked. Free-form maps such
as `model_aliases` and endpoint `headers` are never reported. Each key is listed once, and both the WARN and the report show at most 100, ending with `and N more`; the count covers every key found. Reported by @matthewjhunter.

### Fixed: Anthropic translation sends each backend its own chat path

In translation mode Olla sent every backend `/v1/chat/completions`. Docker Model Runner serves the OpenAI API under
`/engines/v1/` (the root `/v1/` alias only exists in recent builds) and Lemonade under `/api/v1/`, so on those hosts
translated requests 404ed, and because a 404 is not retried the client saw the error even when another healthy
backend could have answered. Olla now resolves the chat path from each endpoint's profile and applies it per attempt,
so a fleet mixing DMR, Lemonade, vLLM and Ollama works, and failover between them uses the right path each time.
Backends that speak Anthropic natively are still passed through to their `anthropic_support.messages_path`. Endpoints
whose profile has no chat path (Text Embeddings Inference) are never sent translated requests; if nothing selected can
serve chat the request is refused with a 404 that is logged as a sampled "Request rejected" line.

### Fixed: `/olla/models/{id}` for IDs containing slashes

`GET /olla/models/BAAI/bge-small-en-v1.5` returned 404 because only the first path
segment was read as the ID. The whole remainder of the path is now the ID, so slash IDs
resolve both raw and percent-encoded (`BAAI%2Fbge-small-en-v1.5`), and aliases still
resolve to them. An empty ID or one ending in a slash returns 400.

### Fixed: model discovery now retries failed fetches

`discovery.model_discovery.retry_attempts` and `retry_backoff` were accepted but never used, so one dropped
connection failed discovery for that endpoint until the next interval. Discovery now retries connection errors,
timeouts and 5xx responses, doubling `retry_backoff` (capped at 30s) between attempts. All attempts share the
existing `timeout`, so a cycle never runs longer than it did before. Parse errors, 4xx responses and listings over
the 10MB cap are not retried, and a cycle that still fails on a retryable error counts as one failure towards the five-failure cooldown.
A negative `retry_backoff` is now rejected at startup.

### Fixed: model discovery no longer sends backend credentials to `HTTP_PROXY` or across hosts

Model discovery still honoured `HTTP_PROXY`/`HTTPS_PROXY`, so with a proxy set in the environment, a plain-HTTP
endpoint's `auth:` token or API key went to that proxy on every discovery cycle. Discovery and the health checker also
followed redirects to other hosts, and Go only strips `Authorization` on those, so an `api_key` header or custom
`headers:` entry could be replayed at the redirect target. Both now connect directly and stop at a redirect that
leaves the configured host and port (a same-host `http` to `https` upgrade is still followed), so a cross-host redirect
counts as a failed discovery or health check. Loopback endpoints were never proxied, so only setups that relied on `HTTP_PROXY`
to let discovery reach a non-loopback backend are affected: that backend now needs to be reachable directly from the Olla host,
as chat traffic already required.

### Changed: SGLang and LMDeploy use native Anthropic passthrough by default

SGLang (v0.5.9+) and LMDeploy (v0.13.0+) serve the Anthropic Messages API themselves, so their shipped
profiles now carry `anthropic_support` and `/olla/anthropic/v1/messages` forwards to them as-is
(`X-Olla-Mode: passthrough`) rather than translating through `/v1/chat/completions`. Older builds answer
404 for `/v1/messages`, and `min_version` is informational: Olla does not enforce it. If any SGLang or
LMDeploy in your fleet predates those versions, copy the profile to `config/profiles/zz-sglang.yaml` or
`zz-lmdeploy.yaml` and set `api.anthropic_support.enabled: false` in the copy (steps on the
[SGLang](../integrations/backend/sglang.md#switching-sglang-or-lmdeploy-back-to-translation) and
[LMDeploy](../integrations/backend/lmdeploy.md#switching-sglang-or-lmdeploy-back-to-translation) pages), or
set `translators.anthropic.passthrough_enabled: false` to translate for every backend. LMDeploy v0.13.0 to
v0.17.x reject Anthropic `tools`, so its profile declares `no_tool_use` and requests carrying tools are still
translated, including on v0.18.0 and later.

### Added: model kind in the models API

`/olla/models` now reports each model's kind (`llm`, `vlm`, `embeddings`, `reranker` or `classifier`) as
`olla.type`, and `/olla/models/{id}` as a top-level `type`. `?type=` matches that kind exactly, so an
embedder whose name says `instruct` is listed under `embeddings` and no longer also under `llm`, and a
model that reports `vlm` or carries the `vision` capability is `vlm` only. The legacy `?capability=chat` still returns every model that can chat, vision
models included. vLLM, SGLang, Lemonade, LMDeploy, llama.cpp, vLLM-MLX and Docker Model Runner models no
longer carry the provider name as their `type`; `/internal/status/models` now omits it unless the backend
reported a kind. TEI rerankers and classifiers were already typed in the LM Studio-format listing; what is
new is that rerankers and classifiers recognised by name on backends that report no kind are typed there
too instead of falling back to `llm`. That `type` can therefore be a value outside LM Studio's own
vocabulary (`reranker`, `classifier`).

### Fixed: Ollama vision models list as `vlm` and tool support is recorded

Newer Ollama releases report `capabilities` in `/api/tags`, but only `embedding` was read, so models such as
`qwen3-vl:8b` listed as `llm` and `?type=vlm` and `?capability=vision` returned nothing. A model that lists
`vision` is now `vlm` with the `vision` capability (it keeps `text-generation`), and `tools` becomes the
`function_calling` capability. Model-less requests that carry images or `tools` now prefer hosts with a
capable model, so hosts whose models carry no such tag are left out of those requests while an Ollama host
has a match; name the model to reach them. If the capable hosts are all down the request gets a 503
`capability_unavailable`. Older Ollama versions report no capabilities and are unchanged.

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

### Added: rerankers and embedders recognised by name on backends that report no kind

vLLM and other OpenAI-style servers don't say what a model is, so a reranker or embedder looked like
a chat model. Two new `capabilities.name_patterns` entries tag `rerank` for names containing `rerank`
(`bge-reranker-v2-m3`, `Qwen3-Reranker`) and `embeddings`, `similarity` and `vector-search` for the common
embedders (`bge-m3`, `e5-large`, `nomic-embed`, `snowflake-arctic-embed2`, `all-MiniLM`, `multi-qa-*`,
`msmarco-*`, `stella-*`). Those models drop `text-generation`, and `chat` too, so an embedder named
`gte-Qwen2-7B-instruct` is no longer a chat model. Model-less rerank and embeddings requests are steered to
them. A kind the backend does report, and capabilities it declares, are never overridden. A custom
`models.yaml` with its own `name_patterns` list needs the two entries added to benefit.

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
`/v1/messages` (vLLM, SGLang, LMDeploy, llama.cpp, LM Studio, vLLM-MLX, LiteLLM, Lemonade, Ollama),
`/v1/messages/count_tokens` (vLLM, SGLang, LMDeploy, llama.cpp, vLLM-MLX, LiteLLM) and `/v1/rerank`
(SGLang, llama.cpp, vLLM-MLX). These paths only exist in newer backend releases (for example Ollama 0.14.0 for `/v1/messages`, vLLM 0.11.1, LM Studio 0.4.1), and older ones answer 404; `/v1/rerank` on llama.cpp also needs `--reranking`. Ollama also declares `/api/ps` and `/api/version`, so a proxy call to
them goes to Ollama hosts only. Lemonade serves `/v1/responses` at `/api/v1/responses` and
`/v1/responses`, and Docker Model Runner registers it under `/v1` and `/engines/v1`; both are declared
at the unprefixed path so a standard `/olla/proxy/v1/responses` call keeps them as candidates.
Docker Model Runner also declares `/anthropic/v1/messages` and `/anthropic/v1/messages/count_tokens`,
which a direct proxy call used to miss in a mixed fleet.

The generic OpenAI-compatible profile stands in for servers Olla has no profile for, such as a remote
vLLM or SGLang typed `openai`, so it declares `/v1/responses` and `/v1/messages`: it stays a candidate
for each of them in a fleet where other profiles declare the path. It does not declare
`/v1/messages/count_tokens` or `/v1/rerank`, because the paths of this profile also admit Ollama and
LM Studio, which answer 404 on both; a remote server that serves them needs its own profile. Declaring a path is not Anthropic passthrough
(`anthropic_support` stays disabled), and a request that names a model is still routed by that
model. A custom profile in `config/profiles/` that serves these paths needs the same entries.

### Changed: one-segment profile paths no longer claim deeper request paths

TEI declares single-segment paths such as `/embed`, `/rerank`, `/health`, `/metrics` and `/tokenize`.
Request path inspection matched paths by suffix, so those would also have claimed deeper paths on
other backends (for example `/api/embed` or a `.../v1/rerank`). One-segment profile paths now match
only a request path of that single segment.

### Fixed: one client could flood the log with rejections

Olla wrote a WARN for every request it rejected, so a client hammering a rate limit produced millions of identical lines in minutes and pushed useful history out of the rotated log. Rate limit (`429`), request size (`413`/`431`), dashboard access (`403`) and proxy routing rejections are now sampled: the first in each 10 second window logs in full, and the rest become one summary such as `WARN Rate limit exceeded, repeats suppressed layer=rate_limit client_ip=203.0.113.7 suppressed=19994 window=10s`. The `Model routing rejected request` and `Large request blocked` WARNs are gone, since the sampled `Request rejected` line carries the same detail. Responses, and the rate limit and size limit counters, are unchanged, and a backend's own 4xx and 5xx responses are still logged per request.

### Fixed: the global rate limit applies when the per-IP limit is 0

`per_ip_requests_per_minute: 0` returned early and skipped the global limiter as well, while `global_requests_per_minute: 0` only disabled itself. The two limits are now independent switches, and `0` disables only its own tier. Two further faults went with it. A request already over its per-IP limit still took a global token, so one noisy client could starve everyone else, even on the default config; the per-IP limit is now checked first and a request rejected by either limit is charged to neither. A global rejection also reported the per-IP value in `X-RateLimit-Limit` and a fixed `Retry-After: 60`; it now reports the global limit in `X-RateLimit-Limit`, logs the reason `Global rate limit exceeded` and sets `Retry-After` from the global bucket's refill. Rejections of either kind also report `X-RateLimit-Remaining: 0`. Reported by @matthewjhunter.

### Fixed: `Content-Encoding: identity` bodies and unreadable `/v1/embeddings` bodies

A client that sent `Content-Encoding: identity` had its body skipped, so the model name was never read and the
request was routed as if it named none. `identity` means no encoding and is now read like any plain body; `gzip`,
`br` and lists such as `gzip, identity` are still skipped. Separately, `/v1/embeddings` is now an embeddings path
for capability routing, so a model-less request whose body Olla can't read is still kept off chat-only hosts, and
is refused with `503` (`capability_unavailable`) when embedders exist but none is healthy. Requests that name a model are unaffected.

### Fixed: a backend 5xx no longer looks like a normal completion in the log

The per-request `Request completed` line did not say what status the client got, so a backend returning `502` or `503`
read the same as a success. It now carries `http_status` on every proxy route (`/olla/proxy/`, provider routes and the
Anthropic route, passthrough and translation, streaming included), and a backend `5xx` is logged at `WARN`. A backend `4xx`
stays at `INFO`. Olla's own rejections are unchanged and still go through the sampled `Request rejected` line, and a
rejected request no longer also writes an `INFO` `Request received` line.

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

### Fixed: `/olla/ollama/api/ps` covers every Ollama endpoint; `/api/list` removed

`/olla/ollama/api/ps` was forwarded to one arbitrary instance, so with several Ollama
endpoints it reported a fraction of what was loaded. It now queries every healthy Ollama
endpoint and merges the results, one entry per loaded model per host with an added
`olla_endpoint` field; endpoints that fail are skipped and flagged with `X-Olla-Partial`.
Each endpoint is queried with its own credentials only, ignoring `HTTP_PROXY`.

`POST /olla/ollama/api/blobs/{digest}` now returns 501 like the other model-management
routes, which answer 501 to every method. `/olla/ollama/api/list` returned 501 but is not
an Ollama endpoint, so the route is gone: requests to it now reach the backend and get
Ollama's own 404.

### Fixed: a model hosted on several backends no longer flips its listed id

When the same model (matched by digest) was registered from more than one backend, the merged alias list was rebuilt in random order, and `/olla/models` and the provider listings emit the first alias as the model id. The id could change between refreshes. Merging now keeps a stable order and leads with the model's own id.

### Fixed: config and docs no longer promise settings Olla ignores

The shipped `config.yaml`, the examples and the docs carried keys the config schema has no field for, which Olla silently drops: `server.rate_limits.per_endpoint`, the whole `proxy.retry` block, `discovery.health_check`, `unification.enabled` and endpoint `tags`. They are gone from the shipped configs and doc snippets. Failover to another healthy endpoint on a connection failure is automatic, and the deprecation comment that pointed at a non-existent `retry.max_attempts` now says so (`proxy.retry` has since become a real block, see [above](#added-proxyretry-controls-failover)). Unification is controlled by `model_registry.enable_unifier`.

`characteristics.max_concurrent_requests` and `resources.concurrency_limits` in a profile are read as metadata only and do not limit concurrency. The profile and backend docs now say so, and that the backend's own concurrency settings are what limit parallelism. The rate-limit docs also stop claiming health and system endpoints have their own limits: the reference now lists which routes are rate limited and which aren't. A test now decodes the shipped configs against the full schema so a dead key fails the build. Behaviour is unchanged. Reported by @matthewjhunter.

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

The Overview herd table stays readable at phone width (the status column and figures
fit without sideways scrolling), response rate and latency show a single dash where an
endpoint has no traffic, the sparkline says "no recent traffic" instead of drawing an empty
axis, and the commit hash is shortened to 8 characters with the full hash on hover.

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
