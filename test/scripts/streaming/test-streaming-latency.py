#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Olla Streaming Latency Test Script
Tests streaming latency and choppiness by simulating OpenWebUI behavior
"""

import sys
import json
import time
import argparse
import requests
import random
import os
from typing import Dict, List, Tuple, Optional, Any
from collections import deque
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
DEFAULT_QUESTION_FILE = "questions.txt"
DEFAULT_TIMEOUT = 60

class StreamingLatencyTester:
    def __init__(self, base_url: str, timeout: int):
        self.base_url = base_url
        self.timeout = timeout
        self.available_models = []
        self.questions = []
        
    def print_color(self, color: str, message: str, end='\n'):
        print(f"{color}{message}{RESET}", end=end)
        
    def print_header(self):
        self.print_color(PURPLE, "=" * 60)
        self.print_color(PURPLE, f"  {CYAN}Olla Streaming Latency Test{RESET}")
        self.print_color(PURPLE, f"  {GREY}Simulating OpenWebUI streaming behavior{RESET}")
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
        
    def fetch_models(self) -> List[str]:
        """Fetch available models from Olla"""
        self.print_color(YELLOW, "Fetching available models...")
        
        try:
            response = requests.get(f"{self.base_url}/olla/models", timeout=10)
            if response.status_code != 200:
                self.print_color(RED, f"Failed to fetch models: HTTP {response.status_code}")
                return []
                
            data = response.json()
            models = data.get('data', data.get('models', []))
            
            model_names = []
            for model in models:
                model_id = model.get('id', model.get('name', ''))
                # Filter out embedding models
                if model_id and 'embed' not in model_id.lower():
                    model_names.append(model_id)
                    
            self.available_models = sorted(model_names)
            self.print_color(GREEN, f"Found {len(self.available_models)} available models")
            
            # Display models with numbers for selection
            for i, model in enumerate(self.available_models, 1):
                self.print_color(GREY, f"  {i:2d}. {model}")
                
            return self.available_models
            
        except Exception as e:
            self.print_color(RED, f"Error fetching models: {e}")
            return []
            
    def load_questions(self, question_file: str) -> bool:
        """Load questions from file"""
        if not os.path.exists(question_file):
            self.print_color(YELLOW, f"Questions file '{question_file}' not found. Creating default questions...")
            self.create_default_questions(question_file)
            
        try:
            with open(question_file, 'r', encoding='utf-8') as f:
                lines = f.readlines()
                self.questions = [line.strip() for line in lines if line.strip() and not line.strip().startswith('#')]
                
            if not self.questions:
                self.print_color(RED, f"No valid questions found in {question_file}")
                return False
                
            self.print_color(GREEN, f"Loaded {len(self.questions)} questions from {question_file}")
            return True
            
        except Exception as e:
            self.print_color(RED, f"Error loading questions: {e}")
            return False
            
    def create_default_questions(self, question_file: str):
        """Create a default questions file"""
        default_questions = [
            "# Questions for testing streaming latency",
            "# One question per line, lines starting with # are ignored",
            "",
            "Tell me a short story about a robot learning to paint.",
            "Explain how photosynthesis works in simple terms.",
            "What are the main differences between machine learning and artificial intelligence?",
            "Describe the process of making bread from scratch.",
            "Write a haiku about the ocean.",
            "Explain the concept of recursion in programming with an example.",
            "What would happen if gravity suddenly became twice as strong?",
            "Tell me about the history of the internet in three paragraphs.",
            "How do solar panels convert sunlight into electricity?",
            "Write a dialogue between two characters meeting for the first time.",
            "Explain why the sky appears blue during the day.",
            "What are the benefits and drawbacks of renewable energy?",
            "Describe how a computer processor works at a basic level.",
            "Tell me about an interesting historical event from the 1960s.",
            "How do vaccines help prevent diseases?",
            "Write a short poem about autumn leaves.",
            "Explain the water cycle and its importance to life on Earth.",
            "What makes a good leader, and can leadership skills be learned?",
            "Describe the process of evolution by natural selection.",
            "How do search engines like Google find and rank web pages?"
        ]
        
        try:
            with open(question_file, 'w', encoding='utf-8') as f:
                f.write('\n'.join(default_questions))
            self.print_color(GREEN, f"Created default questions file: {question_file}")
        except Exception as e:
            self.print_color(RED, f"Error creating questions file: {e}")
            
    def select_model(self, model_arg: Optional[str]) -> Optional[str]:
        """Select model from CLI arg or user input"""
        if model_arg:
            if model_arg in self.available_models:
                self.print_color(GREEN, f"Using specified model: {model_arg}")
                return model_arg
            else:
                self.print_color(RED, f"Model '{model_arg}' not found in available models")
                return None
                
        print()
        self.print_color(WHITE, "Select a model to test:")
        
        try:
            choice = input(f"{CYAN}Enter model number (1-{len(self.available_models)}): {RESET}")
            model_index = int(choice.strip()) - 1
            
            if 0 <= model_index < len(self.available_models):
                selected = self.available_models[model_index]
                self.print_color(GREEN, f"Selected model: {selected}")
                return selected
            else:
                self.print_color(RED, "Invalid selection")
                return None
                
        except (ValueError, KeyboardInterrupt):
            self.print_color(RED, "Invalid input or cancelled")
            return None
            
    def get_random_question(self) -> str:
        """Get a random question from the loaded questions"""
        return random.choice(self.questions)
        
    def test_streaming_latency(self, model: str, question: str) -> Dict[str, Any]:
        """Test streaming latency for a specific model and question"""
        result = {
            'model': model,
            'question': question[:50] + "..." if len(question) > 50 else question,
            'success': False,
            'chunks_received': 0,
            'total_chars': 0,
            'time_to_first_token': 0.0,
            'total_time': 0.0,
            'endpoint_used': '',
            'chunk_intervals': [],
            'error': None,
            'streaming_quality': 'unknown'
        }
        
        # Use OpenAI-compatible endpoint (most common)
        url = f"{self.base_url}/olla/openai/v1/chat/completions"
        payload = {
            "model": model,
            "messages": [
                {"role": "user", "content": question}
            ],
            "stream": True,
            "max_tokens": 200,
            "temperature": 0.7
        }
        
        try:
            start_time = time.time()
            last_chunk_time = start_time
            
            self.print_color(GREY, f"    Question: {question[:60]}{'...' if len(question) > 60 else ''}")
            self.print_color(YELLOW, f"    Streaming: ", end='')
            sys.stdout.flush()
            
            with requests.post(url, json=payload, stream=True, timeout=self.timeout) as response:
                if response.status_code != 200:
                    result['error'] = f"HTTP {response.status_code}: {response.text[:100]}"
                    return result
                    
                # Extract response headers
                result['endpoint_used'] = response.headers.get('X-Olla-Endpoint', 'unknown')
                
                first_token_received = False
                content_buffer = ""
                chunk_count = 0
                
                # Process streaming response
                for line in response.iter_lines(decode_unicode=True, chunk_size=1):
                    if not line or not line.strip():
                        continue
                        
                    current_time = time.time()
                    
                    # Handle Server-Sent Events format
                    if line.startswith('data: '):
                        data = line[6:].strip()  # Remove 'data: ' prefix
                        
                        if data == '[DONE]':
                            break
                            
                        try:
                            json_data = json.loads(data)
                            choices = json_data.get('choices', [])
                            
                            if choices and 'delta' in choices[0]:
                                delta = choices[0]['delta']
                                content = delta.get('content', '')
                                
                                if content:
                                    if not first_token_received:
                                        result['time_to_first_token'] = current_time - start_time
                                        first_token_received = True
                                        self.print_color(GREEN, "●", end='')
                                    else:
                                        self.print_color(CYAN, "●", end='')
                                        # Record interval between chunks
                                        interval = current_time - last_chunk_time
                                        result['chunk_intervals'].append(interval)
                                        
                                    sys.stdout.flush()
                                    content_buffer += content
                                    chunk_count += 1
                                    last_chunk_time = current_time
                                    
                        except json.JSONDecodeError:
                            continue
                            
                result['success'] = chunk_count > 0
                result['chunks_received'] = chunk_count
                result['total_chars'] = len(content_buffer)
                result['total_time'] = time.time() - start_time
                
                # Analyze streaming quality
                if result['chunk_intervals']:
                    avg_interval = sum(result['chunk_intervals']) / len(result['chunk_intervals'])
                    max_interval = max(result['chunk_intervals'])
                    
                    # Classify based on chunk patterns
                    if len(result['chunk_intervals']) < 3:
                        # Too few chunks to be streaming
                        result['streaming_quality'] = 'single_chunk'
                    elif max_interval > 1.0:  # Long pauses between chunks
                        result['streaming_quality'] = 'choppy'
                    elif avg_interval < 0.01:  # Very fast (<10ms) suggests batched
                        result['streaming_quality'] = 'batched'
                    elif avg_interval < 0.05:  # 10-50ms is typical for good streaming
                        result['streaming_quality'] = 'smooth'
                    elif avg_interval < 0.2:  # 50-200ms is acceptable streaming
                        result['streaming_quality'] = 'smooth'
                    else:
                        # Slower than 200ms average
                        result['streaming_quality'] = 'choppy'
                else:
                    result['streaming_quality'] = 'single_chunk'
                    
        except requests.exceptions.Timeout:
            result['error'] = "Request timeout"
        except Exception as e:
            result['error'] = str(e)
            
        return result
        
    def analyze_results(self, results: List[Dict[str, Any]]):
        """Analyze and display results"""
        print("\n")
        self.print_color(PURPLE, "=" * 60)
        self.print_color(WHITE, f"{BOLD}Streaming Latency Analysis{RESET}")
        self.print_color(PURPLE, "=" * 60)
        
        successful_results = [r for r in results if r['success']]
        
        if not successful_results:
            self.print_color(RED, "No successful streaming tests to analyze")
            return
            
        # Overall statistics
        total_tests = len(results)
        successful_tests = len(successful_results)
        success_rate = (successful_tests / total_tests) * 100
        
        self.print_color(WHITE, f"\nOverall Results:")
        self.print_color(GREY, f"  Total tests: {total_tests}")
        self.print_color(GREY, f"  Successful: {successful_tests}")
        self.print_color(GREY, f"  Success rate: {success_rate:.1f}%")
        
        # Latency analysis
        ttft_times = [r['time_to_first_token'] for r in successful_results if r['time_to_first_token'] > 0]
        if ttft_times:
            avg_ttft = sum(ttft_times) / len(ttft_times)
            min_ttft = min(ttft_times)
            max_ttft = max(ttft_times)
            
            self.print_color(WHITE, f"\nTime to First Token:")
            self.print_color(GREY, f"  Average: {avg_ttft:.3f}s")
            self.print_color(GREY, f"  Range: {min_ttft:.3f}s - {max_ttft:.3f}s")
            
        # Streaming quality analysis
        quality_counts = {}
        for result in successful_results:
            quality = result['streaming_quality']
            quality_counts[quality] = quality_counts.get(quality, 0) + 1
            
        self.print_color(WHITE, f"\nStreaming Quality:")
        for quality, count in quality_counts.items():
            percentage = (count / len(successful_results)) * 100
            color = GREEN if quality == 'smooth' else YELLOW if quality == 'choppy' else RED
            self.print_color(color, f"  {quality}: {count} tests ({percentage:.1f}%)")
            
        # Detailed chunk interval analysis
        self.print_color(WHITE, f"\nChunk Interval Analysis:")
        
        for result in successful_results:
            if result['chunk_intervals']:
                avg_interval = sum(result['chunk_intervals']) / len(result['chunk_intervals'])
                max_interval = max(result['chunk_intervals'])
                min_interval = min(result['chunk_intervals'])
                
                quality_color = GREEN if result['streaming_quality'] == 'smooth' else YELLOW if result['streaming_quality'] == 'choppy' else RED
                
                self.print_color(GREY, f"\n  {result['question']}:")
                self.print_color(GREY, f"    Chunks: {result['chunks_received']}")
                self.print_color(GREY, f"    Avg interval: {avg_interval:.3f}s")
                self.print_color(GREY, f"    Range: {min_interval:.3f}s - {max_interval:.3f}s")
                self.print_color(quality_color, f"    Quality: {result['streaming_quality']}")
                
                # Highlight problematic intervals
                if max_interval > 0.5:
                    self.print_color(YELLOW, f"    ⚠ Long pause detected: {max_interval:.3f}s")
                    
        print()

def main():
    parser = argparse.ArgumentParser(description='Test Olla streaming latency and choppiness')
    parser.add_argument('--url', default=TARGET_URL, 
                       help=f'Olla base URL (default: {TARGET_URL})')
    parser.add_argument('--model', 
                       help='Specific model to test (if not provided, will show selection menu)')
    parser.add_argument('--questions', default=DEFAULT_QUESTION_FILE,
                       help=f'Questions file (default: {DEFAULT_QUESTION_FILE})')
    parser.add_argument('--count', type=int, default=5,
                       help='Number of questions to test (default: 5)')
    parser.add_argument('--timeout', type=int, default=DEFAULT_TIMEOUT,
                       help=f'Request timeout in seconds (default: {DEFAULT_TIMEOUT})')
    
    args = parser.parse_args()
    
    tester = StreamingLatencyTester(args.url, args.timeout)
    tester.print_header()
    
    # Check health
    if not tester.check_health():
        sys.exit(1)
        
    # Load questions
    if not tester.load_questions(args.questions):
        sys.exit(1)
        
    # Fetch and select model
    if not tester.fetch_models():
        sys.exit(1)
        
    selected_model = tester.select_model(args.model)
    if not selected_model:
        sys.exit(1)
        
    # Run tests
    print()
    tester.print_color(WHITE, f"Running {args.count} streaming latency tests with model: {CYAN}{selected_model}{RESET}")
    tester.print_color(GREY, f"Each ● represents a received chunk (green=first, cyan=subsequent)")
    print()
    
    results = []
    
    for i in range(args.count):
        question = tester.get_random_question()
        tester.print_color(WHITE, f"Test {i+1}/{args.count}:")
        
        result = tester.test_streaming_latency(selected_model, question)
        results.append(result)
        
        # Show immediate result
        if result['success']:
            tester.print_color(GREEN, f" [OK]")
            tester.print_color(GREY, f"    Endpoint: {result['endpoint_used']}")
            tester.print_color(GREY, f"    Chunks: {result['chunks_received']}, TTFT: {result['time_to_first_token']:.3f}s, Quality: {result['streaming_quality']}")
        else:
            tester.print_color(RED, f" [FAIL]")
            if result['error']:
                tester.print_color(RED, f"    Error: {result['error']}")
        print()
        
    # Analyze results
    tester.analyze_results(results)
    
    # Exit with appropriate code
    successful_count = sum(1 for r in results if r['success'])
    if successful_count == len(results):
        tester.print_color(GREEN, "All tests completed successfully!")
        sys.exit(0)
    elif successful_count > 0:
        tester.print_color(YELLOW, f"Some tests failed ({successful_count}/{len(results)} successful)")
        sys.exit(1)
    else:
        tester.print_color(RED, "All tests failed!")
        sys.exit(1)

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Test interrupted by user (Ctrl+C){RESET}")
        sys.exit(130)  # Standard exit code for SIGINT