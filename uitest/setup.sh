#!/bin/bash
# Setup script for Multi-App UI Testing Framework
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Multi-App UI Testing Setup ==="
echo ""

# Create Python virtual environment
if [ ! -d ".venv" ]; then
    echo "Creating Python virtual environment..."
    python3 -m venv .venv
else
    echo "Virtual environment already exists."
fi

# Activate venv
source .venv/bin/activate

# Install dependencies
echo "Installing Python dependencies..."
pip install -q --upgrade pip
pip install -q -r requirements.txt

# Install Playwright browser
echo "Installing Playwright Chromium..."
playwright install chromium --quiet 2>/dev/null || playwright install chromium

# Create output directories
mkdir -p screenshots content reports browser_state

# Check for .env
if [ ! -f ".env" ]; then
    echo ""
    echo "WARNING: No .env file found."
    echo "Copy .env.example and add your API key:"
    echo "  cp .env.example .env"
    echo "  # Then edit .env with your DEMETERICS_API_KEY"
else
    echo ".env file found."
fi

# Check for config.yaml
if [ ! -f "config.yaml" ]; then
    echo ""
    echo "WARNING: No config.yaml found."
    echo "Copy the example and customize:"
    echo "  cp config.yaml.example config.yaml"
else
    echo "config.yaml found."
fi

echo ""
echo "=== Setup complete ==="
echo "Run tests with: ./run.sh"
echo "Or:             ./run.sh --apps feedback --skip-llm"
