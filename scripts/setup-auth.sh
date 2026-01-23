#!/bin/bash
# Setup OpenCode authentication
# Called once when container starts or API key changes

set -e

TEMPLATE="/root/.local/share/opencode/auth.json.template"
AUTH_FILE="/root/.local/share/opencode/auth.json"

if [ -z "$LLM_API_KEY" ]; then
    echo "ERROR: LLM_API_KEY environment variable not set"
    exit 1
fi

# Substitute API key
envsubst '${LLM_API_KEY}' < "$TEMPLATE" > "$AUTH_FILE"

echo "OpenCode auth configured successfully"
