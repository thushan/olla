# Sticky Session Test Scripts

Validates Olla's KV cache affinity routing (sticky sessions). Tests session pinning via explicit
session IDs, prefix hash routing for identical system prompts, session independence, key source
reporting, and disabled-state behaviour.

## What's Being Tested

| Behaviour | Header | Expected values |
|-----------|--------|----------------|
| Session state | `X-Olla-Sticky-Session` | `hit`, `miss`, `repin`, `disabled` |
| Key derivation | `X-Olla-Sticky-Key-Source` | `session_header`, `prefix_hash`, `auth_hash`, `ip`, `none` |
| Session echo | `X-Olla-Session-ID` | Echoed back when provided in request |

### Test Cases

1. **Header presence** — `X-Olla-Sticky-Session` is always set to a known value.
2. **Key source validity** — `X-Olla-Sticky-Key-Source` is always one of the known valid values (or absent when disabled).
3. **Session ID pin** — First request with `X-Olla-Session-ID` returns `miss`; second with same ID returns `hit` on the same endpoint.
4. **Session ID echo** — The `X-Olla-Session-ID` request header is echoed in the response.
5. **Prefix hash hit** — Identical system prompts (same first 512 bytes) produce a `hit` on the second request.
6. **Session independence** — Two different session IDs each pin to their own endpoint without interfering.
7. **Multi-backend (2+ backends)** — Sessions pinned across multiple backends do not cross-contaminate.

Affinity tests (3–7) are automatically skipped with an advisory message when `sticky_sessions.enabled: false`.

## Prerequisites

- Olla running and reachable (default `http://localhost:40114`)
- Python 3.8+ with `requests` installed (`pip install -r requirements.txt` from `test/scripts/`)
- At least one healthy backend with a loaded model
- `sticky_sessions.enabled: true` in `config.yaml` for affinity tests

## Enabling Sticky Sessions

Add the following to `config.yaml` under `proxy:`:

```yaml
proxy:
  sticky_sessions:
    enabled: true
    idle_ttl_seconds: 600
    max_sessions: 10000
    key_sources:
      - "session_header"
      - "prefix_hash"
      - "auth_header"
    prefix_hash_bytes: 512
```

## Usage

```bash
# Run via Makefile (recommended)
make test-script-sticky

# Custom URL
make test-script-sticky ARGS="--url http://localhost:11435"

# Custom URL and model
make test-script-sticky ARGS="--url http://localhost:11435 --model llama3.2"

# Run directly
cd test/scripts
python sticky/test-sticky-sessions.py
python sticky/test-sticky-sessions.py --url http://localhost:11435 --model phi4:latest
python sticky/test-sticky-sessions.py --verbose
python sticky/test-sticky-sessions.py --skip-stats
```

## Expected Output

```
========================================================================
  Olla Sticky Session Test
  Validates KV cache affinity routing behaviour
========================================================================

Checking Olla availability...
[OK] Olla is reachable

Discovering endpoints...
[OK] Found 2 endpoint(s)
...

Detecting sticky session status...
  [OK] Sticky sessions active (probe returned: miss)

Header Validation
========================================================================
  X-Olla-Sticky-Session present:  [PASS]  value='miss'
  X-Olla-Sticky-Key-Source valid: [PASS]  source='prefix_hash'

Affinity Tests
========================================================================
  Session ID: miss → hit:         [PASS]  endpoint=local-ollama
  Session ID echoed in response:  [PASS]  echoed='test-sticky-1234567890'
  Prefix hash: same prompt → hit: [PASS]  source=prefix_hash endpoint=local-ollama
  Two sessions pin independently: [PASS]  both sessions independently pinned

========================================================================
  Results Summary
========================================================================

  Test                                             Result
  ------------------------------------------------ ------
  header/sticky-session-present                    PASS
  header/key-source-valid                          PASS
  affinity/session-id-miss-then-hit                PASS
  affinity/session-id-echoed                       PASS
  affinity/prefix-hash-hit                         PASS
  affinity/sessions-independent                    PASS

  Total: 6  |  Passed: 6  |  Failed: 0

  All tests passed.
```

## Response Headers Reference

| Header | When Set | Values |
|--------|----------|--------|
| `X-Olla-Sticky-Session` | All proxy requests | `hit` — pinned endpoint served the request |
| | | `miss` — no existing pin, new pin created |
| | | `repin` — previous endpoint unavailable, repinned |
| | | `disabled` — feature is off |
| `X-Olla-Sticky-Key-Source` | When enabled | `session_header`, `prefix_hash`, `auth_hash`, `ip`, `none` |
| `X-Olla-Session-ID` | When provided in request | Echoed back unchanged |

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All tests passed |
| `1` | One or more tests failed, or Olla unreachable |
| `130` | Interrupted by Ctrl+C |
