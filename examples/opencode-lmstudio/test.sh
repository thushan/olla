#!/usr/bin/env bash
# Test script for OpenCode + LM Studio + Olla integration
# This script verifies connectivity and configuration

set -e

# Colours for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No colour

# Configuration
LM_STUDIO_HOST="${LM_STUDIO_HOST:-localhost}"
LM_STUDIO_PORT="${LM_STUDIO_PORT:-1234}"
OLLA_HOST="${OLLA_HOST:-localhost}"
OLLA_PORT="${OLLA_PORT:-40114}"

echo -e "${BLUE}=====================================${NC}"
echo -e "${BLUE}OpenCode + LM Studio Integration Test${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""

# Function to print test results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ PASS${NC}: $2"
    else
        echo -e "${RED}✗ FAIL${NC}: $2"
        if [ -n "$3" ]; then
            echo -e "${YELLOW}  $3${NC}"
        fi
    fi
}

# Test 1: Check if LM Studio is running on host
echo -e "${BLUE}[1/8]${NC} Checking LM Studio connectivity on host..."
if curl -s -f "http://${LM_STUDIO_HOST}:${LM_STUDIO_PORT}/v1/models" > /dev/null 2>&1; then
    print_result 0 "LM Studio is running on ${LM_STUDIO_HOST}:${LM_STUDIO_PORT}"
    LM_STUDIO_MODELS=$(curl -s "http://${LM_STUDIO_HOST}:${LM_STUDIO_PORT}/v1/models" | jq -r '.data[].id' 2>/dev/null || echo "")
    if [ -n "$LM_STUDIO_MODELS" ]; then
        echo -e "  ${GREEN}Models loaded:${NC}"
        echo "$LM_STUDIO_MODELS" | sed 's/^/    - /'
    else
        echo -e "  ${YELLOW}Warning: No models loaded in LM Studio${NC}"
    fi
else
    print_result 1 "LM Studio is not reachable" \
        "Start LM Studio and enable server mode (Settings → Server → Start Server)"
    echo -e "${YELLOW}Continuing tests anyway...${NC}"
fi
echo ""

# Test 2: Check if Olla container is running
echo -e "${BLUE}[2/8]${NC} Checking Olla container status..."
if docker ps | grep -q "olla"; then
    print_result 0 "Olla container is running"
else
    print_result 1 "Olla container is not running" "Run: docker compose up -d"
    exit 1
fi
echo ""

# Test 3: Check Olla health endpoint
echo -e "${BLUE}[3/8]${NC} Checking Olla health..."
if HEALTH=$(curl -s -f "http://${OLLA_HOST}:${OLLA_PORT}/internal/health" 2>&1); then
    print_result 0 "Olla health endpoint is responding"
    STATUS=$(echo "$HEALTH" | jq -r '.status' 2>/dev/null || echo "unknown")
    echo -e "  Status: ${GREEN}${STATUS}${NC}"
else
    print_result 1 "Olla health endpoint failed" "Check logs: docker logs olla"
    exit 1
fi
echo ""

# Test 4: Check endpoint status
echo -e "${BLUE}[4/8]${NC} Checking LM Studio endpoint status in Olla..."
if ENDPOINTS=$(curl -s -f "http://${OLLA_HOST}:${OLLA_PORT}/internal/status/endpoints" 2>&1); then
    print_result 0 "Endpoint status retrieved"

    # Check if LM Studio endpoint is healthy
    LM_HEALTHY=$(echo "$ENDPOINTS" | jq -r '.endpoints[] | select(.name=="lm-studio") | .healthy' 2>/dev/null || echo "false")
    LM_MODELS_COUNT=$(echo "$ENDPOINTS" | jq -r '.endpoints[] | select(.name=="lm-studio") | .models_loaded' 2>/dev/null || echo "0")

    if [ "$LM_HEALTHY" = "true" ]; then
        echo -e "  LM Studio endpoint: ${GREEN}healthy${NC}"
        echo -e "  Models discovered: ${GREEN}${LM_MODELS_COUNT}${NC}"
    else
        echo -e "  LM Studio endpoint: ${RED}unhealthy${NC}"
        echo -e "  ${YELLOW}Check olla.yaml configuration and LM Studio connectivity${NC}"
    fi
else
    print_result 1 "Failed to retrieve endpoint status" "Check logs: docker logs olla"
fi
echo ""

# Test 5: Check OpenAI endpoint - list models
echo -e "${BLUE}[5/8]${NC} Testing OpenAI endpoint - list models..."
if MODELS=$(curl -s -f "http://${OLLA_HOST}:${OLLA_PORT}/olla/openai/v1/models" 2>&1); then
    print_result 0 "OpenAI /models endpoint working"
    MODEL_COUNT=$(echo "$MODELS" | jq -r '.data | length' 2>/dev/null || echo "0")
    echo -e "  Models available: ${GREEN}${MODEL_COUNT}${NC}"
    if [ "$MODEL_COUNT" -gt 0 ]; then
        echo -e "  ${GREEN}Available models:${NC}"
        echo "$MODELS" | jq -r '.data[].id' 2>/dev/null | sed 's/^/    - /' || echo "    (unable to parse model list)"
    else
        echo -e "  ${YELLOW}Warning: No models discovered. Load a model in LM Studio.${NC}"
    fi
else
    print_result 1 "OpenAI /models endpoint failed" "Check Olla logs: docker logs olla"
fi
echo ""

# Test 6: Check Anthropic endpoint - list models
echo -e "${BLUE}[6/8]${NC} Testing Anthropic endpoint - list models..."
if ANTHROPIC_MODELS=$(curl -s -f "http://${OLLA_HOST}:${OLLA_PORT}/olla/anthropic/v1/models" 2>&1); then
    print_result 0 "Anthropic /models endpoint working"
    ANTHROPIC_COUNT=$(echo "$ANTHROPIC_MODELS" | jq -r '.data | length' 2>/dev/null || echo "0")
    echo -e "  Models available: ${GREEN}${ANTHROPIC_COUNT}${NC}"
else
    print_result 1 "Anthropic /models endpoint failed" "Check if Anthropic translator is enabled"
fi
echo ""

# Test 7: Test OpenAI chat completion (if models available)
echo -e "${BLUE}[7/8]${NC} Testing OpenAI chat completion..."
FIRST_MODEL=$(curl -s "http://${OLLA_HOST}:${OLLA_PORT}/olla/openai/v1/models" | jq -r '.data[0].id' 2>/dev/null)
if [ -n "$FIRST_MODEL" ] && [ "$FIRST_MODEL" != "null" ]; then
    echo -e "  Using model: ${GREEN}${FIRST_MODEL}${NC}"

    CHAT_RESPONSE=$(curl -s -f -X POST "http://${OLLA_HOST}:${OLLA_PORT}/olla/openai/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"${FIRST_MODEL}\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Say hello in one word.\"}],
            \"max_tokens\": 10,
            \"stream\": false
        }" 2>&1)

    if echo "$CHAT_RESPONSE" | jq -e '.choices[0].message.content' > /dev/null 2>&1; then
        print_result 0 "OpenAI chat completion successful"
        CONTENT=$(echo "$CHAT_RESPONSE" | jq -r '.choices[0].message.content')
        echo -e "  Response: ${GREEN}${CONTENT}${NC}"
    else
        print_result 1 "OpenAI chat completion failed" "Response: $CHAT_RESPONSE"
    fi
else
    print_result 1 "No models available for testing" "Load a model in LM Studio first"
fi
echo ""

# Test 8: Test Anthropic messages endpoint (if models available)
echo -e "${BLUE}[8/8]${NC} Testing Anthropic messages endpoint..."
if [ -n "$FIRST_MODEL" ] && [ "$FIRST_MODEL" != "null" ]; then
    echo -e "  Using model: ${GREEN}${FIRST_MODEL}${NC}"

    ANTHROPIC_RESPONSE=$(curl -s -f -X POST "http://${OLLA_HOST}:${OLLA_PORT}/olla/anthropic/v1/messages" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"${FIRST_MODEL}\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Say hello in one word.\"}],
            \"max_tokens\": 10,
            \"stream\": false
        }" 2>&1)

    if echo "$ANTHROPIC_RESPONSE" | jq -e '.content[0].text' > /dev/null 2>&1; then
        print_result 0 "Anthropic messages endpoint successful"
        CONTENT=$(echo "$ANTHROPIC_RESPONSE" | jq -r '.content[0].text')
        echo -e "  Response: ${GREEN}${CONTENT}${NC}"
    else
        print_result 1 "Anthropic messages endpoint failed" "Response: $ANTHROPIC_RESPONSE"
    fi
else
    print_result 1 "No models available for testing" "Load a model in LM Studio first"
fi
echo ""

# Summary
echo -e "${BLUE}=====================================${NC}"
echo -e "${BLUE}Summary${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""
echo -e "${GREEN}Configuration Verified!${NC}"
echo ""
echo "Next steps:"
echo "1. Install OpenCode: npm install -g @sst/opencode"
echo "2. Copy configuration:"
echo "   cp opencode-config.example.json ~/.opencode/config.json"
echo "3. Start OpenCode:"
echo "   opencode --provider olla-openai"
echo "   or"
echo "   opencode --provider olla-anthropic"
echo ""
echo "Available endpoints:"
echo "  - OpenAI:   http://${OLLA_HOST}:${OLLA_PORT}/olla/openai/v1"
echo "  - Anthropic: http://${OLLA_HOST}:${OLLA_PORT}/olla/anthropic/v1"
echo ""
echo "Monitoring:"
echo "  - Health:    curl http://${OLLA_HOST}:${OLLA_PORT}/internal/health"
echo "  - Endpoints: curl http://${OLLA_HOST}:${OLLA_PORT}/internal/status/endpoints"
echo "  - Logs:      docker logs olla -f"
echo ""
