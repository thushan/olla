#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Comprehensive Integration Test Script

Validates all major API endpoints and functionality against a running Olla
instance. Covers health checks, internal monitoring, unified model endpoints,
proxy (OpenAI format), Anthropic translator, provider-specific routes, response
header validation, and error handling.

Auto-discovers available backends and models to dynamically build the test
matrix, giving confidence there are no major regressions before release.
"""

import sys
import json
import time
import argparse

import requests
import os
from typing import Dict, List, Optional, Any, Tuple

# Fix Windows console encoding for Unicode
if sys.platform == 'win32':
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')
    os.environ['PYTHONIOENCODING'] = 'utf-8'

# ANSI colour codes
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[1;33m'
BLUE = '\033[0;34m'
PURPLE = '\033[0;35m'
CYAN = '\033[0;36m'
WHITE = '\033[1;37m'
GREY = '\033[0;37m'
RESET = '\033[0m'
BOLD = '\033[1m'

# Configuration
TARGET_URL = "http://localhost:40114"
DEFAULT_TIMEOUT = 30

# Backend types that natively support the Anthropic Messages API (passthrough)
PASSTHROUGH_TYPES = {"vllm", "vllm-mlx", "lm-studio", "ollama", "llamacpp", "lemonade"}
# Backend types that require translation (Anthropic -> OpenAI -> backend -> OpenAI -> Anthropic)
TRANSLATION_TYPES = {"openai-compatible", "litellm"}

# Anthropic SSE event types expected in well-formed streaming responses
ANTHROPIC_FULL_EVENTS = {
    "message_start", "content_block_start", "content_block_delta",
    "content_block_stop", "message_delta", "message_stop",
}
ANTHROPIC_MIN_EVENTS = {"message_start", "message_delta", "message_stop"}


class TestResult:
    """Outcome of a single test case."""
    __slots__ = ("name", "passed", "detail", "phase")

    def __init__(self, name: str, passed: bool, detail: str = "", phase: str = ""):
        self.name = name
        self.passed = passed
        self.detail = detail
        self.phase = phase


class IntegrationTester:
    def __init__(self, base_url: str, timeout: int, verbose: bool,
                 skip_streaming: bool, skip_anthropic: bool, skip_providers: bool):
        self.base_url = base_url
        self.timeout = timeout
        self.verbose = verbose
        self.skip_streaming = skip_streaming
        self.skip_anthropic = skip_anthropic
        self.skip_providers = skip_providers
        self.results: List[TestResult] = []
        self.endpoints: List[Dict] = []
        self.models: List[Dict] = []
        self.selected_model: Optional[str] = None
        self.passthrough_model: Optional[str] = None
        self.passthrough_backend: Optional[str] = None
        self.translation_model: Optional[str] = None
        self.translation_backend: Optional[str] = None
        self.discovered_providers: List[str] = []
        self.endpoint_types: Dict[str, str] = {}  # endpoint name -> backend type

    # -- Helpers --------------------------------------------------------------

    def pcolor(self, color: str, msg: str, end: str = '\n'):
        print(f"{color}{msg}{RESET}", end=end)
        sys.stdout.flush()

    def print_header(self):
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(PURPLE, f"  {CYAN}Olla Comprehensive Integration Test{RESET}")
        self.pcolor(PURPLE, f"  {GREY}Validates all major API endpoints and functionality{RESET}")
        self.pcolor(PURPLE, "=" * 72)
        print()

    def record(self, name: str, passed: bool, detail: str = "", phase: str = "") -> bool:
        self.results.append(TestResult(name, passed, detail, phase))
        return passed

    def _print_result(self, label: str, ok: bool, detail: str = ""):
        status = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        extra = f"  {GREY}{detail}{RESET}" if detail else ""
        print(f"  {status} {label}{extra}")

    def _get(self, path: str, **kwargs) -> Optional[requests.Response]:
        """GET helper with error handling."""
        try:
            return requests.get(f"{self.base_url}{path}", timeout=self.timeout, **kwargs)
        except Exception as e:
            self._last_error = e
            if self.verbose:
                self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
            return None

    def _post(self, path: str, json_body: Dict, **kwargs) -> Optional[requests.Response]:
        """POST helper with error handling."""
        try:
            return requests.post(
                f"{self.base_url}{path}",
                json=json_body,
                timeout=self.timeout,
                **kwargs,
            )
        except Exception as e:
            self._last_error = e
            if self.verbose:
                self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
            return None

    def _error_detail(self, r: Optional[requests.Response]) -> str:
        """Build a descriptive error string from a failed response or last exception."""
        if r is not None:
            return f"HTTP {r.status_code}"
        err = getattr(self, "_last_error", None)
        if err:
            return f"{type(err).__name__}: {err}"
        return "connection error"

    def _anthropic_headers(self) -> Dict[str, str]:
        return {
            "Content-Type": "application/json",
            "x-api-key": "test-key",
            "anthropic-version": "2023-06-01",
        }

    def _anthropic_body(self, model: str, stream: bool = False,
                        max_tokens: int = 10) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "model": model,
            "messages": [{"role": "user", "content": "Say hello briefly"}],
            "max_tokens": max_tokens,
        }
        if stream:
            body["stream"] = True
        return body

    def _phase_header(self, num: int, title: str):
        print()
        self.pcolor(WHITE, f"Phase {num}: {title}")
        self.pcolor(PURPLE, "-" * 72)

    # -- Phase 1: Health Check ------------------------------------------------

    def check_health(self) -> bool:
        self.pcolor(YELLOW, "Phase 1: Health Check")
        self.pcolor(PURPLE, "-" * 72)
        try:
            r = requests.get(f"{self.base_url}/internal/health", timeout=5)
            if r.status_code == 200:
                self.pcolor(GREEN, "  [OK] Olla is reachable")
                return True
        except Exception:
            pass
        self.pcolor(RED, f"  [FAIL] Cannot reach Olla at {self.base_url}")
        return False

    # -- Discovery (not a test phase, used to inform later phases) ------------

    def discover(self) -> bool:
        """Discover endpoints and models for use in later phases."""
        self.pcolor(YELLOW, "\nDiscovering endpoints and models...")

        # Endpoints
        r = self._get("/internal/status/endpoints")
        if r and r.status_code == 200:
            self.endpoints = r.json().get("endpoints", [])
            self.pcolor(GREEN, f"  [OK] Found {len(self.endpoints)} endpoint(s)")
        else:
            self.pcolor(RED, "  [FAIL] Could not discover endpoints")
            return False

        # Build provider list and endpoint type map
        provider_types = set()
        for ep in self.endpoints:
            ep_type = ep.get("type", "")
            ep_name = ep.get("name", "")
            if ep_type:
                provider_types.add(ep_type)
            if ep_name and ep_type:
                self.endpoint_types[ep_name] = ep_type
        self.discovered_providers = sorted(provider_types)

        # Unified models (includes availability with endpoint names)
        r = self._get("/olla/models")
        if r and r.status_code == 200:
            data = r.json()
            self.models = data.get("data", data.get("models", []))
            self.pcolor(GREEN, f"  [OK] Found {len(self.models)} model(s)")
        else:
            self.pcolor(YELLOW, "  [WARN] Could not discover models")

        # Build candidate lists for each mode using olla.availability from unified models
        import random
        all_candidates = []
        passthrough_candidates: List[Tuple[str, str]] = []  # (model, endpoint_name)
        translation_candidates: List[Tuple[str, str]] = []

        # Build healthy endpoint set for quick lookup
        healthy_endpoints = {ep.get("name") for ep in self.endpoints if ep.get("status") == "healthy"}

        for m in self.models:
            mid = m.get("id", m.get("name", ""))
            if not mid or "embed" in mid.lower():
                continue
            all_candidates.append(mid)

            # Use olla.availability to find the endpoint name for this model
            olla_meta = m.get("olla", {})
            availability = olla_meta.get("availability", [])
            for avail in availability:
                ep_name = avail.get("endpoint", "")
                if not ep_name or ep_name not in healthy_endpoints:
                    continue
                ep_type = self.endpoint_types.get(ep_name, "")

                if ep_type in PASSTHROUGH_TYPES:
                    passthrough_candidates.append((mid, ep_name))
                elif ep_type in TRANSLATION_TYPES:
                    translation_candidates.append((mid, ep_name))
                break  # Use the first healthy endpoint for this model

        # Select models: random general, plus specific for passthrough/translation
        if all_candidates:
            self.selected_model = random.choice(all_candidates)
        if passthrough_candidates:
            pt = random.choice(passthrough_candidates)
            self.passthrough_model, self.passthrough_backend = pt
        if translation_candidates:
            tr = random.choice(translation_candidates)
            self.translation_model, self.translation_backend = tr

        if self.selected_model:
            self.pcolor(GREY, f"  Selected model for general tests: {self.selected_model}")
        else:
            self.pcolor(YELLOW, "  [WARN] No suitable model found for inference tests")
        if self.passthrough_model:
            pt_type = self.endpoint_types.get(self.passthrough_backend, "?")
            self.pcolor(CYAN, f"  Passthrough model: {self.passthrough_model} "
                               f"({self.passthrough_backend}, {pt_type})")
        if self.translation_model:
            tr_type = self.endpoint_types.get(self.translation_backend, "?")
            self.pcolor(YELLOW, f"  Translation model: {self.translation_model} "
                                 f"({self.translation_backend}, {tr_type})")

        # Print discovery summary
        print()
        self.pcolor(WHITE, "  Discovered Backends")
        self.pcolor(GREY, f"  {'Name':<20} {'Type':<18} {'Status':<10} {'Models'}")
        self.pcolor(GREY, f"  {'-'*20} {'-'*18} {'-'*10} {'-'*6}")
        for ep in self.endpoints:
            name = ep.get("name", "?")
            etype = ep.get("type", "?")
            status = ep.get("status", "?")
            mcount = ep.get("model_count", 0)
            sc = GREEN if status == "healthy" else RED
            print(f"  {name:<20} {etype:<18} {sc}{status:<10}{RESET} {mcount}")

        return True

    # -- Phase 2: Internal/Monitoring Endpoints --------------------------------

    def phase_internal_endpoints(self):
        self._phase_header(2, "Internal/Monitoring Endpoints")
        phase = "internal"

        tests = [
            ("/internal/health", "Health endpoint", ["status"]),
            ("/internal/status", "Status endpoint", []),
            ("/internal/status/endpoints", "Endpoints status", ["endpoints"]),
            ("/internal/status/models", "Models status", []),
            ("/internal/stats/models", "Model statistics", []),
            ("/internal/stats/translators", "Translator statistics", []),
            ("/internal/process", "Process statistics", []),
            ("/version", "Version info", []),
        ]

        for path, label, required_keys in tests:
            r = self._get(path)
            ok = False
            detail = ""

            if r is None:
                detail = self._error_detail(r)
            elif r.status_code != 200:
                detail = f"HTTP {r.status_code}"
            else:
                try:
                    data = r.json()
                    # Check required keys if specified
                    missing = [k for k in required_keys if k not in data]
                    if missing:
                        detail = f"missing keys: {missing}"
                    else:
                        ok = True
                        if self.verbose:
                            detail = json.dumps(data, indent=2)[:200]
                except Exception:
                    detail = "invalid JSON"

            self._print_result(f"{label} ({path})", ok, detail)
            self.record(f"{phase}/{label}", ok, detail, phase)

    # -- Phase 3: Unified Model Endpoints -------------------------------------

    def phase_unified_models(self):
        self._phase_header(3, "Unified Model Endpoints")
        phase = "models"

        # GET /olla/models
        r = self._get("/olla/models")
        ok = False
        detail = ""
        if r and r.status_code == 200:
            data = r.json()
            model_list = data.get("data", data.get("models", []))
            if isinstance(model_list, list):
                ok = True
                detail = f"{len(model_list)} models"
            else:
                detail = "unexpected response structure"
        else:
            detail = self._error_detail(r)

        self._print_result("Unified model list (/olla/models)", ok, detail)
        self.record(f"{phase}/list", ok, detail, phase)

        # GET /olla/models/{id} using discovered model
        if self.selected_model:
            r = self._get(f"/olla/models/{self.selected_model}")
            ok = False
            detail = ""
            if r and r.status_code == 200:
                ok = True
                detail = f"found {self.selected_model}"
            else:
                detail = self._error_detail(r)

            self._print_result(f"Model lookup (/olla/models/{{id}})", ok, detail)
            self.record(f"{phase}/lookup", ok, detail, phase)
        else:
            self.pcolor(GREY, "  [SKIP] No model available for lookup test")

    # -- Phase 4: Proxy Endpoints (OpenAI format) -----------------------------

    def phase_proxy_endpoints(self):
        self._phase_header(4, "Proxy Endpoints (OpenAI format)")
        phase = "proxy"

        # GET /olla/proxy/v1/models
        r = self._get("/olla/proxy/v1/models")
        ok = False
        detail = ""
        if r and r.status_code == 200:
            try:
                data = r.json()
                ok = True
                model_list = data.get("data", [])
                detail = f"{len(model_list)} models"
            except Exception:
                detail = "invalid JSON"
        else:
            detail = self._error_detail(r)

        self._print_result("OpenAI model list (/olla/proxy/v1/models)", ok, detail)
        self.record(f"{phase}/models", ok, detail, phase)

        if not self.selected_model:
            self.pcolor(GREY, "  [SKIP] No model for inference tests")
            return

        # POST non-streaming chat completion via proxy
        body = {
            "model": self.selected_model,
            "messages": [{"role": "user", "content": "Say hello briefly"}],
            "max_tokens": 10,
            "stream": False,
        }
        r = self._post("/olla/proxy/v1/chat/completions", body)
        ok = False
        detail = ""
        if r and r.status_code == 200:
            try:
                data = r.json()
                ok = True
                # Show token usage if present
                usage = data.get("usage", {})
                if usage:
                    detail = (f"prompt={usage.get('prompt_tokens', '?')}, "
                              f"completion={usage.get('completion_tokens', '?')}, "
                              f"total={usage.get('total_tokens', '?')}")
                else:
                    detail = "no usage block in response"
            except Exception:
                detail = "invalid JSON response"
        else:
            detail = self._error_detail(r)

        self._print_result("Non-streaming chat completion", ok, detail)
        self.record(f"{phase}/chat-nonstream", ok, detail, phase)

        # Validate token usage is present in response
        if r and r.status_code == 200:
            try:
                data = r.json()
                usage = data.get("usage", {})
                has_prompt = isinstance(usage.get("prompt_tokens"), int) and usage["prompt_tokens"] > 0
                has_completion = isinstance(usage.get("completion_tokens"), int) and usage["completion_tokens"] >= 0
                has_total = isinstance(usage.get("total_tokens"), int) and usage["total_tokens"] > 0
                ok = has_prompt and has_completion and has_total
                if ok:
                    detail = (f"prompt_tokens={usage['prompt_tokens']}, "
                              f"completion_tokens={usage['completion_tokens']}, "
                              f"total_tokens={usage['total_tokens']}")
                else:
                    missing = []
                    if not has_prompt:
                        missing.append("prompt_tokens")
                    if not has_completion:
                        missing.append("completion_tokens")
                    if not has_total:
                        missing.append("total_tokens")
                    detail = f"missing or invalid: {missing}, usage={usage}"
            except Exception:
                ok = False
                detail = "could not parse usage from response"

            self._print_result("Token usage in non-streaming response", ok, detail)
            self.record(f"{phase}/token-usage-nonstream", ok, detail, phase)

        # POST streaming chat completion via proxy
        if not self.skip_streaming:
            body_stream = {
                "model": self.selected_model,
                "messages": [{"role": "user", "content": "Say hello briefly"}],
                "max_tokens": 10,
                "stream": True,
                "stream_options": {"include_usage": True},
            }
            r = self._post("/olla/proxy/v1/chat/completions", body_stream, stream=True)
            ok = False
            detail = ""
            stream_usage = {}
            if r and r.status_code == 200:
                has_data = False
                has_done = False
                for line in r.iter_lines(decode_unicode=True):
                    if not line:
                        continue
                    if line.startswith("data: "):
                        payload = line[6:].strip()
                        has_data = True
                        if payload == "[DONE]":
                            has_done = True
                        else:
                            try:
                                chunk = json.loads(payload)
                                chunk_usage = chunk.get("usage")
                                if chunk_usage:
                                    stream_usage = chunk_usage
                            except (json.JSONDecodeError, ValueError):
                                pass
                ok = has_data
                if not has_data:
                    detail = "no data: lines in SSE stream"
                elif not has_done:
                    detail = "stream completed (no [DONE] marker)"
            else:
                detail = self._error_detail(r)

            self._print_result("Streaming chat completion (SSE)", ok, detail)
            self.record(f"{phase}/chat-stream", ok, detail, phase)

            # Validate token usage in streaming response
            if r and r.status_code == 200:
                if stream_usage:
                    has_prompt = isinstance(stream_usage.get("prompt_tokens"), int) and stream_usage["prompt_tokens"] > 0
                    has_completion = isinstance(stream_usage.get("completion_tokens"), int) and stream_usage["completion_tokens"] >= 0
                    has_total = isinstance(stream_usage.get("total_tokens"), int) and stream_usage["total_tokens"] > 0
                    ok = has_prompt and has_completion and has_total
                    if ok:
                        detail = (f"prompt_tokens={stream_usage['prompt_tokens']}, "
                                  f"completion_tokens={stream_usage['completion_tokens']}, "
                                  f"total_tokens={stream_usage['total_tokens']}")
                    else:
                        missing = []
                        if not has_prompt:
                            missing.append("prompt_tokens")
                        if not has_completion:
                            missing.append("completion_tokens")
                        if not has_total:
                            missing.append("total_tokens")
                        detail = f"missing or invalid: {missing}, usage={stream_usage}"
                else:
                    ok = False
                    detail = "no usage block in streaming response (stream_options.include_usage may not be supported)"

                self._print_result("Token usage in streaming response", ok, detail)
                self.record(f"{phase}/token-usage-stream", ok, detail, phase)

        # Response header validation on a proxy request
        r = self._post("/olla/proxy/v1/chat/completions", {
            "model": self.selected_model,
            "messages": [{"role": "user", "content": "Hi"}],
            "max_tokens": 5,
            "stream": False,
        })
        ok = False
        detail = ""
        if r and r.status_code == 200:
            expected_headers = ["X-Olla-Endpoint", "X-Olla-Model",
                                "X-Olla-Backend-Type", "X-Olla-Request-ID"]
            present = [h for h in expected_headers if r.headers.get(h)]
            missing = [h for h in expected_headers if not r.headers.get(h)]
            ok = len(missing) == 0
            if missing:
                detail = f"missing headers: {missing}"
            else:
                detail = f"all {len(present)} headers present"
        else:
            detail = self._error_detail(r)

        self._print_result("Response headers (X-Olla-*)", ok, detail)
        self.record(f"{phase}/headers", ok, detail, phase)

    # -- Phase 5: Anthropic Translator Endpoints ------------------------------

    def phase_anthropic_translator(self):
        self._phase_header(5, "Anthropic Translator Endpoints")
        phase = "anthropic"

        if self.skip_anthropic:
            self.pcolor(GREY, "  [SKIP] Anthropic tests skipped via --skip-anthropic")
            return

        headers = {
            "Content-Type": "application/json",
            "x-api-key": "test-key",
            "anthropic-version": "2023-06-01",
        }

        # GET /olla/anthropic/v1/models
        r = self._get("/olla/anthropic/v1/models")
        ok = False
        detail = ""
        if r and r.status_code == 200:
            try:
                r.json()
                ok = True
            except Exception:
                detail = "invalid JSON"
        elif r and r.status_code == 404:
            detail = "endpoint not configured (404)"
            self.pcolor(YELLOW, f"  [SKIP] Anthropic translator not configured, skipping phase")
            self.record(f"{phase}/models", False, detail, phase)
            return
        else:
            detail = self._error_detail(r)

        self._print_result("Anthropic model list (/olla/anthropic/v1/models)", ok, detail)
        self.record(f"{phase}/models", ok, detail, phase)

        if not self.selected_model:
            self.pcolor(GREY, "  [SKIP] No model for Anthropic inference tests")
            return

        # POST /olla/anthropic/v1/messages non-streaming
        body = {
            "model": self.selected_model,
            "messages": [{"role": "user", "content": "Say hello briefly"}],
            "max_tokens": 10,
        }
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=headers, json=body, timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            if self.verbose:
                self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
            r = None

        ok = False
        detail = ""
        mode_header = ""
        if r and r.status_code == 200:
            try:
                r.json()
                ok = True
                mode_header = r.headers.get("X-Olla-Mode", "")
                if mode_header:
                    detail = f"mode={mode_header}"
            except Exception:
                detail = "invalid JSON"
        else:
            detail = self._error_detail(r)

        self._print_result("Non-streaming messages", ok, detail)
        self.record(f"{phase}/messages-nonstream", ok, detail, phase)

        # Passthrough mode header check
        if r and r.status_code == 200:
            mode = r.headers.get("X-Olla-Mode", "")
            # We just verify the header is either "passthrough" or absent (translation)
            ok = True
            detail = f"X-Olla-Mode={mode}" if mode else "no X-Olla-Mode (translation mode)"
            self._print_result("Passthrough mode check", ok, detail)
            self.record(f"{phase}/passthrough-check", ok, detail, phase)

        # POST /olla/anthropic/v1/messages streaming
        if not self.skip_streaming:
            body_stream = {
                "model": self.selected_model,
                "messages": [{"role": "user", "content": "Say hello briefly"}],
                "max_tokens": 10,
                "stream": True,
            }
            try:
                r = requests.post(
                    f"{self.base_url}/olla/anthropic/v1/messages",
                    headers=headers, json=body_stream, timeout=self.timeout,
                    stream=True,
                )
            except Exception as e:
                self._last_error = e
                if self.verbose:
                    self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
                r = None

            ok = False
            detail = ""
            if r and r.status_code == 200:
                ct = r.headers.get("Content-Type", "")
                event_count = 0
                for line in r.iter_lines(decode_unicode=True):
                    if line and line.startswith("data: "):
                        event_count += 1

                if "text/event-stream" not in ct:
                    detail = f"unexpected content-type: {ct}"
                elif event_count == 0:
                    detail = "no SSE data events"
                else:
                    ok = True
                    detail = f"{event_count} SSE events"
            else:
                detail = self._error_detail(r)

            self._print_result("Streaming messages (SSE)", ok, detail)
            self.record(f"{phase}/messages-stream", ok, detail, phase)

        # POST /olla/anthropic/v1/messages/count_tokens
        token_body = {
            "model": self.selected_model,
            "messages": [{"role": "user", "content": "Count these tokens please"}],
        }
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages/count_tokens",
                headers=headers, json=token_body, timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            if self.verbose:
                self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
            r = None

        ok = False
        detail = ""
        if r and r.status_code == 200:
            try:
                data = r.json()
                ok = True
                token_count = data.get("input_tokens", "?")
                detail = f"input_tokens={token_count}"
            except Exception:
                detail = "invalid JSON"
        elif r and r.status_code == 404:
            # Token counting may not be implemented for all backends
            ok = True
            detail = "not implemented (404) - acceptable"
        else:
            detail = self._error_detail(r)

        self._print_result("Token count estimation", ok, detail)
        self.record(f"{phase}/count-tokens", ok, detail, phase)

    # -- Phase 6: Passthrough/Translation Mode Validation --------------------

    def phase_passthrough_translation(self):
        self._phase_header(6, "Passthrough/Translation Mode Validation")
        phase = "passthrough"

        if self.skip_anthropic:
            self.pcolor(GREY, "  [SKIP] Skipped via --skip-anthropic")
            return

        has_pt = self.passthrough_model is not None
        has_tr = self.translation_model is not None

        if not has_pt and not has_tr:
            self.pcolor(GREY, "  [SKIP] No passthrough or translation backends with models")
            return

        # -- Passthrough mode tests --
        if has_pt:
            pt_type = self.endpoint_types.get(self.passthrough_backend, "?")
            self.pcolor(WHITE, f"  Passthrough: {CYAN}{self.passthrough_model}{RESET} "
                               f"({self.passthrough_backend}, {pt_type})")

            # Non-streaming passthrough
            self._test_mode_nonstreaming(
                model=self.passthrough_model,
                expected_mode="passthrough",
                label="passthrough/nonstream",
                phase=phase,
            )

            # Streaming passthrough
            if not self.skip_streaming:
                self._test_mode_streaming(
                    model=self.passthrough_model,
                    expected_mode="passthrough",
                    label="passthrough/stream",
                    phase=phase,
                )

            # Passthrough response structure validation
            self._test_passthrough_response_structure(
                model=self.passthrough_model,
                label="passthrough/response-structure",
                phase=phase,
            )
        else:
            self.pcolor(GREY, "  [SKIP] No healthy passthrough-capable backends with models")

        # -- Translation mode tests --
        if has_tr:
            tr_type = self.endpoint_types.get(self.translation_backend, "?")
            self.pcolor(WHITE, f"  Translation: {YELLOW}{self.translation_model}{RESET} "
                               f"({self.translation_backend}, {tr_type})")

            # Non-streaming translation
            self._test_mode_nonstreaming(
                model=self.translation_model,
                expected_mode="translation",
                label="translation/nonstream",
                phase=phase,
            )

            # Streaming translation
            if not self.skip_streaming:
                self._test_mode_streaming(
                    model=self.translation_model,
                    expected_mode="translation",
                    label="translation/stream",
                    phase=phase,
                )

            # Translation response structure validation
            self._test_translation_response_structure(
                model=self.translation_model,
                label="translation/response-structure",
                phase=phase,
            )
        else:
            self.pcolor(GREY, "  [SKIP] No healthy translation-required backends with models")

        # -- Translator stats validation --
        if has_pt or has_tr:
            self._test_translator_stats(has_pt, has_tr, phase)

    def _test_mode_nonstreaming(self, model: str, expected_mode: str,
                                label: str, phase: str):
        """Test that non-streaming Anthropic request uses the expected mode."""
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(model),
                timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            r = None

        ok = False
        detail = ""

        if r is None:
            detail = self._error_detail(r)
        elif r.status_code != 200:
            detail = f"HTTP {r.status_code}"
        else:
            mode = r.headers.get("X-Olla-Mode", "")
            endpoint = r.headers.get("X-Olla-Endpoint", "")

            if expected_mode == "passthrough":
                ok = mode == "passthrough"
                if not ok:
                    detail = f"expected X-Olla-Mode=passthrough, got '{mode}'"
                else:
                    detail = f"mode=passthrough, endpoint={endpoint}"
            else:
                # Translation: X-Olla-Mode should be absent
                ok = mode != "passthrough"
                if not ok:
                    detail = "unexpected X-Olla-Mode=passthrough for translation backend"
                else:
                    detail = f"mode=translation (no header), endpoint={endpoint}"

        self._print_result(f"{expected_mode.title()} non-streaming", ok, detail)
        self.record(f"{phase}/{label}", ok, detail, phase)

    def _test_mode_streaming(self, model: str, expected_mode: str,
                             label: str, phase: str):
        """Test that streaming Anthropic request uses the expected mode and SSE format."""
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(model, stream=True),
                timeout=self.timeout,
                stream=True,
            )
        except Exception as e:
            self._last_error = e
            r = None

        ok = False
        detail = ""
        notes = []

        if r is None:
            detail = self._error_detail(r)
        elif r.status_code != 200:
            detail = f"HTTP {r.status_code}"
        else:
            mode = r.headers.get("X-Olla-Mode", "")
            ct = r.headers.get("Content-Type", "")

            # Mode check
            if expected_mode == "passthrough":
                if mode != "passthrough":
                    notes.append(f"expected X-Olla-Mode=passthrough, got '{mode}'")
            else:
                if mode == "passthrough":
                    notes.append("unexpected X-Olla-Mode=passthrough")

            # Content-Type check
            if "text/event-stream" not in ct:
                notes.append(f"content-type={ct}, expected text/event-stream")

            # Parse SSE events
            event_types = set()
            event_count = 0
            for line in r.iter_lines(decode_unicode=True):
                if not line:
                    continue
                if line.startswith("event: "):
                    event_types.add(line[7:].strip())
                if line.startswith("data: "):
                    event_count += 1

            # Validate event types
            missing = ANTHROPIC_MIN_EVENTS - event_types
            if missing:
                notes.append(f"missing events: {missing}")

            if event_count == 0:
                notes.append("no SSE data events")

            ok = len(notes) == 0
            if ok:
                detail = f"{event_count} events, {len(event_types)} types"
            else:
                detail = " | ".join(notes)

        self._print_result(f"{expected_mode.title()} streaming", ok, detail)
        self.record(f"{phase}/{label}", ok, detail, phase)

    def _test_passthrough_response_structure(self, model: str, label: str, phase: str):
        """Verify passthrough response has native Anthropic structure."""
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(model),
                timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            r = None

        ok = False
        detail = ""

        if r is None:
            detail = self._error_detail(r)
        elif r.status_code != 200:
            detail = f"HTTP {r.status_code}"
        else:
            try:
                data = r.json()
                notes = []
                # Anthropic response should have: id, type, role, content, model, usage
                if data.get("type") != "message":
                    notes.append(f"type={data.get('type', 'missing')}")
                if data.get("role") != "assistant":
                    notes.append(f"role={data.get('role', 'missing')}")
                if not isinstance(data.get("content"), list):
                    notes.append("content not a list")
                if not data.get("model"):
                    notes.append("model missing")

                ok = len(notes) == 0
                if ok:
                    usage = data.get("usage", {})
                    detail = (f"type=message, role=assistant, "
                              f"input={usage.get('input_tokens', '?')}/"
                              f"output={usage.get('output_tokens', '?')} tokens")
                else:
                    detail = " | ".join(notes)
            except Exception:
                detail = "invalid JSON"

        self._print_result("Passthrough response structure", ok, detail)
        self.record(f"{phase}/{label}", ok, detail, phase)

    def _test_translation_response_structure(self, model: str, label: str, phase: str):
        """Verify translation response has valid Anthropic-format structure."""
        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(model),
                timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            r = None

        ok = False
        detail = ""

        if r is None:
            detail = self._error_detail(r)
        elif r.status_code != 200:
            detail = f"HTTP {r.status_code}"
        else:
            try:
                data = r.json()
                notes = []
                # Translated response should also follow Anthropic format
                if data.get("type") != "message":
                    notes.append(f"type={data.get('type', 'missing')}")
                if data.get("role") != "assistant":
                    notes.append(f"role={data.get('role', 'missing')}")
                if not isinstance(data.get("content"), list):
                    notes.append("content not a list")
                # Check content has text blocks
                content = data.get("content", [])
                has_text = any(c.get("type") == "text" for c in content if isinstance(c, dict))
                if content and not has_text:
                    notes.append("no text content blocks")

                ok = len(notes) == 0
                if ok:
                    detail = "valid Anthropic format (translated)"
                else:
                    detail = " | ".join(notes)
            except Exception:
                detail = "invalid JSON"

        self._print_result("Translation response structure", ok, detail)
        self.record(f"{phase}/{label}", ok, detail, phase)

    def _test_translator_stats(self, had_passthrough: bool, had_translation: bool,
                               phase: str):
        """Verify translator stats reflect the requests we just made."""
        r = self._get("/internal/stats/translators")
        ok = False
        detail = ""

        if r is None or r.status_code != 200:
            detail = self._error_detail(r)
            self._print_result("Translator stats validation", ok, detail)
            self.record(f"{phase}/stats", ok, detail, phase)
            return

        try:
            data = r.json()
            translators = data.get("translators", [])
            # Find the anthropic translator
            anthro = None
            for t in translators:
                if "anthropic" in t.get("translator_name", "").lower():
                    anthro = t
                    break

            if not anthro:
                detail = "no anthropic translator in stats"
            else:
                notes = []
                total = anthro.get("total_requests", 0)
                pt_req = anthro.get("passthrough_requests", 0)
                tr_req = anthro.get("translation_requests", 0)
                pt_rate = anthro.get("passthrough_rate", "")

                if total == 0:
                    notes.append("total_requests=0 (expected > 0)")
                if had_passthrough and pt_req == 0:
                    notes.append("passthrough_requests=0 (expected > 0)")
                if had_translation and tr_req == 0:
                    notes.append("translation_requests=0 (expected > 0)")

                ok = len(notes) == 0
                if ok:
                    detail = (f"total={total}, passthrough={pt_req}, "
                              f"translation={tr_req}, rate={pt_rate}")
                else:
                    detail = " | ".join(notes)
        except Exception:
            detail = "invalid JSON"

        self._print_result("Translator stats validation", ok, detail)
        self.record(f"{phase}/stats", ok, detail, phase)

    # -- Phase 7: Provider-Specific Routes -----------------------------------

    def phase_provider_routes(self):
        self._phase_header(7, "Provider-Specific Routes")
        phase = "providers"

        if self.skip_providers:
            self.pcolor(GREY, "  [SKIP] Provider tests skipped via --skip-providers")
            return

        if not self.discovered_providers:
            self.pcolor(GREY, "  [SKIP] No providers discovered")
            return

        for provider in self.discovered_providers:
            # Try the provider-specific models endpoint
            # The URL pattern is /olla/{provider}/v1/models for most providers
            provider_slug = provider
            # Normalise some common provider type names to URL slugs
            slug_map = {
                "openai-compatible": "openai",
                "lm-studio": "lm-studio",
                "llamacpp": "llamacpp",
                "vllm-mlx": "vllm-mlx",
            }
            provider_slug = slug_map.get(provider, provider)

            r = self._get(f"/olla/{provider_slug}/v1/models")
            ok = False
            detail = ""

            if r and r.status_code == 200:
                try:
                    data = r.json()
                    model_list = data.get("data", data.get("models", []))
                    ok = True
                    detail = f"{len(model_list)} models"
                except Exception:
                    detail = "invalid JSON"
            elif r and r.status_code == 404:
                # Provider route may not exist for all types
                ok = True
                detail = "no dedicated route (404) - acceptable"
            else:
                detail = self._error_detail(r)

            self._print_result(f"{provider} models (/olla/{provider_slug}/v1/models)", ok, detail)
            self.record(f"{phase}/{provider}", ok, detail, phase)

    # -- Phase 7: Response Header Validation (cross-cutting) ------------------

    def phase_response_headers(self):
        self._phase_header(8, "Response Header Validation")
        phase = "headers"

        if not self.selected_model:
            self.pcolor(GREY, "  [SKIP] No model for header validation")
            return

        # Check version endpoint has expected structure
        r = self._get("/version")
        ok = False
        detail = ""
        if r and r.status_code == 200:
            try:
                data = r.json()
                has_version = "version" in data or "Version" in data
                has_commit = "commit" in data or "Commit" in data
                ok = has_version or has_commit
                if not ok:
                    detail = f"missing version/commit fields, keys: {list(data.keys())}"
                else:
                    detail = "version and commit fields present"
            except Exception:
                detail = "invalid JSON"
        else:
            detail = self._error_detail(r)

        self._print_result("Version response structure", ok, detail)
        self.record(f"{phase}/version-fields", ok, detail, phase)

        # Verify X-Olla-Response-Time header on proxy request
        r = self._post("/olla/proxy/v1/chat/completions", {
            "model": self.selected_model,
            "messages": [{"role": "user", "content": "Hi"}],
            "max_tokens": 5,
            "stream": False,
        })
        ok = False
        detail = ""
        if r and r.status_code == 200:
            resp_time = r.headers.get("X-Olla-Response-Time", "")
            req_id = r.headers.get("X-Olla-Request-ID", "")
            ok = bool(req_id)
            parts = []
            if req_id:
                parts.append(f"request-id={req_id[:16]}...")
            if resp_time:
                parts.append(f"response-time={resp_time}")
            detail = ", ".join(parts) if parts else "missing tracking headers"
        else:
            detail = self._error_detail(r)

        self._print_result("Request tracking headers", ok, detail)
        self.record(f"{phase}/tracking", ok, detail, phase)

    # -- Phase 9: Error Handling ----------------------------------------------

    def phase_error_handling(self):
        self._phase_header(9, "Error Handling")
        phase = "errors"

        # Non-existent model
        r = self._post("/olla/proxy/v1/chat/completions", {
            "model": "nonexistent-model-xyz-999",
            "messages": [{"role": "user", "content": "Hello"}],
            "max_tokens": 5,
        })
        ok = False
        detail = ""
        # Note: use `is not None` not `if r:` because requests.Response.__bool__
        # returns False for 4xx/5xx status codes, which are the expected outcomes here
        if r is not None:
            ok = 400 <= r.status_code < 600
            detail = f"HTTP {r.status_code}"
        else:
            detail = self._error_detail(r)

        self._print_result("Non-existent model returns error", ok, detail)
        self.record(f"{phase}/nonexistent-model", ok, detail, phase)

        # Invalid request body (not valid JSON structure for chat)
        try:
            r = requests.post(
                f"{self.base_url}/olla/proxy/v1/chat/completions",
                data="this is not json",
                headers={"Content-Type": "application/json"},
                timeout=self.timeout,
            )
        except Exception as e:
            self._last_error = e
            if self.verbose:
                self.pcolor(GREY, f"    [{type(e).__name__}] {e}")
            r = None

        ok = False
        detail = ""
        if r is not None:
            ok = 400 <= r.status_code < 600
            detail = f"HTTP {r.status_code}"
        else:
            detail = self._error_detail(r)

        self._print_result("Invalid request body returns error", ok, detail)
        self.record(f"{phase}/invalid-body", ok, detail, phase)

        # Missing model field - Olla may forward to a backend which handles it
        # gracefully, so accept either an error (4xx/5xx) or a successful response
        r = self._post("/olla/proxy/v1/chat/completions", {
            "messages": [{"role": "user", "content": "Hello"}],
            "max_tokens": 5,
        })
        ok = False
        detail = ""
        if r is not None:
            # A missing model field may be handled gracefully by the backend
            # (returning 200 with a default model), or rejected - both are valid
            ok = r.status_code < 500 or r.status_code in (502, 503)
            detail = f"HTTP {r.status_code}"
        else:
            detail = self._error_detail(r)

        self._print_result("Missing model field handled", ok, detail)
        self.record(f"{phase}/missing-model", ok, detail, phase)

    # -- Phase 10: Summary ----------------------------------------------------

    def print_summary(self) -> bool:
        print()
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(WHITE, f"  {BOLD}Results Summary{RESET}")
        self.pcolor(PURPLE, "=" * 72)

        # Group results by phase
        phases = {}
        for r in self.results:
            phase = r.phase or "other"
            phases.setdefault(phase, []).append(r)

        phase_labels = {
            "internal": "Internal/Monitoring",
            "models": "Unified Models",
            "proxy": "Proxy (OpenAI)",
            "anthropic": "Anthropic Translator",
            "passthrough": "Passthrough/Translation Mode",
            "providers": "Provider Routes",
            "headers": "Response Headers",
            "errors": "Error Handling",
        }

        for phase_key, label in phase_labels.items():
            if phase_key not in phases:
                continue
            results = phases[phase_key]
            passed = sum(1 for r in results if r.passed)
            total = len(results)
            color = GREEN if passed == total else (YELLOW if passed > 0 else RED)
            print(f"  {color}{passed}/{total}{RESET}  {label}")

            # Show failures
            for r in results:
                if not r.passed:
                    self.pcolor(RED, f"        [FAIL] {r.name}: {r.detail}")

        # Totals
        passed = sum(1 for r in self.results if r.passed)
        failed = sum(1 for r in self.results if not r.passed)
        total = len(self.results)

        print()
        self.pcolor(GREY, f"  Total: {total}  |  ", end="")
        self.pcolor(GREEN, f"Passed: {passed}", end="")
        self.pcolor(GREY, "  |  ", end="")
        self.pcolor(RED if failed else GREEN, f"Failed: {failed}")
        print()

        if failed == 0:
            self.pcolor(GREEN, "  All tests passed.")
        else:
            self.pcolor(RED, f"  {failed} test(s) failed.")

        return failed == 0


def main():
    parser = argparse.ArgumentParser(
        description="Olla comprehensive integration test - validates all major API endpoints"
    )
    parser.add_argument("--url", default=TARGET_URL,
                        help=f"Olla base URL (default: {TARGET_URL})")
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT,
                        help=f"Request timeout in seconds (default: {DEFAULT_TIMEOUT})")
    parser.add_argument("--skip-streaming", action="store_true",
                        help="Skip streaming tests")
    parser.add_argument("--skip-anthropic", action="store_true",
                        help="Skip Anthropic translator tests")
    parser.add_argument("--skip-providers", action="store_true",
                        help="Skip provider-specific route tests")
    parser.add_argument("--verbose", action="store_true",
                        help="Show response bodies")

    args = parser.parse_args()

    tester = IntegrationTester(
        base_url=args.url,
        timeout=args.timeout,
        verbose=args.verbose,
        skip_streaming=args.skip_streaming,
        skip_anthropic=args.skip_anthropic,
        skip_providers=args.skip_providers,
    )
    tester.print_header()

    # Phase 1: Health check (gate)
    if not tester.check_health():
        sys.exit(1)

    # Discovery (informational, not scored)
    if not tester.discover():
        sys.exit(1)

    # Phase 2-9: Test phases
    tester.phase_internal_endpoints()
    tester.phase_unified_models()
    tester.phase_proxy_endpoints()
    tester.phase_anthropic_translator()
    tester.phase_passthrough_translation()
    tester.phase_provider_routes()
    tester.phase_response_headers()
    tester.phase_error_handling()

    # Phase 10: Summary
    all_pass = tester.print_summary()
    sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)
