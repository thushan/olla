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
TEST_PORT=""

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
    
    print_color "$CYAN" "Creating config: engine=$engine, profile=$profile, host=localhost, port=$TEST_PORT"
    
    # Use the create_test_config function from _olla.sh with yq modifications
    if command_exists yq; then
        create_test_config "$BASE_CONFIG" ".proxy.engine = \"$engine\" | .proxy.profile = \"$profile\" | .server.host = \"localhost\" | .server.port = $TEST_PORT"
    else
        # Use awk to modify YAML
        print_color "$YELLOW" "Using awk to modify YAML configuration"
        cp "$BASE_CONFIG" "$PROJECT_ROOT/$TEST_CONFIG"
        
        # Create a temporary file with all modifications
        awk -v engine="$engine" -v profile="$profile" -v port="$TEST_PORT" '
        BEGIN { in_server = 0; in_proxy = 0 }
        
        # Handle server section
        /^server:/ { 
            in_server = 1
            in_proxy = 0
            print
            next
        }
        
        # Handle proxy section
        /^proxy:/ { 
            in_proxy = 1
            in_server = 0
            print
            next
        }
        
        # Reset section flags when we hit a new top-level key
        /^[a-zA-Z]/ && !/^[[:space:]]/ {
            in_server = 0
            in_proxy = 0
        }
        
        # Update values in server section
        in_server && /^[[:space:]]+host:/ {
            sub(/host:.*/, "host: \"localhost\"")
        }
        in_server && /^[[:space:]]+port:/ {
            sub(/port:.*/, "port: " port)
        }
        
        # Update values in proxy section
        in_proxy && /^[[:space:]]+engine:/ {
            sub(/engine:.*/, "engine: \"" engine "\"")
        }
        in_proxy && /^[[:space:]]+profile:/ {
            sub(/profile:.*/, "profile: \"" profile "\"")
        }
        
        { print }
        ' "$PROJECT_ROOT/$TEST_CONFIG" > "$PROJECT_ROOT/$TEST_CONFIG.tmp" && \
        mv "$PROJECT_ROOT/$TEST_CONFIG.tmp" "$PROJECT_ROOT/$TEST_CONFIG"
        
        # Verify the changes
        local actual_port=$(grep -A10 "^server:" "$PROJECT_ROOT/$TEST_CONFIG" | grep "port:" | head -1 | awk '{print $2}')
        local actual_engine=$(grep -A10 "^proxy:" "$PROJECT_ROOT/$TEST_CONFIG" | grep "engine:" | head -1 | sed 's/.*engine:[[:space:]]*"\?\([^"]*\)"\?.*/\1/')
        local actual_profile=$(grep -A10 "^proxy:" "$PROJECT_ROOT/$TEST_CONFIG" | grep "profile:" | head -1 | sed 's/.*profile:[[:space:]]*"\?\([^"]*\)"\?.*/\1/')
        
        print_color "$CYAN" "Configuration changes:"
        print_color "$CYAN" "  Port: $actual_port (expected: $TEST_PORT)"
        print_color "$CYAN" "  Engine: $actual_engine (expected: $engine)"
        print_color "$CYAN" "  Profile: $actual_profile (expected: $profile)"
        
        if [[ "$actual_port" == "$TEST_PORT" ]]; then
            print_color "$GREEN" "✓ Configuration updated successfully"
        else
            print_color "$RED" "ERROR: Failed to update port correctly"
        fi
        
        # Show what we created
        print_color "$CYAN" "Test configuration server section:"
        grep -A5 "^server:" "$PROJECT_ROOT/$TEST_CONFIG" 2>/dev/null | sed 's/^/  /'
        
        print_color "$CYAN" "Test configuration proxy section:"
        grep -A10 "^proxy:" "$PROJECT_ROOT/$TEST_CONFIG" 2>/dev/null | grep -E "^proxy:|engine:|profile:" | sed 's/^/  /'
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
    
    # Create test case directory structure
    local test_case_dir="$TEST_RESULTS_DIR/$engine-$profile"
    mkdir -p "$test_case_dir/config"
    mkdir -p "$test_case_dir/logs"
    
    # Create test configuration
    if ! create_engine_profile_config "$engine" "$profile"; then
        print_color "$RED" "Failed to create configuration for $engine/$profile"
        return 1
    fi
    
    # Copy the test configuration to the test case folder
    cp "$PROJECT_ROOT/$TEST_CONFIG" "$test_case_dir/config/config.yaml"
    print_color "$CYAN" "Saved test config to: $test_case_dir/config/config.yaml"
    
    # Start Olla with the log in the test case directory
    print_color "$YELLOW" "Starting Olla with $engine/$profile configuration..."
    local log_file="$test_case_dir/logs/olla.log"
    if ! start_olla "$TEST_CONFIG" "$log_file"; then
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
    if run_all_streaming_tests "$test_case_dir/logs"; then
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
    
    # Create a summary file for this test case
    {
        echo "Test Case: $engine/$profile"
        echo "Duration: ${duration}s"
        echo "Binary: ../bin/$(ls "$TEST_RESULTS_DIR/bin" | head -1)"
        echo "Config: config.yaml"
        echo "Port: $TEST_PORT"
        echo "Date: $(date)"
    } > "$test_case_dir/summary.txt"
    
    print_color "$GREEN" "✓ Completed $engine/$profile in ${duration}s"
    print_color "$GREY" "  Test case directory: $test_case_dir/"
    
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
        echo "Runtime: ${minutes}m ${seconds}s"
        echo ""
        
        for engine in "${PROXY_ENGINES[@]}"; do
            for profile in "${PROXY_PROFILES[@]}"; do
                echo -n "$engine/$profile: "
                
                # Check if all test files exist and have content
                local test_dir="$TEST_RESULTS_DIR/$engine-$profile"
                if [[ -f "$test_dir/logs/detection.log" ]] && [[ -f "$test_dir/logs/latency.log" ]] && [[ -f "$test_dir/logs/responses.log" ]]; then
                    # Use check_streaming_test_passed from _streaming_tests.sh
                    local all_passed=true
                    for log in detection.log latency.log responses.log; do
                        if ! check_streaming_test_passed "$test_dir/logs/$log"; then
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
    
    # Record start time
    local start_time=$(date +%s)
    
    # Check virtual environment
    if ! check_venv; then
        exit 1
    fi
    
    # Parse arguments
    parse_args "$@"
    
    # Create results directory
    TEST_RESULTS_DIR=$(create_results_dir "test-results")
    print_color "$CYAN" "Test results will be saved to: $TEST_RESULTS_DIR"
    
    # Generate random port for this test run
    TEST_PORT=$(get_random_port 40114 40144)
    print_color "$CYAN" "Using test port: $TEST_PORT"
    
    # Build Olla
    if ! build_olla; then
        exit 1
    fi
    
    # Copy binary to test results directory
    mkdir -p "$TEST_RESULTS_DIR/bin"
    local binary_name=$(get_git_version)
    cp "$OLLA_BINARY" "$TEST_RESULTS_DIR/bin/$binary_name"
    chmod +x "$TEST_RESULTS_DIR/bin/$binary_name"
    print_color "$CYAN" "Test binary: $TEST_RESULTS_DIR/bin/$binary_name"
    
    # Create test run info file
    {
        echo "Test Run Information"
        echo "==================="
        echo "Date: $(date)"
        echo "Binary: $binary_name"
        echo "Base Config: $BASE_CONFIG"
        echo "Test Port: $TEST_PORT"
        echo "Host: localhost"
        echo "Git Commit: $(cd "$PROJECT_ROOT" && git rev-parse HEAD 2>/dev/null || echo "unknown")"
        echo "Git Status: $(cd "$PROJECT_ROOT" && git status --porcelain 2>/dev/null | wc -l) uncommitted changes"
        echo ""
        echo "Test Matrix:"
        for engine in "${PROXY_ENGINES[@]}"; do
            for profile in "${PROXY_PROFILES[@]}"; do
                echo "  - $engine/$profile"
            done
        done
    } > "$TEST_RESULTS_DIR/test-run-info.txt"
    
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
    
    # Calculate total runtime
    local end_time=$(date +%s)
    local total_time=$((end_time - start_time))
    local minutes=$((total_time / 60))
    local seconds=$((total_time % 60))
    
    # Generate summary report
    generate_summary
    
    # Append runtime to test-run-info
    {
        echo ""
        echo "Test Runtime"
        echo "============"
        echo "Start: $(date -d @$start_time 2>/dev/null || date -r $start_time 2>/dev/null || echo "timestamp: $start_time")"
        echo "End: $(date -d @$end_time 2>/dev/null || date -r $end_time 2>/dev/null || echo "timestamp: $end_time")"
        echo "Total: ${minutes}m ${seconds}s"
    } >> "$TEST_RESULTS_DIR/test-run-info.txt"
    
    print_color "$GREEN" "\n✅ All tests completed!"
    print_color "$CYAN" "Total runtime: ${minutes}m ${seconds}s"
    print_color "$CYAN" "Full results available in: $TEST_RESULTS_DIR"
}

# Run main function
main "$@"