#!/bin/bash
# test model routing strategies

set -e

OLLA_URL=${OLLA_URL:-"http://localhost:8080"}
MODEL=${MODEL:-"phi3.5:latest"}

echo "Testing Model Routing Strategies"
echo "================================"
echo ""

# function to make a request and check headers
test_request() {
    local description="$1"
    local model="$2"
    local expected_strategy="$3"
    
    echo "Test: $description"
    echo "Model: $model"
    echo "Expected Strategy: $expected_strategy"
    
    response=$(curl -s -i -X POST "$OLLA_URL/olla/api/generate" \
        -H "Content-Type: application/json" \
        -d "{\"model\": \"$model\", \"prompt\": \"test\", \"stream\": false}" \
        2>&1 || true)
    
    # extract headers
    strategy_header=$(echo "$response" | grep -i "X-Olla-Routing-Strategy:" | head -n 1 || echo "")
    decision_header=$(echo "$response" | grep -i "X-Olla-Routing-Decision:" | head -n 1 || echo "")
    reason_header=$(echo "$response" | grep -i "X-Olla-Routing-Reason:" | head -n 1 || echo "")
    status_code=$(echo "$response" | head -n 1 | awk '{print $2}')
    
    echo "Response Status: $status_code"
    echo "Routing Strategy: $strategy_header"
    echo "Routing Decision: $decision_header"
    echo "Routing Reason: $reason_header"
    
    # check if strategy matches expected
    if [[ "$strategy_header" == *"$expected_strategy"* ]]; then
        echo "✓ Strategy matches expected: $expected_strategy"
    else
        echo "✗ Strategy mismatch. Expected: $expected_strategy"
    fi
    
    echo "---"
    echo ""
}

# test 1: request with a known model
echo "=== Test 1: Known Model (should use configured strategy) ==="
test_request "Request with known model" "$MODEL" "strict"

# test 2: request with unknown model
echo "=== Test 2: Unknown Model (should reject with strict strategy) ==="
test_request "Request with unknown model" "unknown-model:latest" "strict"

# test 3: request without model (should not trigger routing strategy)
echo "=== Test 3: No Model Specified ==="
response=$(curl -s -i -X POST "$OLLA_URL/olla/api/chat" \
    -H "Content-Type: application/json" \
    -d "{\"messages\": [{\"role\": \"user\", \"content\": \"test\"}]}" \
    2>&1 || true)

strategy_header=$(echo "$response" | grep -i "X-Olla-Routing-Strategy:" | head -n 1 || echo "")
if [[ -z "$strategy_header" ]]; then
    echo "✓ No routing strategy header (as expected for no model)"
else
    echo "✗ Unexpected routing strategy header: $strategy_header"
fi

echo ""
echo "================================"
echo "Model Routing Strategy Tests Complete"