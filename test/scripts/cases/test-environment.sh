#!/bin/bash
# Test script to verify environment setup

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions
source "$SCRIPT_DIR/_common.sh"

print_header "Environment Test"

# Show paths
print_section "Path Information"
echo "Script Directory: $SCRIPT_DIR"
echo "Project Root: $PROJECT_ROOT"
echo "Current Directory: $(pwd)"

# Test virtual environment
print_section "Virtual Environment Test"
if check_venv; then
    echo "Python: $(which python)"
    echo "Python Version: $(python --version)"
else
    echo "Virtual environment check failed"
fi

# Test Go
print_section "Go Environment"
if command_exists go; then
    echo "Go: $(which go)"
    echo "Go Version: $(go version)"
else
    echo "Go not found"
fi

# Test config resolution
print_section "Config Resolution Test"
TEST_CONFIGS=("config.yaml" "config.local.yaml" "config/config.local.yaml")

for config in "${TEST_CONFIGS[@]}"; do
    echo -n "Testing '$config': "
    if [[ -f "$PROJECT_ROOT/$config" ]] || [[ -f "$PROJECT_ROOT/config/$config" ]]; then
        print_color "$GREEN" "Found"
    else
        print_color "$YELLOW" "Not found"
    fi
done

# Cleanup
deactivate_venv

print_color "$GREEN" "\n✅ Environment test completed!"