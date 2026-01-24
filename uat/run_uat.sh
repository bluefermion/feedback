#!/usr/bin/env bash
#
# Feedback Widget UAT Runner
# Activates Python venv and runs browser-use based UAT tests
#
# Usage:
#   ./run_uat.sh                    # Run default workflow (submit)
#   ./run_uat.sh --headed           # Run with visible browser
#   ./run_uat.sh --workflow full    # Run full workflow
#   ./run_uat.sh --task "..."       # Run custom task
#
# Environment:
#   GROQ_API_KEY or LLM_API_KEY must be set in .env
#

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
VENV_DIR="$SCRIPT_DIR/.venv"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[UAT]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[UAT]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[UAT]${NC} $1"
}

log_error() {
    echo -e "${RED}[UAT]${NC} $1"
}

# Check Python version
check_python() {
    if command -v python3 &> /dev/null; then
        PYTHON_CMD="python3"
    elif command -v python &> /dev/null; then
        PYTHON_CMD="python"
    else
        log_error "Python not found. Please install Python 3.11+"
        exit 1
    fi

    # Check version is 3.11+
    VERSION=$($PYTHON_CMD -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
    MAJOR=$(echo "$VERSION" | cut -d. -f1)
    MINOR=$(echo "$VERSION" | cut -d. -f2)

    if [ "$MAJOR" -lt 3 ] || ([ "$MAJOR" -eq 3 ] && [ "$MINOR" -lt 11 ]); then
        log_error "Python 3.11+ required, found $VERSION"
        exit 1
    fi

    log_info "Using Python $VERSION"
}

# Create/activate virtual environment
setup_venv() {
    if [ ! -d "$VENV_DIR" ]; then
        log_info "Creating virtual environment..."
        $PYTHON_CMD -m venv "$VENV_DIR"
        log_success "Virtual environment created at $VENV_DIR"
    fi

    # Activate venv
    source "$VENV_DIR/bin/activate"
    log_info "Virtual environment activated"
}

# Install dependencies
install_deps() {
    if [ ! -f "$VENV_DIR/.deps_installed" ]; then
        log_info "Installing dependencies..."
        pip install --quiet --upgrade pip
        pip install --quiet -r "$SCRIPT_DIR/requirements.txt"

        log_info "Installing Playwright browsers..."
        playwright install chromium --quiet 2>/dev/null || playwright install chromium

        # Mark as installed
        touch "$VENV_DIR/.deps_installed"
        log_success "Dependencies installed"
    else
        log_info "Dependencies already installed (use 'make uat-setup' to reinstall)"
    fi
}

# Load environment variables
load_env() {
    # Try project root .env first
    if [ -f "$PROJECT_ROOT/.env" ]; then
        log_info "Loading environment from $PROJECT_ROOT/.env"
        set -a
        source "$PROJECT_ROOT/.env"
        set +a
    fi

    # Override with local UAT .env if exists
    if [ -f "$SCRIPT_DIR/.env.local" ]; then
        log_info "Loading local overrides from $SCRIPT_DIR/.env.local"
        set -a
        source "$SCRIPT_DIR/.env.local"
        set +a
    fi

    # Check for API key
    if [ -z "${GROQ_API_KEY:-}" ] && [ -z "${LLM_API_KEY:-}" ]; then
        log_warn "No GROQ_API_KEY or LLM_API_KEY found"
        log_warn "LLM-driven browser automation will not work"
        log_warn "Add GROQ_API_KEY to $PROJECT_ROOT/.env"
    fi
}

# Check if server is running
check_server() {
    local BASE_URL="${BASE_URL:-http://localhost:8080}"

    if ! curl -s --connect-timeout 2 "$BASE_URL/health" > /dev/null 2>&1; then
        log_warn "Server not responding at $BASE_URL"
        log_warn "Start with: make run (in another terminal)"
        echo ""
        read -p "Continue anyway? [y/N] " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        log_success "Server running at $BASE_URL"
    fi
}

# Main
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║           Feedback Widget UAT (Browser-Use)                  ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo ""

    check_python
    setup_venv
    install_deps
    load_env
    check_server

    echo ""
    log_info "Starting UAT tests..."
    echo ""

    # Run the Python script with all arguments passed through
    python "$SCRIPT_DIR/run_uat.py" "$@"

    EXIT_CODE=$?

    echo ""
    if [ $EXIT_CODE -eq 0 ]; then
        log_success "UAT completed successfully"
    else
        log_error "UAT failed with exit code $EXIT_CODE"
    fi

    exit $EXIT_CODE
}

# Help
if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
    echo "Feedback Widget UAT Runner"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --headed              Run with visible browser"
    echo "  --workflow NAME       Run predefined workflow (submit, verify, full)"
    echo "  --task \"TEXT\"         Run custom natural language task"
    echo "  --page NAME           Test specific page (demo, feedback_list)"
    echo "  --all                 Run all page tests"
    echo "  --base-url URL        Override base URL"
    echo "  --model MODEL         Override LLM model"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Run submit workflow"
    echo "  $0 --headed --workflow full           # Run full test with visible browser"
    echo "  $0 --task \"Click the feedback button\" # Run custom task"
    echo ""
    echo "Environment:"
    echo "  GROQ_API_KEY    Groq API key for LLM"
    echo "  LLM_API_KEY     Alternative API key name"
    echo "  BASE_URL        Server URL (default: http://localhost:8080)"
    echo ""
    exit 0
fi

main "$@"
