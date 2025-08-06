# Olla Test Scripts

This directory contains various test scripts for validating Olla's functionality across different scenarios.

## Setup

### Prerequisites
- Python 3.8 or higher
- pip (Python package installer)

### Installation

1. Create a virtual environment (recommended):
```bash
python -m venv .venv

# On Windows
.venv\Scripts\activate

# On Unix/macOS
source .venv/bin/activate
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

## Script Categories

### `/streaming` - Streaming Behavior Tests
Tests for validating Olla's streaming behavior for LLM responses.
- Detects whether responses are streamed in real-time or use standard HTTP delivery
- Measures streaming latency and quality
- Validates streaming across different API formats

### `/logic` - Model Routing & Header Tests
Tests for Olla's request routing logic and header propagation.
- Model routing validation
- Header preservation and modification
- Endpoint selection behavior

### `/load` - Load Testing Scripts
Performance and stress testing scripts.
- Concurrent request handling
- Throughput measurement
- Resource usage monitoring

### `/security` - Security & Rate Limiting Tests
Security-focused test scripts.
- Rate limiting validation
- Request size limits
- Authentication and authorization

## Common Usage Patterns

Most scripts support these common arguments:
- `--url` - Olla base URL (default: http://localhost:40114)
- `--help` - Show script-specific help

Example:
```bash
python streaming/test-streaming-detection.py --url http://localhost:8080
```

## Script Conventions

1. **Exit Codes**:
   - 0: Success
   - 1: Test failure
   - 130: User interrupted (Ctrl+C)

2. **Output Format**:
   - Colored terminal output with ANSI codes
   - JSON output available with `--json` flag (where supported)
   - Progress indicators for long-running operations

3. **Windows Compatibility**:
   - All scripts handle Windows console encoding
   - Unicode support for visual indicators

## Troubleshooting

### Windows Unicode Issues
If you see encoding errors on Windows, ensure:
1. Your terminal supports UTF-8 (Windows Terminal recommended)
2. Python is using UTF-8 encoding (scripts auto-configure this)

### Connection Errors
If scripts can't connect to Olla:
1. Verify Olla is running: `curl http://localhost:40114/internal/health`
2. Check the URL matches your Olla instance
3. Ensure no firewall is blocking the connection