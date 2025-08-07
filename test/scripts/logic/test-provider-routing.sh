#!/bin/bash

# Test provider-specific routing endpoints
# Tests the new /olla/{provider}/api/* endpoints

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
CURL_TIMEOUT=30
VERBOSE="${VERBOSE:-false}"

# Track results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

function banner() {
    echo -e "${PURPLE}╔═════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}  ${CYAN}Olla Provider-Specific Routing Test${RESET}                    ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╚═════════════════════════════════════════════════════════╝${RESET}"
    echo
}

# Test a specific endpoint
test_endpoint() {
    local endpoint=$1
    local description=$2
    local expected_format=$3
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo -e "\n${WHITE}Testing:${RESET} ${CYAN}$endpoint${RESET}"
    echo -e "${GREY}Description: $description${RESET}"
    echo -e "${GREY}Expected format: $expected_format${RESET}"
    
    local response
    local http_code
    
    response=$(curl -s -w "\n%{http_code}" \
        -X GET \
        -H "User-Agent: OllaProviderTest/1.0" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}${endpoint}" 2>&1)
    
    http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "  ${GREEN}✓ Success${RESET} (HTTP $http_code)"
        
        # Validate response format
        case "$expected_format" in
            "ollama")
                if echo "$body" | grep -q '"models"' && echo "$body" | grep -q '"name"'; then
                    echo -e "  ${GREEN}✓ Valid Ollama format${RESET}"
                    # Count models
                    model_count=$(echo "$body" | grep -o '"name"' | wc -l)
                    echo -e "  ${CYAN}→ Found $model_count Ollama models${RESET}"
                    PASSED_TESTS=$((PASSED_TESTS + 1))
                else
                    echo -e "  ${RED}✗ Invalid Ollama format${RESET}"
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                fi
                ;;
            "openai")
                if echo "$body" | grep -q '"object":"list"' && echo "$body" | grep -q '"data"'; then
                    echo -e "  ${GREEN}✓ Valid OpenAI format${RESET}"
                    # Count models
                    model_count=$(echo "$body" | grep -o '"id"' | wc -l)
                    echo -e "  ${CYAN}→ Found $model_count models${RESET}"
                    PASSED_TESTS=$((PASSED_TESTS + 1))
                else
                    echo -e "  ${RED}✗ Invalid OpenAI format${RESET}"
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                fi
                ;;
        esac
        
        if [ "$VERBOSE" = "true" ]; then
            echo -e "  ${GREY}Response preview: ${body:0:200}...${RESET}"
        fi
    else
        echo -e "  ${RED}✗ Failed${RESET} (HTTP $http_code)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        
        if [ "$http_code" = "404" ]; then
            echo -e "  ${YELLOW}→ No providers of this type available${RESET}"
        elif [ "$http_code" = "000" ]; then
            echo -e "  ${YELLOW}→ Connection timeout or DNS resolution failed${RESET}"
        elif [ -n "$body" ]; then
            echo -e "  ${YELLOW}→ Error: $body${RESET}"
        fi
    fi
}

# Test provider proxy routing
test_proxy_routing() {
    local provider=$1
    local endpoint=$2
    local description=$3
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo -e "\n${WHITE}Testing:${RESET} ${CYAN}/olla/$provider$endpoint${RESET}"
    echo -e "${GREY}Description: $description${RESET}"
    
    # Simple test request
    local request_body='{"model": "test", "messages": [{"role": "user", "content": "test"}], "max_tokens": 1}'
    
    local response
    local http_code
    local headers_file="/tmp/olla_provider_headers_$$"
    
    response=$(curl -s -w "\n%{http_code}" \
        -D "$headers_file" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "User-Agent: OllaProviderTest/1.0" \
        -d "$request_body" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}/olla/$provider$endpoint" 2>&1)
    
    http_code=$(echo "$response" | tail -n1)
    
    # Parse headers
    local olla_endpoint=""
    local olla_backend_type=""
    if [ -f "$headers_file" ]; then
        olla_endpoint=$(grep -i "^X-Olla-Endpoint:" "$headers_file" | cut -d' ' -f2- | tr -d '\r' || true)
        olla_backend_type=$(grep -i "^X-Olla-Backend-Type:" "$headers_file" | cut -d' ' -f2- | tr -d '\r' || true)
        rm -f "$headers_file"
    fi
    
    # We expect either 404 (no providers) or 200/400/500 (request processed)
    if [ "$http_code" = "404" ]; then
        echo -e "  ${YELLOW}→ No $provider endpoints available${RESET}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    elif [[ "$http_code" =~ ^[245][0-9][0-9]$ ]]; then
        echo -e "  ${GREEN}✓ Request routed${RESET} (HTTP $http_code)"
        
        if [ -n "$olla_endpoint" ]; then
            echo -e "  ${CYAN}→ Routed to: $olla_endpoint${RESET}"
            
            # Verify it's the correct provider type
            # normalize provider names for comparison
            normalized_backend="$olla_backend_type"
            normalized_provider="$provider"
            
            # handle lm-studio variations
            if [[ "$olla_backend_type" == "lm-studio" ]] && [[ "$provider" == "lmstudio" ]]; then
                normalized_provider="lm-studio"
            fi
            
            if [ -n "$olla_backend_type" ] && [ "$normalized_backend" = "$normalized_provider" ]; then
                echo -e "  ${GREEN}✓ Correct provider type: $olla_backend_type${RESET}"
                PASSED_TESTS=$((PASSED_TESTS + 1))
            else
                echo -e "  ${RED}✗ Wrong provider type: $olla_backend_type (expected: $provider)${RESET}"
                FAILED_TESTS=$((FAILED_TESTS + 1))
            fi
        else
            PASSED_TESTS=$((PASSED_TESTS + 1))
        fi
    else
        echo -e "  ${RED}✗ Unexpected response${RESET} (HTTP $http_code)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# Compare unified vs provider-specific models
compare_model_counts() {
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Model Count Comparison:${RESET}"
    
    # Get unified model count
    local unified_response=$(curl -s "${TARGET_URL}/olla/models" 2>/dev/null || echo '{}')
    local unified_count=$(echo "$unified_response" | grep -o '"id"' | wc -l)
    echo -e "  ${CYAN}Unified models (/olla/models):${RESET} $unified_count"
    
    # Get Ollama model count
    local ollama_response=$(curl -s "${TARGET_URL}/olla/ollama/api/tags" 2>/dev/null || echo '{}')
    local ollama_count=$(echo "$ollama_response" | grep -o '"name"' | wc -l)
    echo -e "  ${CYAN}Ollama models (/olla/ollama/api/tags):${RESET} $ollama_count"
    
    # Get LM Studio model count
    local lmstudio_response=$(curl -s "${TARGET_URL}/olla/lmstudio/api/v1/tags" 2>/dev/null || echo '{}')
    local lmstudio_count=$(echo "$lmstudio_response" | grep -o '"name"' | wc -l)
    echo -e "  ${CYAN}LM Studio models (/olla/lmstudio/api/v1/tags):${RESET} $lmstudio_count"
    
    # Get vLLM model count
    local vllm_response=$(curl -s "${TARGET_URL}/olla/vllm/v1/models" 2>/dev/null || echo '{}')
    local vllm_count=$(echo "$vllm_response" | grep -o '"id"' | wc -l)
    echo -e "  ${CYAN}vLLM models (/olla/vllm/v1/models):${RESET} $vllm_count"
    
    # Get OpenAI model count
    local openai_response=$(curl -s "${TARGET_URL}/olla/openai/v1/models" 2>/dev/null || echo '{}')
    local openai_count=$(echo "$openai_response" | grep -o '"id"' | wc -l)
    echo -e "  ${CYAN}OpenAI-compatible models (/olla/openai/v1/models):${RESET} $openai_count"
    
    # Sanity check: provider models should sum to <= unified count
    local provider_total=$((ollama_count + lmstudio_count + vllm_count + openai_count))
    if [ $provider_total -le $unified_count ] || [ $unified_count -eq 0 ]; then
        echo -e "  ${GREEN}✓ Model counts are consistent${RESET}"
    else
        echo -e "  ${YELLOW}⚠ Provider models ($provider_total) exceed unified count ($unified_count)${RESET}"
    fi
}

show_summary() {
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Test Summary:${RESET}"
    echo -e "  Total Tests:   ${CYAN}$TOTAL_TESTS${RESET}"
    echo -e "  Passed:        ${GREEN}$PASSED_TESTS${RESET}"
    echo -e "  Failed:        ${RED}$FAILED_TESTS${RESET}"
    
    if [ $TOTAL_TESTS -gt 0 ]; then
        local success_rate=$((PASSED_TESTS * 100 / TOTAL_TESTS))
        echo -e "  Success Rate:  ${YELLOW}${success_rate}%${RESET}"
        
        echo
        if [ $success_rate -eq 100 ]; then
            echo -e "${GREEN}✓ All provider routing tests passed!${RESET}"
        elif [ $success_rate -ge 80 ]; then
            echo -e "${YELLOW}⚠ Most tests passed, but some issues detected${RESET}"
        else
            echo -e "${RED}✗ Significant issues with provider routing${RESET}"
        fi
    fi
    echo
}

# Main execution
main() {
    if [[ "${1:-}" =~ ^(-h|--help|help)$ ]]; then
        banner
        echo -e "${WHITE}Usage:${RESET} $0"
        echo
        echo -e "${YELLOW}Environment Variables:${RESET}"
        echo -e "  TARGET_URL      Olla proxy URL (default: http://localhost:40114)"
        echo -e "  VERBOSE         Show detailed response info (default: false)"
        echo
        echo -e "${YELLOW}Description:${RESET}"
        echo -e "  Tests provider-specific routing endpoints that filter models"
        echo -e "  and requests by provider type (Ollama, LM Studio, etc.)"
        echo
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
    
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Testing Provider Model Endpoints:${RESET}"
    
    # Test Ollama endpoints
    test_endpoint "/olla/ollama/api/tags" "Ollama models in native format" "ollama"
    
    # Test LM Studio endpoints
    test_endpoint "/olla/lmstudio/api/v1/tags" "LM Studio models in Ollama format" "ollama"
    test_endpoint "/olla/lmstudio/v1/models" "LM Studio models in OpenAI format" "openai"
    test_endpoint "/olla/lmstudio/api/v1/models" "LM Studio models in OpenAI format (alt path)" "openai"
    
    # Test vLLM endpoints
    test_endpoint "/olla/vllm/v1/models" "vLLM models in OpenAI format with enhanced metadata" "openai"
    
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Testing Provider Proxy Routing:${RESET}"
    
    # Test proxy routing
    test_proxy_routing "ollama" "/api/generate" "Ollama generate endpoint"
    test_proxy_routing "ollama" "/api/chat" "Ollama chat endpoint"
    test_proxy_routing "lmstudio" "/v1/chat/completions" "LM Studio chat completions"
    test_proxy_routing "openai" "/v1/chat/completions" "OpenAI chat completions"
    test_proxy_routing "vllm" "/v1/chat/completions" "vLLM chat completions"
    test_proxy_routing "vllm" "/v1/completions" "vLLM completions"
    
    # Compare model counts
    compare_model_counts
    
    show_summary
}

main "$@"