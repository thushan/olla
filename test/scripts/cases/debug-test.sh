#!/bin/bash
# Debug version of the test script

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions
source "$SCRIPT_DIR/_common.sh"
source "$SCRIPT_DIR/_olla.sh"

echo "Script directory: $SCRIPT_DIR"
echo "Project root: $PROJECT_ROOT"
echo ""

# Test matrix
PROXY_ENGINES=("sherpa" "olla")
PROXY_PROFILES=("auto" "standard" "streaming")

echo "Test matrix:"
for engine in "${PROXY_ENGINES[@]}"; do
    for profile in "${PROXY_PROFILES[@]}"; do
        echo "  - $engine/$profile"
    done
done

echo ""
echo "Testing loop:"
count=0
for engine in "${PROXY_ENGINES[@]}"; do
    echo "Engine: $engine"
    for profile in "${PROXY_PROFILES[@]}"; do
        ((count++))
        echo "  Profile: $profile (test $count)"
    done
done

echo ""
echo "Total tests: $count"