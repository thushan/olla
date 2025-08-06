# Test Cases

This directory contains automated test scripts for comprehensive testing of Olla's functionality.

## test-proxy-engine-profiles.sh

Automated test suite that validates all combinations of proxy engines and profiles.

### Features
- Tests both proxy engines: `sherpa` and `olla`
- Tests all profiles: `auto`, `standard`, and `streaming`
- Automatically builds Olla from source
- Runs streaming behavior tests for each configuration
- Generates comprehensive test reports

### Prerequisites
1. Python virtual environment activated
2. Go installed for building Olla
3. At least one phi model available (phi4:latest, phi3.5:latest, or phi3:latest)

### Usage
```bash
# Run with your base configuration
./test-proxy-engine-profiles.sh -c /path/to/config.yaml

# Example with default config
./test-proxy-engine-profiles.sh -c config.yaml
```

### What It Does
1. Verifies Python virtual environment is active
2. Builds Olla from source
3. For each engine/profile combination:
   - Creates a test configuration
   - Starts Olla with that configuration
   - Verifies phi models are available
   - Runs streaming detection, latency, and response tests
   - Collects results and logs
4. Generates a summary report

### Test Matrix
The script tests these 6 combinations:
- sherpa/auto
- sherpa/standard  
- sherpa/streaming
- olla/auto
- olla/standard
- olla/streaming

### Output
- Test results are saved to `test-results-YYYYMMDD-HHMMSS/`
- Each configuration gets its own subdirectory with logs
- Summary report shows PASSED/FAILED/INCOMPLETE for each combination

### Configuration
The script modifies only the proxy section of your config:
```yaml
proxy:
  engine: sherpa  # or olla
  profile: auto   # or standard/streaming
```

All other settings from your base configuration are preserved.

### Troubleshooting
- **"Not in a Python virtual environment"**: Activate your venv first
- **"Go is not installed"**: Install Go from https://golang.org/dl/
- **"No phi models found"**: Run `ollama pull phi4:latest`
- **"Olla failed to start"**: Check the logs in the test-results directory

### Example Output
```
============================================================
  Olla Proxy Engine & Profile Test Suite
============================================================
✓ Virtual environment detected: /path/to/.venv
Building Olla from source...
✓ Olla built successfully

Testing: sherpa engine with auto profile
----------------------------------------
✓ Test configuration created
✓ Olla started successfully (PID: 12345)
✓ phi4:latest model found
  ✓ Streaming detection test passed
  ✓ Streaming latency test passed
  ✓ Streaming responses test passed
✓ Completed tests for sherpa/auto

[... more test results ...]

Test Summary
=================================
sherpa/auto: PASSED
sherpa/standard: PASSED
sherpa/streaming: PASSED
olla/auto: PASSED
olla/standard: PASSED
olla/streaming: PASSED

✅ All tests completed!
```

## Modular Architecture

The test suite uses a modular architecture with reusable components:

### Include Files
- **_common.sh**: Common functions (colors, printing, virtual environment checks)
- **_olla.sh**: Olla-specific functions (build, start, stop, health checks)
- **_streaming_tests.sh**: Streaming test runners and result analysis

These modules can be sourced in other test scripts to reuse functionality:
```bash
source "$SCRIPT_DIR/_common.sh"
source "$SCRIPT_DIR/_olla.sh"
source "$SCRIPT_DIR/_streaming_tests.sh"
```