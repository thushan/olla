#!/bin/bash
# _olla.sh
# Olla-specific helper functions for test scripts

# Global variable for Olla PID
OLLA_PID=""

# Function to get git version info for binary naming
get_git_version() {
    local current_dir=$(pwd)
    cd "$PROJECT_ROOT"
    
    # Get short hash
    local git_hash=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    
    # Check if working directory is clean
    if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
        echo "olla-${git_hash}-wip"
    else
        echo "olla-${git_hash}"
    fi
    
    cd "$current_dir"
}

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
    
    # Ensure config directory exists
    mkdir -p "$PROJECT_ROOT/config"
    
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
        print_color "$GREEN" "✓ Test configuration created at: $PROJECT_ROOT/$TEST_CONFIG"
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
    local port="${TEST_PORT:-40114}"
    if ! is_port_available $port; then
        print_color "$RED" "ERROR: Port $port is already in use"
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
    local port="${TEST_PORT:-40114}"
    
    # Ensure config file is absolute path
    if [[ ! "$config_file" = /* ]]; then
        config_file="$PROJECT_ROOT/$config_file"
    fi
    
    print_color "$CYAN" "Config file: $config_file"
    print_color "$CYAN" "Expected port: $port"
    
    # Verify config has correct port
    if [[ -f "$config_file" ]]; then
        local config_port=$(grep -E "^\s*port:" "$config_file" | awk '{print $2}')
        print_color "$CYAN" "Port in config file: $config_port"
        if [[ "$config_port" != "$port" ]]; then
            print_color "$YELLOW" "WARNING: Config port ($config_port) doesn't match expected port ($port)"
        fi
    fi
    
    print_color "$CYAN" "Starting: OLLA_CONFIG_FILE=\"$config_file\" ./olla server"
    
    OLLA_CONFIG_FILE="$config_file" ./olla server > "$log_file" 2>&1 &
    OLLA_PID=$!
    
    print_color "$CYAN" "Started Olla process with PID: $OLLA_PID"
    
    # Wait for Olla to start
    print_color "$CYAN" "Health check URL: http://localhost:$port/internal/health"
    if wait_for_condition "check_olla_health" 30 "Waiting for Olla to start on port $port"; then
        print_color "$GREEN" "✓ Olla started successfully (PID: $OLLA_PID)"
        
        # Show that it's working
        local health_response=$(curl -s "http://localhost:$port/internal/health" 2>/dev/null)
        if [[ -n "$health_response" ]]; then
            print_color "$GREEN" "✓ Health check response: $health_response"
        fi
        
        # Strip ANSI codes from Olla log
        if [[ -f "$log_file" ]]; then
            strip_ansi_codes "$log_file"
        fi
        
        cd "$current_dir"
        return 0
    else
        print_color "$RED" "ERROR: Olla failed to start within 30 seconds"
        
        # Check if process is still running
        if ! kill -0 $OLLA_PID 2>/dev/null; then
            print_color "$RED" "Olla process died. Last 20 lines of log:"
            tail -20 "$log_file" | sed 's/^/  /'
        else
            print_color "$YELLOW" "Olla process is still running but not responding on port $port"
            print_color "$YELLOW" "Checking if port is actually in use..."
            if is_port_available $port; then
                print_color "$RED" "Port $port is not in use - Olla may be listening on wrong port"
            else
                print_color "$YELLOW" "Port $port is in use - health endpoint may be failing"
            fi
            print_color "$YELLOW" "Last 10 lines of log:"
            tail -10 "$log_file" | sed 's/\x1b\[[0-9;]*m//g' | sed 's/^/  /'
        fi
        
        # Strip ANSI codes from Olla log even on failure
        if [[ -f "$log_file" ]]; then
            strip_ansi_codes "$log_file"
        fi
        
        cd "$current_dir"
        return 1
    fi
}

# Function to check Olla health
check_olla_health() {
    local port="${TEST_PORT:-40114}"
    local url="http://localhost:$port/internal/health"
    
    # Debug mode - show what we're checking
    if [[ "${DEBUG:-}" == "true" ]]; then
        print_color "$GREY" "  Checking: $url"
    fi
    
    # Check if we get a successful response with JSON
    local response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null)
    local http_code=$(echo "$response" | tail -1)
    
    # Check if we got HTTP 200
    [[ "$http_code" == "200" ]]
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
    local port="${TEST_PORT:-40114}"
    local response=$(curl -s http://localhost:$port/internal/status 2>/dev/null)
    if [[ $? -eq 0 ]]; then
        echo "$response"
        return 0
    else
        return 1
    fi
}

# Function to get available models
get_available_models() {
    local port="${TEST_PORT:-40114}"
    curl -s http://localhost:$port/olla/models 2>/dev/null | jq -r '.data[].id // .models[].name // .models[].id' 2>/dev/null || true
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