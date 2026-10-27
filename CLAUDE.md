# CLAUDE.md

> Single source of truth for agent guidance. `AGENTS.md` is a symlink to this file - edit here only.

## Overview
Olla is a high-performance proxy and load balancer for LLM infrastructure, written in Go. It routes requests across local inference nodes (Ollama, LM Studio, LiteLLM, vLLM, vLLM-MLX, SGLang, llama.cpp, Lemonade, LMDeploy, Docker Model Runner, Text Embeddings Inference, and OpenAI-compatible endpoints). The Anthropic Messages API is supported via passthrough or translation.

Two proxy engines: **Sherpa** (simple, maintenance-mode) and **Olla** (high-performance, where all new engine work goes). Olla is local-only - hosted API providers (Groq, Gemini, etc.) are out of scope.

Full documentation: https://thushan.github.io/olla/

## Commands
```bash
make ready           # Pre-commit gate: test-short + test-race + fmt + vet + lint + align
make ready-tools     # Tools only: fmt + vet + lint + align
make ci-web          # Frontend gate: vitest + svelte-check + font integrity
make test            # All tests          make test-race   # With race detection
make test-stress     # Stress tests       make bench        # All benchmarks
make build           # Optimised binary   make build-local  # Fast build to ./build/olla
make run             # Run                make run-debug    # Run with debug logging
make docker-build-local        # Build amd64 image locally (no goreleaser)
make docker-build-local-arm64  # Build ARM64 image locally (no goreleaser)
make ci              # Full CI pipeline locally
make help            # All targets
```
Always run `make ready` before reporting work complete or committing. `make ready` is Go-only, so also run `make ci-web` when anything under `web/` changed - CI runs it and it catches what `make ready` cannot (svelte-check type errors, frontend test failures).

Specific test patterns:
```bash
go test -v ./internal/adapter/proxy -run TestAllProxies   # or TestSherpa / TestOlla
```

## Project Structure
Hexagonal architecture - domain (`internal/core`) → infrastructure (`internal/adapter`) → application (`internal/app`):

- `main.go` - entry point
- `config.yaml` - default config; `config/config.local.yaml` - user overrides (gitignored)
- `config/profiles/*.yaml` - one profile per backend type (ollama, vllm, vllm-mlx, sglang, llamacpp, lemonade, litellm, lmdeploy, dmr, text-embeddings-inference, openai-compatible). `type: "openai"` aliases `openai-compatible`.
- `config/models.yaml` - model configuration
- `internal/core/` - domain layer: `domain/` (entities), `ports/` (interfaces), `constants/`
- `internal/adapter/` - infrastructure: `balancer/`, `health/` (+ circuit breakers), `proxy/` (`sherpa/`, `olla/`, shared `core/`), `registry/`, `translator/` (OpenAI ↔ provider), `unifier/`, `stats/`, `security/`, `discovery/`, `inspector/`, `metrics/`, `converter/`, `filter/`
- `internal/app/` - `handlers/` (HTTP), `middleware/`, `services/`
- `internal/` (other) - `config/`, `router/`, `logger/`, `version/`, `integration/`, `env/`, `util/`
- `pkg/` - reusable: `container/` (DI), `eventbus/` (pub/sub), `pool/` (object pooling), `format/`, `nerdstats/`, `profiler/`
- `test/cmd/ollamock/` - multi-protocol mock backend with fault injection; `test/cmd/mockbackend/` - minimal auth-test backend
- `test/validate/` - `/olla-validate` harness configs; `test/scripts/` - e2e scenarios (auth, load, logic, platform, security, streaming)

### Key Files
- `internal/app/handlers/server_routes.go` - route registration & API setup
- `internal/app/handlers/handler_proxy.go` - request routing logic
- `internal/app/handlers/handler_translation.go` - translation handler with passthrough logic
- `internal/adapter/proxy/{sherpa,olla}/service.go` - proxy engine implementations
- `internal/adapter/translator/` - translation layer; `types.go` (PassthroughCapable interface), `anthropic/` (impl)
- `internal/adapter/stats/translator_collector.go` - translator metrics collector
- `internal/adapter/balancer/sticky.go` - sticky session wrapper
- `internal/core/constants/translator.go` - TranslatorMode and FallbackReason constants
- `internal/core/ports/stats.go` - StatsCollector interface (incl. translator tracking)
- `internal/core/domain/profile_config.go` - AnthropicSupportConfig for backend profiles
- `internal/version/version.go` - build-time version info
- `test/scripts/logic/test-model-routing.sh` - routing & header tests

## API Endpoints
**Internal** (`/internal/...`): `health`, `status`, `status/endpoints`, `status/models`, `stats/models`, `stats/translators`, `stats/sticky` (`{"enabled":false}` when off), `process`. Plus `/version`.

**Unified models**: `/olla/models` (listing with filtering), `/olla/models/{id}` (by ID or alias).

**Proxy**: `/olla/proxy/` (POST), `/olla/proxy/v1/models` (GET, OpenAI-compatible).

**Provider prefixes** (profile-driven, see `server_routes.go`): `/olla/{ollama,vllm,vllm-mlx,sglang,lmdeploy,llamacpp,lemonade,litellm,dmr,tei,text-embeddings-inference}/`, `/olla/lmstudio/` (+ `lm-studio`, `lm_studio` aliases), `/olla/openai/` and `/olla/openai-compatible/` (both served by `openai-compatible.yaml`).

**Translator** (dynamically registered): `/olla/anthropic/v1/messages` (POST, passthrough + translation), `/olla/anthropic/v1/models` (GET), `/olla/anthropic/v1/messages/count_tokens` (POST).

## Response Headers
`X-Olla-Endpoint` (backend name), `X-Olla-Model`, `X-Olla-Backend-Type` (ollama, lm-studio, litellm, vllm, vllm-mlx, sglang, llamacpp, lmdeploy, lemonade, openai, openai-compatible, docker-model-runner, omlx, text-embeddings-inference), `X-Olla-Request-ID`, `X-Olla-Response-Time`, `X-Olla-Mode` (`passthrough` or absent for translation), `X-Olla-Routing-{Strategy,Decision,Reason}`, `X-Olla-Sticky-Session` (hit/miss/repin/disabled), `X-Olla-Sticky-Key-Source`, `X-Olla-Session-ID`.

## Architecture Notes
- **Translator layer** - API format translation (e.g. OpenAI ↔ Anthropic) with passthrough optimisation. **Passthrough mode**: when a backend natively supports the Anthropic Messages API (vLLM, vLLM-MLX, SGLang, LMDeploy, llama.cpp, LM Studio, Ollama, oMLX, Docker Model Runner), requests bypass translation entirely.
- **Translator metrics** - thread-safe per-translator stats (passthrough/translation rates, fallback reasons, latency, streaming breakdown) in `stats/translator_collector.go`.
- **Sticky sessions** - optional decorator on the endpoint selector that pins multi-turn conversations to the backend that handled the first turn, maximising KV-cache reuse. 64-bit FNV-1a hashed keys, TTL + LRU bounded, purged on routable→non-routable health transitions (`balancer/sticky.go`).
- **Capability routing** - only model-less requests are narrowed by capability (vision, tools, embeddings, TEI `rerank`/`classification` from the path); a request naming a model skips it. The capabilities must all sit on one model. TEI kinds (embedding, reranker, classifier) come from `/info` `model_type` and are never `text-generation`.
- **Load balancing** - priority-based recommended for production.
- **Version management** - build-time injection via `internal/version`.

## Testing Strategy
Unit (isolation) · integration (full request flow through engines) · benchmark (balancers, engines, repos) · security (rate/size limits, `test/scripts/security/`) · stress (under load) · script (e2e, `test/scripts/`).

### Validation Harness (`/olla-validate`)
Agent-driven end-to-end validation against mock backends - no Docker, no real inference, CI-safe. Defined in `.claude/skills/olla-validate/`.

- `/olla-validate --quick` - 5–10 min gate after major changes
- `/olla-validate --nightly` - multi-hour pre-release gate (chaos, soak, Sherpa pass, forced-translation pass, benchmarks)
- No flag - prompts for depth

Gates on `make ready` first, then boots seven `test/cmd/ollamock` instances (ports 19431–19437) plus two Olla instances (`test/validate/config.validate*.yaml`, ports 41141/41142) and fans out parallel agents per area. Reports land in `test/results/`. ollamock speaks OpenAI, Ollama-native, LM Studio, Lemonade and Anthropic wire formats with runtime fault injection via `/_mock/behaviour` (see `test/cmd/ollamock/README.md`). Full docs: `docs/content/development/validation.md`.

## Development Guidelines
- **Go 1.24** - do not bump to 1.25 (see pins below).
- Australian English for comments and documentation. No em-dashes.
- Comment on **why**, not **what**. Concise and direct.
- Production code must not panic - guard closes with CAS or `sync.Once`.
- Always run `make ready` before committing, plus `make ci-web` if `web/` changed.

## Dependencies (Endorsed)
Do not add dependencies unless explicitly asked.
```go
"github.com/docker/go-units"     // Human-readable sizes
"github.com/json-iterator/go"    // High-performance JSON
"github.com/puzpuzpuz/xsync/v4"  // Concurrent maps/counters
"github.com/tidwall/gjson"       // Fast JSON parsing
"github.com/jellydator/ttlcache" // TTL cache
"github.com/rs/cors"             // CORS middleware (off by default)
"golang.org/x/sync"              // errgroup
"golang.org/x/time"              // rate limiting
```

### Go 1.24 Compatibility Pins
From the versions below onward the upstream `go` directive moves to 1.25, so these are held back. `go get -u ./...` silently bumps the toolchain by pulling them - afterwards re-pin in `go.mod`, or use `go get -u=patch ./...`.

- `golang.org/x/sys` v0.41.0 · `golang.org/x/term` v0.40.0 · `golang.org/x/text` v0.34.0
- `golang.org/x/sync` v0.19.0 · `golang.org/x/time` v0.14.0 · `atomicgo.dev/keyboard` v0.2.9

## Sub-Agent Delegation
CRITICAL: delegate work to the appropriate subagent; use the main context only for orchestration and task decomposition.

- Code review / code changes → language subagent (e.g. Go Architect) or reviewer/implementer
- Research / exploration → explore subagent
- Testing → test subagent
