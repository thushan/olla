#!/bin/bash

# Test script for Olla rate limiting
# Run this after starting Olla with rate limiting configured

# 30-05-2025  [TF]  - Fixed test logic, improved burst handling, better timing
# 21-05-2025  [ML]  - Added more context to CLI output, shows when tests fail at the end
# 15-05-2025  [GW]  - Burst capacity test added and fix sleep timing for other tests
# 14-05-2025  [GW]  - Initial version

# Ensure required environment variables are set:
# export OLLA_SERVER_GLOBAL_RATE_LIMIT="100"
# export OLLA_SERVER_PER_IP_RATE_LIMIT="5"
# export OLLA_SERVER_RATE_BURST_SIZE="3"
# export OLLA_SERVER_HEALTH_RATE_LIMIT="20"
# export OLLA_SERVER_TRUST_PROXY_HEADERS="true"
##
OLLA_URL="http://localhost:19841"
PROXY_ENDPOINT="/proxy/api/generate"
HEALTH_ENDPOINT="/internal/health"
MODEL_NAME="phi4:latest"

echo "------- SECURITY TEST SUITE -------"
echo "🦙 Testing Olla Request Rate Limit"
echo "-----------------------------------"
echo "Olla URL:         $OLLA_URL"
echo "Proxy Endpoint:   $PROXY_ENDPOINT"
echo "Health Endpoint:  $HEALTH_ENDPOINT"
echo "Model:            $MODEL_NAME"
echo "-----------------------------------"

# Test tracking variables
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=()

# Function to record test result
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

# Function to make request and extract rate limit info
make_request() {
    local endpoint="$1"
    local description="$2"
    local client_ip="$3"

    local response
    if [[ -n "$client_ip" ]]; then
        response=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
            -X POST \
            -H "Content-Type: application/json" \
            -H "X-Forwarded-For: $client_ip" \
            -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"test\"}" \
            "$OLLA_URL$endpoint" 2>/dev/null)
    else
        response=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
            -X POST \
            -H "Content-Type: application/json" \
            -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"test\"}" \
            "$OLLA_URL$endpoint" 2>/dev/null)
    fi

    local http_status=$(echo "$response" | grep "HTTP_STATUS:" | cut -d: -f2)

    echo "  Request: $description"
    echo "  Status: $http_status"

    # Extract rate limit headers
    local headers
    if [[ -n "$client_ip" ]]; then
        headers=$(curl -s -I -X POST \
            -H "Content-Type: application/json" \
            -H "X-Forwarded-For: $client_ip" \
            -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"test\"}" \
            "$OLLA_URL$endpoint" 2>/dev/null)
    else
        headers=$(curl -s -I -X POST \
            -H "Content-Type: application/json" \
            -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"test\"}" \
            "$OLLA_URL$endpoint" 2>/dev/null)
    fi

    local rate_limit=$(echo "$headers" | grep -i "x-ratelimit-limit:" | cut -d: -f2 | tr -d ' \r')
    local rate_remaining=$(echo "$headers" | grep -i "x-ratelimit-remaining:" | cut -d: -f2 | tr -d ' \r')
    local retry_after=$(echo "$headers" | grep -i "retry-after:" | cut -d: -f2 | tr -d ' \r')

    if [[ -n "$rate_limit" ]]; then
        echo "  Rate Limit: $rate_limit"
    fi
    if [[ -n "$rate_remaining" ]]; then
        echo "  Remaining: $rate_remaining"
    fi
    if [[ -n "$retry_after" ]]; then
        echo "  Retry After: ${retry_after}s"
    fi

    echo "  ---"
    echo "$http_status"
}

# Function to test rate limiting behavior
test_rate_limit() {
    local test_name="$1"
    local endpoint="$2"
    local requests="$3"
    local expected_pattern="$4"
    local client_ip="$5"

    echo
    echo "Testing: $test_name"
    echo "Endpoint: $endpoint"
    echo "Requests: $requests"
    if [[ -n "$client_ip" ]]; then
        echo "Client IP: $client_ip"
    fi
    echo

    local statuses=()

    for ((i=1; i<=requests; i++)); do
        echo "Request $i/$requests:"
        local status=$(make_request "$endpoint" "Request $i" "$client_ip")
        statuses+=("$status")

        # Small delay between requests to avoid overwhelming
        sleep 0.1
    done

    echo
    echo "Status sequence: ${statuses[*]}"

    # Analyze pattern
    local passed="false"
    case "$expected_pattern" in
        "allow_then_block")
            # Should start with 200s/502s then get 429s
            local found_success="false"
            local found_rate_limit="false"
            local success_count=0

            for status in "${statuses[@]}"; do
                if [[ "$status" == "200" || "$status" == "502" ]]; then
                    found_success="true"
                    success_count=$((success_count + 1))
                elif [[ "$status" == "429" ]]; then
                    found_rate_limit="true"
                fi
            done

            # Should have some success (burst) followed by rate limiting
            if [[ "$found_success" == "true" && "$found_rate_limit" == "true" && $success_count -le 3 ]]; then
                passed="true"
            fi
            ;;
        "all_success")
            # All should be 200 or 502 (no rate limiting)
            passed="true"
            for status in "${statuses[@]}"; do
                if [[ "$status" != "200" && "$status" != "502" ]]; then
                    passed="false"
                    break
                fi
            done
            ;;
        "some_rate_limited")
            # Should have at least one 429
            for status in "${statuses[@]}"; do
                if [[ "$status" == "429" ]]; then
                    passed="true"
                    break
                fi
            done
            ;;
    esac

    if [[ "$passed" == "true" ]]; then
        echo "✅ Test passed - Rate limiting behavior as expected"
        record_test_result "$test_name" "true"
    else
        echo "❌ Test failed - Unexpected rate limiting behavior"
        record_test_result "$test_name" "false"
    fi
}

# Function to test burst capacity
test_burst_capacity() {
    echo
    echo "Testing: Burst Capacity"
    echo "Endpoint: $PROXY_ENDPOINT"
    echo

    local burst_requests=6  # More than burst size (3) but less than total limit
    local rapid_statuses=()

    echo "Making $burst_requests rapid requests to test burst handling..."

    for ((i=1; i<=burst_requests; i++)); do
        local status=$(curl -s -o /dev/null -w "%{http_code}" \
            -X POST \
            -H "Content-Type: application/json" \
            -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"burst test $i\"}" \
            "$OLLA_URL$PROXY_ENDPOINT")
        rapid_statuses+=("$status")
        echo "  Rapid request $i: $status"

        # Very short delay for rapid requests
        sleep 0.05
    done

    echo
    echo "Rapid request statuses: ${rapid_statuses[*]}"

    # Check pattern: should allow burst (3) then start rate limiting
    local found_rate_limit="false"
    local success_count=0

    for status in "${rapid_statuses[@]}"; do
        if [[ "$status" == "429" ]]; then
            found_rate_limit="true"
        elif [[ "$status" == "200" || "$status" == "502" ]]; then
            success_count=$((success_count + 1))
        fi
    done

    echo "Successful requests before rate limiting: $success_count"

    # Should allow burst size (3) then rate limit
    if [[ "$found_rate_limit" == "true" && $success_count -le 4 ]]; then
        echo "✅ Burst capacity test passed - Rate limiting activated after burst"
        record_test_result "Burst Capacity" "true"
    else
        echo "❌ Burst capacity test failed - Expected rate limiting after ~3 requests"
        record_test_result "Burst Capacity" "false"
    fi
}

# Check if Olla is running
echo "Checking if Olla is running..."
if ! curl -s "$OLLA_URL$HEALTH_ENDPOINT" > /dev/null; then
    echo "❌ Olla is not running or not accessible at $OLLA_URL"
    echo "Please start Olla with rate limiting configured:"
    echo "  export OLLA_SERVER_PER_IP_RATE_LIMIT=\"5\""
    echo "  export OLLA_SERVER_RATE_BURST_SIZE=\"3\""
    echo "  export OLLA_SERVER_HEALTH_RATE_LIMIT=\"20\""
    echo "  make run"
    exit 1
fi

echo "✅ Olla is running!"
echo

# Test 1: Basic rate limiting (should allow burst then block)
test_rate_limit "Basic Rate Limiting" "$PROXY_ENDPOINT" 6 "allow_then_block"

# Test 2: Health endpoint (should have higher limits)
test_rate_limit "Health Endpoint Rate Limiting" "$HEALTH_ENDPOINT" 10 "all_success"

# Test 3: Per-IP isolation - different IPs should have separate quotas
test_rate_limit "Per-IP Isolation - IP 1" "$PROXY_ENDPOINT" 6 "allow_then_block" "203.0.113.1"

# Brief pause between IP tests
sleep 0.5

test_rate_limit "Per-IP Isolation - IP 2" "$PROXY_ENDPOINT" 6 "allow_then_block" "203.0.113.2"

# Test 4: Burst capacity
test_burst_capacity

# Test 5: Rate limit headers
echo
echo "Testing: Rate Limit Headers"
echo "Endpoint: $PROXY_ENDPOINT"
echo

headers=$(curl -s -I -X POST \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"header test\"}" \
    "$OLLA_URL$PROXY_ENDPOINT")

rate_limit_limit=$(echo "$headers" | grep -i "x-ratelimit-limit:" | cut -d: -f2 | tr -d ' \r')
rate_limit_remaining=$(echo "$headers" | grep -i "x-ratelimit-remaining:" | cut -d: -f2 | tr -d ' \r')
rate_limit_reset=$(echo "$headers" | grep -i "x-ratelimit-reset:" | cut -d: -f2 | tr -d ' \r')

echo "X-RateLimit-Limit: $rate_limit_limit"
echo "X-RateLimit-Remaining: $rate_limit_remaining"
echo "X-RateLimit-Reset: $rate_limit_reset"

if [[ -n "$rate_limit_limit" && -n "$rate_limit_remaining" ]]; then
    echo "✅ Rate limit headers present"
    record_test_result "Rate Limit Headers" "true"
else
    echo "❌ Rate limit headers missing"
    record_test_result "Rate Limit Headers" "false"
fi

# Test 6: Token refill (wait and retry)
echo
echo "Testing: Token Refill"
echo "Using unique IP for this test to avoid interference..."

# Use unique IP for clean test
test_ip="203.0.113.200"

# Exhaust quota with rapid requests
echo "Exhausting quota with rapid requests..."
for ((i=1; i<=5; i++)); do
    curl -s -o /dev/null \
        -X POST \
        -H "Content-Type: application/json" \
        -H "X-Forwarded-For: $test_ip" \
        -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"exhaust quota\"}" \
        "$OLLA_URL$PROXY_ENDPOINT"
    sleep 0.05
done

# Should be rate limited now
status_before=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-Forwarded-For: $test_ip" \
    -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"should be blocked\"}" \
    "$OLLA_URL$PROXY_ENDPOINT")

echo "Status before wait: $status_before"

# Wait for token refill (5 requests per minute = 1 token every 12 seconds)
echo "Waiting 13 seconds for token refill..."
sleep 13

# Try again
status_after=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST \
    -H "Content-Type: application/json" \
    -H "X-Forwarded-For: $test_ip" \
    -d "{\"model\":\"$MODEL_NAME\",\"prompt\":\"should work after refill\"}" \
    "$OLLA_URL$PROXY_ENDPOINT")

echo "Status after wait: $status_after"

if [[ "$status_before" == "429" && ("$status_after" == "200" || "$status_after" == "502") ]]; then
    echo "✅ Token refill working"
    record_test_result "Token Refill" "true"
else
    echo "❌ Token refill may not be working as expected"
    echo "   Expected: 429 -> 200/502, Got: $status_before -> $status_after"
    record_test_result "Token Refill" "false"
fi

echo
echo "🦙 Rate Limiting Test Summary"
echo "=============================="
echo "Tests completed: $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS"
echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
echo

if [[ ${#FAILED_TESTS[@]} -eq 0 ]]; then
    echo "🎉 ALL TESTS PASSED! Your Olla rate limiting is working correctly!"
    echo
    echo "✅ Security Assessment: Rate limiting is protecting your Olla instance!"
else
    echo "⚠️  SOME TESTS FAILED:"
    for failed_test in "${FAILED_TESTS[@]}"; do
        echo "   ❌ $failed_test"
    done
    echo
    echo "💡 This might indicate:"
    echo "   - Rate limits are different than expected"
    echo "   - Rate limiting is disabled"
    echo "   - Configuration needs adjustment"
fi

echo
echo "Status Code Reference:"
echo "- HTTP 200: Request allowed, backend responded"
echo "- HTTP 502: Request allowed, but no backend available"
echo "- HTTP 429: Rate limited (SECURITY WORKING!)"
echo
echo "Rate Limit Headers:"
echo "- X-RateLimit-Limit: Maximum requests allowed per time window"
echo "- X-RateLimit-Remaining: Requests remaining in current window"
echo "- X-RateLimit-Reset: When the rate limit window resets"
echo "- Retry-After: Seconds to wait before retrying (on 429 responses)"
echo
echo "To test with different limits, restart Olla with:"
echo "  export OLLA_SERVER_PER_IP_RATE_LIMIT=\"10\""
echo "  export OLLA_SERVER_RATE_BURST_SIZE=\"5\""
echo "  export OLLA_SERVER_GLOBAL_RATE_LIMIT=\"100\""
echo "  make run"

# Exit with appropriate code
if [[ ${#FAILED_TESTS[@]} -eq 0 ]]; then
    exit 0
else
    exit 1
fi