#!/bin/bash
# Run all streaming tests

set -e

echo "===================================="
echo "    Running All Streaming Tests     "
echo "===================================="
echo

# Activate virtual environment if it exists
if [ -d "../.venv" ]; then
    if [ -f "../.venv/Scripts/activate" ]; then
        # Windows
        source ../.venv/Scripts/activate
    elif [ -f "../.venv/bin/activate" ]; then
        # Unix/macOS
        source ../.venv/bin/activate
    fi
fi

# Install dependencies if needed
if ! python -c "import requests" 2>/dev/null; then
    echo "Installing dependencies..."
    pip install -r ../requirements.txt
    echo
fi

# Default URL
TARGET_URL="${TARGET_URL:-http://localhost:40114}"

echo "Testing against: $TARGET_URL"
echo

# Test 1: Streaming Detection (quick mode)
echo "1. Running streaming detection test (quick mode)..."
python test-streaming-detection.py --url "$TARGET_URL" --quick || {
    echo "❌ Streaming detection test failed"
    exit 1
}
echo

# Test 2: Streaming Latency (3 questions)
echo "2. Running streaming latency test..."
python test-streaming-latency.py --url "$TARGET_URL" --count 3 || {
    echo "❌ Streaming latency test failed"
    exit 1
}
echo

# Test 3: Streaming Responses (sample mode)
echo "3. Running streaming responses test (sample mode)..."
python test-streaming-responses.py --url "$TARGET_URL" --sample || {
    echo "❌ Streaming responses test failed"
    exit 1
}
echo

echo "===================================="
echo "  ✅ All streaming tests passed!    "
echo "===================================="