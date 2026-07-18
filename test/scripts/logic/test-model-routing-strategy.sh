#!/bin/bash
# test model routing strategies

set -e

OLLA_URL=${OLLA_URL:-"http://localhost:8080"}
MODEL=${MODEL:-"phi3.5:latest"}
# ALIAS_MODEL must be a `model_aliases` key in the running Olla's config whose
# listed real model names exist on none of the configured endpoints - see #191.
ALIAS_MODEL=${ALIAS_MODEL:-"nonexistent-alias-target"}

# FAILED_TESTS follows the same pass/fail convention as the other scripts in
# test/scripts/logic (test-model-routing.sh, test-provider-models.sh,
# test-provider-routing.sh): increment on a real assertion failure, exit 1 at
# the end if non-zero. Most tests in this script are informational only
# (echo ✓/✗ without affecting the exit code), but Test 4 asserts a concrete
# regression (#191) and must fail the script when it doesn't hold.
FAILED_TESTS=0

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

# test 4: alias pointing at a model that exists on no endpoint (#191)
# requires the running Olla's config to have routing_strategy.type: strict and a
# model_aliases entry:
#   model_aliases:
#     nonexistent-alias-target:
#       - some-model-that-is-not-served-anywhere
# under strict routing this must reject fast (404, model_not_found) rather than
# silently proxying to a compatible-but-wrong backend and returning 200.
echo "=== Test 4: Alias With No Routable Target (strict routing must reject, not proxy) ==="
response=$(curl -s -i -X POST "$OLLA_URL/olla/proxy/api/generate" \
    -H "Content-Type: application/json" \
    -d "{\"model\": \"$ALIAS_MODEL\", \"prompt\": \"test\", \"stream\": false}" \
    2>&1 || true)

status_code=$(echo "$response" | head -n 1 | awk '{print $2}')
decision_header=$(echo "$response" | grep -i "X-Olla-Routing-Decision:" | head -n 1 || echo "")

echo "Response Status: $status_code"
echo "Routing Decision: $decision_header"

if [[ ( "$status_code" == "404" || "$status_code" == "503" ) && "$decision_header" == *"rejected"* ]]; then
    echo "✓ Alias with no routable target was rejected (status $status_code), not proxied"
else
    echo "✗ Expected a rejection status (404/503) with X-Olla-Routing-Decision: rejected but got status $status_code, decision '$decision_header' - request may have been proxied to the wrong backend"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

echo "---"
echo ""

echo "================================"
echo "Model Routing Strategy Tests Complete"

if [[ $FAILED_TESTS -gt 0 ]]; then
    echo "$FAILED_TESTS test(s) failed"
    exit 1
fi