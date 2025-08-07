#!/bin/bash

# Olla Provider-Specific Model Endpoints Test Script
# Tests the new provider-specific model listing endpoints
####
# Usage: ./test-provider-models.sh
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
CURL_TIMEOUT=30
VERBOSE="${VERBOSE:-false}"

# Track results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

function banner() {
    echo -e "${PURPLE}╔═════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}  ${CYAN}Olla Provider Model Endpoints Test${RESET}                     ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╚═════════════════════════════════════════════════════════╝${RESET}"
    echo
    echo -e "${WHITE}Expected Behavior:${RESET}"
    echo -e "${GREY}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${CYAN}/olla/ollama/api/tags${RESET}        ${BLUE}→${RESET} Ollama models (native format)"
    echo -e "${CYAN}/olla/ollama/v1/models${RESET}       ${BLUE}→${RESET} Ollama models (OpenAI format)"
    echo -e "${CYAN}/olla/lmstudio/v1/models${RESET}     ${BLUE}→${RESET} LM Studio models (OpenAI format)"
    echo -e "${CYAN}/olla/lmstudio/api/v0/*${RESET}      ${BLUE}→${RESET} LM Studio models (enhanced format)"
    echo -e "${CYAN}/olla/openai/v1/models${RESET}       ${BLUE}→${RESET} OpenAI-compatible models"
    echo -e "${CYAN}/olla/vllm/v1/models${RESET}         ${BLUE}→${RESET} vLLM models (enhanced metadata)"
    echo -e "${GREY}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${CYAN}/olla/models${RESET}                 ${BLUE}→${RESET} Olla Unified format (default)"
    echo -e "${CYAN}/olla/models?format=unified${RESET}  ${BLUE}→${RESET} Olla Unified format (explicit)"
    echo -e "${CYAN}/olla/models?format=openai${RESET}   ${BLUE}→${RESET} OpenAI format (all models)"
    echo -e "${CYAN}/olla/models?format=ollama${RESET}   ${BLUE}→${RESET} Ollama format (filters to Ollama only)"
    echo -e "${CYAN}/olla/models?format=lmstudio${RESET} ${BLUE}→${RESET} LM Studio format (filters to LM Studio only)"
    echo -e "${CYAN}/olla/models?format=vllm${RESET}     ${BLUE}→${RESET} vLLM format (filters to vLLM only)"
    echo
}

usage() {
    echo -e "${WHITE}Usage:${RESET} $0"
    echo
    echo -e "${YELLOW}Environment Variables:${RESET}"
    echo -e "  TARGET_URL      Olla proxy URL (default: http://localhost:40114)"
    echo -e "  VERBOSE         Show detailed request/response info (default: false)"
    echo
    echo -e "${YELLOW}Description:${RESET}"
    echo -e "  This script tests the provider-specific model listing endpoints"
    echo -e "  to ensure they return models in the correct format for each provider."
    echo
}

test_endpoint() {
    local endpoint=$1
    local expected_format=$2
    local description=$3
    
    echo -e "\n${WHITE}Testing:${RESET} ${CYAN}$endpoint${RESET}"
    echo -e "${GREY}Expected format: $expected_format${RESET}"
    echo -e "${GREY}Description: $description${RESET}"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # Make the request
    local response
    local http_code
    
    response=$(curl -s -w "\n%{http_code}" \
        -X GET \
        -H "User-Agent: OllaProviderTest/1.0" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}${endpoint}" 2>&1)
    
    http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | sed '$d')
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "  ${GREEN}✓ Success${RESET} (HTTP $http_code)"
        
        # Validate response format
        local format_valid=false
        local format_error=""
        
        case "$expected_format" in
            "ollama")
                # Check for Ollama format: {"models": [...]}
                if echo "$response_body" | grep -q '"models"' && echo "$response_body" | grep -q '"name"'; then
                    format_valid=true
                    local model_count=$(echo "$response_body" | grep -o '"name"' | wc -l)
                    echo -e "  ${CYAN}→ Found ${model_count} models in Ollama format${RESET}"
                else
                    format_error="Missing 'models' array or 'name' fields"
                fi
                ;;
            "openai")
                # Check for OpenAI format: {"object": "list", "data": [...]}
                if echo "$response_body" | grep -q '"object".*"list"' && echo "$response_body" | grep -q '"data"'; then
                    format_valid=true
                    local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                    echo -e "  ${CYAN}→ Found ${model_count} models in OpenAI format${RESET}"
                else
                    format_error="Missing 'object: list' or 'data' array"
                fi
                ;;
            "lmstudio")
                # LM Studio enhanced format is actually OpenAI format with extra fields
                if echo "$response_body" | grep -q '"object".*"list"' && echo "$response_body" | grep -q '"data"'; then
                    format_valid=true
                    local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                    echo -e "  ${CYAN}→ Found ${model_count} models in LM Studio enhanced format${RESET}"
                else
                    format_error="Missing 'object: list' or 'data' array"
                fi
                ;;
        esac
        
        if [ "$format_valid" = true ]; then
            echo -e "  ${GREEN}✓ Response format is correct${RESET}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            
            # Show sample of response in verbose mode
            if [ "$VERBOSE" = "true" ]; then
                echo -e "  ${GREY}Sample response:${RESET}"
                echo "$response_body" | head -n 10 | sed 's/^/    /'
                if [ $(echo "$response_body" | wc -l) -gt 10 ]; then
                    echo "    ..."
                fi
            fi
        else
            echo -e "  ${RED}✗ Invalid response format${RESET}"
            echo -e "  ${YELLOW}Error: $format_error${RESET}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            
            # Show response for debugging
            if [ ${#response_body} -gt 200 ]; then
                echo -e "  ${GREY}Response: ${response_body:0:200}...${RESET}"
            else
                echo -e "  ${GREY}Response: $response_body${RESET}"
            fi
        fi
    else
        echo -e "  ${RED}✗ Failed${RESET} (HTTP $http_code)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        
        # Show error details
        if [ -n "$response_body" ]; then
            if [ ${#response_body} -gt 200 ]; then
                echo -e "  ${YELLOW}Error: ${response_body:0:200}...${RESET}"
            else
                echo -e "  ${YELLOW}Error: $response_body${RESET}"
            fi
        fi
    fi
}

test_unified_endpoint() {
    local endpoint=$1
    local query=$2
    local description=$3
    
    echo -e "\n${WHITE}Testing:${RESET} ${CYAN}$endpoint$query${RESET}"
    echo -e "${GREY}Description: $description${RESET}"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # Make the request
    local response
    local http_code
    
    response=$(curl -s -w "\n%{http_code}" \
        -X GET \
        -H "User-Agent: OllaProviderTest/1.0" \
        --max-time "$CURL_TIMEOUT" \
        "${TARGET_URL}${endpoint}${query}" 2>&1)
    
    http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | sed '$d')
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "  ${GREEN}✓ Success${RESET} (HTTP $http_code)"
        
        # Check format based on query parameter
        local format_valid=false
        local format_type="unified"
        
        if [[ "$query" == *"format=ollama"* ]]; then
            format_type="ollama"
            if echo "$response_body" | grep -q '"models"' && echo "$response_body" | grep -q '"name"'; then
                format_valid=true
                local model_count=$(echo "$response_body" | grep -o '"name"' | wc -l)
                echo -e "  ${CYAN}→ Found ${model_count} models in Ollama format${RESET}"
            fi
        elif [[ "$query" == *"format=lmstudio"* ]]; then
            format_type="lmstudio"
            if echo "$response_body" | grep -q '"object".*"list"' && echo "$response_body" | grep -q '"data"'; then
                format_valid=true
                local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                echo -e "  ${CYAN}→ Found ${model_count} models in LM Studio format${RESET}"
            fi
        elif [[ "$query" == *"format=openai"* ]]; then
            format_type="openai"
            if echo "$response_body" | grep -q '"object".*"list"' && echo "$response_body" | grep -q '"data"'; then
                format_valid=true
                local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                echo -e "  ${CYAN}→ Found ${model_count} models in OpenAI format${RESET}"
            fi
        elif [[ "$query" == *"format=vllm"* ]]; then
            format_type="vllm"
            if echo "$response_body" | grep -q '"object".*"list"' && echo "$response_body" | grep -q '"data"'; then
                format_valid=true
                local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                echo -e "  ${CYAN}→ Found ${model_count} models in vLLM format${RESET}"
                # Check for vLLM-specific fields
                if echo "$response_body" | grep -q '"max_model_len"'; then
                    echo -e "  ${GREEN}✓ Contains vLLM-specific metadata (max_model_len)${RESET}"
                fi
            fi
        else
            # Default unified format
            if echo "$response_body" | grep -q '"data"' && echo "$response_body" | grep -q '"olla"'; then
                format_valid=true
                local model_count=$(echo "$response_body" | grep -o '"id"' | wc -l)
                echo -e "  ${CYAN}→ Found ${model_count} models in unified format${RESET}"
            fi
        fi
        
        if [ "$format_valid" = true ]; then
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            echo -e "  ${RED}✗ Invalid ${format_type} format${RESET}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            
            # Show sample for debugging
            if [ "$VERBOSE" = "true" ]; then
                echo -e "  ${GREY}Response: ${response_body:0:200}...${RESET}"
            fi
        fi
    else
        echo -e "  ${RED}✗ Failed${RESET} (HTTP $http_code)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

run_tests() {
    echo -e "${WHITE}Running provider-specific model endpoint tests...${RESET}"
    echo -e "${GREY}Target: ${TARGET_URL}${RESET}"
    
    # Test Ollama endpoints
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Ollama Provider Endpoints${RESET}"
    
    test_endpoint "/olla/ollama/api/tags" "ollama" "Ollama native format model listing"
    test_endpoint "/olla/ollama/v1/models" "openai" "Ollama OpenAI-compatible format"
    
    # Test LM Studio endpoints
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}LM Studio Provider Endpoints${RESET}"
    
    test_endpoint "/olla/lmstudio/v1/models" "openai" "LM Studio OpenAI format"
    test_endpoint "/olla/lmstudio/api/v1/models" "openai" "LM Studio OpenAI format (alt path)"
    test_endpoint "/olla/lmstudio/api/v0/models" "lmstudio" "LM Studio enhanced format"
    
    # Test OpenAI-compatible endpoints
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}OpenAI-Compatible Provider Endpoints${RESET}"
    
    test_endpoint "/olla/openai/v1/models" "openai" "OpenAI-compatible model listing"
    
    # Test vLLM endpoints
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}vLLM Provider Endpoints${RESET}"
    
    test_endpoint "/olla/vllm/v1/models" "openai" "vLLM OpenAI-compatible model listing"
    
    # Test unified endpoints with filtering
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Unified Model Endpoints${RESET}"
    
    test_unified_endpoint "/olla/models" "" "Unified models (all providers)"
    test_unified_endpoint "/olla/models" "?format=unified" "Unified format (default)"
    test_unified_endpoint "/olla/models" "?format=openai" "OpenAI format (all models)"
    test_unified_endpoint "/olla/models" "?format=ollama" "Ollama format (Ollama models only)"
    test_unified_endpoint "/olla/models" "?format=lmstudio" "LM Studio format (LM Studio models only)"
    test_unified_endpoint "/olla/models" "?format=vllm" "vLLM format (vLLM models only)"
}

show_summary() {
    echo -e "\n${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo -e "${WHITE}Test Summary:${RESET}"
    echo -e "  Total Tests:     ${CYAN}$TOTAL_TESTS${RESET}"
    echo -e "  Passed:          ${GREEN}$PASSED_TESTS${RESET}"
    echo -e "  Failed:          ${RED}$FAILED_TESTS${RESET}"
    
    if [ $TOTAL_TESTS -gt 0 ]; then
        local success_rate=$((PASSED_TESTS * 100 / TOTAL_TESTS))
        echo -e "  Success Rate:    ${YELLOW}${success_rate}%${RESET}"
        
        echo
        if [ $success_rate -eq 100 ]; then
            echo -e "${GREEN}✓ All provider model endpoint tests passed!${RESET}"
        elif [ $success_rate -ge 80 ]; then
            echo -e "${YELLOW}⚠ Most tests passed, but some issues detected${RESET}"
        else
            echo -e "${RED}✗ Significant issues with provider model endpoints${RESET}"
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
    
    run_tests
    show_summary
    
    # Exit with error code if tests failed
    if [ $FAILED_TESTS -gt 0 ]; then
        exit 1
    fi
}

main "$@"