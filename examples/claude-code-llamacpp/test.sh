#!/bin/bash
# Test script for Claude Code + llama.cpp + Olla setup

set -e

OLLA_URL="http://localhost:40114"
ANTHROPIC_URL="${OLLA_URL}/olla/anthropic/v1"
LLAMACPP_URL="http://localhost:8080"

echo "🧪 Testing Olla + llama.cpp + Anthropic Translation..."
echo ""

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test 1: llama.cpp health
echo -e "${YELLOW}Test 1: Checking llama.cpp health...${NC}"
if curl -sf "${LLAMACPP_URL}/health" > /dev/null; then
    echo -e "${GREEN}✓ llama.cpp is healthy${NC}"
else
    echo -e "${RED}✗ llama.cpp health check failed${NC}"
    echo "Possible issues:"
    echo "  - llama.cpp still loading model (wait 30-60 seconds)"
    echo "  - Model file path incorrect in compose.yaml"
    echo "  - Insufficient memory for model"
    exit 1
fi
echo ""

# Test 2: Olla health
echo -e "${YELLOW}Test 2: Checking Olla health...${NC}"
if curl -sf "${OLLA_URL}/internal/health" > /dev/null; then
    echo -e "${GREEN}✓ Olla is healthy${NC}"
else
    echo -e "${RED}✗ Olla health check failed${NC}"
    exit 1
fi
echo ""

# Test 3: List models
echo -e "${YELLOW}Test 3: Listing available models...${NC}"
MODELS=$(curl -s "${ANTHROPIC_URL}/models")
if echo "$MODELS" | jq -e '.data | length > 0' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Models available:${NC}"
    echo "$MODELS" | jq -r '.data[].id' | sed 's/^/  - /'
else
    echo -e "${RED}✗ No models available${NC}"
    echo "Check llama.cpp logs:"
    echo "  docker compose logs llama-cpp"
    exit 1
fi
echo ""

MODEL=$(echo "$MODELS" | jq -r '.data[0].id')
echo -e "${YELLOW}Using model: ${MODEL}${NC}"
echo ""

# Test 4: Simple message
echo -e "${YELLOW}Test 4: Testing simple message (non-streaming)...${NC}"
RESPONSE=$(curl -s -X POST "${ANTHROPIC_URL}/messages" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${MODEL}\",
    \"max_tokens\": 50,
    \"messages\": [
      {\"role\": \"user\", \"content\": \"Say 'Hello from llama.cpp!'\"}
    ]
  }")

if echo "$RESPONSE" | jq -e '.content[0].text' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Non-streaming message successful${NC}"
    echo "Response:"
    echo "$RESPONSE" | jq -r '.content[0].text' | sed 's/^/  /'
else
    echo -e "${RED}✗ Non-streaming message failed${NC}"
    echo "$RESPONSE" | jq .
    exit 1
fi
echo ""

# Test 5: Streaming message
echo -e "${YELLOW}Test 5: Testing streaming message...${NC}"
STREAM_OUTPUT=$(curl -sN -X POST "${ANTHROPIC_URL}/messages" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${MODEL}\",
    \"max_tokens\": 30,
    \"messages\": [
      {\"role\": \"user\", \"content\": \"Count: 1, 2, 3\"}
    ],
    \"stream\": true
  }")

if echo "$STREAM_OUTPUT" | grep -q "content_block_delta"; then
    echo -e "${GREEN}✓ Streaming message successful${NC}"
else
    echo -e "${RED}✗ Streaming message failed${NC}"
    exit 1
fi
echo ""

# Test 6: llama.cpp native endpoint
echo -e "${YELLOW}Test 6: Testing llama.cpp native endpoint...${NC}"
NATIVE_RESPONSE=$(curl -s "${LLAMACPP_URL}/v1/models")
if echo "$NATIVE_RESPONSE" | jq -e '.data | length > 0' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ llama.cpp native endpoint working${NC}"
else
    echo -e "${YELLOW}⚠ llama.cpp native endpoint returned unexpected format${NC}"
fi
echo ""

# Summary
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✓ All tests passed!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "llama.cpp backend ready for Claude Code!"
echo ""
echo "Configure Claude Code:"
echo "  export ANTHROPIC_API_BASE_URL=\"${ANTHROPIC_URL}\""
echo ""
echo "Useful commands:"
echo "  - llama.cpp logs: docker compose logs -f llama-cpp"
echo "  - Olla logs: docker compose logs -f olla"
echo "  - llama.cpp metrics: curl ${LLAMACPP_URL}/metrics"
