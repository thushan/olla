#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Streaming Response Test Script
Tests that LLM responses actually stream data incrementally
"""

import sys
import json
import time
import argparse
import requests
import os
from typing import Dict, List, Tuple, Optional, Any
from collections import defaultdict
from datetime import datetime

# Fix Windows console encoding for Unicode
if sys.platform == 'win32':
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')
    os.environ['PYTHONIOENCODING'] = 'utf-8'

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
DEFAULT_MAX_STREAM_TIME = 30  # seconds
DEFAULT_MAX_TOKENS = 100
CHUNK_REPORT_INTERVAL = 0.5  # Report streaming progress every N seconds

class StreamingTester:
    def __init__(self, base_url: str, max_stream_time: int):
        self.base_url = base_url
        self.max_stream_time = max_stream_time
        self.all_models = []
        self.provider_models = {
            'openai': [],
            'ollama': [],
            'lmstudio': []
        }
        self.test_results = []
        self.total_tests = 0
        self.successful_tests = 0
        self.failed_tests = 0
        
    def print_color(self, color: str, message: str, end='\n'):
        print(f"{color}{message}{RESET}", end=end)
        
    def print_header(self):
        self.print_color(PURPLE, "=" * 58)
        self.print_color(PURPLE, f"  {CYAN}Olla Streaming Response Test{RESET}")
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
        
    def fetch_models(self) -> bool:
        self.print_color(YELLOW, "Fetching available models...")
        
        # Fetch provider-specific models
        for provider in ['openai', 'ollama', 'lmstudio']:
            try:
                response = requests.get(f"{self.base_url}/olla/models?format={provider}", timeout=10)
                if response.status_code == 200:
                    data = response.json()
                    models = data.get('data', data.get('models', []))
                    
                    for model in models:
                        model_id = model.get('id', model.get('name', ''))
                        if model_id and 'embed' not in model_id.lower():
                            self.provider_models[provider].append(model_id)
                    
                    if self.provider_models[provider]:
                        self.print_color(GREEN, f"  {provider}: {len(self.provider_models[provider])} models")
                    else:
                        self.print_color(YELLOW, f"  {provider}: No models available")
            except Exception as e:
                self.print_color(RED, f"  {provider}: Failed to fetch - {str(e)}")
                
        # Collect all unique models
        all_models_set = set()
        for models in self.provider_models.values():
            all_models_set.update(models)
        self.all_models = sorted(list(all_models_set))
        
        return len(self.all_models) > 0
        
    def parse_sse_chunk(self, line: str) -> Optional[Dict[str, Any]]:
        """Parse Server-Sent Events format chunk"""
        if not line.startswith('data: '):
            return None
            
        data_str = line[6:]  # Remove 'data: ' prefix
        if data_str == '[DONE]':
            return {'done': True}
            
        try:
            return json.loads(data_str)
        except:
            return None
            
    def test_openai_streaming(self, model: str) -> Dict[str, Any]:
        """Test OpenAI-format streaming endpoint"""
        url = f"{self.base_url}/olla/openai/v1/chat/completions"
        data = {
            "model": model,
            "messages": [
                {"role": "user", "content": "Tell me a story about a robot. Be creative and descriptive."}
            ],
            "stream": True,
            "max_tokens": DEFAULT_MAX_TOKENS
        }
        
        return self._test_streaming(url, data, 'openai', model)
        
    def test_ollama_streaming(self, model: str) -> Dict[str, Any]:
        """Test Ollama-format streaming endpoint"""
        url = f"{self.base_url}/olla/ollama/api/generate"
        data = {
            "model": model,
            "prompt": "Tell me a story about a robot. Be creative and descriptive.",
            "stream": True,
            "options": {
                "num_predict": DEFAULT_MAX_TOKENS
            }
        }
        
        return self._test_streaming(url, data, 'ollama', model)
        
    def test_lmstudio_streaming(self, model: str) -> Dict[str, Any]:
        """Test LM Studio-format streaming endpoint"""
        url = f"{self.base_url}/olla/lm-studio/v1/chat/completions"
        data = {
            "model": model,
            "messages": [
                {"role": "user", "content": "Tell me a story about a robot. Be creative and descriptive."}
            ],
            "stream": True,
            "max_tokens": DEFAULT_MAX_TOKENS
        }
        
        return self._test_streaming(url, data, 'lmstudio', model)
        
    def _test_streaming(self, url: str, data: Dict, provider: str, model: str) -> Dict[str, Any]:
        """Generic streaming test implementation"""
        result = {
            'provider': provider,
            'model': model,
            'success': False,
            'chunks_received': 0,
            'time_to_first_token': None,
            'total_time': 0,
            'tokens_per_second': 0,
            'content_length': 0,
            'endpoint_used': None,
            'error': None,
            'chunk_times': []
        }
        
        start_time = time.time()
        first_chunk_time = None
        last_report_time = start_time
        content_pieces = []
        
        try:
            # Make streaming request
            response = requests.post(
                url,
                json=data,
                stream=True,
                timeout=(5, self.max_stream_time),
                headers={'Accept': 'text/event-stream'}
            )
            
            if response.status_code != 200:
                result['error'] = f"HTTP {response.status_code}: {response.text[:200]}"
                return result
                
            # Extract endpoint from headers
            result['endpoint_used'] = response.headers.get('X-Olla-Endpoint', 'Unknown')
            
            # Process streaming response
            for line_num, line in enumerate(response.iter_lines(decode_unicode=True)):
                current_time = time.time()
                
                # Check timeout
                if current_time - start_time > self.max_stream_time:
                    self.print_color(YELLOW, " [Timeout reached]")
                    break
                    
                if not line:
                    continue
                    
                # Record first chunk time
                if first_chunk_time is None:
                    first_chunk_time = current_time
                    result['time_to_first_token'] = first_chunk_time - start_time
                    
                result['chunks_received'] += 1
                result['chunk_times'].append(current_time - start_time)
                
                # Parse chunk based on provider format
                if provider in ['openai', 'lmstudio']:
                    # SSE format
                    chunk_data = self.parse_sse_chunk(line)
                    if chunk_data and not chunk_data.get('done'):
                        choices = chunk_data.get('choices', [])
                        if choices:
                            delta = choices[0].get('delta', {})
                            content = delta.get('content', '')
                            if content:
                                content_pieces.append(content)
                else:
                    # Ollama format (newline-delimited JSON)
                    try:
                        chunk_data = json.loads(line)
                        if chunk_data.get('response'):
                            content_pieces.append(chunk_data['response'])
                    except:
                        pass
                        
                # Show progress indicator
                if current_time - last_report_time > CHUNK_REPORT_INTERVAL:
                    self.print_color(CYAN, ".", end='')
                    sys.stdout.flush()
                    last_report_time = current_time
                    
            # Calculate final metrics
            end_time = time.time()
            result['total_time'] = end_time - start_time
            result['content_length'] = sum(len(piece) for piece in content_pieces)
            result['success'] = result['chunks_received'] > 1  # Must have multiple chunks for streaming
            
            if result['content_length'] > 0 and result['total_time'] > 0:
                # Rough estimate of tokens (assuming ~4 chars per token)
                estimated_tokens = result['content_length'] / 4
                result['tokens_per_second'] = estimated_tokens / result['total_time']
                
        except requests.exceptions.Timeout:
            result['error'] = "Request timeout"
        except Exception as e:
            result['error'] = str(e)
            
        return result
        
    def test_model_streaming(self, model: str, providers: List[str], sample_only: bool = False):
        """Test streaming for a model across specified providers"""
        print()
        self.print_color(WHITE, f"Testing model: {CYAN}{model}{RESET}")
        
        tested_count = 0
        
        for provider in providers:
            if sample_only and tested_count >= 1:  # Only test first available provider in sample mode
                break
                
            if model not in self.provider_models.get(provider, []):
                continue
                
            self.print_color(YELLOW, f"  Testing {provider} streaming: ", end='')
            sys.stdout.flush()
            
            self.total_tests += 1
            
            # Run appropriate test based on provider
            if provider == 'openai':
                result = self.test_openai_streaming(model)
            elif provider == 'ollama':
                result = self.test_ollama_streaming(model)
            elif provider == 'lmstudio':
                result = self.test_lmstudio_streaming(model)
            else:
                continue
                
            # Store result
            self.test_results.append(result)
            
            # Display result
            if result['success']:
                self.successful_tests += 1
                self.print_color(GREEN, f" [OK]")
                self.print_color(GREY, f"    → Endpoint: {result['endpoint_used']}")
                self.print_color(GREY, f"    → Chunks: {result['chunks_received']}")
                self.print_color(GREY, f"    → Time to first token: {result['time_to_first_token']:.3f}s")
                self.print_color(GREY, f"    → Total time: {result['total_time']:.2f}s")
                self.print_color(GREY, f"    → Tokens/sec: ~{result['tokens_per_second']:.1f}")
                
                # Check streaming quality
                if result['chunks_received'] < 5:
                    self.print_color(YELLOW, "    ⚠ Low chunk count - may not be true streaming")
                elif result['time_to_first_token'] > 5:
                    self.print_color(YELLOW, "    ⚠ Slow time to first token")
                    
            else:
                self.failed_tests += 1
                self.print_color(RED, f" [FAIL]")
                if result['error']:
                    self.print_color(RED, f"    → Error: {result['error']}")
                elif result['chunks_received'] <= 1:
                    self.print_color(RED, f"    → No streaming detected (only {result['chunks_received']} chunk)")
                    
            tested_count += 1
            
    def analyze_streaming_patterns(self):
        """Analyze chunk arrival patterns to verify true streaming"""
        print()
        self.print_color(WHITE, "Streaming Pattern Analysis:")
        self.print_color(PURPLE, "-" * 58)
        
        for result in self.test_results:
            if not result['success'] or len(result['chunk_times']) < 3:
                continue
                
            # Calculate inter-chunk delays
            delays = []
            for i in range(1, len(result['chunk_times'])):
                delays.append(result['chunk_times'][i] - result['chunk_times'][i-1])
                
            if delays:
                avg_delay = sum(delays) / len(delays)
                max_delay = max(delays)
                min_delay = min(delays)
                
                self.print_color(CYAN, f"\n{result['model']} ({result['provider']}):")
                self.print_color(GREY, f"  Average chunk interval: {avg_delay:.3f}s")
                self.print_color(GREY, f"  Min/Max interval: {min_delay:.3f}s / {max_delay:.3f}s")
                
                # Check for batching (all chunks arrive at once)
                if max_delay < 0.1 and result['chunks_received'] > 10:
                    self.print_color(YELLOW, "  ⚠ Possible batched response (not true streaming)")
                elif avg_delay > 1.0:
                    self.print_color(YELLOW, "  ⚠ Large gaps between chunks")
                else:
                    self.print_color(GREEN, "  ✓ Good streaming pattern")
                    
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
            
        # Provider summary
        provider_stats = defaultdict(lambda: {'total': 0, 'success': 0})
        for result in self.test_results:
            provider = result['provider']
            provider_stats[provider]['total'] += 1
            if result['success']:
                provider_stats[provider]['success'] += 1
                
        if provider_stats:
            print()
            self.print_color(WHITE, "Provider Summary:")
            for provider, stats in sorted(provider_stats.items()):
                success_rate = (stats['success'] * 100) // stats['total'] if stats['total'] > 0 else 0
                color = GREEN if success_rate >= 80 else YELLOW if success_rate >= 50 else RED
                print(f"  {provider:10} {color}{stats['success']}/{stats['total']} ({success_rate}%){RESET}")
                
        # Performance summary
        successful_results = [r for r in self.test_results if r['success']]
        if successful_results:
            avg_ttft = sum(r['time_to_first_token'] for r in successful_results) / len(successful_results)
            avg_tps = sum(r['tokens_per_second'] for r in successful_results) / len(successful_results)
            
            print()
            self.print_color(WHITE, "Performance Summary:")
            print(f"  Avg time to first token: {CYAN}{avg_ttft:.3f}s{RESET}")
            print(f"  Avg tokens per second:   {CYAN}{avg_tps:.1f}{RESET}")

def main():
    parser = argparse.ArgumentParser(description='Test Olla streaming responses')
    parser.add_argument('--url', default=TARGET_URL, help='Olla base URL')
    parser.add_argument('--max-time', type=int, default=DEFAULT_MAX_STREAM_TIME,
                        help='Maximum streaming time per test (seconds)')
    parser.add_argument('--models', nargs='+', help='Specific models to test')
    parser.add_argument('--providers', nargs='+', choices=['openai', 'ollama', 'lmstudio'],
                        help='Providers to test (default: all)')
    parser.add_argument('--sample', action='store_true',
                        help='Test only a few models per provider')
    parser.add_argument('--analyze', action='store_true',
                        help='Show detailed streaming pattern analysis')
    
    args = parser.parse_args()
    
    providers = args.providers or ['openai', 'ollama', 'lmstudio']
    
    tester = StreamingTester(args.url, args.max_time)
    tester.print_header()
    
    if not tester.check_health():
        sys.exit(1)
        
    print()
    
    if not tester.fetch_models():
        self.print_color(RED, "No models found!")
        sys.exit(1)
        
    print()
    self.print_color(WHITE, f"Configuration:")
    print(f"  Max streaming time: {CYAN}{args.max_time}s{RESET}")
    print(f"  Max tokens:         {CYAN}{DEFAULT_MAX_TOKENS}{RESET}")
    print(f"  Providers:          {CYAN}{', '.join(providers)}{RESET}")
    
    # Determine which models to test
    if args.models:
        models_to_test = args.models
    elif args.sample:
        # Sample 2-3 models per provider
        models_to_test = []
        for provider in providers:
            provider_models = tester.provider_models.get(provider, [])
            models_to_test.extend(provider_models[:3])
        models_to_test = list(set(models_to_test))  # Remove duplicates
    else:
        models_to_test = tester.all_models
        
    self.print_color(WHITE, f"\nTesting {len(models_to_test)} models for streaming capability...")
    
    # Test each model
    for model in models_to_test:
        tester.test_model_streaming(model, providers, sample_only=args.sample)
        
    # Show analysis if requested
    if args.analyze and tester.test_results:
        tester.analyze_streaming_patterns()
        
    tester.print_summary()

def run_main():
    """Main execution function wrapped for interrupt handling"""
    # Fix 'self' references in main()
    class MainRunner:
        @staticmethod
        def print_color(color: str, message: str):
            print(f"{color}{message}{RESET}")
            
    runner = MainRunner()
    
    parser = argparse.ArgumentParser(description='Test Olla streaming responses')
    parser.add_argument('--url', default=TARGET_URL, help='Olla base URL')
    parser.add_argument('--max-time', type=int, default=DEFAULT_MAX_STREAM_TIME,
                        help='Maximum streaming time per test (seconds)')
    parser.add_argument('--models', nargs='+', help='Specific models to test')
    parser.add_argument('--providers', nargs='+', choices=['openai', 'ollama', 'lmstudio'],
                        help='Providers to test (default: all)')
    parser.add_argument('--sample', action='store_true',
                        help='Test only a few models per provider')
    parser.add_argument('--analyze', action='store_true',
                        help='Show detailed streaming pattern analysis')
    
    args = parser.parse_args()
    
    providers = args.providers or ['openai', 'ollama', 'lmstudio']
    
    tester = StreamingTester(args.url, args.max_time)
    tester.print_header()
    
    if not tester.check_health():
        sys.exit(1)
        
    print()
    
    if not tester.fetch_models():
        runner.print_color(RED, "No models found!")
        sys.exit(1)
        
    print()
    runner.print_color(WHITE, f"Configuration:")
    print(f"  Max streaming time: {CYAN}{args.max_time}s{RESET}")
    print(f"  Max tokens:         {CYAN}{DEFAULT_MAX_TOKENS}{RESET}")
    print(f"  Providers:          {CYAN}{', '.join(providers)}{RESET}")
    
    # Determine which models to test
    if args.models:
        models_to_test = args.models
    elif args.sample:
        # Sample 2-3 models per provider
        models_to_test = []
        for provider in providers:
            provider_models = tester.provider_models.get(provider, [])
            models_to_test.extend(provider_models[:3])
        models_to_test = list(set(models_to_test))  # Remove duplicates
    else:
        models_to_test = tester.all_models
        
    runner.print_color(WHITE, f"\nTesting {len(models_to_test)} models for streaming capability...")
    
    # Test each model
    for model in models_to_test:
        tester.test_model_streaming(model, providers, sample_only=args.sample)
        
    # Show analysis if requested
    if args.analyze and tester.test_results:
        tester.analyze_streaming_patterns()
        
    tester.print_summary()


if __name__ == '__main__':
    try:
        run_main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)  # Standard exit code for SIGINT
