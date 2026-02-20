#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Anthropic Passthrough Test Script

Validates that the Anthropic Messages API translator correctly uses passthrough
mode for backends with native support (vllm, vllm-mlx, lm-studio, ollama,
llamacpp) and falls back to translation for others (openai-compatible, litellm).

Auto-discovers available backends and models, then runs a test matrix covering
non-streaming, streaming, OpenAI baseline, edge cases, and translator stats.
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

# Backend types that natively support the Anthropic Messages API
PASSTHROUGH_TYPES = {"vllm", "vllm-mlx", "lm-studio", "ollama", "llamacpp", "lemonade"}
# Backend types that require translation (Anthropic -> OpenAI -> backend -> OpenAI -> Anthropic)
TRANSLATION_TYPES = {"openai-compatible", "litellm"}

# Anthropic SSE event types expected in a well-formed streaming response
PASSTHROUGH_EVENTS = {
    "message_start", "content_block_start", "content_block_delta",
    "content_block_stop", "message_delta", "message_stop",
}
TRANSLATION_MIN_EVENTS = {"message_start", "message_delta", "message_stop"}


class BackendInfo:
    """Discovered backend with its health, type, and selected model."""
    __slots__ = ("name", "backend_type", "status", "models", "selected_model")

    def __init__(self, name: str, backend_type: str, status: str):
        self.name = name
        self.backend_type = backend_type
        self.status = status
        self.models: List[str] = []
        self.selected_model: Optional[str] = None

    @property
    def expects_passthrough(self) -> bool:
        return self.backend_type in PASSTHROUGH_TYPES

    @property
    def mode_label(self) -> str:
        return "passthrough" if self.expects_passthrough else "translation"


class TestResult:
    """Outcome of a single test case."""
    __slots__ = ("name", "passed", "detail")

    def __init__(self, name: str, passed: bool, detail: str = ""):
        self.name = name
        self.passed = passed
        self.detail = detail


class PassthroughTester:
    def __init__(self, base_url: str, timeout: int, verbose: bool):
        self.base_url = base_url
        self.timeout = timeout
        self.verbose = verbose
        self.backends: List[BackendInfo] = []
        self.results: List[TestResult] = []

    # ── Helpers ─────────────────────────────────────────────────────────

    def pcolor(self, color: str, msg: str, end: str = '\n'):
        print(f"{color}{msg}{RESET}", end=end)
        sys.stdout.flush()

    def print_header(self):
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(PURPLE, f"  {CYAN}Olla Anthropic Passthrough Test{RESET}")
        self.pcolor(PURPLE, f"  {GREY}Validates passthrough vs translation mode selection{RESET}")
        self.pcolor(PURPLE, "=" * 72)
        print()

    def record(self, name: str, passed: bool, detail: str = "") -> bool:
        self.results.append(TestResult(name, passed, detail))
        return passed

    def _anthropic_headers(self) -> Dict[str, str]:
        return {
            "Content-Type": "application/json",
            "x-api-key": "test-key",
            "anthropic-version": "2023-06-01",
        }

    def _anthropic_body(self, model: str, stream: bool = False,
                        system: Optional[str] = None,
                        messages: Optional[List[Dict]] = None) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "model": model,
            "messages": messages or [{"role": "user", "content": "Say hello briefly"}],
            "max_tokens": 10,
        }
        if stream:
            body["stream"] = True
        if system:
            body["system"] = system
        return body

    # ── Discovery ───────────────────────────────────────────────────────

    def check_health(self) -> bool:
        self.pcolor(YELLOW, "Checking Olla availability...")
        try:
            r = requests.get(f"{self.base_url}/internal/health", timeout=5)
            if r.status_code == 200:
                self.pcolor(GREEN, "[OK] Olla is reachable")
                return True
        except Exception:
            pass
        self.pcolor(RED, f"[FAIL] Cannot reach Olla at {self.base_url}")
        return False

    def discover(self) -> bool:
        """Discover endpoints, map models to backends, pick one model each."""
        # Fetch endpoints
        self.pcolor(YELLOW, "Discovering endpoints...")
        try:
            r = requests.get(f"{self.base_url}/internal/status/endpoints", timeout=self.timeout)
            r.raise_for_status()
            endpoints = r.json().get("endpoints", [])
        except Exception as e:
            self.pcolor(RED, f"[FAIL] Endpoint discovery failed: {e}")
            return False

        ep_map: Dict[str, BackendInfo] = {}
        for ep in endpoints:
            name = ep.get("name", "unknown")
            btype = ep.get("type", "unknown")
            status = ep.get("status", "unknown")
            bi = BackendInfo(name, btype, status)
            ep_map[name] = bi
            self.backends.append(bi)

        if not self.backends:
            self.pcolor(RED, "[FAIL] No endpoints discovered")
            return False

        self.pcolor(GREEN, f"[OK] Found {len(self.backends)} endpoint(s)")

        # Fetch models and map to endpoints
        self.pcolor(YELLOW, "Discovering models...")
        try:
            r = requests.get(f"{self.base_url}/internal/status/models", timeout=self.timeout)
            r.raise_for_status()
            data = r.json()
            recent = data.get("recent_models", [])
        except Exception as e:
            self.pcolor(RED, f"[FAIL] Model discovery failed: {e}")
            return False

        # Build URL -> endpoint name reverse map since models reference
        # endpoints by URL, not by name.
        url_to_name: Dict[str, str] = {}
        for ep in endpoints:
            url = ep.get("url", "")
            name = ep.get("name", "")
            if url and name:
                url_to_name[url.rstrip("/")] = name

        # Also index backends by type for fallback matching
        type_to_backends: Dict[str, List[str]] = {}
        for ep in endpoints:
            btype = ep.get("type", "")
            name = ep.get("name", "")
            if btype and name:
                type_to_backends.setdefault(btype, []).append(name)

        for m in recent:
            model_name = m.get("name", "")
            model_endpoints = m.get("endpoints", [])
            model_type = m.get("type", "")
            matched = False

            # Primary: match by URL using the reverse map
            for ep_url in model_endpoints:
                normalised = ep_url.rstrip("/")
                ep_name = url_to_name.get(normalised)
                if ep_name and ep_name in ep_map:
                    ep_map[ep_name].models.append(model_name)
                    matched = True

            # Fallback: match by backend type if URL lookup missed
            if not matched and model_type and model_type in type_to_backends:
                for ep_name in type_to_backends[model_type]:
                    if ep_name in ep_map:
                        ep_map[ep_name].models.append(model_name)
                        matched = True

        # Select one non-embedding model per backend
        for bi in self.backends:
            candidates = [m for m in bi.models if "embed" not in m.lower()]
            if candidates:
                bi.selected_model = candidates[0]

        self._print_discovery_summary()
        return True

    def _print_discovery_summary(self):
        print()
        self.pcolor(WHITE, "Configuration Summary")
        self.pcolor(PURPLE, "-" * 72)
        header = f"  {'Backend':<20} {'Type':<18} {'Status':<10} {'Models':<6} {'Mode'}"
        self.pcolor(GREY, header)
        self.pcolor(GREY, f"  {'-'*20} {'-'*18} {'-'*10} {'-'*6} {'-'*14}")

        for bi in self.backends:
            status_c = GREEN if bi.status == "healthy" else RED
            mode_c = CYAN if bi.expects_passthrough else YELLOW
            model_str = str(len(bi.models))
            status_str = f"{status_c}{bi.status:<10}{RESET}"
            mode_str = f"{mode_c}{bi.mode_label:<14}{RESET}"
            print(f"  {bi.name:<20} {bi.backend_type:<18} {status_str} {model_str:<6} {mode_str}")

        pt = [b for b in self.backends if b.expects_passthrough and b.status == "healthy"]
        tr = [b for b in self.backends if not b.expects_passthrough and b.status == "healthy"]
        print()
        self.pcolor(GREY, f"  Passthrough-capable: {len(pt)}  |  Translation-required: {len(tr)}")

    # ── Test Matrix ─────────────────────────────────────────────────────

    def _testable_backends(self) -> List[BackendInfo]:
        return [b for b in self.backends if b.status == "healthy" and b.selected_model]

    def run_matrix(self, skip_streaming: bool):
        """Run the core test matrix for every testable backend."""
        testable = self._testable_backends()
        if not testable:
            self.pcolor(YELLOW, "\nNo healthy backends with models to test")
            return

        print()
        self.pcolor(WHITE, "Test Matrix")
        self.pcolor(PURPLE, "=" * 72)

        for bi in testable:
            print()
            self.pcolor(WHITE, f"Backend: {CYAN}{bi.name}{RESET}  "
                                f"type={bi.backend_type}  model={bi.selected_model}")
            self.pcolor(GREY, f"  Expected mode: {bi.mode_label}")

            self._test_anthropic_nonstreaming(bi)
            if not skip_streaming:
                self._test_anthropic_streaming(bi)
            self._test_openai_baseline(bi)

    # -- Non-streaming Anthropic ----------------------------------------

    def _test_anthropic_nonstreaming(self, bi: BackendInfo):
        label = f"{bi.name}/anthropic-nonstream"
        self.pcolor(YELLOW, f"  Non-streaming: ", end="")

        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(bi.selected_model),
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        ok = True
        notes = []

        # HTTP status
        if r.status_code != 200:
            self.pcolor(RED, f"[FAIL] HTTP {r.status_code}")
            self.record(label, False, f"HTTP {r.status_code}")
            return

        # Mode header
        mode = r.headers.get("X-Olla-Mode", "")
        if bi.expects_passthrough:
            if mode != "passthrough":
                ok = False
                notes.append(f"expected X-Olla-Mode=passthrough, got '{mode}'")
        else:
            # Translation mode: header should be absent
            if mode == "passthrough":
                ok = False
                notes.append("unexpected X-Olla-Mode=passthrough for translation backend")

        # Endpoint header
        ep = r.headers.get("X-Olla-Endpoint", "")
        if ep and ep != bi.name:
            notes.append(f"routed to {ep} (expected {bi.name})")

        # Backend type header
        bt = r.headers.get("X-Olla-Backend-Type", "")
        if bt and bt != bi.backend_type:
            notes.append(f"backend-type={bt} (expected {bi.backend_type})")

        # Valid JSON
        try:
            r.json()
        except Exception:
            ok = False
            notes.append("invalid JSON response")

        if self.verbose and ok:
            self.pcolor(GREY, "")
            self.pcolor(GREY, f"    {json.dumps(r.json(), indent=2)[:300]}")

        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else ""
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    # -- Streaming Anthropic -------------------------------------------

    def _test_anthropic_streaming(self, bi: BackendInfo):
        label = f"{bi.name}/anthropic-stream"
        self.pcolor(YELLOW, f"  Streaming:     ", end="")

        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(bi.selected_model, stream=True),
                timeout=self.timeout,
                stream=True,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        ok = True
        notes = []

        if r.status_code != 200:
            self.pcolor(RED, f"[FAIL] HTTP {r.status_code}")
            self.record(label, False, f"HTTP {r.status_code}")
            return

        # Mode header
        mode = r.headers.get("X-Olla-Mode", "")
        if bi.expects_passthrough:
            if mode != "passthrough":
                ok = False
                notes.append(f"expected X-Olla-Mode=passthrough, got '{mode}'")
        else:
            if mode == "passthrough":
                ok = False
                notes.append("unexpected passthrough for translation backend")

        # Content type
        ct = r.headers.get("Content-Type", "")
        if "text/event-stream" not in ct:
            ok = False
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
                self.pcolor(CYAN, ".", end="")

        # Validate event types
        if bi.expects_passthrough:
            missing = PASSTHROUGH_EVENTS - event_types
            # Some events are optional depending on content length; require at least the minimum set
            if TRANSLATION_MIN_EVENTS - event_types:
                ok = False
                notes.append(f"missing events: {TRANSLATION_MIN_EVENTS - event_types}")
        else:
            missing = TRANSLATION_MIN_EVENTS - event_types
            if missing:
                ok = False
                notes.append(f"missing events: {missing}")

        if event_count == 0:
            ok = False
            notes.append("no SSE data lines received")

        status_str = f" {GREEN}[PASS]{RESET}" if ok else f" {RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else f"  ({event_count} events)"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    # -- OpenAI baseline ------------------------------------------------

    def _test_openai_baseline(self, bi: BackendInfo):
        label = f"{bi.name}/openai-baseline"
        self.pcolor(YELLOW, f"  OpenAI check:  ", end="")

        try:
            r = requests.post(
                f"{self.base_url}/olla/proxy/v1/chat/completions",
                headers={"Content-Type": "application/json"},
                json={
                    "model": bi.selected_model,
                    "messages": [{"role": "user", "content": "Say hello briefly"}],
                    "max_tokens": 10,
                },
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        ok = r.status_code == 200
        ep = r.headers.get("X-Olla-Endpoint", "")
        notes = []

        if not ok:
            notes.append(f"HTTP {r.status_code}")
        if ep and ep != bi.name:
            notes.append(f"routed to {ep} (expected {bi.name})")

        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else ""
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    # ── Edge Cases ──────────────────────────────────────────────────────

    def run_edge_cases(self):
        print()
        self.pcolor(WHITE, "Edge Cases")
        self.pcolor(PURPLE, "=" * 72)

        self._test_nonexistent_model()

        pt_backends = [b for b in self._testable_backends() if b.expects_passthrough]
        if pt_backends:
            bi = pt_backends[0]
            self._test_system_param(bi)
            self._test_multiturn(bi)
        else:
            self.pcolor(GREY, "  Skipping system/multiturn tests (no passthrough backends)")

    def _test_nonexistent_model(self):
        label = "edge/nonexistent-model"
        self.pcolor(YELLOW, f"  Non-existent model: ", end="")

        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body("nonexistent-model-xyz-999"),
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        # Accept any 4xx as correct behaviour; the exact code may vary
        ok = 400 <= r.status_code < 500
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  HTTP {r.status_code}"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, f"HTTP {r.status_code}")

    def _test_system_param(self, bi: BackendInfo):
        label = f"edge/system-param ({bi.name})"
        self.pcolor(YELLOW, f"  System parameter: ", end="")

        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=self._anthropic_body(
                    bi.selected_model,
                    system="You are a pirate",
                    messages=[{"role": "user", "content": "Say hello"}],
                ),
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        ok = r.status_code == 200
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        print(f"{status_str}{GREY}  HTTP {r.status_code}{RESET}")
        self.record(label, ok, f"HTTP {r.status_code}")

    def _test_multiturn(self, bi: BackendInfo):
        label = f"edge/multiturn ({bi.name})"
        self.pcolor(YELLOW, f"  Multi-turn conversation: ", end="")

        messages = [
            {"role": "user", "content": "My name is Test."},
            {"role": "assistant", "content": "Hello, Test!"},
            {"role": "user", "content": "What is my name?"},
        ]
        body = self._anthropic_body(bi.selected_model, messages=messages)
        body["max_tokens"] = 20

        try:
            r = requests.post(
                f"{self.base_url}/olla/anthropic/v1/messages",
                headers=self._anthropic_headers(),
                json=body,
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"[FAIL] {e}")
            self.record(label, False, str(e))
            return

        ok = r.status_code == 200
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        print(f"{status_str}{GREY}  HTTP {r.status_code}{RESET}")
        self.record(label, ok, f"HTTP {r.status_code}")

    # ── Translator Stats ────────────────────────────────────────────────

    def report_translator_stats(self):
        print()
        self.pcolor(WHITE, "Translator Stats")
        self.pcolor(PURPLE, "=" * 72)

        try:
            r = requests.get(f"{self.base_url}/internal/stats/translators", timeout=self.timeout)
            r.raise_for_status()
            data = r.json()
        except Exception as e:
            self.pcolor(RED, f"  [FAIL] Could not fetch translator stats: {e}")
            return

        translators = data.get("translators", [])
        summary = data.get("summary", {})

        if not translators:
            self.pcolor(GREY, "  No translator stats available yet")
            return

        for t in translators:
            name = t.get("translator_name", "?")
            total = t.get("total_requests", 0)
            pt_req = t.get("passthrough_requests", 0)
            tr_req = t.get("translation_requests", 0)
            pt_rate = t.get("passthrough_rate", "N/A")
            stream = t.get("streaming_requests", 0)
            nonstream = t.get("non_streaming_requests", 0)

            self.pcolor(CYAN, f"  {name}")
            self.pcolor(GREY, f"    Requests: {total} total  |  "
                               f"{pt_req} passthrough  |  {tr_req} translation")
            self.pcolor(GREY, f"    Passthrough rate: {pt_rate}  |  "
                               f"Streaming: {stream}  |  Non-streaming: {nonstream}")

            fb1 = t.get("fallback_no_compatible_endpoints", 0)
            fb2 = t.get("fallback_translator_does_not_support_passthrough", 0)
            fb3 = t.get("fallback_cannot_passthrough", 0)
            if fb1 or fb2 or fb3:
                self.pcolor(YELLOW, f"    Fallbacks: no_compatible={fb1}  "
                                     f"no_support={fb2}  cannot={fb3}")

        overall_pt = summary.get("overall_passthrough_rate", "N/A")
        overall_sr = summary.get("overall_success_rate", "N/A")
        print()
        self.pcolor(GREY, f"  Overall: passthrough={overall_pt}  success={overall_sr}")

    # ── Summary ─────────────────────────────────────────────────────────

    def print_summary(self) -> bool:
        print()
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(WHITE, f"  {BOLD}Results Summary{RESET}")
        self.pcolor(PURPLE, "=" * 72)

        # Main results table
        testable = self._testable_backends()
        if testable:
            print()
            header = (f"  {'Backend':<18} {'Type':<18} {'Model':<24} "
                      f"{'Mode':<14} {'Stream':<8} {'Result'}")
            self.pcolor(GREY, header)
            self.pcolor(GREY, f"  {'-'*18} {'-'*18} {'-'*24} {'-'*14} {'-'*8} {'-'*6}")

            for bi in testable:
                ns_ok = self._result_ok(f"{bi.name}/anthropic-nonstream")
                st_ok = self._result_ok(f"{bi.name}/anthropic-stream")

                # Stream column: tick/cross for nonstream/stream
                ns_mark = f"{GREEN}v{RESET}" if ns_ok else f"{RED}x{RESET}"
                st_mark = f"{GREEN}v{RESET}" if st_ok else (f"{RED}x{RESET}" if st_ok is not None else f"{GREY}-{RESET}")
                stream_col = f"{ns_mark}/{st_mark}"

                all_ok = ns_ok and (st_ok is not False)
                result_c = GREEN if all_ok else RED
                result_str = f"{result_c}{'PASS' if all_ok else 'FAIL'}{RESET}"
                mode_c = CYAN if bi.expects_passthrough else YELLOW

                model_short = (bi.selected_model[:22] + "..") if len(bi.selected_model or "") > 24 else (bi.selected_model or "N/A")

                print(f"  {bi.name:<18} {bi.backend_type:<18} {model_short:<24} "
                      f"{mode_c}{bi.mode_label:<14}{RESET} {stream_col}    {result_str}")

        # Edge case results
        edge_results = [r for r in self.results if r.name.startswith("edge/")]
        if edge_results:
            print()
            self.pcolor(WHITE, "  Edge Cases:")
            for r in edge_results:
                c = GREEN if r.passed else RED
                mark = "PASS" if r.passed else "FAIL"
                detail = f"  ({r.detail})" if r.detail else ""
                self.pcolor(c, f"    [{mark}] {r.name}{GREY}{detail}{RESET}")

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

        all_pass = failed == 0
        if all_pass:
            self.pcolor(GREEN, "  All tests passed.")
        else:
            self.pcolor(RED, f"  {failed} test(s) failed.")

        return all_pass

    def _result_ok(self, label: str) -> Optional[bool]:
        for r in self.results:
            if r.name == label:
                return r.passed
        return None


def main():
    parser = argparse.ArgumentParser(
        description="Test Olla Anthropic passthrough vs translation mode selection"
    )
    parser.add_argument("--url", default=TARGET_URL,
                        help=f"Olla base URL (default: {TARGET_URL})")
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT,
                        help=f"Request timeout in seconds (default: {DEFAULT_TIMEOUT})")
    parser.add_argument("--skip-edge-cases", action="store_true",
                        help="Skip edge case tests")
    parser.add_argument("--skip-streaming", action="store_true",
                        help="Skip streaming tests")
    parser.add_argument("--verbose", action="store_true",
                        help="Show full response bodies")

    args = parser.parse_args()

    tester = PassthroughTester(args.url, args.timeout, args.verbose)
    tester.print_header()

    # Phase 1: Health and discovery
    if not tester.check_health():
        sys.exit(1)
    print()
    if not tester.discover():
        sys.exit(1)

    # Phase 2: Test matrix
    tester.run_matrix(skip_streaming=args.skip_streaming)

    # Phase 3: Edge cases
    if not args.skip_edge_cases:
        tester.run_edge_cases()

    # Phase 4: Translator stats
    tester.report_translator_stats()

    # Phase 5: Summary
    all_pass = tester.print_summary()
    sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)
