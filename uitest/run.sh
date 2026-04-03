#!/bin/bash
# Run Multi-App UI Testing
#
# Usage:
#   ./run.sh                                  # Test all apps
#   ./run.sh --apps feedback                  # Test specific app
#   ./run.sh --apps feedback --skip-llm       # Screenshots only
#   ./run.sh --reuse-sessions                 # Reuse saved logins
#   ./run.sh --headed                         # Watch the browser
#   ./run.sh --skip-browser-use               # No browser-use actions
#   ./run.sh --config config.local.yaml       # Custom config
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Check venv exists
if [ ! -d ".venv" ]; then
    echo "Virtual environment not found. Run ./setup.sh first."
    exit 1
fi

# Activate venv
source .venv/bin/activate

# Load .env if present
if [ -f ".env" ]; then
    set -a
    source .env
    set +a
fi

# Check API key (warn, don't block — user may be running --skip-llm)
if [ -z "$DEMETERICS_API_KEY" ]; then
    echo "WARNING: No DEMETERICS_API_KEY set."
    echo "LLM analysis and browser-use actions will fail."
    echo "Set in .env or export the variable."
    echo ""
fi

# Run the tester, passing all arguments through
python run_uitest.py "$@"
