---
title: Validation Harness - Agent-Driven Release Gating
description: How to validate Olla end-to-end with the olla-validate skill and the ollamock mock backend, without Docker or real inference infrastructure.
keywords: olla validation, release gate, mock backend, ollamock, regression testing
---

# Validation Harness

Beyond unit and integration tests, Olla has an agent-driven validation harness
that exercises a running Olla against a fleet of mock backends. It covers the
ground that in-process tests cannot: real HTTP routing across every provider
namespace, streaming, Anthropic translation and passthrough, sticky sessions,
health transitions, failover and recovery, and request limits.

It needs no Docker and no real inference backends, so it runs the same way on
a laptop and in CI.

## The `/olla-validate` skill

The harness is driven by a Claude Code skill at
`.claude/skills/olla-validate/`. It has two depths:

| Mode | Time | Use |
|---|---|---|
| `/olla-validate --quick` | 5–10 min | Gate after major changes |
| `/olla-validate --nightly` | 2–4 h | Pre-release gate: exhaustive checklists, chaos, soak, Sherpa pass, forced-translation pass, benchmarks |

Without a flag it asks which depth to run.

The skill pins itself to Sonnet (`model: sonnet` in its frontmatter), so it
costs the same regardless of which model the session is using. Area agents
are balanced for token efficiency: mechanical curl-and-assert checklists run
on Haiku, while anything that mutates mock state or validates protocol
sequences (resilience, the Anthropic areas) runs on Sonnet. `tei-routing` is
read-only, so it runs on Haiku.

Every run starts with `make ready` as a hard gate, then boots the mock fleet
and two Olla instances, fans out parallel validation agents (one per area
checklist in `.claude/skills/olla-validate/areas/`), and writes a report to
`test/results/olla-validate-<timestamp>.md` with a one-line history entry in
`test/results/last-runs.md`. Any failure anywhere fails the gate.

## ollamock

`test/cmd/ollamock` is a stdlib-only mock LLM backend that speaks the wire
formats of every provider Olla fronts: OpenAI chat completions (including SSE
streaming), Ollama's native `/api/*` protocol (NDJSON streaming), LM Studio,
Lemonade, Text Embeddings Inference (TEI) and the Anthropic Messages API. Responses carry real token-usage
fields so metrics extraction works, and every response is tagged with a
`BACKEND:{name}` marker plus an `X-Ollamock-Instance` header so tests can
assert exactly which backend served a request.

Fault injection is controlled at runtime through `/_mock/behaviour`: forced
error statuses, flaky error rates, hangs, slow first byte, mid-stream
connection drops, malformed JSON and health-check failure. `/_mock/stats`
exposes per-path request counters. See `test/cmd/ollamock/README.md` for
flags and examples.

## Harness topology

`test/validate/config.validate.yaml` wires seven endpoints (openai-compatible
×2, ollama, lm-studio, vllm, litellm, llamacpp) across seven ollamock
instances on ports 19431–19437 into an Olla on 41141, with the unifier,
sticky sessions and the Anthropic translator enabled. The model lists overlap
deliberately so unification and model routing are observable.
`test/validate/config.validate.limits.yaml` runs a second Olla on 41142 with
a 256KB body cap and a tight per-IP rate limit so 413/429 behaviour is
testable in seconds.

Nightly runs add a third Olla on 41143
(`test/validate/config.validate.kinds.yaml`) fronting eight further ollamock
instances on ports 19438-19445: three TEI instances (one per kind), two Ollamas
(one whose `/api/tags` reports `capabilities` like Ollama 0.30+, one that omits
them like older releases, each with a different loaded model for `/api/ps`), and
vLLM, SGLang and LMDeploy hosts. The `tei-routing` area uses it to check
path-based routing of model-less TEI requests, `/olla/tei/*` isolation,
named-model `/v1/embeddings` and Anthropic passthrough in a mixed fleet. It is
separate from the main fleet so the main checklists' endpoint counts and
per-route host sets stay fixed, and `--quick` never boots it. Its Section B
checks (model `type`, slash-id lookup, vision and tools capabilities, merged
`/api/ps`, native Anthropic passthrough on sglang and lmdeploy) need
work that may not be merged yet; each is probed first and recorded as a skip,
never a failure, when the feature is absent.

Each endpoint gets its own ollamock instance because Olla keys endpoint
identity on the URL: two endpoints sharing a URL silently collapse into one.

## Timing expectations

Health probes run on a global 30-second ticker regardless of the configured
`check_interval`, so checklists allow up to 40 seconds for probe-driven
health transitions and up to 60 seconds for recovery. Request-path connection
failures mark an endpoint unhealthy immediately, so failover assertions are
fast; only probe-driven transitions need the generous windows.
