#!/bin/bash
# Simple test to verify the test matrix works

# Test matrix
PROXY_ENGINES=("sherpa" "olla")
PROXY_PROFILES=("auto" "standard" "streaming")

echo "Starting test matrix..."

total=$((${#PROXY_ENGINES[@]} * ${#PROXY_PROFILES[@]}))
current=0

for engine in "${PROXY_ENGINES[@]}"; do
    for profile in "${PROXY_PROFILES[@]}"; do
        current=$((current + 1))
        echo "[$current/$total] Testing: $engine/$profile"
        sleep 1  # Simulate work
    done
done

echo "All tests completed!"