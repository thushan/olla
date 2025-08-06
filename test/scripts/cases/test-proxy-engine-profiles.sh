#!/bin/bash
# test-proxy-engine-profiles.sh
# Tests all combinations of proxy engines (olla/sherpa) and profiles (auto/standard/streaming)
# This script automates the testing of streaming behavior across different configurations

set -uo pipefail
# Temporarily disable -e to debug

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions and variables
source "$SCRIPT_DIR/_common.sh"
source "$SCRIPT_DIR/_olla.sh"
source "$SCRIPT_DIR/_streaming_tests.sh"

# Configuration
BASE_CONFIG=""
TEST_RESULTS_DIR=""

# Test configuration matrix
PROXY_ENGINES=("sherpa" "olla")
PROXY_PROFILES=("auto" "standard" "streaming")

# Function to parse command line arguments
parse_args() {
    while getopts "c:h" opt; do
        case ${opt} in
            c )
                BASE_CONFIG="$OPTARG"
                ;;
            h )
                echo "Usage: $0 -c <config_file>"
                echo "  -c: Base configuration file for Olla (relative to project root or absolute path)"
                echo "  -h: Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0 -c config.yaml"
                echo "  $0 -c config.local.yaml"
                echo "  $0 -c config/config.local.yaml"
                echo "  $0 -c ../../../config/config.local.yaml"
                echo "  $0 -c /absolute/path/to/config.yaml"
                exit 0
                ;;
            \? )
                print_color "$RED" "Invalid option: -$OPTARG"
                exit 1
                ;;
        esac
    done
    
    if [[ -z "$BASE_CONFIG" ]]; then
        print_color "$RED" "ERROR: Configuration file required!"
        echo "Usage: $0 -c <config_file>"
        exit 1
    fi
    
    # Handle config path resolution
    if [[ ! "$BASE_CONFIG" = /* ]]; then
        # Not an absolute path, try to resolve it
        # First check if it exists relative to current directory
        if [[ -f "$BASE_CONFIG" ]]; then
            BASE_CONFIG="$(cd "$(dirname "$BASE_CONFIG")" && pwd)/$(basename "$BASE_CONFIG")"
        # Then check relative to project root
        elif [[ -f "$PROJECT_ROOT/$BASE_CONFIG" ]]; then
            BASE_CONFIG="$PROJECT_ROOT/$BASE_CONFIG"
        # Check in config directory
        elif [[ -f "$PROJECT_ROOT/config/$BASE_CONFIG" ]]; then
            BASE_CONFIG="$PROJECT_ROOT/config/$BASE_CONFIG"
        else
            print_color "$RED" "ERROR: Configuration file not found: $BASE_CONFIG"
            print_color "$YELLOW" "Searched in:"
            print_color "$GREY" "  - Current directory: $(pwd)"
            print_color "$GREY" "  - Project root: $PROJECT_ROOT"
            print_color "$GREY" "  - Config directory: $PROJECT_ROOT/config"
            exit 1
        fi
    fi
    
    if [[ ! -f "$BASE_CONFIG" ]]; then
        print_color "$RED" "ERROR: Configuration file not found: $BASE_CONFIG"
        exit 1
    fi
    
    print_color "$GREEN" "Using configuration: $BASE_CONFIG"
}


# Function to create test configuration for engine and profile
create_engine_profile_config() {
    local engine=$1
    local profile=$2
    
    print_color "$CYAN" "Creating config: engine=$engine, profile=$profile"
    
    # Use the create_test_config function from _olla.sh with yq modifications
    if command_exists yq; then
        create_test_config "$BASE_CONFIG" ".proxy.engine = \"$engine\" | .proxy.profile = \"$profile\""
    else
        # Fallback to sed
        cp "$BASE_CONFIG" "$PROJECT_ROOT/$TEST_CONFIG"
        
        # First update the engine
        sed -i.bak "s/^[[:space:]]*engine:.*/  engine: $engine/" "$PROJECT_ROOT/$TEST_CONFIG"
        
        # Then update the profile
        sed -i.bak "s/^[[:space:]]*profile:.*/  profile: $profile/" "$PROJECT_ROOT/$TEST_CONFIG"
        
        rm -f "$PROJECT_ROOT/$TEST_CONFIG.bak"
        
        # Verify the changes
        if grep -q "engine: $engine" "$PROJECT_ROOT/$TEST_CONFIG" && grep -q "profile: $profile" "$PROJECT_ROOT/$TEST_CONFIG"; then
            print_color "$GREEN" "✓ Test configuration created successfully"
        else
            print_color "$RED" "ERROR: Failed to update configuration"
            return 1
        fi
    fi
}

# Function to start Olla with custom log file
start_olla_with_log() {
    local engine=$1
    local profile=$2
    local log_file="$TEST_RESULTS_DIR/olla-$engine-$profile.log"
    
    # Call the start_olla function from _olla.sh
    start_olla "$TEST_CONFIG" "$log_file"
}




# Function to run a single test configuration
run_test_case() {
    local engine=$1
    local profile=$2
    local test_start=$(date +%s)
    
    print_section "Testing: $engine engine with $profile profile"
    
    # Create test configuration
    if ! create_engine_profile_config "$engine" "$profile"; then
        print_color "$RED" "Failed to create configuration for $engine/$profile"
        return 1
    fi
    
    # Start Olla
    print_color "$YELLOW" "Starting Olla with $engine/$profile configuration..."
    if ! start_olla_with_log "$engine" "$profile"; then
        print_color "$RED" "Failed to start Olla for $engine/$profile"
        return 1
    fi
    
    # Check for phi model
    print_color "$YELLOW" "Checking for phi models..."
    if ! check_phi_models; then
        print_color "$RED" "No phi models available, skipping tests"
        stop_olla
        return 1
    fi
    
    # Run streaming tests
    print_color "$YELLOW" "Running streaming test suite..."
    local test_dir="$TEST_RESULTS_DIR/$engine-$profile"
    if run_all_streaming_tests "$test_dir"; then
        print_color "$GREEN" "All streaming tests passed"
    else
        print_color "$YELLOW" "Some streaming tests failed (check logs)"
    fi
    
    # Stop Olla
    print_color "$YELLOW" "Stopping Olla..."
    stop_olla
    
    # Calculate test duration
    local test_end=$(date +%s)
    local duration=$((test_end - test_start))
    
    print_color "$GREEN" "✓ Completed $engine/$profile in ${duration}s"
    print_color "$GREY" "  Logs: $TEST_RESULTS_DIR/olla-$engine-$profile.log"
    print_color "$GREY" "  Test results: $test_dir/"
    
    # Small delay to ensure clean shutdown
    sleep 2
}

# Function to generate summary report
generate_summary() {
    local report_file="$TEST_RESULTS_DIR/summary.txt"
    
    print_header "Test Summary"
    
    {
        echo "Proxy Engine Profile Test Results"
        echo "================================="
        echo ""
        echo "Test Run: $(date)"
        echo "Base Config: $BASE_CONFIG"
        echo ""
        
        for engine in "${PROXY_ENGINES[@]}"; do
            for profile in "${PROXY_PROFILES[@]}"; do
                echo -n "$engine/$profile: "
                
                # Check if all test files exist and have content
                local test_dir="$TEST_RESULTS_DIR/$engine-$profile"
                if [[ -f "$test_dir/detection.log" ]] && [[ -f "$test_dir/latency.log" ]] && [[ -f "$test_dir/responses.log" ]]; then
                    # Use check_streaming_test_passed from _streaming_tests.sh
                    local all_passed=true
                    for log in detection.log latency.log responses.log; do
                        if ! check_streaming_test_passed "$test_dir/$log"; then
                            all_passed=false
                            break
                        fi
                    done
                    
                    if $all_passed; then
                        echo "PASSED"
                    else
                        echo "FAILED"
                    fi
                else
                    echo "INCOMPLETE"
                fi
            done
        done
        
        echo ""
        echo "Detailed logs available in: $TEST_RESULTS_DIR"
        
        # Include streaming test analysis for each configuration
        echo ""
        echo "Detailed Analysis:"
        echo "=================="
        for engine in "${PROXY_ENGINES[@]}"; do
            for profile in "${PROXY_PROFILES[@]}"; do
                echo ""
                echo "$engine/$profile:"
                analyze_streaming_results "$TEST_RESULTS_DIR/$engine-$profile" | sed 's/^/  /'
            done
        done
    } | tee "$report_file"
    
    print_color "$CYAN" "\nSummary saved to: $report_file"
}

# Cleanup function
cleanup() {
    print_color "$YELLOW" "\nCleaning up..."
    cleanup_olla
    deactivate_venv
}

# Set trap for cleanup
trap 'echo -e "\n${YELLOW}Interrupted! Cleaning up...${RESET}"; cleanup; exit 130' INT TERM
trap cleanup EXIT

# Main execution
main() {
    print_header "Olla Proxy Engine & Profile Test Suite"
    
    # Check virtual environment
    if ! check_venv; then
        exit 1
    fi
    
    # Parse arguments
    parse_args "$@"
    
    # Create results directory
    TEST_RESULTS_DIR=$(create_results_dir "test-results")
    print_color "$CYAN" "Test results will be saved to: $TEST_RESULTS_DIR"
    
    # Build Olla
    if ! build_olla; then
        exit 1
    fi
    
    # Show test matrix
    print_section "Test Matrix"
    print_color "$CYAN" "Will test the following combinations:"
    for engine in "${PROXY_ENGINES[@]}"; do
        for profile in "${PROXY_PROFILES[@]}"; do
            echo "  - $engine/$profile"
        done
    done
    echo ""
    echo "Starting tests..."
    
    # Track progress
    local total_tests=$((${#PROXY_ENGINES[@]} * ${#PROXY_PROFILES[@]}))
    local current_test=0
    
    # Run all test combinations
    for engine in "${PROXY_ENGINES[@]}"; do
        for profile in "${PROXY_PROFILES[@]}"; do
            current_test=$((current_test + 1))
            print_color "$PURPLE" "\n[$current_test/$total_tests] Testing: $engine/$profile"
            print_color "$PURPLE" "=========================================="
            
            if run_test_case "$engine" "$profile"; then
                print_color "$GREEN" "✓ $engine/$profile completed successfully"
            else
                print_color "$RED" "✗ $engine/$profile failed"
            fi
        done
    done
    
    # Generate summary report
    generate_summary
    
    print_color "$GREEN" "\n✅ All tests completed!"
    print_color "$CYAN" "Full results available in: $TEST_RESULTS_DIR"
}

# Run main function
main "$@"