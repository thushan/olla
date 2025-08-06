#!/bin/bash
# _olla.sh
# Olla-specific helper functions for test scripts

# Global variable for Olla PID
OLLA_PID=""

# Function to build Olla
build_olla() {
    print_color "$YELLOW" "Building Olla from source..."
    
    # Save current directory
    local current_dir=$(pwd)
    
    # Change to project root
    cd "$PROJECT_ROOT"
    
    # Clean previous build
    if [[ -f "$OLLA_BINARY" ]]; then
        rm -f "$OLLA_BINARY"
    fi
    
    # Build with go
    if command_exists go; then
        print_color "$CYAN" "Building with Go from: $(pwd)"
        
        # Ensure we're in the right directory and use proper go build command
        if [[ -f "main.go" ]]; then
            go build -o olla .
        elif [[ -f "cmd/olla/main.go" ]]; then
            go build -o olla ./cmd/olla
        else
            print_color "$RED" "ERROR: Cannot find main.go or cmd/olla/main.go from $(pwd)"
            cd "$current_dir"
            return 1
        fi
        
        if [[ -f "$OLLA_BINARY" ]]; then
            print_color "$GREEN" "✓ Olla built successfully"
            cd "$current_dir"
            return 0
        else
            print_color "$RED" "ERROR: Failed to build Olla"
            cd "$current_dir"
            return 1
        fi
    else
        print_color "$RED" "ERROR: Go is not installed"
        print_color "$YELLOW" "Please install Go from https://golang.org/dl/"
        cd "$current_dir"
        return 1
    fi
}

# Function to create test configuration from base config
create_test_config() {
    local base_config=$1
    local modifications=$2  # String of yq modifications or sed commands
    
    if [[ ! -f "$base_config" ]]; then
        print_color "$RED" "ERROR: Base configuration not found: $base_config"
        return 1
    fi
    
    print_color "$CYAN" "Creating test configuration..."
    
    # Use yq if available for reliable YAML manipulation
    if command_exists yq; then
        eval "yq eval '$modifications' \"$base_config\"" > "$PROJECT_ROOT/$TEST_CONFIG"
    else
        # Fallback to sed (less reliable but works for simple cases)
        cp "$base_config" "$PROJECT_ROOT/$TEST_CONFIG"
        
        # Apply modifications using sed
        # This is a simplified approach - real implementation would need better parsing
        print_color "$YELLOW" "Warning: Using sed for YAML manipulation (install yq for better reliability)"
        eval "$modifications"
    fi
    
    if [[ -f "$PROJECT_ROOT/$TEST_CONFIG" ]]; then
        print_color "$GREEN" "✓ Test configuration created"
        return 0
    else
        print_color "$RED" "ERROR: Failed to create test configuration"
        return 1
    fi
}

# Function to start Olla with configuration
start_olla() {
    local config_file="${1:-$TEST_CONFIG}"
    local log_file="${2:-olla.log}"
    
    print_color "$YELLOW" "Starting Olla..."
    
    # Save current directory
    local current_dir=$(pwd)
    cd "$PROJECT_ROOT"
    
    # Check if port is available
    if ! is_port_available 40114; then
        print_color "$RED" "ERROR: Port 40114 is already in use"
        cd "$current_dir"
        return 1
    fi
    
    # Ensure binary exists
    if [[ ! -f "$OLLA_BINARY" ]]; then
        print_color "$RED" "ERROR: Olla binary not found at $OLLA_BINARY"
        cd "$current_dir"
        return 1
    fi
    
    # Start Olla in background
    print_color "$CYAN" "Starting: OLLA_CONFIG=\"$config_file\" ./olla server"
    OLLA_CONFIG="$config_file" ./olla server > "$log_file" 2>&1 &
    OLLA_PID=$!
    
    # Wait for Olla to start
    if wait_for_condition "check_olla_health" 30 "Waiting for Olla to start"; then
        print_color "$GREEN" "✓ Olla started successfully (PID: $OLLA_PID)"
        cd "$current_dir"
        return 0
    else
        print_color "$RED" "ERROR: Olla failed to start within 30 seconds"
        
        # Check if process is still running
        if ! kill -0 $OLLA_PID 2>/dev/null; then
            print_color "$RED" "Olla process died. Last 20 lines of log:"
            tail -20 "$log_file"
        fi
        cd "$current_dir"
        return 1
    fi
}

# Function to check Olla health
check_olla_health() {
    curl -s http://localhost:40114/internal/health > /dev/null 2>&1
}

# Function to stop Olla
stop_olla() {
    if [[ -n "$OLLA_PID" ]] && kill -0 $OLLA_PID 2>/dev/null; then
        print_color "$YELLOW" "Stopping Olla (PID: $OLLA_PID)..."
        
        # Try graceful shutdown first
        kill -TERM $OLLA_PID
        
        # Wait for process to stop
        local count=0
        while kill -0 $OLLA_PID 2>/dev/null && [[ $count -lt 10 ]]; do
            sleep 1
            ((count++))
        done
        
        # Force kill if necessary
        if kill -0 $OLLA_PID 2>/dev/null; then
            print_color "$YELLOW" "Force stopping Olla..."
            kill -9 $OLLA_PID
        fi
        
        print_color "$GREEN" "✓ Olla stopped"
    fi
    OLLA_PID=""
}

# Function to get Olla status
get_olla_status() {
    local response=$(curl -s http://localhost:40114/internal/status 2>/dev/null)
    if [[ $? -eq 0 ]]; then
        echo "$response"
        return 0
    else
        return 1
    fi
}

# Function to get available models
get_available_models() {
    curl -s http://localhost:40114/olla/models 2>/dev/null | jq -r '.data[].id // .models[].name // .models[].id' 2>/dev/null || true
}

# Function to check for specific model
check_model_exists() {
    local model=$1
    get_available_models | grep -q "^${model}$"
}

# Function to check for phi models
check_phi_models() {
    print_color "$YELLOW" "Checking for phi models..."
    
    local phi_models=("phi4:latest" "phi3.5:latest" "phi3:latest")
    local found_model=""
    
    for model in "${phi_models[@]}"; do
        if check_model_exists "$model"; then
            found_model=$model
            break
        fi
    done
    
    if [[ -n "$found_model" ]]; then
        print_color "$GREEN" "✓ Found phi model: $found_model"
        return 0
    else
        print_color "$RED" "ERROR: No phi models found!"
        print_color "$YELLOW" "Please pull a phi model first:"
        print_color "$GREY" "  ollama pull phi4:latest"
        return 1
    fi
}

# Cleanup function for Olla
cleanup_olla() {
    stop_olla
    
    # Remove test config
    if [[ -f "$PROJECT_ROOT/$TEST_CONFIG" ]]; then
        rm -f "$PROJECT_ROOT/$TEST_CONFIG"
    fi
}