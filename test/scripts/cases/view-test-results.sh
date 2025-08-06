#!/bin/bash
# view-test-results.sh
# View results from the latest test run

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions
source "$SCRIPT_DIR/_common.sh"

# Find the latest test results directory
find_latest_results() {
    local latest=$(ls -dt "$PROJECT_ROOT"/test-results-* 2>/dev/null | head -1)
    if [[ -n "$latest" ]]; then
        echo "$latest"
    else
        return 1
    fi
}

# Main execution
main() {
    print_header "Test Results Viewer"
    
    # Find latest results
    local results_dir=$(find_latest_results)
    if [[ -z "$results_dir" ]]; then
        print_color "$RED" "No test results found!"
        exit 1
    fi
    
    print_color "$CYAN" "Latest test results: $results_dir"
    echo ""
    
    # Show summary if exists
    if [[ -f "$results_dir/summary.txt" ]]; then
        print_section "Test Summary"
        cat "$results_dir/summary.txt"
        echo ""
    fi
    
    # Show all test combinations
    print_section "Test Logs Available"
    
    # Find all test directories
    for engine in sherpa olla; do
        for profile in auto standard streaming; do
            local test_dir="$results_dir/$engine-$profile"
            if [[ -d "$test_dir" ]]; then
                echo ""
                print_color "$WHITE" "$engine/$profile:"
                
                # Check Olla log
                local olla_log="$results_dir/olla-$engine-$profile.log"
                if [[ -f "$olla_log" ]]; then
                    local size=$(wc -l < "$olla_log" 2>/dev/null || echo "0")
                    print_color "$GREY" "  - Olla log: $olla_log ($size lines)"
                fi
                
                # Check test logs
                for log in detection latency responses; do
                    local log_file="$test_dir/$log.log"
                    if [[ -f "$log_file" ]]; then
                        local size=$(wc -l < "$log_file" 2>/dev/null || echo "0")
                        local status="Unknown"
                        
                        # Quick status check
                        if grep -q "FAIL\|ERROR\|failed" "$log_file"; then
                            status=$(print_color "$RED" "FAILED")
                        elif grep -q "completed\|passed\|success\|✓" "$log_file"; then
                            status=$(print_color "$GREEN" "PASSED")
                        fi
                        
                        print_color "$GREY" "  - $log test: $log_file ($size lines) - $status"
                    else
                        print_color "$YELLOW" "  - $log test: Not found"
                    fi
                done
            fi
        done
    done
    
    echo ""
    print_color "$CYAN" "To view a specific log:"
    print_color "$GREY" "  cat $results_dir/<engine>-<profile>/<test>.log"
    print_color "$GREY" "  Example: cat $results_dir/sherpa-auto/detection.log"
}

# Run main
main "$@"