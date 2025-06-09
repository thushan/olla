#!/bin/bash

# Olla Load Test Script
# Tests the proxy endpoint with configurable concurrency and duration
# Reads config from .env file, takes duration and concurrency as args
####
# Usage: ./test-load-limits.sh <duration> <concurrency>
# Example: ./test-load-limits.sh 300 10 # Defaults: 5 minutes duration, 10 concurrent workers

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GREY='\033[0;37m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

# Global variables for tracking
TOTAL_REQUESTS=0
SUCCESSFUL_REQUESTS=0
FAILED_REQUESTS=0
TOTAL_RESPONSE_TIME=0
MIN_RESPONSE_TIME=999999
MAX_RESPONSE_TIME=0
START_TIME=""
STATS_DIR=""
WORKER_PIDS=""

# Configuration defaults
PROXY_ENDPOINT="/olla/"
TARGET_URL=""
RATE_LIMIT_DELAY=0
MODEL_NAME="phi4:latest"
DURATION=60
CONCURRENCY=5
CURL_MAX_TIME=300

# Test questions for the model
QUESTIONS=(
    "What is the capital of Australia?"
    "Explain quantum computing in simple terms"
    "Write a haiku about coffee"
    "What are the benefits of renewable energy?"
    "How does machine learning work?"
    "Describe the water cycle"
    "What is the difference between AI and ML?"
    "List 3 interesting facts about space"
    "How do you make a perfect cup of tea?"
    "What is the meaning of life?"
    "Explain photosynthesis briefly"
    "What are the main programming paradigms?"
    "How does the internet work?"
    "What is climate change?"
    "Describe how a computer processor works"
    "What are the health benefits of exercise?"
    "How do vaccines work?"
    "What is blockchain technology?"
    "Explain the concept of time zones"
    "What makes a good story?"
)

function banner() {
    echo -e "${PURPLE}╔═════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}  ${CYAN}🦙 Olla Load Test${RESET} ${GREY}- Continuous Proxy Testing${RESET}           ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╚═════════════════════════════════════════════════════════╝${RESET}"
    echo
}

usage() {
    echo -e "${WHITE}Usage:${RESET} $0 <duration> <concurrency>"
    echo
    echo -e "${YELLOW}Arguments:${RESET}"
    echo -e "  duration     Test duration in seconds"
    echo -e "  concurrency  Number of concurrent workers"
    echo
    echo -e "${YELLOW}Configuration (.env file):${RESET}"
    echo -e "  TARGET_URL      Olla proxy URL (e.g., http://localhost:19841)"
    echo -e "  RATE_LIMIT      Delay between requests in seconds (default: 0)"
    echo -e "  MODEL_NAME      Model to test with (default: phi4:latest)"
    echo
    echo -e "${YELLOW}Example:${RESET}"
    echo -e "  $0 300 10   # Run for 5 minutes with 10 concurrent workers"
    echo
}

load_env() {
    if [ ! -f ".env" ]; then
        echo -e "${RED}ERROR:${RESET} .env file not found"
        echo
        echo -e "${YELLOW}Create a .env file with:${RESET}"
        echo -e "TARGET_URL=http://localhost:19841"
        echo -e "RATE_LIMIT=0.1"
        echo -e "MODEL_NAME=phi4:latest"
        exit 1
    fi

    # Load .env file the simple way
    while IFS='=' read -r key value; do
        # Skip comments and empty lines
        case "$key" in
            '#'*|'') continue ;;
        esac

        # Remove quotes if present
        value=$(echo "$value" | sed 's/^["'\'']//' | sed 's/["'\'']$//')

        case "$key" in
            'TARGET_URL') TARGET_URL="$value" ;;
            'RATE_LIMIT') RATE_LIMIT_DELAY="$value" ;;
            'MODEL_NAME') MODEL_NAME="$value" ;;
        esac
    done < .env

    if [ -z "$TARGET_URL" ]; then
        echo -e "${RED}ERROR:${RESET} TARGET_URL not set in .env file"
        exit 1
    fi
}

validate_args() {
    if [ $# -ne 2 ]; then
        usage
        exit 1
    fi

    if ! echo "$1" | grep -q '^[0-9]\+$' || [ "$1" -le 0 ]; then
        echo -e "${RED}ERROR:${RESET} Duration must be a positive integer"
        exit 1
    fi

    if ! echo "$2" | grep -q '^[0-9]\+$' || [ "$2" -le 0 ]; then
        echo -e "${RED}ERROR:${RESET} Concurrency must be a positive integer"
        exit 1
    fi

    DURATION=$1
    CONCURRENCY=$2
}

setup_temp_files() {
    STATS_DIR="/tmp/olla_load_test_$$"
    mkdir -p "$STATS_DIR"

    # Initialise stats
    echo "0" > "$STATS_DIR/total"
    echo "0" > "$STATS_DIR/success"
    echo "0" > "$STATS_DIR/failed"
    echo "0" > "$STATS_DIR/total_time"
    echo "999999" > "$STATS_DIR/min_time"
    echo "0" > "$STATS_DIR/max_time"
    echo "" > "$STATS_DIR/errors"
    echo "" > "$STATS_DIR/pids"
}

cleanup() {
    echo
    echo -e "${YELLOW}Cleaning up...${RESET}"

    # Kill all worker processes
    if [ -f "$STATS_DIR/pids" ]; then
        while read -r pid; do
            if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
                kill "$pid" 2>/dev/null || true
            fi
        done < "$STATS_DIR/pids"

        sleep 1

        # Force kill if needed
        while read -r pid; do
            if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        done < "$STATS_DIR/pids"
    fi

    # Clean up temp files
    if [ -n "$STATS_DIR" ] && [ -d "$STATS_DIR" ]; then
        rm -rf "$STATS_DIR"
    fi
}

get_random_question() {
    local index=$((RANDOM % ${#QUESTIONS[@]}))
    echo "${QUESTIONS[$index]}"
}

update_stats() {
    local response_code="$1"
    local response_time_ms="$2"
    local worker_id="$3"
    local question="$4"

    # Simple file-based locking using mkdir (atomic operation)
    while ! mkdir "$STATS_DIR/lock" 2>/dev/null; do
        sleep 0.001
    done

    # Read current stats
    local total=$(cat "$STATS_DIR/total")
    local success=$(cat "$STATS_DIR/success")
    local failed=$(cat "$STATS_DIR/failed")
    local total_time=$(cat "$STATS_DIR/total_time")
    local min_time=$(cat "$STATS_DIR/min_time")
    local max_time=$(cat "$STATS_DIR/max_time")

    # Update counters
    total=$((total + 1))

    if echo "$response_code" | grep -q '^2[0-9][0-9]$'; then
        success=$((success + 1))

        # Update response time stats
        total_time=$((total_time + response_time_ms))

        if [ "$response_time_ms" -lt "$min_time" ]; then
            min_time=$response_time_ms
        fi

        if [ "$response_time_ms" -gt "$max_time" ]; then
            max_time=$response_time_ms
        fi
    else
        failed=$((failed + 1))
        echo "Worker $worker_id: HTTP $response_code - $question" >> "$STATS_DIR/errors"
    fi

    # Write updated stats
    echo "$total" > "$STATS_DIR/total"
    echo "$success" > "$STATS_DIR/success"
    echo "$failed" > "$STATS_DIR/failed"
    echo "$total_time" > "$STATS_DIR/total_time"
    echo "$min_time" > "$STATS_DIR/min_time"
    echo "$max_time" > "$STATS_DIR/max_time"

    # Release lock
    rmdir "$STATS_DIR/lock"
}

make_request() {
    local question="$1"
    local worker_id="$2"

    local request_body="{\"model\": \"$MODEL_NAME\", \"messages\": [{\"role\": \"user\", \"content\": \"$question\"}], \"max_tokens\": 150, \"temperature\": 0.7}"

    local start_time=$(date +%s)
    local response_code

    # Make the request
    response_code=$(curl -s -w "%{http_code}" -o /dev/null \
        -X POST \
        -H "Content-Type: application/json" \
        -H "User-Agent: OllaLoadTest/1.0" \
        -d "$request_body" \
        --max-time "${CURL_MAX_TIME:-120}" \
        "${TARGET_URL}${PROXY_ENDPOINT}v1/chat/completions" 2>/dev/null || echo "000")

    local end_time=$(date +%s)
    local response_time_ms=$(((end_time - start_time) * 1000))

    update_stats "$response_code" "$response_time_ms" "$worker_id" "$question"
}

worker() {
    local worker_id=$1
    local end_time=$(($(date +%s) + DURATION))

    while [ $(date +%s) -lt $end_time ]; do
        local question=$(get_random_question)
        make_request "$question" "$worker_id"

        # Simple rate limiting
        if [ "$RATE_LIMIT_DELAY" != "0" ] && [ -n "$RATE_LIMIT_DELAY" ]; then
            sleep "$RATE_LIMIT_DELAY"
        fi
    done
}

display_progress() {
    local duration=$1
    local start_time=$(date +%s)
    local end_time=$((start_time + duration))

    while [ $(date +%s) -lt $end_time ]; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        local remaining=$((end_time - current_time))

        # Read current stats
        local total=$(cat "$STATS_DIR/total" 2>/dev/null || echo "0")
        local success=$(cat "$STATS_DIR/success" 2>/dev/null || echo "0")
        local failed=$(cat "$STATS_DIR/failed" 2>/dev/null || echo "0")
        local total_time=$(cat "$STATS_DIR/total_time" 2>/dev/null || echo "0")
        local min_time=$(cat "$STATS_DIR/min_time" 2>/dev/null || echo "999999")
        local max_time=$(cat "$STATS_DIR/max_time" 2>/dev/null || echo "0")

        # Calculate metrics using simple arithmetic
        local success_rate=0
        local avg_response_time=0
        local requests_per_sec=0

        if [ "$total" -gt 0 ]; then
            success_rate=$((success * 100 / total))
            if [ "$elapsed" -gt 0 ]; then
                requests_per_sec=$((total / elapsed))
            fi

            if [ "$success" -gt 0 ]; then
                avg_response_time=$((total_time / success))
            fi
        fi

        # Progress bar
        local progress=$((elapsed * 50 / duration))
        local bar=""
        local i=0
        while [ $i -lt $progress ]; do
            bar="${bar}█"
            i=$((i + 1))
        done
        while [ $i -lt 50 ]; do
            bar="${bar}░"
            i=$((i + 1))
        done

        # Clear screen and display dashboard
        clear
        banner

        echo -e "${WHITE}Test Configuration:${RESET}"
        echo -e "  Target URL:    ${CYAN}${TARGET_URL}${PROXY_ENDPOINT}${RESET}"
        echo -e "  Model:         ${CYAN}${MODEL_NAME}${RESET}"
        echo -e "  Concurrency:   ${CYAN}${CONCURRENCY}${RESET} workers"
        echo -e "  Rate Limit:    ${CYAN}${RATE_LIMIT_DELAY}s${RESET} delay"
        echo

        echo -e "${WHITE}Progress:${RESET}"
        echo -e "  ${GREY}[${RESET}${GREEN}${bar}${RESET}${GREY}]${RESET} ${YELLOW}${elapsed}s${RESET}/${YELLOW}${duration}s${RESET} (${YELLOW}${remaining}s${RESET} remaining)"
        echo

        echo -e "${WHITE}Real-time Stats:${RESET}"
        echo -e "  Total Requests:   ${CYAN}${total}${RESET}"
        echo -e "  Successful:       ${GREEN}${success}${RESET} (${GREEN}${success_rate}%${RESET})"
        echo -e "  Failed:           ${RED}${failed}${RESET}"
        echo -e "  Requests/sec:     ${YELLOW}${requests_per_sec}${RESET}"
        echo

        echo -e "${WHITE}Response Times:${RESET}"
        if [ "$success" -gt 0 ]; then
            echo -e "  Average:          ${CYAN}${avg_response_time}ms${RESET}"
            echo -e "  Minimum:          ${GREEN}${min_time}ms${RESET}"
            echo -e "  Maximum:          ${YELLOW}${max_time}ms${RESET}"
        else
            echo -e "  ${GREY}No successful responses yet...${RESET}"
        fi

        sleep 1
    done
}

show_final_summary() {
    local actual_duration=$(($(date +%s) - START_TIME))

    # Read final stats
    local total=$(cat "$STATS_DIR/total" 2>/dev/null || echo "0")
    local success=$(cat "$STATS_DIR/success" 2>/dev/null || echo "0")
    local failed=$(cat "$STATS_DIR/failed" 2>/dev/null || echo "0")
    local total_time=$(cat "$STATS_DIR/total_time" 2>/dev/null || echo "0")
    local min_time=$(cat "$STATS_DIR/min_time" 2>/dev/null || echo "0")
    local max_time=$(cat "$STATS_DIR/max_time" 2>/dev/null || echo "0")

    # Calculate final metrics
    local success_rate=0
    local failure_rate=0
    local avg_response_time=0
    local requests_per_sec=0
    local throughput=0

    if [ "$total" -gt 0 ]; then
        success_rate=$((success * 100 / total))
        failure_rate=$((failed * 100 / total))
        if [ "$actual_duration" -gt 0 ]; then
            requests_per_sec=$((total / actual_duration))
            throughput=$((success / actual_duration))
        fi

        if [ "$success" -gt 0 ]; then
            avg_response_time=$((total_time / success))
        fi
    fi

    clear
    banner

    echo -e "${WHITE}${BOLD}Load Test Complete!${RESET}"
    echo

    echo -e "${PURPLE}╔═══════════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${PURPLE}║${RESET}                        ${WHITE}FINAL SUMMARY${RESET}                         ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╠═══════════════════════════════════════════════════════════════╣${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Test Duration:${RESET}     ${CYAN}${actual_duration}s${RESET} (planned: ${YELLOW}${DURATION}s${RESET})                 ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Concurrency:${RESET}       ${CYAN}${CONCURRENCY}${RESET} workers                           ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Target:${RESET}            ${CYAN}${MODEL_NAME}${RESET} @ ${CYAN}${TARGET_URL}${RESET}    ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╠═══════════════════════════════════════════════════════════════╣${RESET}"
    echo -e "${PURPLE}║${RESET}                       ${WHITE}REQUEST STATS${RESET}                        ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╠═══════════════════════════════════════════════════════════════╣${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Total Requests:${RESET}    ${CYAN}${total}${RESET}                                 ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Successful:${RESET}        ${GREEN}${success}${RESET} (${GREEN}${success_rate}%${RESET})                       ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Failed:${RESET}            ${RED}${failed}${RESET} (${RED}${failure_rate}%${RESET})                         ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Requests/sec:${RESET}      ${YELLOW}${requests_per_sec}${RESET}                               ${PURPLE}║${RESET}"
    echo -e "${PURPLE}║${RESET} ${WHITE}Success/sec:${RESET}       ${GREEN}${throughput}${RESET}                                 ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╠═══════════════════════════════════════════════════════════════╣${RESET}"
    echo -e "${PURPLE}║${RESET}                     ${WHITE}RESPONSE TIMES${RESET}                        ${PURPLE}║${RESET}"
    echo -e "${PURPLE}╠═══════════════════════════════════════════════════════════════╣${RESET}"

    if [ "$success" -gt 0 ]; then
        echo -e "${PURPLE}║${RESET} ${WHITE}Average:${RESET}           ${CYAN}${avg_response_time}ms${RESET}                             ${PURPLE}║${RESET}"
        echo -e "${PURPLE}║${RESET} ${WHITE}Minimum:${RESET}           ${GREEN}${min_time}ms${RESET}                               ${PURPLE}║${RESET}"
        echo -e "${PURPLE}║${RESET} ${WHITE}Maximum:${RESET}           ${YELLOW}${max_time}ms${RESET}                              ${PURPLE}║${RESET}"
    else
        echo -e "${PURPLE}║${RESET} ${RED}No successful responses recorded${RESET}                     ${PURPLE}║${RESET}"
    fi

    echo -e "${PURPLE}╚═══════════════════════════════════════════════════════════════╝${RESET}"

    # Show errors if any
    if [ "$failed" -gt 0 ] && [ -f "$STATS_DIR/errors" ] && [ -s "$STATS_DIR/errors" ]; then
        echo
        echo -e "${YELLOW}Recent Errors:${RESET}"
        tail -10 "$STATS_DIR/errors" | while read -r line; do
            echo -e "  ${RED}•${RESET} ${GREY}$line${RESET}"
        done
    fi

    echo

    # Performance assessment
    if [ "$success" -gt 0 ]; then
        if [ "$success_rate" -ge 95 ]; then
            echo -e "${GREEN}✓ Excellent performance! Success rate above 95%${RESET}"
        elif [ "$success_rate" -ge 80 ]; then
            echo -e "${YELLOW}⚠ Good performance, but some failures detected${RESET}"
        else
            echo -e "${RED}⚠ Performance issues detected - high failure rate${RESET}"
        fi
    else
        echo -e "${RED}✗ All requests failed - check Olla proxy configuration${RESET}"
    fi

    echo
}

# Main execution
main() {
    # Setup signal handlers
    trap cleanup EXIT INT TERM

    banner

    # Validate arguments
    validate_args "$@"

    # Load configuration
    load_env

    # Check dependencies
    if ! command -v curl >/dev/null 2>&1; then
        echo -e "${RED}ERROR:${RESET} curl is required"
        exit 1
    fi

    # Setup
    setup_temp_files
    START_TIME=$(date +%s)

    echo -e "${GREEN}Starting load test...${RESET}"
    echo -e "Duration: ${YELLOW}${DURATION}s${RESET}, Concurrency: ${YELLOW}${CONCURRENCY}${RESET}, Target: ${CYAN}${TARGET_URL}${RESET}"
    echo -e "Model: ${CYAN}${MODEL_NAME}${RESET}, Rate limit: ${CYAN}${RATE_LIMIT_DELAY}s${RESET}"
    echo
    sleep 2

    # Start worker processes
    local i=1
    while [ $i -le $CONCURRENCY ]; do
        worker "$i" &
        echo $! >> "$STATS_DIR/pids"
        i=$((i + 1))
    done

    # Display progress
    display_progress "$DURATION"

    # Wait for all workers to finish
    while read -r pid; do
        if [ -n "$pid" ]; then
            wait "$pid" 2>/dev/null || true
        fi
    done < "$STATS_DIR/pids"

    # Show final summary
    show_final_summary
}

# Run the main function with all arguments
main "$@"