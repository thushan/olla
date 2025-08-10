#!/usr/bin/env python3
"""
Olla Provider-Specific Model Routing Test Script
Tests model routing with different provider formats (OpenAI, Ollama, LM Studio, vLLM)
"""

import sys
import json
import time
import argparse
import requests
from typing import Dict, List, Tuple
from collections import defaultdict

# ANSI color codes
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[1;33m'
BLUE = '\033[0;34m'
PURPLE = '\033[0;35m'
CYAN = '\033[0;36m'
WHITE = '\033[1;37m'
GREY = '\033[0;37m'
RESET = '\033[0m'

# Configuration
TARGET_URL = "http://localhost:40114"
TIMEOUT = 180

class ProviderTester:
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.all_models = []
        self.available_endpoints = []
        self.provider_models = {
            'openai': [],
            'ollama': [],
            'lmstudio': [],
            'vllm': []
        }
        self.endpoint_usage = defaultdict(int)
        self.endpoint_success = defaultdict(int)
        self.endpoint_failure = defaultdict(int)
        self.total_tests = 0
        self.successful_tests = 0
        self.failed_tests = 0
        
    def print_color(self, color: str, message: str):
        print(f"{color}{message}{RESET}")
        
    def print_header(self):
        self.print_color(PURPLE, "=" * 58)
        self.print_color(PURPLE, f"  {CYAN}Olla Provider-Specific Model Routing Test{RESET}")
        self.print_color(PURPLE, "=" * 58)
        print()
        
    def check_health(self) -> bool:
        self.print_color(YELLOW, "Checking Olla availability...")
        try:
            response = requests.get(f"{self.base_url}/internal/health", timeout=5)
            if response.status_code == 200:
                self.print_color(GREEN, "[OK] Olla is reachable")
                return True
        except Exception:
            pass
        self.print_color(RED, f"[FAIL] Cannot reach Olla at {self.base_url}")
        return False
        
    def fetch_endpoints(self) -> bool:
        self.print_color(YELLOW, "Fetching available endpoints...")
        try:
            response = requests.get(f"{self.base_url}/internal/status/endpoints", timeout=TIMEOUT)
            if response.status_code == 200:
                data = response.json()
                self.available_endpoints = data.get('endpoints', [])
                if self.available_endpoints:
                    self.print_color(GREEN, f"[OK] Found {len(self.available_endpoints)} endpoints")
                    self.print_color(GREY, "Available endpoints:")
                    for ep in self.available_endpoints:
                        is_healthy = ep.get('status', '').lower() == 'healthy'
                        status = "[HEALTHY]" if is_healthy else "[UNHEALTHY]"
                        success_rate = ep.get('success_rate', 'N/A')
                        model_count = ep.get('model_count', 0)
                        print(f"  - {CYAN}{ep.get('name', 'unknown')}{RESET} {GREEN if is_healthy else RED}{status}{RESET} - {model_count} models, {success_rate} success")
                    return True
        except Exception as e:
            print(f"Error fetching endpoints: {e}")
        self.print_color(RED, "[FAIL] Failed to fetch endpoints")
        return False
        
    def fetch_models(self) -> bool:
        # First fetch all models
        self.print_color(YELLOW, "Fetching all available models...")
        try:
            response = requests.get(f"{self.base_url}/olla/models", timeout=TIMEOUT)
            if response.status_code == 200:
                data = response.json()
                # Try different JSON structures
                models = data.get('data', [])
                if models:
                    self.all_models = [m.get('id', m.get('name', '')) for m in models]
                else:
                    models = data.get('models', [])
                    if models:
                        self.all_models = [m.get('name', m.get('id', '')) for m in models]
                        
                if self.all_models:
                    self.print_color(GREEN, f"[OK] Found {len(self.all_models)} total models")
                    print()
        except Exception as e:
            print(f"Error fetching all models: {e}")
            self.print_color(RED, "[FAIL] Failed to fetch models")
            return False
            
        # Now fetch provider-specific models
        self.print_color(YELLOW, "Fetching provider-specific models...")
        for provider in ['openai', 'ollama', 'lmstudio', 'vllm']:
            try:
                response = requests.get(f"{self.base_url}/olla/models?format={provider}", timeout=TIMEOUT)
                if response.status_code == 200:
                    data = response.json()
                    models = data.get('data', [])
                    if models:
                        self.provider_models[provider] = [m.get('id', m.get('name', '')) for m in models]
                    else:
                        models = data.get('models', [])
                        if models:
                            self.provider_models[provider] = [m.get('name', m.get('id', '')) for m in models]
                    
                    if self.provider_models[provider]:
                        self.print_color(GREEN, f"  {provider}: {len(self.provider_models[provider])} models")
                    else:
                        self.print_color(YELLOW, f"  {provider}: No models available")
            except Exception as e:
                print(f"Error fetching {provider} models: {e}")
                self.print_color(RED, f"  {provider}: Failed to fetch")
                
        return len(self.all_models) > 0
        
    def display_model_summary(self):
        self.print_color(WHITE, "\nModel Summary:")
        self.print_color(PURPLE, "-" * 58)
        
        self.print_color(CYAN, f"Total models available: {len(self.all_models)}")
        
        # Show models per provider
        for provider in ['openai', 'ollama', 'lmstudio', 'vllm']:
            if self.provider_models[provider]:
                print()
                self.print_color(YELLOW, f"{provider.upper()} models ({len(self.provider_models[provider])}):")
                for i, model in enumerate(self.provider_models[provider][:5]):
                    print(f"  - {model}")
                if len(self.provider_models[provider]) > 5:
                    self.print_color(GREY, f"  ... and {len(self.provider_models[provider]) - 5} more")
        
    def test_endpoint(self, desc: str, url: str, data: dict = None, method: str = "POST") -> Tuple[bool, str]:
        print(f"{desc}: ", end='', flush=True)
        self.total_tests += 1
        
        try:
            start_time = time.time()
            
            if method == "GET":
                response = requests.get(url, timeout=TIMEOUT)
            else:
                response = requests.post(url, json=data, timeout=TIMEOUT)
                
            duration = time.time() - start_time
            
            # Extract endpoint header
            endpoint_header = response.headers.get('X-Olla-Endpoint', '')
            
            if response.status_code == 200:
                self.print_color(GREEN, f"[OK] Success ({duration:.3f}s)")
                if endpoint_header:
                    print(f"  -> Routed to: {endpoint_header}")
                    self.endpoint_usage[endpoint_header] += 1
                    self.endpoint_success[endpoint_header] += 1
                self.successful_tests += 1
                return True, endpoint_header
            else:
                self.print_color(RED, f"[FAIL] Failed (HTTP {response.status_code})")
                if endpoint_header:
                    self.endpoint_usage[endpoint_header] += 1
                    self.endpoint_failure[endpoint_header] += 1
                self.failed_tests += 1
                return False, ""
                
        except requests.exceptions.Timeout:
            self.print_color(RED, "[FAIL] Failed (Timeout)")
            self.failed_tests += 1
            return False, ""
        except Exception as e:
            self.print_color(RED, f"[FAIL] Failed ({type(e).__name__})")
            self.failed_tests += 1
            return False, ""
            
    def test_models_endpoints(self, providers: List[str]):
        self.print_color(WHITE, "\nTesting Models Discovery Endpoints:")
        self.print_color(PURPLE, "-" * 58)
        
        if 'openai' in providers:
            self.test_endpoint(
                "OpenAI Models (/olla/openai/v1/models)",
                f"{self.base_url}/olla/openai/v1/models",
                method="GET"
            )
            
        if 'ollama' in providers:
            self.test_endpoint(
                "Ollama Models (/olla/ollama/api/tags)",
                f"{self.base_url}/olla/ollama/api/tags",
                method="GET"
            )
            self.test_endpoint(
                "Ollama Models OpenAI Format (/olla/ollama/v1/models)",
                f"{self.base_url}/olla/ollama/v1/models",
                method="GET"
            )
            
        if 'lmstudio' in providers:
            self.test_endpoint(
                "LM Studio Models (/olla/lm-studio/v1/models)",
                f"{self.base_url}/olla/lm-studio/v1/models",
                method="GET"
            )
            self.test_endpoint(
                "LM Studio Enhanced Models (/olla/lm-studio/api/v0/models)",
                f"{self.base_url}/olla/lm-studio/api/v0/models",
                method="GET"
            )
            
        if 'vllm' in providers:
            self.test_endpoint(
                "vLLM Models (/olla/vllm/v1/models)",
                f"{self.base_url}/olla/vllm/v1/models",
                method="GET"
            )
            
    def test_model_routing(self, providers: List[str], max_models: int = 3):
        self.print_color(WHITE, "\nTesting Model Routing with Provider Formats:")
        self.print_color(PURPLE, "-" * 58)
        
        tested_per_provider = defaultdict(int)
        
        for provider in providers:
            if provider not in self.provider_models or not self.provider_models[provider]:
                self.print_color(YELLOW, f"\nNo models available for {provider} format, skipping...")
                continue
                
            print()
            self.print_color(WHITE, f"Testing {provider.upper()} provider models:")
            
            for model in self.provider_models[provider]:
                if max_models is not None and tested_per_provider[provider] >= max_models:
                    self.print_color(GREY, f"(Limited to {max_models} models per provider for brevity)")
                    break
                    
                print()
                self.print_color(CYAN, f"Model: {model}")
                
                if provider == 'openai':
                    if 'embed' not in model:
                        self.test_endpoint(
                            "  Chat Completions (/v1/chat/completions)",
                            f"{self.base_url}/olla/openai/v1/chat/completions",
                            {
                                "model": model,
                                "messages": [{"role": "user", "content": "Hello"}],
                                "max_tokens": 10,
                                "stream": False
                            }
                        )
                        self.test_endpoint(
                            "  Completions (/v1/completions)",
                            f"{self.base_url}/olla/openai/v1/completions",
                            {
                                "model": model,
                                "prompt": "Hello",
                                "max_tokens": 10,
                                "stream": False
                            }
                        )
                    else:
                        self.test_endpoint(
                            "  Embeddings (/v1/embeddings)",
                            f"{self.base_url}/olla/openai/v1/embeddings",
                            {
                                "model": model,
                                "input": "Test text"
                            }
                        )
                        
                elif provider == 'ollama':
                    if 'embed' not in model:
                        self.test_endpoint(
                            "  Generate (/api/generate)",
                            f"{self.base_url}/olla/ollama/api/generate",
                            {
                                "model": model,
                                "prompt": "Hello",
                                "stream": False
                            }
                        )
                        self.test_endpoint(
                            "  Chat (/api/chat)",
                            f"{self.base_url}/olla/ollama/api/chat",
                            {
                                "model": model,
                                "messages": [{"role": "user", "content": "Hello"}],
                                "stream": False
                            }
                        )
                    else:
                        self.test_endpoint(
                            "  Embeddings (/api/embeddings)",
                            f"{self.base_url}/olla/ollama/api/embeddings",
                            {
                                "model": model,
                                "prompt": "Test text"
                            }
                        )
                        
                elif provider == 'lmstudio':
                    self.test_endpoint(
                        "  Chat Completions (/v1/chat/completions)",
                        f"{self.base_url}/olla/lm-studio/v1/chat/completions",
                        {
                            "model": model,
                            "messages": [{"role": "user", "content": "Hello"}],
                            "max_tokens": 10,
                            "stream": False
                        }
                    )
                    self.test_endpoint(
                        "  Chat Completions (/api/v1/chat/completions)",
                        f"{self.base_url}/olla/lm-studio/api/v1/chat/completions",
                        {
                            "model": model,
                            "messages": [{"role": "user", "content": "Hello"}],
                            "max_tokens": 10,
                            "stream": False
                        }
                    )
                    
                elif provider == 'vllm':
                    if 'embed' not in model:
                        self.test_endpoint(
                            "  Chat Completions (/v1/chat/completions)",
                            f"{self.base_url}/olla/vllm/v1/chat/completions",
                            {
                                "model": model,
                                "messages": [{"role": "user", "content": "Hello"}],
                                "max_tokens": 10,
                                "stream": False
                            }
                        )
                        self.test_endpoint(
                            "  Completions (/v1/completions)",
                            f"{self.base_url}/olla/vllm/v1/completions",
                            {
                                "model": model,
                                "prompt": "Hello",
                                "max_tokens": 10,
                                "stream": False
                            }
                        )
                    else:
                        self.test_endpoint(
                            "  Embeddings (/v1/embeddings)",
                            f"{self.base_url}/olla/vllm/v1/embeddings",
                            {
                                "model": model,
                                "input": "Test text"
                            }
                        )
                    
                tested_per_provider[provider] += 1
            
    def print_summary(self):
        self.print_color(PURPLE, "\n" + "-" * 58)
        self.print_color(WHITE, "Test Summary:")
        print(f"  Total Tests:        {CYAN}{self.total_tests}{RESET}")
        print(f"  Successful Tests:   {GREEN}{self.successful_tests}{RESET}")
        print(f"  Failed Tests:       {RED}{self.failed_tests}{RESET}")
        
        if self.total_tests > 0:
            success_rate = (self.successful_tests * 100) // self.total_tests
            color = GREEN if success_rate >= 80 else YELLOW if success_rate >= 50 else RED
            print(f"  Success Rate:       {color}{success_rate}%{RESET}")
            
        if self.endpoint_usage:
            self.print_color(WHITE, "\nEndpoint Usage:")
            print(f"  {'Endpoint':<20} {'Total':<8} {'Success':<8} {'Failed':<8}")
            print(f"  {'-'*20} {'-'*8} {'-'*8} {'-'*8}")
            
            for endpoint in sorted(self.endpoint_usage.keys()):
                total = self.endpoint_usage[endpoint]
                success = self.endpoint_success.get(endpoint, 0)
                failed = self.endpoint_failure.get(endpoint, 0)
                
                # Color code based on failure rate
                if failed == 0:
                    color = GREEN
                elif failed > success:
                    color = RED
                else:
                    color = YELLOW
                    
                print(f"  {color}{endpoint:<20}{RESET} {total:<8} {GREEN}{success:<8}{RESET} {RED if failed > 0 else GREY}{failed:<8}{RESET}")

def main():
    parser = argparse.ArgumentParser(description='Test Olla provider-specific routing')
    parser.add_argument('--openai', action='store_true', help='Test OpenAI format')
    parser.add_argument('--ollama', action='store_true', help='Test Ollama format')
    parser.add_argument('--lmstudio', '--lm-studio', action='store_true', help='Test LM Studio format')
    parser.add_argument('--vllm', action='store_true', help='Test vLLM format')
    parser.add_argument('--all', action='store_true', help='Test all models (default: 3 per provider)')
    parser.add_argument('--url', default=TARGET_URL, help='Olla base URL')
    
    args = parser.parse_args()
    
    # If no provider specified, test all
    providers = []
    if args.openai:
        providers.append('openai')
    if args.ollama:
        providers.append('ollama')
    if args.lmstudio:
        providers.append('lmstudio')
    if args.vllm:
        providers.append('vllm')
        
    if not providers:
        providers = ['openai', 'ollama', 'lmstudio', 'vllm']
        
    tester = ProviderTester(args.url)
    tester.print_header()
    
    if not tester.check_health():
        sys.exit(1)
        
    print()
    
    if not tester.fetch_endpoints():
        sys.exit(1)
        
    print()
    
    if not tester.fetch_models():
        sys.exit(1)
        
    tester.display_model_summary()
    print()
    
    tester.test_models_endpoints(providers)
    
    # Determine max models to test
    max_models = None if args.all else 3
    tester.test_model_routing(providers, max_models=max_models)
    tester.print_summary()

if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)  # Standard exit code for SIGINT