# Area: tei-routing

Validates the routing stages that depend on what a request is rather than which
route it arrived on: the path/profile filter, the capability filter for
model-less requests, and model routing, over a fleet with three TEI kinds, two
Ollama versions and vLLM, SGLang and LMDeploy hosts. Target: olla-kinds on
`http://127.0.0.1:41143` (`test/validate/config.validate.kinds.yaml`). **Nightly
only; not part of `--quick`.** **Read-only - never call `POST /_mock/behaviour` or
`/_mock/reset`.** `GET /_mock/stats` is allowed. This instance and its eight mocks
are separate from the main fleet.

| Port | Endpoint (type) | Mock marker | Models | Notes |
|---|---|---|---|---|
| 19438 | mock-tei-embed (text-embeddings-inference) | mock-tei-embed | BAAI/bge-small-en-v1.5 | `/info` model_type `embedding` |
| 19439 | mock-tei-rerank (text-embeddings-inference) | mock-tei-rerank | BAAI/bge-reranker-base | model_type `reranker` |
| 19440 | mock-tei-class (text-embeddings-inference) | mock-tei-class | SamLowe/roberta-go-emotions | model_type `classifier` |
| 19441 | mock-ollama-h (ollama) | mock-ollama-h | llama3.2:3b, nomic-embed-text:latest, qwen3-vl:8b | `/api/tags` carries `capabilities` (qwen3-vl: completion, vision, tools); loaded: llama3.2:3b |
| 19442 | mock-ollama-i (ollama) | mock-ollama-i | qwen3:8b, mxbai-embed-large | old Ollama: tags carry no `capabilities`; loaded: qwen3:8b |
| 19443 | mock-vllm-j (vllm) | mock-vllm-j | intfloat/e5-base-v2, vllm-chat-model | |
| 19444 | mock-sglang-k (sglang) | mock-sglang-k | sglang-chat-model | |
| 19445 | mock-lmdeploy-l (lmdeploy) | mock-lmdeploy-l | lmdeploy-chat-model | |

Every response carries `X-Olla-Endpoint` (Olla's view). Responses that Olla passes
through also keep `X-Ollamock-Instance` (the mock's own marker): assert the two
agree where it is present. Translated replies (the sglang and lmdeploy rows of
check 6 on main) drop it, so use the `BACKEND:<mock>` text there. The mocks enforce TEI's kind rules, so
a request routed to the wrong TEI kind fails loudly with 424 `error_type: "Backend"`
rather than quietly succeeding.

Headers for every call: `Content-Type: application/json`, except where a check says
otherwise (check 5 deliberately sends curl's default). Anthropic calls also
send `anthropic-version: 2023-06-01` and `x-api-key: validate`.

The checklist has two sections. **Section A** (checks 1-12) holds on current
main and a failure there is a FAIL. **Section B** (checks B1-B7) needs work that
is built on other branches and may not be merged yet; see its probes. Never
count a Section B check as a failure because its feature is absent: record a skip
naming the missing item.

## Section A: holds on main

### Quick

1. **Inventory.** `GET /internal/status/endpoints` lists eight endpoints, all
   healthy. `GET /olla/models` lists exactly these ids: BAAI/bge-small-en-v1.5,
   BAAI/bge-reranker-base, SamLowe/roberta-go-emotions, intfloat/e5-base-v2,
   nomic-embed-text:latest, mxbai-embed-large, llama3.2:3b, qwen3-vl:8b,
   qwen3:8b, vllm-chat-model, sglang-chat-model, lmdeploy-chat-model.
   `nomic-embed-text:latest` is listed with its tag, as a real Ollama does, and
   every request below asks for it bare: a bare name must resolve to the
   `:latest` entry. (Their `type` values are B1.)
2. **Ollama native embed.** `POST /olla/proxy/api/embed`
   `{"model":"nomic-embed-text","input":"hi"}` → 200, served by mock-ollama-h,
   body has `embeddings` (array of arrays). Model-less
   `{"input":"hi"}` ×6 → every response 200 from mock-ollama-h or mock-ollama-i,
   never a TEI, vLLM, SGLang or LMDeploy host (`/api/embed` is Ollama's path
   alone; a TEI `/embed` profile path must not claim it).
3. **Model-less native TEI routing** (`/olla/proxy/`, bodies carry no `model`).
   Three requests each; every one 200 from the named endpoint:
   - `POST /olla/proxy/embed` `{"inputs":["a","b"]}` → mock-tei-embed (array of 2 vectors)
   - `POST /olla/proxy/rerank` `{"query":"q","texts":["a","b"]}` → mock-tei-rerank
   - `POST /olla/proxy/predict` `{"inputs":"hello"}` → mock-tei-class or mock-tei-rerank (TEI serves `/predict` on both classifiers and rerankers, so the mock does too; never mock-tei-embed)
   - `POST /olla/proxy/similarity` `{"inputs":{"source_sentence":"a","sentences":["b"]}}` → mock-tei-embed
4. **`/olla/tei/` prefix reaches only TEI.** 10 requests each to
   `/olla/tei/embed` (`{"inputs":"x"}`), `/olla/tei/rerank`
   (`{"query":"q","texts":["a"]}`) and `/olla/tei/predict` (`{"inputs":"x"}`):
   all 200, `X-Olla-Backend-Type: text-embeddings-inference` on every response,
   endpoints mock-tei-embed / mock-tei-rerank / (mock-tei-class or mock-tei-rerank for
   `/predict`) respectively, and never an Ollama, vLLM, SGLang or LMDeploy host, and never mock-tei-embed for `/predict`.
5. **Named-model `/v1/embeddings`** on `/olla/proxy/v1/embeddings` with
   `"input":"x"`; 4 requests per model, all 200 from the same endpoint, with
   `X-Olla-Routing-Reason: model_found`:
   | model | endpoint |
   |---|---|
   | nomic-embed-text | mock-ollama-h |
   | mxbai-embed-large | mock-ollama-i |
   | intfloat/e5-base-v2 | mock-vllm-j |
   | BAAI/bge-small-en-v1.5 | mock-tei-embed |

   **Content-Type must not matter.** Repeat `nomic-embed-text` ×6 with curl's
   default (`curl -d '{"model":"nomic-embed-text","input":"x"}'`, which sends
   `application/x-www-form-urlencoded`), then with `Content-Type: text/plain` and
   with no Content-Type header at all: every response is 200 from mock-ollama-h
   with `X-Olla-Model: nomic-embed-text`, and 424 must never appear (a 424 means
   the request was routed as model-less onto a TEI host of the wrong kind).
   **Bare and tagged are one model.** `nomic-embed-text:latest` ×4 also reaches
   mock-ollama-h, and after the bare and tagged requests
   `GET /internal/stats/models` has a single `nomic-embed-text:latest` row (no
   `nomic-embed-text` row) counting both. A name no host serves, such as
   `nomic-embed-text-v2`, is not asserted on here: this fleet's optimistic routing
   falls back (`X-Olla-Routing-Reason: model_not_found_fallback`), and on main the
   fallback is unfiltered, so it can land on a TEI host of the wrong kind (see B7).
6. **Anthropic passthrough in a mixed fleet.** `POST /olla/anthropic/v1/messages`
   `{"model":"<m>","max_tokens":32,"messages":[{"role":"user","content":"ping"}]}`
   → 200, `type:"message"`, text containing `BACKEND:<mock>`, served by the
   endpoint below. `X-Olla-Mode: passthrough` is required for vllm-chat-model and
   llama3.2:3b; for the sglang and lmdeploy rows it is checked in B6.
   | model | endpoint |
   |---|---|
   | vllm-chat-model | mock-vllm-j |
   | sglang-chat-model | mock-sglang-k |
   | lmdeploy-chat-model | mock-lmdeploy-l |
   | llama3.2:3b | mock-ollama-h |

   Streaming (`"stream":true`) with `sglang-chat-model` → events in order
   `message_start`, `content_block_start`, `content_block_delta`+,
   `content_block_stop`, `message_delta`, `message_stop`.

### Nightly additions

7. **TEI error contract through Olla.** Responses pass through unmodified.
   `POST /olla/tei/embed` `{"inputs":""}` → 400, `error_type: "Empty"`;
   `{"inputs":5}` → 422 with axum's plain-text body (`Failed to deserialize the
   JSON body ...`, no JSON envelope);
   `POST /olla/tei/rerank` `{"query":"q","texts":[]}` → 400 `Empty`;
   `POST /olla/tei/predict` `{"inputs":[]}` → 400 `Empty`;
   `POST /olla/proxy/v1/embeddings` `{"model":"BAAI/bge-small-en-v1.5","input":[]}`
   → 400 with `type: "Empty"`, `message` and `code` (OpenAI-compatible
   shape; the key is `type`, not `error_type`). Each is served by the right kind of TEI endpoint.
8. **TEI tokenise and info.** `POST /olla/tei/tokenize` `{"inputs":"hello world"}`
   → 200, an array of one array of token objects (`id`, `text`, `special`).
   Any TEI mock's `GET /info` (direct to the mock on 19438-19440) reports the `model_type` in the
   table above: `model_type` is an object keyed by kind (`embedding`, `reranker` or
   `classifier`).
9. **No wrong-kind traffic.** 30 model-less requests mixing check 3's four calls
    (shuffled): every one 200. Then `GET /_mock/stats` on the three TEI mocks:
    `by_path` for mock-tei-embed has no `/rerank` or `/predict`; mock-tei-rerank
    has no `/embed`; mock-tei-class has no `/embed` or `/rerank`.
10. **Concurrency.** 50 parallel `POST /olla/proxy/rerank` → all 200 from
    mock-tei-rerank with unique `X-Olla-Request-ID` values.
11. **Provider prefixes stay in their lane.** `POST /olla/ollama/api/embed`
    (`{"model":"nomic-embed-text","input":"x"}`) ×4 → only mock-ollama-h.
    `POST /olla/vllm/v1/embeddings` (`{"model":"intfloat/e5-base-v2","input":"x"}`)
    ×4 → only mock-vllm-j.
12. **Anthropic count_tokens and models** still answer on this fleet:
    `POST /olla/anthropic/v1/messages/count_tokens` with `sglang-chat-model` → 200
    with `input_tokens` ≥ 0; `GET /olla/anthropic/v1/models` → 200 with
    `type: "model"` entries.

## Section B: requires #3 / #4 / #5 / #6 / #8 merged

These depend on work that lives on other branches. Probe first, run what the
probes allow, and record a skip (with the item number) for the rest. A tree that
has none of it must finish Section B with skips only.

| Probe | Run with | If the probe fails |
|---|---|---|
| #6 `olla.type` | `GET /olla/models`: entries carry an `olla.type` field | skip B1, B2, B3 (needs #6) |
| #4 vision/tools | `GET /olla/models`: `qwen3-vl:8b` has `olla.type` `vlm` (or `type=vlm` filter returns it) | skip B4 (needs #4) |
| #8 `/api/ps` | `GET /olla/ollama/api/ps` lists both `llama3.2:3b` and `qwen3:8b` with `olla_endpoint` on each entry (on a tree without #8 it answers 200 from a single Ollama and no `olla_endpoint`) | skip B5 (needs #8) |
| #3 native Anthropic for sglang and lmdeploy | `POST /olla/anthropic/v1/messages` for `sglang-chat-model` carries `X-Olla-Mode: passthrough` | skip B6 (needs #3; main's sglang and lmdeploy profiles have no `anthropic_support`, so they are translated) |
| #5 slash-id lookup | `GET /olla/models/BAAI/bge-reranker-base` is not 404 | skip B3 (needs #5) |
| unfiltered-fallback fix | none can be probed reliably (candidate order varies per request); run B7 only when the caller says the fix is merged | skip B7 by default (needs the unfiltered-fallback fix; known follow-up) |

B1. **Types.** `olla.type` values: BAAI/bge-small-en-v1.5, intfloat/e5-base-v2,
    nomic-embed-text:latest, mxbai-embed-large = `embeddings`;
    BAAI/bge-reranker-base = `reranker`; SamLowe/roberta-go-emotions =
    `classifier`; llama3.2:3b, qwen3:8b, vllm-chat-model, sglang-chat-model,
    lmdeploy-chat-model = `llm`. `mxbai-embed-large` is the old-Ollama case (no
    `capabilities` from its host), so its `embeddings` type must come from the
    model name.
B2. **`?type=` filters** on `/olla/models`: `type=reranker` → exactly
    [BAAI/bge-reranker-base]; `type=classifier` → exactly
    [SamLowe/roberta-go-emotions]; `type=embeddings` → exactly the four embedders
    from B1; `type=llm` → exactly the five chat models from B1, plus `qwen3-vl:8b`
    when #4 is absent (with #4 it is `vlm`, B4, and in neither set). The sets are
    disjoint.
B3. **Slash ids.** `GET /olla/models/BAAI/bge-reranker-base` → 200, `id` equal to
    the path, `type: "reranker"`, `endpoints` = [mock-tei-rerank]. Same for
    `/olla/models/intfloat/e5-base-v2` (`type: "embeddings"`, `endpoints` =
    [mock-vllm-j]). An unknown slash id such as `/olla/models/no/such-model` → 404.
    (Needs #5; skip with "needs #5" if the slash id answers 404.)
B4. **Ollama vision and tools capabilities** (mock-ollama-h lists `qwen3-vl:8b`
    with `completion, vision, tools` and `llama3.2:3b` with `completion, tools`).
    `qwen3-vl:8b` is typed `vlm` and its capabilities include `vision` and
    `function_calling`; `llama3.2:3b` is `llm` with `function_calling` and no
    `vision`. `mxbai-embed-large` (old Ollama, no capabilities) carries neither.
B5. **`/olla/ollama/api/ps` merges both Ollamas.** → 200 JSON with a `models`
    array holding two entries: `llama3.2:3b` with `olla_endpoint: "mock-ollama-h"`
    and `qwen3:8b` with `olla_endpoint: "mock-ollama-i"`. Every entry carries
    `olla_endpoint`. No `X-Olla-Partial` header (both hosts answered).

B6. **Passthrough reaches non-Ollama hosts (needs #3).** `sglang-chat-model` and
    `lmdeploy-chat-model` Anthropic requests (check 6's body) carry
    `X-Olla-Mode: passthrough`, served by mock-sglang-k and mock-lmdeploy-l.
B7. **Unknown model never reaches a TEI host (default skip).** `nomic-embed-text-v2` ×8 on
    `/olla/proxy/v1/embeddings` (check 5 body): every response is 200 or 404 and
    never 424, which would mean the fallback picked a TEI host of the wrong kind.

Not covered here because it needs fault injection: the `X-Olla-Partial` header
when one Ollama fails `/api/ps` (unit-tested in
`internal/app/handlers/handler_provider_ollama_ps_test.go`).
