#!/usr/bin/env bash
# Test script for Crush CLI + vLLM integration via Olla
# This script verifies that both OpenAI and Anthropic API endpoints work correctly

set -e  # Exit on error

# Colours for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Colour

# Configuration
OLLA_BASE_URL="http://localhost:40114"
VLLM_BASE_URL="http://localhost:8000"
MODEL_NAME="meta-llama/Meta-Llama-3.1-8B-Instruct"  # Update if you change the model

echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Crush CLI + vLLM Integration Test Suite     ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""

# Helper function for test status
test_passed() {
    echo -e "${GREEN}✓ PASSED${NC}: $1"
}

test_failed() {
    echo -e "${RED}✗ FAILED${NC}: $1"
    echo -e "${RED}  Error: $2${NC}"
}

test_info() {
    echo -e "${YELLOW}→${NC} $1"
}

# Test 1: vLLM Health Check
echo -e "${BLUE}[Test 1]${NC} Checking vLLM health..."
if curl -sf "${VLLM_BASE_URL}/health" > /dev/null; then
    test_passed "vLLM is healthy"
else
    test_failed "vLLM health check" "vLLM is not responding at ${VLLM_BASE_URL}"
    exit 1
fi
echo ""

# Test 2: Olla Health Check
echo -e "${BLUE}[Test 2]${NC} Checking Olla health..."
if curl -sf "${OLLA_BASE_URL}/internal/health" > /dev/null; then
    test_passed "Olla is healthy"
else
    test_failed "Olla health check" "Olla is not responding at ${OLLA_BASE_URL}"
    exit 1
fi
echo ""

# Test 3: Endpoint Status
echo -e "${BLUE}[Test 3]${NC} Checking endpoint status..."
ENDPOINT_STATUS=$(curl -s "${OLLA_BASE_URL}/internal/status/endpoints")
if echo "$ENDPOINT_STATUS" | grep -q "vllm-primary"; then
    test_passed "vLLM endpoint discovered"

    # Check if healthy
    if echo "$ENDPOINT_STATUS" | grep -q '"healthy":true'; then
        test_passed "vLLM endpoint is healthy"
    else
        test_failed "vLLM endpoint health" "Endpoint discovered but not healthy"
        test_info "Endpoint status: $ENDPOINT_STATUS"
    fi
else
    test_failed "vLLM endpoint discovery" "vLLM endpoint not found in status"
    test_info "Status response: $ENDPOINT_STATUS"
    exit 1
fi
echo ""

# Test 4: OpenAI Models Endpoint
echo -e "${BLUE}[Test 4]${NC} Testing OpenAI models endpoint..."
OPENAI_MODELS=$(curl -s "${OLLA_BASE_URL}/olla/openai/v1/models")
if echo "$OPENAI_MODELS" | grep -q '"object":"list"'; then
    test_passed "OpenAI models endpoint responding"

    # Check if our model is listed
    if echo "$OPENAI_MODELS" | grep -q "$MODEL_NAME"; then
        test_passed "Model '${MODEL_NAME}' discovered via OpenAI API"
    else
        test_failed "Model discovery" "Model '${MODEL_NAME}' not found"
        test_info "Available models: $OPENAI_MODELS"
    fi
else
    test_failed "OpenAI models endpoint" "Invalid response format"
    test_info "Response: $OPENAI_MODELS"
fi
echo ""

# Test 5: Anthropic Models Endpoint
echo -e "${BLUE}[Test 5]${NC} Testing Anthropic models endpoint..."
ANTHROPIC_MODELS=$(curl -s "${OLLA_BASE_URL}/olla/anthropic/v1/models")
if echo "$ANTHROPIC_MODELS" | grep -q '"object":"list"'; then
    test_passed "Anthropic models endpoint responding"

    # Check if our model is listed
    if echo "$ANTHROPIC_MODELS" | grep -q "$MODEL_NAME"; then
        test_passed "Model '${MODEL_NAME}' discovered via Anthropic API"
    else
        test_failed "Model discovery" "Model '${MODEL_NAME}' not found"
        test_info "Available models: $ANTHROPIC_MODELS"
    fi
else
    test_failed "Anthropic models endpoint" "Invalid response format"
    test_info "Response: $ANTHROPIC_MODELS"
fi
echo ""

# Test 6: OpenAI Chat Completion (Non-Streaming)
echo -e "${BLUE}[Test 6]${NC} Testing OpenAI chat completion (non-streaming)..."
OPENAI_RESPONSE=$(curl -s -X POST "${OLLA_BASE_URL}/olla/openai/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"${MODEL_NAME}\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Say 'test successful' and nothing else.\"}],
        \"max_tokens\": 20,
        \"temperature\": 0.1
    }")

if echo "$OPENAI_RESPONSE" | grep -q '"choices"'; then
    test_passed "OpenAI chat completion succeeded"

    # Extract response content
    CONTENT=$(echo "$OPENAI_RESPONSE" | grep -o '"content":"[^"]*"' | head -1 | cut -d'"' -f4)
    test_info "Response: ${CONTENT}"

    # Check for usage stats
    if echo "$OPENAI_RESPONSE" | grep -q '"usage"'; then
        test_passed "Usage statistics included"
    fi
else
    test_failed "OpenAI chat completion" "Invalid response format"
    test_info "Response: $OPENAI_RESPONSE"
fi
echo ""

# Test 7: Anthropic Messages (Non-Streaming)
echo -e "${BLUE}[Test 7]${NC} Testing Anthropic messages endpoint (non-streaming)..."
ANTHROPIC_RESPONSE=$(curl -s -X POST "${OLLA_BASE_URL}/olla/anthropic/v1/messages" \
    -H "Content-Type: application/json" \
    -H "anthropic-version: 2023-06-01" \
    -d "{
        \"model\": \"${MODEL_NAME}\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Say 'anthropic test successful' and nothing else.\"}],
        \"max_tokens\": 20
    }")

if echo "$ANTHROPIC_RESPONSE" | grep -q '"content"'; then
    test_passed "Anthropic messages endpoint succeeded"

    # Extract response content
    CONTENT=$(echo "$ANTHROPIC_RESPONSE" | grep -o '"text":"[^"]*"' | head -1 | cut -d'"' -f4)
    test_info "Response: ${CONTENT}"

    # Check for usage stats
    if echo "$ANTHROPIC_RESPONSE" | grep -q '"usage"'; then
        test_passed "Usage statistics included"
    fi
else
    test_failed "Anthropic messages endpoint" "Invalid response format"
    test_info "Response: $ANTHROPIC_RESPONSE"
fi
echo ""

# Test 8: OpenAI Streaming
echo -e "${BLUE}[Test 8]${NC} Testing OpenAI streaming..."
STREAM_OUTPUT=$(curl -s -X POST "${OLLA_BASE_URL}/olla/openai/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"${MODEL_NAME}\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3.\"}],
        \"max_tokens\": 50,
        \"stream\": true
    }" | head -20)

if echo "$STREAM_OUTPUT" | grep -q 'data:'; then
    test_passed "OpenAI streaming working"

    # Check for SSE format
    if echo "$STREAM_OUTPUT" | grep -q '"delta"'; then
        test_passed "SSE delta format correct"
    fi
else
    test_failed "OpenAI streaming" "No streaming data received"
    test_info "Output: $STREAM_OUTPUT"
fi
echo ""

# Test 9: Anthropic Streaming
echo -e "${BLUE}[Test 9]${NC} Testing Anthropic streaming..."
STREAM_OUTPUT=$(curl -s -X POST "${OLLA_BASE_URL}/olla/anthropic/v1/messages" \
    -H "Content-Type: application/json" \
    -H "anthropic-version: 2023-06-01" \
    -d "{
        \"model\": \"${MODEL_NAME}\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3.\"}],
        \"max_tokens\": 50,
        \"stream\": true
    }" | head -20)

if echo "$STREAM_OUTPUT" | grep -q 'event:'; then
    test_passed "Anthropic streaming working"

    # Check for Anthropic SSE format
    if echo "$STREAM_OUTPUT" | grep -q 'content_block_delta'; then
        test_passed "Anthropic SSE format correct"
    fi
else
    test_failed "Anthropic streaming" "No streaming data received"
    test_info "Output: $STREAM_OUTPUT"
fi
echo ""

# Test 10: Response Headers
echo -e "${BLUE}[Test 10]${NC} Testing Olla response headers..."
HEADERS=$(curl -s -i -X POST "${OLLA_BASE_URL}/olla/openai/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"${MODEL_NAME}\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
        \"max_tokens\": 5
    }" | head -20)

if echo "$HEADERS" | grep -q "X-Olla-Endpoint"; then
    test_passed "X-Olla-Endpoint header present"
fi

if echo "$HEADERS" | grep -q "X-Olla-Model"; then
    test_passed "X-Olla-Model header present"
fi

if echo "$HEADERS" | grep -q "X-Olla-Backend-Type"; then
    test_passed "X-Olla-Backend-Type header present"
fi
echo ""

# Summary
echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║              Test Suite Complete              ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}All core tests passed!${NC}"
echo ""
echo "Next steps:"
echo "  1. Configure Crush CLI with crush-config.example.json"
echo "  2. Test with: crush chat --provider olla-openai"
echo "  3. Switch providers: crush model switch olla-anthropic"
echo "  4. Monitor performance: curl ${OLLA_BASE_URL}/internal/status/stats"
echo ""
