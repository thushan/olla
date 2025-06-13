#!/usr/bin/env bash

# Test script for Olla request size limits
# [o] AI Enhanced: Claude[AI] 3.5, 3.7, Github Copilot
#
# Run this after starting Olla with custom size limits configured

# 30-05-2025  [TF]  - Moved to test/scripts/security, added more tests, configurable model, ui tweaks
# 21-05-2025  [ML]  - Shows when tests fail at the end (test_tracking), added more context to output
# 20-05-2025  [GW]  - Added large headers tests, improved output formatting
# 15-05-2025  [GW]  - Fix for large payloads not being created correctly
# 12-05-2025  [GW]  - Initial version created with Claude[AI] 3.5

# Ensure required environment variables are set:
# export OLLA_SERVER_MAX_BODY_SIZE="1KB"
# export OLLA_SERVER_MAX_HEADER_SIZE="500B"
##

OLLA_URL="http://localhost:40114"
PROXY_ENDPOINT="/proxy/api/generate"
HEALTH_ENDPOINT="/internal/health"
MODEL_NAME="phi4:latest"
# NOTE about model choice, choose a lighter model for testing request limits so it returns quickly.
# Ideally we find Phi-3/Phi-4 or Llama-3 are light enough for this purpose.

echo "------- SECURITY TEST SUITE -------"
echo "🦙 Testing Olla Request Size Limits"
echo "                            v1.1.12"
echo "-----------------------------------"
echo "Olla URL:         $OLLA_URL"
echo "Proxy Endpoint:   $PROXY_ENDPOINT"
echo "Health Endpoint:  $HEALTH_ENDPOINT"
echo "Model:            $MODEL_NAME"
echo "-----------------------------------"

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=()

record_test_result() {
    local test_name="$1"
    local passed="$2"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if [[ "$passed" == "true" ]]; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS+=("$test_name")
    fi
}

test_request() {
    local description="$1"
    local endpoint="$2"
    local data="$3"
    local expected_statuses="$4"  # Can be multiple like "200|502|413"

    echo
    echo "Testing: $description"
    echo "Endpoint: $endpoint"
    echo "Data size: $(echo -n "$data" | wc -c) bytes"

    response=$(curl -s -w "\nHTTP_STATUS:%{http_code}\nRESPONSE_SIZE:%{size_download}" \
        -X POST \
        -H "Content-Type: application/json" \
        -d "$data" \
        "$OLLA_URL$endpoint" 2>/dev/null)

    http_status=$(echo "$response" | grep "HTTP_STATUS:" | cut -d: -f2)
    response_body=$(echo "$response" | sed '/HTTP_STATUS:/,$d')

    echo "HTTP Status: $http_status"

    # Check if status matches any of the expected statuses
    if [[ "|$expected_statuses|" == *"|$http_status|"* ]]; then
        echo "✅ Got expected status $http_status - PASS"
        record_test_result "$description" "true"

        # Provide context for what the status means
        case "$http_status" in
            "200") echo "   → Request passed, backend responded successfully" ;;
            "502") echo "   → Request passed size limits, backend unavailable" ;;
            "413") echo "   → Request body too large (security limit triggered)" ;;
            "431") echo "   → Request headers too large (security limit triggered)" ;;
            "") echo "   → Connection terminated immediately (very large request blocked)" ;;
        esac
    else
        echo "❌ Expected one of [$expected_statuses], got '$http_status' - FAIL"
        record_test_result "$description" "false"
    fi

    if [[ ${#response_body} -gt 0 ]]; then
        echo "Response: ${response_body:0:100}$([ ${#response_body} -gt 100 ] && echo '...')"
    fi
    echo "---"
}

create_payload() {
    local size_kb="$1"
    local size_bytes=$((size_kb * 1024))

    # Create JSON payload with large prompt
    local padding_size=$((size_bytes - 100)) # Account for JSON structure
    local padding=$(head -c $padding_size < /dev/zero | tr '\0' 'x')

    echo "{\"model\":\"$MODEL_NAME\",\"prompt\":\"$padding\",\"stream\":false}"
}

test_large_headers() {
    local description="$1"
    local header_size="$2"
    local expected_statuses="$3"  # Can be multiple like "431|"

    echo
    echo "Testing: $description"
    echo "Header size: ~$header_size bytes"

    # Create a large header value
    local large_value=$(head -c $header_size < /dev/zero | tr '\0' 'x')

    response=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "X-Large-Header: $large_value" \
        -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"test\"}" \
        "$OLLA_URL$PROXY_ENDPOINT" 2>/dev/null)

    http_status=$(echo "$response" | grep "HTTP_STATUS:" | cut -d: -f2)
    response_body=$(echo "$response" | sed '/HTTP_STATUS:/,$d')

    echo "HTTP Status: $http_status"

    # Check if status matches any of the expected statuses
    if [[ "|$expected_statuses|" == *"|$http_status|"* ]]; then
        echo "✅ Got expected status $http_status - PASS"
        record_test_result "$description" "true"

        case "$http_status" in
            "200") echo "   → Headers within limits, backend responded" ;;
            "502") echo "   → Headers within limits, backend unavailable" ;;
            "431") echo "   → Headers too large (security limit triggered)" ;;
            "") echo "   → Connection terminated immediately (very large headers blocked)" ;;
        esac
    else
        echo "❌ Expected one of [$expected_statuses], got '$http_status' - FAIL"
        record_test_result "$description" "false"
    fi
    echo "---"
}

echo "First, let's check if Olla is running..."
if ! curl -s "$OLLA_URL$HEALTH_ENDPOINT" > /dev/null; then
    echo "❌ Olla is not running or not accessible at $OLLA_URL"
    echo "Please start Olla first:"
    echo "  export OLLA_SERVER_MAX_BODY_SIZE=\"1MB\""
    echo "  export OLLA_SERVER_MAX_HEADER_SIZE=\"10KB\""
    echo "  make run"
    exit 1
fi

echo "✅ Olla is running!"

# Test 1: Small request (should pass to backend or get 502 if no backend)
small_payload="{\"model\":\"$MODEL_NAME\",\"prompt\":\"Hello, how are you?\",\"stream\":false}"
test_request "Small request (should pass or 502)" "$PROXY_ENDPOINT" "$small_payload" "200|502"

# Test 2: Health endpoint (should always pass, no size limits)
test_request "Health endpoint (should pass)" "$HEALTH_ENDPOINT" "$small_payload" "200"

# Test 3: Medium request (~10KB, may pass or fail depending on limits)
medium_payload=$(create_payload 10)
test_request "Medium request ~10KB (pass if limit >10KB, fail if <10KB)" "$PROXY_ENDPOINT" "$medium_payload" "200|502|413"

# Test 4: Large request (~2MB, should fail if limit is reasonable)
echo "Creating large payload (~2MB)..."
large_payload=$(create_payload 2048)
test_request "Large request ~2MB (should fail with 413 or connection close)" "$PROXY_ENDPOINT" "$large_payload" "413|"

# Test 5: Very large request (~10MB, should definitely fail)
echo "Creating very large payload (~10MB)..."
very_large_payload=$(create_payload 10240)
test_request "Very large request ~10MB (should fail with 413 or connection close)" "$PROXY_ENDPOINT" "$very_large_payload" "413|"

# Test 6: Large headers
test_large_headers "Small headers (may pass or fail depending on limits)" 1000 "200|502|431"
test_large_headers "Large headers ~50KB (should fail with 431 or connection close)" 51200 "431|"

echo
echo "🦙 Test Summary"
echo "==============="
echo "Tests completed: $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS"
echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
echo

if [[ ${#FAILED_TESTS[@]} -eq 0 ]]; then
    echo "🎉 ALL TESTS PASSED! Olla security features are working correctly!"
    echo
    echo "✅ Security Assessment: Request size limits are protecting your Olla instance!"
else
    echo "⚠️  SOME TESTS FAILED:"
    for failed_test in "${FAILED_TESTS[@]}"; do
        echo "   ❌ $failed_test"
    done
    echo
    echo "💡 This might indicate:"
    echo "   - Your limits are different than expected"
    echo "   - Configuration needs adjustment"
    echo "   - Unexpected behavior that needs investigation"
fi

echo
echo "Status Code Reference:"
echo "- HTTP 200: Request passed, Ollama backend responded"
echo "- HTTP 502: Request passed size limits, but no Ollama backend"
echo "- HTTP 413: Request body too large (SECURITY WORKING!)"
echo "- HTTP 431: Request headers too large (SECURITY WORKING!)"
echo "- Empty status: Connection terminated immediately (SECURITY WORKING!)"
echo
echo "To test with different limits, restart Olla with:"
echo "  export OLLA_SERVER_MAX_BODY_SIZE=\"50KB\""
echo "  export OLLA_SERVER_MAX_HEADER_SIZE=\"5KB\""
echo "  make run"

if [[ ${#FAILED_TESTS[@]} -eq 0 ]]; then
    exit 0
else
    exit 1
fi