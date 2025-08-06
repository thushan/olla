#!/bin/bash
# _common.sh
# Common functions and variables for test scripts

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GREY='\033[0;37m'
RESET='\033[0m'

# Common paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
TEST_CONFIG="config.test.local.yaml"
OLLA_BINARY="$PROJECT_ROOT/olla"

# Function to print colored output
print_color() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${RESET}"
    # Force flush output
    if [[ -t 1 ]]; then
        tput sgr0 2>/dev/null || true
    fi
}

# Function to print section header
print_header() {
    local title=$1
    print_color "$PURPLE" "============================================================"
    print_color "$PURPLE" "  $title"
    print_color "$PURPLE" "============================================================"
}

# Function to print subsection
print_section() {
    local title=$1
    print_color "$WHITE" "\n$title"
    print_color "$GREY" "----------------------------------------"
}

# Function to check and activate virtual environment
check_venv() {
    # If already in a venv, use it
    if [[ -n "${VIRTUAL_ENV:-}" ]]; then
        print_color "$GREEN" "✓ Virtual environment detected: $VIRTUAL_ENV"
        return 0
    fi
    
    # Look for .venv in test/scripts directory
    local venv_path="$PROJECT_ROOT/test/scripts/.venv"
    
    if [[ -d "$venv_path" ]]; then
        print_color "$YELLOW" "Found virtual environment at: $venv_path"
        print_color "$YELLOW" "Activating virtual environment..."
        
        # Activate based on OS
        if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
            # Windows
            if [[ -f "$venv_path/Scripts/activate" ]]; then
                source "$venv_path/Scripts/activate"
            else
                print_color "$RED" "ERROR: Cannot find activation script in $venv_path/Scripts/"
                return 1
            fi
        else
            # Unix/Linux/macOS
            if [[ -f "$venv_path/bin/activate" ]]; then
                source "$venv_path/bin/activate"
            else
                print_color "$RED" "ERROR: Cannot find activation script in $venv_path/bin/"
                return 1
            fi
        fi
        
        # Verify activation
        if [[ -n "${VIRTUAL_ENV:-}" ]]; then
            print_color "$GREEN" "✓ Virtual environment activated: $VIRTUAL_ENV"
            export VENV_WAS_ACTIVATED=true
            return 0
        else
            print_color "$RED" "ERROR: Failed to activate virtual environment"
            return 1
        fi
    else
        print_color "$RED" "ERROR: No virtual environment found at $venv_path"
        print_color "$YELLOW" "Please create one first:"
        print_color "$GREY" "  cd $PROJECT_ROOT/test/scripts"
        print_color "$GREY" "  python -m venv .venv"
        print_color "$GREY" "  source .venv/bin/activate  # On Unix/macOS"
        print_color "$GREY" "  .venv\\Scripts\\activate    # On Windows"
        print_color "$GREY" "  pip install -r requirements.txt"
        return 1
    fi
}

# Function to deactivate virtual environment if we activated it
deactivate_venv() {
    if [[ "${VENV_WAS_ACTIVATED:-false}" == "true" ]] && [[ -n "${VIRTUAL_ENV:-}" ]]; then
        print_color "$YELLOW" "Deactivating virtual environment..."
        deactivate 2>/dev/null || true
        unset VENV_WAS_ACTIVATED
    fi
}

# Function to check if a command exists
command_exists() {
    command -v "$1" &> /dev/null
}

# Function to create results directory with timestamp
create_results_dir() {
    local prefix="${1:-test-results}"
    local results_base="$PROJECT_ROOT/test/results"
    mkdir -p "$results_base"
    local results_dir="$results_base/$prefix-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$results_dir"
    echo "$results_dir"
}

# Function to log with timestamp
log_with_timestamp() {
    local message=$1
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $message"
}

# Function to check if port is available
is_port_available() {
    local port=$1
    ! nc -z localhost "$port" 2>/dev/null
}

# Function to wait for condition with timeout
wait_for_condition() {
    local condition_fn=$1
    local timeout=${2:-30}
    local message=${3:-"Waiting for condition"}
    
    local count=0
    while [[ $count -lt $timeout ]]; do
        if $condition_fn; then
            return 0
        fi
        sleep 1
        ((count++))
        echo -n "."
    done
    echo ""
    return 1
}

# Function to run command with timeout
run_with_timeout() {
    local timeout=$1
    shift
    timeout --preserve-status "$timeout" "$@"
}