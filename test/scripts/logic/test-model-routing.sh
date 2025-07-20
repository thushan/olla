#!/bin/bash

# Olla Model Routing Test Script
# Tests model routing with different models and capabilities
####
# Usage: ./test-model-routing.sh [additional_model1] [additional_model2] ...
# Configuration: Set TARGET_URL environment variable if needed

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GREY='\033[0;37m'
RESET='\033[0m'

# Configuration
TARGET_URL="${TARGET_URL:-http://localhost:40114}"
PROXY_ENDPOINT="/olla/"
CURL_TIMEOUT=180
VERBOSE="${VERBOSE:-false}"

# Arrays for models
declare -a AVAILABLE_MODELS=()
declare -a ADDITIONAL_MODELS=()
declare -a ALL_MODELS=()

# Test cases with different capabilities
declare -A TEST_CASES
TEST_CASES["simple_chat"]='{"model": "MODEL_NAME", "messages": [{"role": "user", "content": "Say hello"}], "max_tokens": 50}'
TEST_CASES["vision_request"]='{"model": "MODEL_NAME", "messages": [{"role": "user", "content": [{"type": "text", "text": "What do you see?"}, {"type": "image_url", "image_url": {"url": "https://example.com/test.jpg"}}]}], "max_tokens": 50}'
TEST_CASES["function_call"]='{"model": "MODEL_NAME", "messages": [{"role": "user", "content": "Get the weather"}], "tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather"}}], "max_tokens": 50}'
TEST_CASES["code_request"]='{"model": "MODEL_NAME", "messages": [{"role": "system", "content": "You are a code assistant"}, {"role": "user", "content": "Write a function"}], "max_tokens": 100}'
TEST_CASES["embedding_request"]='{"model": "MODEL_NAME", "input": "This is a test sentence for embeddings"}'

# Track results
TOTAL_TESTS=0
SUCCESSFUL_ROUTES=0
FAILED_ROUTES=0

function banner() {
    echo -e "${PURPLE}╔═════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}  ${CYAN}Olla Model Routing Test${RESET}                                ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╚═════════════════════════════════════════════════════════╝${RESET}"
    echo
}

usage() {
    echo -e "${WHITE}Usage:${RESET} $0 [additional_model1] [additional_model2] ..."
    echo
    echo -e "${YELLOW}Environment Variables:${RESET}"
    echo -e "  TARGET_URL      Olla proxy URL (default: http://localhost:40114)"
    echo -e "  VERBOSE         Show detailed request/response info (default: false)"
    echo
    echo -e "${YELLOW}Description:${RESET}"
    echo -e "  This script automatically discovers available models from Olla"
    echo -e "  and tests routing for each model with appropriate capabilities."
    echo -e "  You can also specify additional models to test as arguments."
    echo
    echo -e "${YELLOW}Examples:${RESET}"
    echo -e "  $0                               # Test only discovered models"
    echo -e "  $0 gpt-4-turbo claude-3-sonnet   # Test discovered + additional models"
    echo -e "  VERBOSE=true $0                  # Show detailed debug information"
    echo
}

# Fetch available models from Olla
fetch_available_models() {
    echo -e "${YELLOW}Fetching available models from Olla...${RESET}"
    
    local response
    local http_code
    
    response=$(curl -s -w "\n%{http_code}" \
        -X GET \
        -H "User-Agent: OllaRoutingTest/1.0" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}/olla/models" 2>&1)
    
    http_code=$(echo "$response" | tail -n1)
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        # Extract model IDs from JSON response
        local model_data=$(echo "$response" | sed '$d')
        
        # Parse JSON to get model IDs (works with simple JSON structure)
        while IFS= read -r model_id; do
            if [ -n "$model_id" ]; then
                AVAILABLE_MODELS+=("$model_id")
            fi
        done < <(echo "$model_data" | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*: *"\([^"]*\)".*/\1/')
        
        echo -e "${GREEN}✓ Found ${#AVAILABLE_MODELS[@]} models in Olla${RESET}"
        
        # Show discovered models
        if [ ${#AVAILABLE_MODELS[@]} -gt 0 ]; then
            echo -e "${GREY}Available models:${RESET}"
            for model in "${AVAILABLE_MODELS[@]}"; do
                echo -e "  • ${CYAN}$model${RESET}"
            done
        fi
    else
        echo -e "${RED}✗ Failed to fetch models (HTTP $http_code)${RESET}"
        echo -e "${YELLOW}Will proceed with additional models only${RESET}"
    fi
    echo
}

test_model_routing() {
    local model=$1
    local test_name=$2
    local request_body=$3
    local endpoint=$4
    
    # Replace MODEL_NAME with actual model
    request_body="${request_body//MODEL_NAME/$model}"
    
    echo -e "\n${WHITE}Testing:${RESET} ${CYAN}$model${RESET} - ${YELLOW}$test_name${RESET}"
    echo -e "${GREY}Endpoint: ${endpoint}${RESET}"
    
    # Make the request and capture all details
    local response_file="/tmp/olla_test_response_$$"
    local http_code
    local response_time
    
    # Use curl with more detailed output
    local curl_output=$(curl -s -w "\n%{http_code}\n%{time_total}" \
        -o "$response_file" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "User-Agent: OllaRoutingTest/1.0" \
        -d "$request_body" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}${PROXY_ENDPOINT}${endpoint}" 2>&1)
    
    # Extract HTTP code and response time from the last two lines
    http_code=$(echo "$curl_output" | tail -n2 | head -n1)
    response_time=$(echo "$curl_output" | tail -n1)
    
    # Read response body
    local response=""
    if [ -f "$response_file" ]; then
        response=$(cat "$response_file")
        rm -f "$response_file"
    fi
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # Check response
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "  ${GREEN}✓ Success${RESET} (HTTP $http_code) - ${GREY}${response_time}s${RESET}"
        SUCCESSFUL_ROUTES=$((SUCCESSFUL_ROUTES + 1))
        
        # Try to extract endpoint info from response
        if echo "$response" | grep -q '"endpoint"'; then
            endpoint_info=$(echo "$response" | grep -o '"endpoint":"[^"]*"' | cut -d'"' -f4 || true)
            if [ -n "$endpoint_info" ]; then
                echo -e "  ${GREY}Routed to: $endpoint_info${RESET}"
            fi
        fi
    else
        echo -e "  ${RED}✗ Failed${RESET} (HTTP $http_code) - ${GREY}${response_time}s${RESET}"
        FAILED_ROUTES=$((FAILED_ROUTES + 1))
        
        # Show detailed error information
        if [ -n "$response" ]; then
            # Try to parse JSON error
            if echo "$response" | grep -q '"error"'; then
                error_msg=$(echo "$response" | grep -o '"error"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*: *"\([^"]*\)".*/\1/' || true)
                if [ -n "$error_msg" ]; then
                    echo -e "  ${YELLOW}Error:${RESET} ${error_msg}"
                fi
            fi
            
            # Try to parse message field
            if echo "$response" | grep -q '"message"'; then
                message=$(echo "$response" | grep -o '"message"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*: *"\([^"]*\)".*/\1/' || true)
                if [ -n "$message" ] && [ "$message" != "$error_msg" ]; then
                    echo -e "  ${YELLOW}Message:${RESET} ${message}"
                fi
            fi
            
            # If response is not JSON or we couldn't parse it, show raw response (truncated)
            if ! echo "$response" | grep -q '"error"\|"message"' && [ ${#response} -gt 0 ]; then
                if [ ${#response} -gt 200 ]; then
                    echo -e "  ${YELLOW}Response:${RESET} ${response:0:200}..."
                else
                    echo -e "  ${YELLOW}Response:${RESET} ${response}"
                fi
            fi
        elif [ "$http_code" = "000" ]; then
            echo -e "  ${YELLOW}Error:${RESET} Connection timeout or DNS resolution failed"
        elif [ "$http_code" = "404" ]; then
            echo -e "  ${YELLOW}Error:${RESET} Model not found or no compatible endpoint available"
        elif [ "$http_code" = "502" ]; then
            echo -e "  ${YELLOW}Error:${RESET} Bad gateway - backend endpoint may be down"
        elif [ "$http_code" = "503" ]; then
            echo -e "  ${YELLOW}Error:${RESET} Service unavailable - all endpoints may be unhealthy"
        fi
        
        # Show request details for debugging
        if [ "$VERBOSE" = "true" ]; then
            echo -e "  ${GREY}Request body: ${request_body:0:100}...${RESET}"
        fi
    fi
}

run_all_tests() {
    ALL_MODELS=("${AVAILABLE_MODELS[@]}" "${ADDITIONAL_MODELS[@]}")
    
    # Remove duplicates
    declare -A seen
    declare -a UNIQUE_MODELS=()
    for model in "${ALL_MODELS[@]}"; do
        if [[ -z "${seen[$model]:-}" ]]; then
            seen[$model]=1
            UNIQUE_MODELS+=("$model")
        fi
    done
    
    if [ ${#UNIQUE_MODELS[@]} -eq 0 ]; then
        echo -e "${RED}No models to test!${RESET}"
        echo -e "Either Olla has no models or you need to specify models as arguments."
        return
    fi
    
    echo -e "${WHITE}Running routing tests for ${#UNIQUE_MODELS[@]} models...${RESET}"
    echo -e "${GREY}Target: ${TARGET_URL}${PROXY_ENDPOINT}${RESET}"
    echo
    
    for model in "${UNIQUE_MODELS[@]}"; do
        echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
        echo -e "${WHITE}Model: ${CYAN}$model${RESET}"
        
        # Test simple chat (all models should support this)
        test_model_routing "$model" "Simple Chat" "${TEST_CASES[simple_chat]}" "v1/chat/completions"
        
        # Test vision capability (only vision models)
        if [[ "$model" =~ (llava|vision|gpt-4-vision) ]]; then
            test_model_routing "$model" "Vision Request" "${TEST_CASES[vision_request]}" "v1/chat/completions"
        fi
        
        # Test function calling (selected models)
        if [[ "$model" =~ (gpt-4|claude|mistral|llama3) ]]; then
            test_model_routing "$model" "Function Calling" "${TEST_CASES[function_call]}" "v1/chat/completions"
        fi
        
        # Test code generation (code-specific models)
        if [[ "$model" =~ (code|deepseek-coder|codellama) ]]; then
            test_model_routing "$model" "Code Generation" "${TEST_CASES[code_request]}" "v1/chat/completions"
        fi
        
        # Test embeddings (embedding models only)
        if [[ "$model" =~ (embed|embedding) ]]; then
            test_model_routing "$model" "Embeddings" "${TEST_CASES[embedding_request]}" "v1/embeddings"
        fi
    done
}

show_summary() {
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Test Summary:${RESET}"
    echo -e "  Total Tests:        ${CYAN}$TOTAL_TESTS${RESET}"
    echo -e "  Successful Routes:  ${GREEN}$SUCCESSFUL_ROUTES${RESET}"
    echo -e "  Failed Routes:      ${RED}$FAILED_ROUTES${RESET}"
    
    if [ $TOTAL_TESTS -gt 0 ]; then
        local success_rate=$((SUCCESSFUL_ROUTES * 100 / TOTAL_TESTS))
        echo -e "  Success Rate:       ${YELLOW}${success_rate}%${RESET}"
        
        echo
        if [ $success_rate -eq 100 ]; then
            echo -e "${GREEN}✓ All routing tests passed!${RESET}"
        elif [ $success_rate -ge 80 ]; then
            echo -e "${YELLOW}⚠ Most routing tests passed, but some failures detected${RESET}"
        else
            echo -e "${RED}✗ Significant routing issues detected${RESET}"
            echo
            echo -e "${YELLOW}Common issues to check:${RESET}"
            echo -e "  • Ensure models are available on configured endpoints"
            echo -e "  • Check that endpoints are healthy and reachable"
            echo -e "  • Verify model capabilities are properly detected"
            echo -e "  • Confirm routing configuration in Olla"
        fi
    fi
    echo
}


# Main execution
main() {
    if [[ "${1:-}" =~ ^(-h|--help|help)$ ]]; then
        banner
        usage
        exit 0
    fi
    
    banner

    if ! command -v curl >/dev/null 2>&1; then
        echo -e "${RED}ERROR:${RESET} curl is required"
        exit 1
    fi

    echo -e "${YELLOW}Checking Olla availability...${RESET}"
    if ! curl -s -f -o /dev/null --max-time 5 "${TARGET_URL}/internal/health" 2>/dev/null; then
        echo -e "${RED}ERROR:${RESET} Cannot reach Olla at ${TARGET_URL}"
        echo -e "Make sure Olla is running and accessible"
        exit 1
    fi
    echo -e "${GREEN}✓ Olla is reachable${RESET}"
    echo

    fetch_available_models
    
    # Process command-line arguments as additional models
    if [ $# -gt 0 ]; then
        echo -e "${YELLOW}Additional models specified:${RESET}"
        for model in "$@"; do
            ADDITIONAL_MODELS+=("$model")
            echo -e "  • ${CYAN}$model${RESET}"
        done
        echo
    fi

    run_all_tests
    show_summary
}

main "$@"