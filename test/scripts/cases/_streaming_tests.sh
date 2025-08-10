#!/bin/bash
# _streaming_tests.sh
# Functions for running streaming test suites

# Function to run streaming detection test
run_streaming_detection_test() {
    local output_file=$1
    local args="${2:---quick}"
    
    print_color "$YELLOW" "  Running streaming detection test..."
    
    local current_dir=$(pwd)
    cd "$PROJECT_ROOT/test/scripts/streaming"
    
    local port="${TEST_PORT:-40114}"
    if python test-streaming-detection.py --url "http://localhost:$port" $args > "$output_file" 2>&1; then
        print_color "$GREEN" "  ✓ Streaming detection test passed"
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 0
    else
        print_color "$RED" "  ✗ Streaming detection test failed"
        print_color "$GREY" "  Last 10 lines of output:"
        tail -10 "$output_file" | sed 's/\x1b\[[0-9;]*m//g' | sed 's/^/    /'
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 1
    fi
}

# Function to run streaming latency test
run_streaming_latency_test() {
    local output_file=$1
    local args="${2:---count 3}"
    
    print_color "$YELLOW" "  Running streaming latency test..."
    
    local current_dir=$(pwd)
    cd "$PROJECT_ROOT/test/scripts/streaming"
    
    local port="${TEST_PORT:-40114}"
    if python test-streaming-latency.py --url "http://localhost:$port" $args > "$output_file" 2>&1; then
        print_color "$GREEN" "  ✓ Streaming latency test passed"
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 0
    else
        print_color "$RED" "  ✗ Streaming latency test failed"
        print_color "$GREY" "  Last 10 lines of output:"
        tail -10 "$output_file" | sed 's/\x1b\[[0-9;]*m//g' | sed 's/^/    /'
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 1
    fi
}

# Function to run streaming responses test
run_streaming_responses_test() {
    local output_file=$1
    local args="${2:---sample}"
    
    print_color "$YELLOW" "  Running streaming responses test..."
    
    local current_dir=$(pwd)
    cd "$PROJECT_ROOT/test/scripts/streaming"
    
    local port="${TEST_PORT:-40114}"
    if python test-streaming-responses.py --url "http://localhost:$port" $args > "$output_file" 2>&1; then
        print_color "$GREEN" "  ✓ Streaming responses test passed"
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 0
    else
        print_color "$RED" "  ✗ Streaming responses test failed"
        print_color "$GREY" "  Last 10 lines of output:"
        tail -10 "$output_file" | sed 's/\x1b\[[0-9;]*m//g' | sed 's/^/    /'
        # Strip ANSI codes from the log file
        strip_ansi_codes "$output_file"
        cd "$current_dir"
        return 1
    fi
}

# Function to run all streaming tests
run_all_streaming_tests() {
    local test_dir=$1
    local detection_args="${2:---quick}"
    local latency_args="${3:---count 3}"
    local responses_args="${4:---sample}"
    
    local all_passed=true
    
    mkdir -p "$test_dir"
    
    # Run detection test
    if ! run_streaming_detection_test "$test_dir/detection.log" "$detection_args"; then
        all_passed=false
    fi
    
    # Run latency test
    if ! run_streaming_latency_test "$test_dir/latency.log" "$latency_args"; then
        all_passed=false
    fi
    
    # Run responses test
    if ! run_streaming_responses_test "$test_dir/responses.log" "$responses_args"; then
        all_passed=false
    fi
    
    if $all_passed; then
        return 0
    else
        return 1
    fi
}

# Function to analyze streaming test results
analyze_streaming_results() {
    local test_dir=$1
    local summary_file="$test_dir/summary.txt"
    
    {
        echo "Streaming Test Results Analysis"
        echo "==============================="
        echo ""
        
        # Analyze detection test
        if [[ -f "$test_dir/detection.log" ]]; then
            echo "Detection Test:"
            grep -E "Mode:|Profile:|Auto-selected|STREAMING|STANDARD" "$test_dir/detection.log" | head -10 | sed 's/^/  /'
            echo ""
        fi
        
        # Analyze latency test
        if [[ -f "$test_dir/latency.log" ]]; then
            echo "Latency Test:"
            grep -E "Average time to first token:|Streaming quality:|tokens/sec" "$test_dir/latency.log" | head -10 | sed 's/^/  /'
            echo ""
        fi
        
        # Analyze responses test
        if [[ -f "$test_dir/responses.log" ]]; then
            echo "Responses Test:"
            grep -E "Testing .* models|PASS|FAIL|Streaming:|Buffered:" "$test_dir/responses.log" | head -10 | sed 's/^/  /'
            echo ""
        fi
        
        # Check for errors
        echo "Errors/Warnings:"
        grep -E "ERROR|FAIL|Warning" "$test_dir"/*.log 2>/dev/null | head -10 | sed 's/^/  /' || echo "  None found"
        
    } > "$summary_file"
    
    cat "$summary_file"
}

# Function to check if streaming test passed
check_streaming_test_passed() {
    local log_file=$1
    
    # Check for common failure indicators
    if grep -q -E "FAIL|ERROR|failed|Traceback|Exception" "$log_file"; then
        return 1
    fi
    
    # Check for success indicators
    if grep -q -E "completed|passed|success|✓" "$log_file"; then
        return 0
    fi
    
    # Default to passed if no clear indicators
    return 0
}