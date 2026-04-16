#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Sticky Session Test Script

Validates that sticky session (KV cache affinity) routing works correctly.
Sends repeated requests with identical content or explicit session IDs and
verifies that the same backend is selected on subsequent turns.

Auto-discovers available backends and models, then runs a test matrix covering
session ID pinning, prefix-hash affinity, session independence, and header
validation.
"""

import sys
import json
import time
import argparse
import requests
import os
from typing import Dict, List, Optional, Any

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

# Valid values for sticky session response headers
VALID_STICKY_RESULTS = {"hit", "miss", "repin", "disabled"}
VALID_KEY_SOURCES = {"session_header", "prefix_hash", "auth_header", "ip", "none"}

# Long system prompt used for prefix-hash tests — must be > 512 bytes so the
# hash captures real content rather than colliding on short strings.
LONG_SYSTEM_PROMPT = (
    "You are a highly knowledgeable Go programming expert. "
    "You specialise in concurrent systems, high-performance proxies, and load "
    "balancing algorithms. You write clean, idiomatic Go following effective Go "
    "conventions. You always explain the why behind design decisions, not just "
    "the what. You are familiar with the Olla project and its hexagonal "
    "architecture. When reviewing code you consider correctness, performance, "
    "and maintainability equally."
)

ALTERNATIVE_SYSTEM_PROMPT = (
    "You are a Python data science expert specialising in pandas, NumPy, and "
    "scikit-learn. You focus on efficient data pipelines, reproducible research, "
    "and clear visualisations. Your answers are concise and include runnable "
    "code examples."
)


class BackendInfo:
    """Discovered backend with its health, type, and selected model."""
    __slots__ = ("name", "backend_type", "status", "models", "selected_model")

    def __init__(self, name: str, backend_type: str, status: str):
        self.name = name
        self.backend_type = backend_type
        self.status = status
        self.models: List[str] = []
        self.selected_model: Optional[str] = None


class TestResult:
    """Outcome of a single test case."""
    __slots__ = ("name", "passed", "detail")

    def __init__(self, name: str, passed: bool, detail: str = ""):
        self.name = name
        self.passed = passed
        self.detail = detail


class StickySessionTester:
    def __init__(self, base_url: str, timeout: int, verbose: bool, model: Optional[str]):
        self.base_url = base_url
        self.timeout = timeout
        self.verbose = verbose
        self.forced_model = model
        self.backends: List[BackendInfo] = []
        self.results: List[TestResult] = []
        self.sticky_enabled: Optional[bool] = None  # detected from first request

    # ── Helpers ─────────────────────────────────────────────────────────

    def pcolor(self, color: str, msg: str, end: str = '\n'):
        print(f"{color}{msg}{RESET}", end=end)
        sys.stdout.flush()

    def print_header(self):
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(PURPLE, f"  {CYAN}Olla Sticky Session Test{RESET}")
        self.pcolor(PURPLE, f"  {GREY}Validates KV cache affinity routing behaviour{RESET}")
        self.pcolor(PURPLE, "=" * 72)
        print()

    def record(self, name: str, passed: bool, detail: str = "") -> bool:
        self.results.append(TestResult(name, passed, detail))
        return passed

    def _chat_headers(self, session_id: Optional[str] = None) -> Dict[str, str]:
        h = {"Content-Type": "application/json"}
        if session_id:
            h["X-Olla-Session-ID"] = session_id
        return h

    def _chat_body(self, model: str, system: Optional[str] = None,
                   user_turn: str = "Reply with one word: yes") -> Dict[str, Any]:
        messages: List[Dict[str, str]] = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": user_turn})
        return {
            "model": model,
            "messages": messages,
            "max_tokens": 5,
        }

    def _post(self, body: Dict[str, Any],
              headers: Optional[Dict[str, str]] = None) -> Optional[requests.Response]:
        """POST to the OpenAI-compatible proxy endpoint. Returns None on network error."""
        try:
            return requests.post(
                f"{self.base_url}/olla/proxy/v1/chat/completions",
                headers=headers or {"Content-Type": "application/json"},
                json=body,
                timeout=self.timeout,
            )
        except Exception as e:
            self.pcolor(RED, f"  [ERROR] Request failed: {e}")
            return None

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
        """Discover endpoints and pick a model for testing."""
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

        # Fetch models
        self.pcolor(YELLOW, "Discovering models...")
        try:
            r = requests.get(f"{self.base_url}/internal/status/models", timeout=self.timeout)
            r.raise_for_status()
            data = r.json()
            recent = data.get("recent_models", [])
        except Exception as e:
            self.pcolor(RED, f"[FAIL] Model discovery failed: {e}")
            return False

        # Build URL → endpoint name reverse map
        url_to_name: Dict[str, str] = {}
        for ep in endpoints:
            url = ep.get("url", "")
            name = ep.get("name", "")
            if url and name:
                url_to_name[url.rstrip("/")] = name

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

            for ep_url in model_endpoints:
                ep_name = url_to_name.get(ep_url.rstrip("/"))
                if ep_name and ep_name in ep_map:
                    ep_map[ep_name].models.append(model_name)
                    matched = True

            if not matched and model_type and model_type in type_to_backends:
                for ep_name in type_to_backends[model_type]:
                    if ep_name in ep_map:
                        ep_map[ep_name].models.append(model_name)

        # Select one non-embedding model per backend
        for bi in self.backends:
            if self.forced_model:
                bi.selected_model = self.forced_model
            else:
                candidates = [m for m in bi.models if "embed" not in m.lower()]
                if candidates:
                    bi.selected_model = candidates[0]

        self._print_discovery_summary()
        return True

    def _print_discovery_summary(self):
        print()
        self.pcolor(WHITE, "Configuration Summary")
        self.pcolor(PURPLE, "-" * 72)
        header = f"  {'Backend':<20} {'Type':<18} {'Status':<10} {'Model'}"
        self.pcolor(GREY, header)
        self.pcolor(GREY, f"  {'-'*20} {'-'*18} {'-'*10} {'-'*24}")

        for bi in self.backends:
            status_c = GREEN if bi.status == "healthy" else RED
            status_str = f"{status_c}{bi.status:<10}{RESET}"
            model_str = bi.selected_model or "(none)"
            print(f"  {bi.name:<20} {bi.backend_type:<18} {status_str} {model_str}")

    def _testable(self) -> List[BackendInfo]:
        return [b for b in self.backends if b.status == "healthy" and b.selected_model]

    # ── Feature detection ────────────────────────────────────────────────

    def detect_sticky_enabled(self, model: str) -> bool:
        """
        Make one probe request and inspect X-Olla-Sticky-Session.
        Returns True if sticky sessions are active, False if disabled.
        """
        self.pcolor(YELLOW, "\nDetecting sticky session status...")
        r = self._post(self._chat_body(model))
        if r is None or r.status_code != 200:
            self.pcolor(RED, "  [FAIL] Probe request failed — cannot detect feature status")
            return False

        val = r.headers.get("X-Olla-Sticky-Session", "")
        if val == "disabled":
            self.pcolor(YELLOW, (
                "  [WARN] Sticky sessions are disabled in Olla config.\n"
                "         Affinity tests will be skipped.\n"
                "         To enable, add to config.yaml under proxy:\n"
                "           sticky_sessions:\n"
                "             enabled: true"
            ))
            self.sticky_enabled = False
            return False

        if val in VALID_STICKY_RESULTS:
            self.pcolor(GREEN, f"  [OK] Sticky sessions active (probe returned: {val})")
            self.sticky_enabled = True
            return True

        # Header absent — old binary or feature not built
        self.pcolor(YELLOW, f"  [WARN] X-Olla-Sticky-Session header absent (value: '{val}')")
        self.sticky_enabled = False
        return False

    # ── Test Sections ────────────────────────────────────────────────────

    def run_header_validation(self, model: str):
        """
        Basic header presence and value validation — runs regardless of whether
        sticky sessions are enabled, because the 'disabled' value is also valid.
        """
        print()
        self.pcolor(WHITE, "Header Validation")
        self.pcolor(PURPLE, "=" * 72)

        self._test_sticky_header_present(model)
        self._test_key_source_valid(model)

    def run_affinity_tests(self, model: str):
        """Core affinity tests — only meaningful when sticky sessions are enabled."""
        print()
        self.pcolor(WHITE, "Affinity Tests")
        self.pcolor(PURPLE, "=" * 72)

        if not self.sticky_enabled:
            self.pcolor(GREY, "  Skipped — sticky sessions are disabled")
            return

        self._test_session_id_miss_then_hit(model)
        self._test_session_id_echoed(model)
        self._test_prefix_hash_hit(model)
        self._test_sessions_independent(model)

    def run_multi_backend_tests(self):
        """
        Tests that only make sense when multiple healthy backends are available,
        e.g. verifying that different session IDs can land on different backends.
        Skipped silently when only one backend is present.
        """
        testable = self._testable()
        if len(testable) < 2:
            return

        print()
        self.pcolor(WHITE, "Multi-Backend Tests")
        self.pcolor(PURPLE, "=" * 72)

        if not self.sticky_enabled:
            self.pcolor(GREY, "  Skipped — sticky sessions are disabled")
            return

        self._test_different_sessions_may_use_different_backends(testable)

    # ── Individual Tests ─────────────────────────────────────────────────

    def _test_sticky_header_present(self, model: str):
        label = "header/sticky-session-present"
        self.pcolor(YELLOW, "  X-Olla-Sticky-Session present:  ", end="")

        r = self._post(self._chat_body(model))
        if r is None:
            print(f"{RED}[FAIL]{RESET}")
            self.record(label, False, "request failed")
            return

        val = r.headers.get("X-Olla-Sticky-Session", "")
        ok = val in VALID_STICKY_RESULTS
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  value='{val}'" if ok else f"  got '{val}', expected one of {VALID_STICKY_RESULTS}"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, detail.strip())

    def _test_key_source_valid(self, model: str):
        label = "header/key-source-valid"
        self.pcolor(YELLOW, "  X-Olla-Sticky-Key-Source valid: ", end="")

        r = self._post(self._chat_body(model))
        if r is None:
            print(f"{RED}[FAIL]{RESET}")
            self.record(label, False, "request failed")
            return

        sticky_val = r.headers.get("X-Olla-Sticky-Session", "")
        source_val = r.headers.get("X-Olla-Sticky-Key-Source", "")

        # When disabled, source header is absent — that is correct behaviour.
        if sticky_val == "disabled":
            print(f"{GREEN}[PASS]{RESET}{GREY}  (disabled — source header absent as expected){RESET}")
            self.record(label, True, "disabled")
            return

        ok = source_val in VALID_KEY_SOURCES
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  source='{source_val}'" if ok else f"  got '{source_val}', expected one of {VALID_KEY_SOURCES}"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, detail.strip())

    def _test_session_id_miss_then_hit(self, model: str):
        label = "affinity/session-id-miss-then-hit"
        self.pcolor(YELLOW, "  Session ID: miss → hit:         ", end="")

        session_id = f"test-sticky-{int(time.time())}"
        headers = self._chat_headers(session_id)
        body = self._chat_body(model, system=LONG_SYSTEM_PROMPT)

        r1 = self._post(body, headers)
        if r1 is None or r1.status_code != 200:
            print(f"{RED}[FAIL]{RESET}{GREY}  turn-1 request failed{RESET}")
            self.record(label, False, "turn-1 failed")
            return

        r2 = self._post(body, headers)
        if r2 is None or r2.status_code != 200:
            print(f"{RED}[FAIL]{RESET}{GREY}  turn-2 request failed{RESET}")
            self.record(label, False, "turn-2 failed")
            return

        t1_sticky = r1.headers.get("X-Olla-Sticky-Session", "")
        t2_sticky = r2.headers.get("X-Olla-Sticky-Session", "")
        t1_ep = r1.headers.get("X-Olla-Endpoint", "")
        t2_ep = r2.headers.get("X-Olla-Endpoint", "")

        notes = []
        ok = True

        if t1_sticky != "miss":
            ok = False
            notes.append(f"turn-1 expected 'miss', got '{t1_sticky}'")
        if t2_sticky != "hit":
            ok = False
            notes.append(f"turn-2 expected 'hit', got '{t2_sticky}'")
        if t1_ep and t2_ep and t1_ep != t2_ep:
            ok = False
            notes.append(f"endpoint changed: {t1_ep} → {t2_ep}")

        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else f"  endpoint={t1_ep or 'n/a'}"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    def _test_session_id_echoed(self, model: str):
        label = "affinity/session-id-echoed"
        self.pcolor(YELLOW, "  Session ID echoed in response:  ", end="")

        session_id = f"echo-test-{int(time.time())}"
        r = self._post(
            self._chat_body(model),
            self._chat_headers(session_id),
        )
        if r is None or r.status_code != 200:
            print(f"{RED}[FAIL]{RESET}")
            self.record(label, False, "request failed")
            return

        echoed = r.headers.get("X-Olla-Session-ID", "")
        ok = echoed == session_id
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  echoed='{echoed}'" if ok else f"  expected '{session_id}', got '{echoed}'"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, detail.strip())

    def _test_prefix_hash_hit(self, model: str):
        label = "affinity/prefix-hash-hit"
        self.pcolor(YELLOW, "  Prefix hash: same prompt → hit: ", end="")

        # No session header — key must come from prefix hash.
        body = self._chat_body(model, system=LONG_SYSTEM_PROMPT)

        r1 = self._post(body)
        if r1 is None or r1.status_code != 200:
            print(f"{RED}[FAIL]{RESET}{GREY}  turn-1 failed{RESET}")
            self.record(label, False, "turn-1 failed")
            return

        r2 = self._post(body)
        if r2 is None or r2.status_code != 200:
            print(f"{RED}[FAIL]{RESET}{GREY}  turn-2 failed{RESET}")
            self.record(label, False, "turn-2 failed")
            return

        t1_source = r1.headers.get("X-Olla-Sticky-Key-Source", "")
        t2_sticky = r2.headers.get("X-Olla-Sticky-Session", "")
        t1_ep = r1.headers.get("X-Olla-Endpoint", "")
        t2_ep = r2.headers.get("X-Olla-Endpoint", "")

        notes = []
        ok = True

        if t1_source not in ("prefix_hash", "auth_header"):
            # auth_header is also acceptable when Authorization header is present
            notes.append(f"turn-1 source='{t1_source}' (expected prefix_hash or auth_header)")
        if t2_sticky != "hit":
            ok = False
            notes.append(f"turn-2 expected 'hit', got '{t2_sticky}'")
        if t1_ep and t2_ep and t1_ep != t2_ep:
            ok = False
            notes.append(f"endpoint changed: {t1_ep} → {t2_ep}")

        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else f"  source={t1_source} endpoint={t1_ep or 'n/a'}"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    def _test_sessions_independent(self, model: str):
        label = "affinity/sessions-independent"
        self.pcolor(YELLOW, "  Two sessions pin independently: ", end="")

        ts = int(time.time())
        sid_a = f"session-alpha-{ts}"
        sid_b = f"session-beta-{ts}"
        body = self._chat_body(model, system=LONG_SYSTEM_PROMPT)

        # Pin each session with a first (miss) request, then confirm second is a hit.
        ra1 = self._post(body, self._chat_headers(sid_a))
        rb1 = self._post(body, self._chat_headers(sid_b))
        ra2 = self._post(body, self._chat_headers(sid_a))
        rb2 = self._post(body, self._chat_headers(sid_b))

        responses = [ra1, rb1, ra2, rb2]
        if any(r is None or r.status_code != 200 for r in responses):
            print(f"{RED}[FAIL]{RESET}{GREY}  one or more requests failed{RESET}")
            self.record(label, False, "request failed")
            return

        a2_sticky = ra2.headers.get("X-Olla-Sticky-Session", "")  # type: ignore[union-attr]
        b2_sticky = rb2.headers.get("X-Olla-Sticky-Session", "")  # type: ignore[union-attr]

        notes = []
        ok = True

        if a2_sticky != "hit":
            ok = False
            notes.append(f"session-A turn-2 expected 'hit', got '{a2_sticky}'")
        if b2_sticky != "hit":
            ok = False
            notes.append(f"session-B turn-2 expected 'hit', got '{b2_sticky}'")

        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}" if notes else "  both sessions independently pinned"
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    def _test_different_sessions_may_use_different_backends(self, testable: List[BackendInfo]):
        """
        With multiple backends, two sessions with different system prompts *may*
        land on different backends. We cannot assert they must — load balancer
        may fairly assign both to the same backend. We only verify both pin
        (second request is a hit) without interfering with each other.
        """
        label = "multi-backend/sessions-do-not-interfere"
        self.pcolor(YELLOW, "  Multi-backend: no cross-session interference: ", end="")

        model = testable[0].selected_model
        ts = int(time.time())
        body_a = self._chat_body(model, system=LONG_SYSTEM_PROMPT)
        body_b = self._chat_body(model, system=ALTERNATIVE_SYSTEM_PROMPT)

        sid_a = f"multi-alpha-{ts}"
        sid_b = f"multi-beta-{ts}"

        ra1 = self._post(body_a, self._chat_headers(sid_a))
        rb1 = self._post(body_b, self._chat_headers(sid_b))
        ra2 = self._post(body_a, self._chat_headers(sid_a))
        rb2 = self._post(body_b, self._chat_headers(sid_b))

        responses = [ra1, rb1, ra2, rb2]
        if any(r is None or r.status_code != 200 for r in responses):
            print(f"{RED}[FAIL]{RESET}{GREY}  one or more requests failed{RESET}")
            self.record(label, False, "request failed")
            return

        a2_sticky = ra2.headers.get("X-Olla-Sticky-Session", "")  # type: ignore[union-attr]
        b2_sticky = rb2.headers.get("X-Olla-Sticky-Session", "")  # type: ignore[union-attr]
        a_ep = ra1.headers.get("X-Olla-Endpoint", "")             # type: ignore[union-attr]
        b_ep = rb1.headers.get("X-Olla-Endpoint", "")             # type: ignore[union-attr]

        notes = []
        ok = True

        if a2_sticky != "hit":
            ok = False
            notes.append(f"session-A not pinned (got '{a2_sticky}')")
        if b2_sticky != "hit":
            ok = False
            notes.append(f"session-B not pinned (got '{b2_sticky}')")

        ep_note = f"  A→{a_ep or '?'} B→{b_ep or '?'}" if a_ep != b_ep else f"  both→{a_ep or '?'}"
        status_str = f"{GREEN}[PASS]{RESET}" if ok else f"{RED}[FAIL]{RESET}"
        detail = f"  {' | '.join(notes)}{ep_note}" if notes else ep_note
        print(f"{status_str}{GREY}{detail}{RESET}")
        self.record(label, ok, "; ".join(notes))

    # ── Stats ────────────────────────────────────────────────────────────

    def report_sticky_stats(self):
        """Fetch and display the /internal/stats/sticky endpoint if available."""
        print()
        self.pcolor(WHITE, "Sticky Session Stats")
        self.pcolor(PURPLE, "=" * 72)

        try:
            r = requests.get(f"{self.base_url}/internal/stats/sticky", timeout=self.timeout)
            if r.status_code == 404:
                self.pcolor(GREY, "  Stats endpoint not available (HTTP 404)")
                return
            r.raise_for_status()
            data = r.json()
        except Exception as e:
            self.pcolor(GREY, f"  Could not fetch sticky stats: {e}")
            return

        hits = data.get("hits", "N/A")
        misses = data.get("misses", "N/A")
        evictions = data.get("evictions", "N/A")
        sessions = data.get("active_sessions", "N/A")

        self.pcolor(GREY, f"  Active sessions:  {CYAN}{sessions}{RESET}")
        self.pcolor(GREY, f"  Cache hits:       {GREEN}{hits}{RESET}")
        self.pcolor(GREY, f"  Cache misses:     {YELLOW}{misses}{RESET}")
        self.pcolor(GREY, f"  Evictions:        {GREY}{evictions}{RESET}")

    # ── Summary ──────────────────────────────────────────────────────────

    def print_summary(self) -> bool:
        print()
        self.pcolor(PURPLE, "=" * 72)
        self.pcolor(WHITE, f"  {BOLD}Results Summary{RESET}")
        self.pcolor(PURPLE, "=" * 72)
        print()

        if self.results:
            header = f"  {'Test':<48} {'Result'}"
            self.pcolor(GREY, header)
            self.pcolor(GREY, f"  {'-'*48} {'-'*6}")

            for r in self.results:
                c = GREEN if r.passed else RED
                mark = "PASS" if r.passed else "FAIL"
                detail = f"  {GREY}({r.detail}){RESET}" if r.detail and self.verbose else ""
                label_short = r.name[:46] + ".." if len(r.name) > 48 else r.name
                print(f"  {label_short:<48} {c}{mark}{RESET}{detail}")

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


def main():
    parser = argparse.ArgumentParser(
        description="Test Olla sticky session (KV cache affinity) routing"
    )
    parser.add_argument("--url", default=TARGET_URL,
                        help=f"Olla base URL (default: {TARGET_URL})")
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT,
                        help=f"Request timeout in seconds (default: {DEFAULT_TIMEOUT})")
    parser.add_argument("--model",
                        help="Force a specific model name instead of auto-discovery")
    parser.add_argument("--skip-stats", action="store_true",
                        help="Skip the sticky stats endpoint check")
    parser.add_argument("--verbose", action="store_true",
                        help="Show test detail in summary table")
    args = parser.parse_args()

    tester = StickySessionTester(args.url, args.timeout, args.verbose, args.model)
    tester.print_header()

    # Phase 1: Health and discovery
    if not tester.check_health():
        sys.exit(1)
    print()
    if not tester.discover():
        sys.exit(1)

    testable = tester._testable()
    if not testable:
        tester.pcolor(RED, "\n[FAIL] No healthy backends with models available for testing")
        sys.exit(1)

    model = testable[0].selected_model

    # Phase 2: Feature detection
    tester.detect_sticky_enabled(model)

    # Phase 3: Header validation (runs even when disabled)
    tester.run_header_validation(model)

    # Phase 4: Affinity tests (skipped when disabled)
    tester.run_affinity_tests(model)

    # Phase 5: Multi-backend tests
    tester.run_multi_backend_tests()

    # Phase 6: Stats endpoint
    if not args.skip_stats:
        tester.report_sticky_stats()

    # Phase 7: Summary
    all_pass = tester.print_summary()
    sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)
