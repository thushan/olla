#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Streaming Detection Test Script
Detects whether Olla is streaming or buffering responses by analyzing chunk timing patterns
"""

import sys
import json
import time
import argparse
import requests
import os
from typing import Dict, List, Tuple, Optional, Any
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
BOLD = '\033[1m'

# Configuration
TARGET_URL = "http://localhost:40114"
DEFAULT_TIMEOUT = 30

class StreamingDetector:
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.chunk_times: List[Tuple[float, str]] = []
        
    def print_color(self, color: str, message: str, end='\n'):
        print(f"{color}{message}{RESET}", end=end)
        
    def print_header(self):
        self.print_color(PURPLE, "=" * 60)
        self.print_color(PURPLE, f"  {CYAN}Olla Streaming Detection Test{RESET}")
        self.print_color(PURPLE, f"  {GREY}Analyzing proxy streaming vs buffering behavior{RESET}")
        self.print_color(PURPLE, "=" * 60)
        print()
        
    def check_health(self) -> bool:
        """Check if Olla is available"""
        self.print_color(YELLOW, "Checking Olla availability...")
        try:
            response = requests.get(f"{self.base_url}/internal/health", timeout=5)
            if response.status_code == 200:
                self.print_color(GREEN, "[OK] Olla is reachable")
                return True
        except Exception as e:
            pass
        self.print_color(RED, f"[FAIL] Cannot reach Olla at {self.base_url}")
        return False
        
    def get_available_models(self) -> List[str]:
        """Fetch available models from Olla"""
        try:
            response = requests.get(f"{self.base_url}/olla/models", timeout=10)
            if response.status_code != 200:
                return []
                
            data = response.json()
            models = data.get('data', data.get('models', []))
            
            model_names = []
            for model in models:
                model_id = model.get('id', model.get('name', ''))
                if model_id:
                    model_names.append(model_id)
                    
            return sorted(model_names)
            
        except Exception as e:
            self.print_color(RED, f"Error fetching models: {e}")
            return []
            
    def select_model(self, preferred_model: Optional[str] = None, manual_select: bool = False) -> Optional[str]:
        """Select a model from CLI arg, auto-select phi models, or user input"""
        models = self.get_available_models()
        if not models:
            self.print_color(RED, "No models available")
            return None
            
        # Filter out embedding models
        text_models = [m for m in models if 'embed' not in m.lower()]
        if not text_models:
            text_models = models  # Fallback to all models if no text models
            
        # Check if preferred model exists
        if preferred_model:
            if preferred_model in text_models:
                self.print_color(GREEN, f"Using specified model: {preferred_model}")
                return preferred_model
            else:
                self.print_color(RED, f"Model '{preferred_model}' not found in available models")
                return None
        
        # Auto-select phi models if available and not manual selection
        if not manual_select:
            preferred_models = ['phi4:latest', 'phi3.5:latest', 'phi3:latest']
            for model in preferred_models:
                if model in text_models:
                    self.print_color(GREEN, f"Auto-selected model: {model}")
                    return model
            
            # No phi models found
            self.print_color(YELLOW, "No phi models found (phi4:latest, phi3.5:latest, or phi3:latest)")
            self.print_color(YELLOW, "Please select a model manually:")
            
        # Prompt user
        self.print_color(WHITE, "\nAvailable models:")
        for i, model in enumerate(text_models[:10], 1):  # Show max 10 models
            self.print_color(GREY, f"  {i:2d}. {model}")
            
        if len(text_models) > 10:
            self.print_color(GREY, f"  ... and {len(text_models) - 10} more")
            
        try:
            choice = input(f"{CYAN}Select model number (1-{min(10, len(text_models))}): {RESET}")
            model_index = int(choice.strip()) - 1
            
            if 0 <= model_index < len(text_models):
                selected = text_models[model_index]
                self.print_color(GREEN, f"Selected model: {selected}")
                return selected
            else:
                self.print_color(RED, "Invalid selection")
                return None
                
        except (ValueError, KeyboardInterrupt):
            self.print_color(RED, "Invalid input or cancelled")
            return None
        
    def test_streaming(self, model: str = "phi4:latest", prompt: str = "Count from 1 to 10 slowly", endpoint_type: str = "openai") -> Dict[str, Any]:
        """Test if the endpoint is streaming or buffering."""
        # Different endpoint formats
        if endpoint_type == "openai":
            url = f"{self.base_url}/olla/openai/v1/chat/completions"
            payload = {
                "model": model,
                "messages": [{"role": "user", "content": prompt}],
                "stream": True
            }
        else:  # ollama
            url = f"{self.base_url}/olla/ollama/api/generate"
            payload = {
                "model": model,
                "prompt": prompt,
                "stream": True
            }
        
        headers = {
            "Content-Type": "application/json"
        }
        
        self.print_color(WHITE, f"\nTesting {endpoint_type.upper()} streaming behavior:")
        self.print_color(GREY, f"  URL: {url}")
        self.print_color(GREY, f"  Model: {model}")
        self.print_color(GREY, f"  Prompt: {prompt[:50]}{'...' if len(prompt) > 50 else ''}")
        print()
        
        self.chunk_times = []
        start_time = time.time()
        first_chunk_time = None
        chunk_count = 0
        total_tokens = []
        endpoint_used = ''
        
        # Visual streaming indicator
        self.print_color(YELLOW, "  Streaming: ", end='')
        sys.stdout.flush()
        
        try:
            response = requests.post(url, json=payload, headers=headers, stream=True, timeout=DEFAULT_TIMEOUT)
            response.raise_for_status()
            
            # Extract response headers
            endpoint_used = response.headers.get('X-Olla-Endpoint', 'unknown')
            
            for line in response.iter_lines(decode_unicode=True, chunk_size=1):
                if line:
                    current_time = time.time()
                    elapsed = current_time - start_time
                    
                    if endpoint_type == "openai" and line.startswith('data: '):
                        chunk_count += 1
                        data_str = line[6:]
                        
                        if data_str == '[DONE]':
                            break
                            
                        try:
                            data = json.loads(data_str)
                            content = data.get('choices', [{}])[0].get('delta', {}).get('content', '')
                            
                            if content:
                                total_tokens.append(content)
                                self.chunk_times.append((elapsed, content))
                                
                                if first_chunk_time is None:
                                    first_chunk_time = elapsed
                                    self.print_color(GREEN, "●", end='')
                                else:
                                    self.print_color(CYAN, "●", end='')
                                sys.stdout.flush()
                                
                        except json.JSONDecodeError:
                            pass
                    
                    elif endpoint_type == "ollama":
                        # Ollama format - newline-delimited JSON
                        try:
                            data = json.loads(line)
                            response_text = data.get('response', '')
                            
                            if response_text:
                                chunk_count += 1
                                total_tokens.append(response_text)
                                self.chunk_times.append((elapsed, response_text))
                                
                                if first_chunk_time is None:
                                    first_chunk_time = elapsed
                                    self.print_color(GREEN, "●", end='')
                                else:
                                    self.print_color(CYAN, "●", end='')
                                sys.stdout.flush()
                                
                            if data.get('done', False):
                                break
                                
                        except json.JSONDecodeError:
                            pass
            
            self.print_color(GREEN, " [OK]")
            self.print_color(GREY, f"  Endpoint: {endpoint_used}")
            return self._analyze_results(start_time, first_chunk_time, chunk_count, endpoint_type)
            
        except requests.exceptions.RequestException as e:
            self.print_color(RED, " [FAIL]")
            self.print_color(RED, f"  Error: {e}")
            return {"error": str(e)}
    
    def _analyze_results(self, start_time: float, first_chunk_time: Optional[float], chunk_count: int, endpoint_type: str) -> Dict[str, Any]:
        """Analyze the chunk timing to determine streaming behavior."""
        if not self.chunk_times:
            return {"error": "No chunks received", "chunk_count": 0}
        
        total_time = time.time() - start_time
        
        # Calculate inter-chunk delays
        inter_chunk_delays = []
        for i in range(1, len(self.chunk_times)):
            delay = self.chunk_times[i][0] - self.chunk_times[i-1][0]
            inter_chunk_delays.append(delay)
        
        # Analyze the pattern
        avg_delay = sum(inter_chunk_delays) / len(inter_chunk_delays) if inter_chunk_delays else 0
        max_delay = max(inter_chunk_delays) if inter_chunk_delays else 0
        min_delay = min(inter_chunk_delays) if inter_chunk_delays else 0
        
        # Determine if it's streaming or buffering
        is_streaming = self._determine_streaming_mode(
            first_chunk_time or 0, avg_delay, max_delay, len(self.chunk_times)
        )
        
        # Print analysis
        self.print_color(GREY, f"  Total time: {total_time:.3f}s")
        self.print_color(GREY, f"  First chunk: {first_chunk_time:.3f}s" if first_chunk_time else "  First chunk: N/A")
        self.print_color(GREY, f"  Total chunks: {len(self.chunk_times)}")
        if inter_chunk_delays:
            self.print_color(GREY, f"  Avg inter-chunk delay: {avg_delay*1000:.1f}ms")
            self.print_color(GREY, f"  Min/Max delays: {min_delay*1000:.1f}ms / {max_delay*1000:.1f}ms")
        
        # Mode determination with color
        mode_color = GREEN if is_streaming else RED
        mode_text = "STREAMING" if is_streaming else "STANDARD"
        self.print_color(mode_color, f"  Mode: {mode_text}")
        
        return {
            "mode": "streaming" if is_streaming else "standard",
            "total_time": total_time,
            "first_chunk_time": first_chunk_time,
            "chunk_count": len(self.chunk_times),
            "avg_inter_chunk_delay": avg_delay,
            "max_inter_chunk_delay": max_delay,
            "min_inter_chunk_delay": min_delay,
            "is_streaming": is_streaming,
            "endpoint_type": endpoint_type
        }
    
    def _determine_streaming_mode(self, first_chunk_time: float, avg_delay: float, 
                                  max_delay: float, chunk_count: int) -> bool:
        """Determine if the response pattern indicates streaming or buffering."""
        # Criteria for streaming:
        # 1. Multiple chunks received (not just one big response)
        # 2. Regular chunks with reasonable delays between them
        # 3. No extremely large gaps that would indicate buffering
        
        # If we only get 1-2 chunks, it's likely standard mode (no streaming)
        if chunk_count < 3:
            return False
        
        # For many chunks with consistent timing, it's streaming
        # Even if first chunk takes time (model loading), subsequent chunks should be regular
        if chunk_count > 10:
            # Many chunks with reasonable average delay = streaming
            if avg_delay < 0.2:  # 200ms average is fine for streaming
                return True
            # If we have many chunks but long delays, might be choppy streaming
            if avg_delay < 0.5:  # Up to 500ms average could still be streaming
                return True
        
        # Check first chunk time - be more lenient as models need to load
        if first_chunk_time > 5.0:  # 5 seconds is definitely too long
            return False
        
        # If we see very large gaps between chunks, it's likely buffering
        if max_delay > 1.0:  # 1 second gap suggests buffering
            return False
        
        # Default: if we have multiple chunks, assume streaming
        return chunk_count >= 3
    
    def test_stream_false(self, model: str) -> Dict[str, Any]:
        """Test with stream:false to verify buffering behavior."""
        self.print_color(WHITE, "\nTest 4: Text with stream:false (should buffer)")
        
        url = f"{self.base_url}/olla/openai/v1/chat/completions"
        payload = {
            "model": model,
            "messages": [{"role": "user", "content": "Count from 1 to 5"}],
            "stream": False  # Explicitly request non-streaming
        }
        
        headers = {
            "Content-Type": "application/json"
        }
        
        self.print_color(YELLOW, "  Testing non-streaming request: ", end='')
        sys.stdout.flush()
        
        start_time = time.time()
        
        try:
            response = requests.post(url, json=payload, headers=headers, timeout=DEFAULT_TIMEOUT)
            response_time = time.time() - start_time
            
            if response.status_code == 200:
                self.print_color(GREEN, "[OK]")
                endpoint_used = response.headers.get('X-Olla-Endpoint', 'unknown')
                self.print_color(GREY, f"  Endpoint: {endpoint_used}")
                self.print_color(GREY, f"  Response time: {response_time:.3f}s")
                self.print_color(GREEN, f"  Mode: STANDARD (as expected with stream:false)")
                
                return {
                    "mode": "standard",
                    "total_time": response_time,
                    "first_chunk_time": response_time,  # All data arrives at once
                    "chunk_count": 1,
                    "is_standard": True,
                    "endpoint_type": "text_no_stream"
                }
            else:
                self.print_color(RED, " [FAIL]")
                self.print_color(RED, f"  HTTP {response.status_code}")
                return {"error": f"HTTP {response.status_code}"}
                
        except requests.exceptions.RequestException as e:
            self.print_color(RED, " [FAIL]")
            self.print_color(RED, f"  Error: {e}")
            return {"error": str(e)}
    
    def test_image_generation(self, model: str) -> Dict[str, Any]:
        """Test image generation to verify buffering behavior."""
        self.print_color(WHITE, "\nTest 5: Image generation (should buffer)")
        
        # First, try to find an image generation model
        models = self.get_available_models()
        image_models = [m for m in models if any(keyword in m.lower() for keyword in ['image', 'dall', 'stable', 'diffusion', 'sdxl'])]
        
        if not image_models:
            # Try a generic image endpoint
            url = f"{self.base_url}/olla/openai/v1/images/generations"
            payload = {
                "prompt": "A simple test image",
                "n": 1,
                "size": "256x256"
            }
        else:
            # Use the first image model found
            image_model = image_models[0]
            self.print_color(GREY, f"  Using image model: {image_model}")
            url = f"{self.base_url}/olla/openai/v1/chat/completions"
            payload = {
                "model": image_model,
                "messages": [{"role": "user", "content": "Generate a simple test image"}],
                "stream": True  # Test if it properly buffers even with stream:true
            }
        
        headers = {
            "Content-Type": "application/json"
        }
        
        self.print_color(YELLOW, "  Testing image endpoint: ", end='')
        sys.stdout.flush()
        
        start_time = time.time()
        chunk_count = 0
        first_chunk_time = None
        
        try:
            response = requests.post(url, json=payload, headers=headers, stream=True, timeout=DEFAULT_TIMEOUT)
            
            if response.status_code != 200:
                self.print_color(YELLOW, " [SKIP]")
                self.print_color(GREY, f"  Image generation not available (HTTP {response.status_code})")
                return {"skipped": True, "reason": "Image generation not available"}
            
            # Check content type
            content_type = response.headers.get('Content-Type', '')
            endpoint_used = response.headers.get('X-Olla-Endpoint', 'unknown')
            
            # Read response
            for chunk in response.iter_content(chunk_size=8192):
                if chunk:
                    current_time = time.time()
                    elapsed = current_time - start_time
                    
                    if first_chunk_time is None:
                        first_chunk_time = elapsed
                        self.print_color(GREEN, "●", end='')
                    else:
                        self.print_color(CYAN, "●", end='')
                    sys.stdout.flush()
                    
                    chunk_count += 1
            
            self.print_color(GREEN, " [OK]")
            self.print_color(GREY, f"  Endpoint: {endpoint_used}")
            
            # Analyze timing
            total_time = time.time() - start_time
            
            # For images, we expect standard mode (no streaming)
            is_standard = chunk_count == 1 or (chunk_count > 1 and first_chunk_time > 0.5)
            
            self.print_color(GREY, f"  Total time: {total_time:.3f}s")
            self.print_color(GREY, f"  Content-Type: {content_type}")
            
            mode_color = GREEN if is_standard else RED
            mode_text = "STANDARD" if is_standard else "STREAMING"
            self.print_color(mode_color, f"  Mode: {mode_text} (expected: STANDARD for images)")
            
            return {
                "mode": "standard" if is_standard else "streaming",
                "total_time": total_time,
                "first_chunk_time": first_chunk_time,
                "chunk_count": chunk_count,
                "is_standard": is_standard,
                "content_type": content_type,
                "endpoint_type": "image"
            }
            
        except requests.exceptions.RequestException as e:
            self.print_color(RED, " [FAIL]")
            self.print_color(RED, f"  Error: {e}")
            return {"error": str(e)}
    
    def run_comprehensive_test(self, model: str) -> Dict[str, Any]:
        """Run comprehensive streaming detection tests."""
        results = {
            "model": model,
            "tests": {}
        }
        
        # Test 1: Long response (best for detecting buffering)
        self.print_color(WHITE, "\nTest 1: Long response (counting)")
        results["tests"]["long_response"] = self.test_streaming(
            model=model,
            prompt="Count from 1 to 20, saying each number slowly",
            endpoint_type="openai"
        )
        
        # Test 2: Quick response
        self.print_color(WHITE, "\nTest 2: Quick response")
        results["tests"]["quick_response"] = self.test_streaming(
            model=model,
            prompt="Say hello",
            endpoint_type="openai"
        )
        
        # Test 3: Ollama format (if available)
        self.print_color(WHITE, "\nTest 3: Ollama format endpoint")
        results["tests"]["ollama_format"] = self.test_streaming(
            model=model,
            prompt="Count from 1 to 10",
            endpoint_type="ollama"
        )
        
        # Test 4: Text with stream:false (should be standard mode)
        results["tests"]["stream_false"] = self.test_stream_false(model)
        
        # Test 5: Image generation (should be standard mode)
        results["tests"]["image_generation"] = self.test_image_generation(model)
        
        return results
    
    def analyze_comprehensive_results(self, results: Dict[str, Any]):
        """Analyze comprehensive test results."""
        print()
        self.print_color(PURPLE, "=" * 60)
        self.print_color(WHITE, f"{BOLD}Streaming Detection Analysis{RESET}")
        self.print_color(PURPLE, "=" * 60)
        
        model = results["model"]
        tests = results["tests"]
        
        # Count correct behavior (streaming for text, standard for binary)
        correct_behavior_count = 0
        text_streaming_count = 0
        text_test_count = 0
        
        for test_name, test_result in tests.items():
            if "error" in test_result or test_result.get("skipped"):
                continue
                
            endpoint_type = test_result.get("endpoint_type", "text")
            mode = test_result.get("mode", "unknown")
            
            if endpoint_type in ["binary", "image", "text_no_stream"]:
                # These should be standard mode
                if mode == "standard":
                    correct_behavior_count += 1
            else:
                # Regular text should be streaming
                text_test_count += 1
                if mode == "streaming":
                    correct_behavior_count += 1
                    text_streaming_count += 1
        
        total_valid_tests = len([t for t in tests.values() if "error" not in t and not t.get("skipped")])
        
        self.print_color(WHITE, f"\nModel tested: {CYAN}{model}{RESET}")
        self.print_color(WHITE, f"Successful tests: {total_valid_tests}")
        
        if text_test_count > 0:
            text_streaming_percentage = (text_streaming_count / text_test_count) * 100
            if text_streaming_percentage >= 80:
                self.print_color(GREEN, f"\n✅ Olla is configured for STREAMING mode ({text_streaming_percentage:.0f}% of text responses)")
                self.print_color(GREY, "   Tokens are delivered in real-time as they're generated")
            elif text_streaming_percentage >= 50:
                self.print_color(YELLOW, f"\n⚠️  Olla shows MIXED behavior ({text_streaming_percentage:.0f}% text streaming)")
                self.print_color(GREY, "   Some endpoints stream while others buffer")
            else:
                self.print_color(RED, f"\n❌ Olla appears to be BUFFERING text responses ({text_streaming_percentage:.0f}% streaming)")
                self.print_color(GREY, "   Consider checking the proxy profile configuration")
        
        # Show detailed results
        self.print_color(WHITE, "\nDetailed Results:")
        for test_name, test_result in tests.items():
            if "error" in test_result:
                self.print_color(RED, f"  {test_name}: FAILED - {test_result['error']}")
            elif test_result.get("skipped"):
                self.print_color(YELLOW, f"  {test_name}: SKIPPED - {test_result.get('reason', 'Unknown')}")
            else:
                mode = test_result.get("mode", "unknown")
                endpoint_type = test_result.get("endpoint_type", "text")
                
                # Expected behavior:
                # - Binary/image content: standard is good (GREEN)
                # - Text with stream:false: standard is good (GREEN)
                # - Text with stream:true: streaming is good (GREEN)
                if endpoint_type in ["binary", "image", "text_no_stream"]:
                    color = GREEN if mode == "standard" else RED
                    expected = " (expected: standard)"
                else:
                    color = GREEN if mode == "streaming" else RED
                    expected = " (expected: streaming)"
                
                ttft = test_result.get("first_chunk_time", 0)
                chunks = test_result.get("chunk_count", 0)
                self.print_color(color, f"  {test_name}: {mode.upper()}{expected} (TTFT: {ttft:.3f}s, chunks: {chunks})")
        
        # Configuration hints
        self.print_color(WHITE, "\n💡 Configuration Options:")
        self.print_color(GREY, "  profile: 'streaming' - Always stream (best for LLMs)")
        self.print_color(GREY, "  profile: 'standard'  - Normal HTTP delivery (best for files/images)")
        self.print_color(GREY, "  profile: 'auto'      - Detect based on content type")
        
        # Note about stream_buffer_size
        self.print_color(WHITE, "\n📝 Note on stream_buffer_size:")
        self.print_color(GREY, "  In streaming mode, buffer size affects chunk granularity")
        self.print_color(GREY, "  Smaller buffers = more frequent flushes = lower latency")
        self.print_color(GREY, "  Larger buffers = fewer flushes = better throughput")
        self.print_color(GREY, "  Default 8KB works well for most LLM streaming scenarios")

def main():
    parser = argparse.ArgumentParser(description='Test Olla streaming vs buffering behavior')
    parser.add_argument('--url', default=TARGET_URL, 
                       help=f'Olla base URL (default: {TARGET_URL})')
    parser.add_argument('--model',
                       help='Model to test with (default: auto-selects phi4:latest if available)')
    parser.add_argument('--select-model', action='store_true',
                       help='Force model selection menu instead of auto-selecting')
    parser.add_argument('--quick', action='store_true',
                       help='Run quick test only (single endpoint)')
    parser.add_argument('--timeout', type=int, default=DEFAULT_TIMEOUT,
                       help=f'Request timeout in seconds (default: {DEFAULT_TIMEOUT})')
    
    args = parser.parse_args()
    
    detector = StreamingDetector(args.url)
    detector.print_header()
    
    # Check health
    if not detector.check_health():
        sys.exit(1)
    
    # Select model
    detector.print_color(YELLOW, "\nDetecting available models...")
    selected_model = detector.select_model(args.model, args.select_model)
    if not selected_model:
        detector.print_color(RED, "No model selected. Exiting.")
        sys.exit(1)
    
    if args.quick:
        # Quick single test
        detector.print_color(WHITE, f"\nRunning quick streaming test with model: {CYAN}{selected_model}{RESET}")
        result = detector.test_streaming(
            model=selected_model,
            prompt="Count from 1 to 10 slowly",
            endpoint_type="openai"
        )
        
        if result.get("is_streaming"):
            detector.print_color(GREEN, "\n✅ Olla is streaming responses in real-time!")
        else:
            detector.print_color(RED, "\n❌ Olla appears to be buffering responses!")
    else:
        # Comprehensive test
        detector.print_color(WHITE, f"\nRunning comprehensive streaming tests with model: {CYAN}{selected_model}{RESET}")
        results = detector.run_comprehensive_test(selected_model)
        detector.analyze_comprehensive_results(results)
    
    # Exit with appropriate code
    if args.quick:
        success = result.get("is_streaming", False) if "error" not in result else False
    else:
        # Check if all tests behaved correctly
        all_correct = True
        for test_result in results.get("tests", {}).values():
            if "error" in test_result or test_result.get("skipped"):
                continue
            
            endpoint_type = test_result.get("endpoint_type", "text")
            mode = test_result.get("mode", "unknown")
            
            # Expected behavior:
            # - Binary/image: should be standard mode
            # - Text with stream:false: should be standard mode
            # - Text with stream:true: should be streaming
            if endpoint_type in ["binary", "image", "text_no_stream"]:
                if mode != "standard":
                    all_correct = False
            else:
                if mode != "streaming":
                    all_correct = False
        
        success = all_correct
    
    sys.exit(0 if success else 1)

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)  # Standard exit code for SIGINT