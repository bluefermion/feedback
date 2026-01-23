#!/bin/bash
# analyze.sh - The "Brain" running inside the container.
#
# EDUCATIONAL CONTEXT:
# This script is the entry point for the "Worker" container.
# It acts as a bridge between the Feedback system (JSON input) and the
# Coding Agent CLI (OpenCode).
#
# Key Responsibilities:
# 1. Decode inputs (Base64/JSON) to avoid shell injection issues.
# 2. Authenticate the agent session.
# 3. Construct a detailed prompt ("Prompt Engineering").
# 4. Execute the agent binary in the correct workspace context.
# 5. Capture and format the output as structured JSON.
#
# Usage: analyze.sh <base64-encoded-feedback-json>

set -e # Exit immediately if any command fails.

# ------------------------------------------------------------------------------
# 1. INPUT VALIDATION
# ------------------------------------------------------------------------------

if [ -z "$1" ]; then
    echo '{"error": "No feedback data provided"}'
    exit 1
fi

# Decode base64 feedback payload.
# We use Base64 to safely pass complex JSON structures (with quotes/newlines)
# as a single command-line argument.
FEEDBACK=$(echo "$1" | base64 -d 2>/dev/null)
if [ $? -ne 0 ]; then
    echo '{"error": "Invalid base64 encoding"}'
    exit 1
fi

# Extract fields using 'jq' (Command-line JSON processor).
# -r = raw output (no quotes around strings)
# // "default" = fallback value if field is missing/null
TITLE=$(echo "$FEEDBACK" | jq -r '.title // "Unknown issue"')
DESCRIPTION=$(echo "$FEEDBACK" | jq -r '.description // ""')
TYPE=$(echo "$FEEDBACK" | jq -r '.type // "bug"')
CONSOLE_LOGS=$(echo "$FEEDBACK" | jq -r '.consoleLogs // ""')
URL=$(echo "$FEEDBACK" | jq -r '.url // ""')

# ------------------------------------------------------------------------------
# 2. AUTHENTICATION & ENVIRONMENT
# ------------------------------------------------------------------------------

# Ensure the agent has credentials to talk to the LLM API.
AUTH_FILE="/root/.local/share/opencode/auth.json"
if [ ! -f "$AUTH_FILE" ] || [ -z "$(cat "$AUTH_FILE" 2>/dev/null | grep -v 'LLM_API_KEY')" ]; then
    echo "[analyze.sh] Setting up OpenCode auth..." >&2
    if [ -z "$LLM_API_KEY" ]; then
        echo '{"error": "LLM_API_KEY not set"}'
        exit 1
    fi
    # Run the setup script to generate auth.json from env vars
    /app/setup-auth.sh || {
        echo '{"error": "Failed to setup OpenCode auth"}'
        exit 1
    }
fi

# Locate the agent binary
OPENCODE_BIN="${OPENCODE_BIN:-opencode}"
if ! command -v "$OPENCODE_BIN" &> /dev/null; then
    echo '{"error": "OpenCode CLI not found at '"$OPENCODE_BIN"'. Is it installed?"}'
    exit 1
fi

# Verify the repository mount.
# The container expects the user's code to be mounted at /workspace.
WORKSPACE="${OPENCODE_REPO_DIR:-/workspace}"
if [ ! -d "$WORKSPACE" ]; then
    echo "{\"error\": \"Workspace directory not found: $WORKSPACE. Mount your repo with -v /path/to/repo:/workspace\"}"
    exit 1
fi

if [ -z "$(ls -A $WORKSPACE 2>/dev/null)" ]; then
    echo "{\"error\": \"Workspace is empty: $WORKSPACE. Mount your repo with -v /path/to/repo:/workspace\"}"
    exit 1
fi

# ------------------------------------------------------------------------------
# 3. PROMPT CONSTRUCTION
# ------------------------------------------------------------------------------

# We construct a natural language prompt for the agent.
# This is a template that injects the bug report details.
PROMPT="Analyze this $TYPE report and fix it if possible.

Title: $TITLE
Type: $TYPE
URL: $URL

Description:
$DESCRIPTION"

# Add debugging context (Console Logs) if available.
if [ -n "$CONSOLE_LOGS" ] && [ "$CONSOLE_LOGS" != "null" ] && [ "$CONSOLE_LOGS" != "" ]; then
    PROMPT="$PROMPT

Console Logs:
$CONSOLE_LOGS"
fi

# Add "System Instructions" to guide the agent's behavior.
PROMPT="$PROMPT"

Instructions:
1. Explore the codebase to understand the structure
2. Find the relevant files for this issue
3. If it's a bug, identify the root cause and fix it
4. If it's a feature request, implement it if straightforward
5. Create a commit with your changes
6. Provide a summary of what you did"

# ------------------------------------------------------------------------------
# 4. EXECUTION
# ------------------------------------------------------------------------------

# Switch to the mounted repository directory so the agent acts on the right files.
cd "$WORKSPACE"

# Determine the LLM model to use.
MODEL="${LLM_MODEL:-llama-3.3-70b-versatile}"
# Ensure proper provider prefix (e.g., "groq/") required by the tool.
case "$MODEL" in
    groq/*) ;; # Already has prefix
    *) MODEL="groq/$MODEL" ;;
esac

# Logging helper
LOG_FILE="/var/log/opencode.log"
log() {
    # Tee outputs to both stderr (for Docker logs) and a file
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE" >&2
}

log "Running OpenCode in $WORKSPACE"
log "Type: $TYPE, Title: $TITLE"
log "Model: $MODEL"

# Write prompt to a temporary file.
# This avoids issues with shell quoting or argument length limits.
PROMPT_FILE=$(mktemp)
printf '%s' "$PROMPT" > "$PROMPT_FILE"
trap "rm -f '$PROMPT_FILE'" EXIT # Cleanup on exit

log "Starting OpenCode..."

# Execute the agent!
# We pipe the prompt file into stdin of the binary.
# 2>&1 redirects stderr to stdout so we capture all logs in $ANALYSIS.
ANALYSIS=$(cat "$PROMPT_FILE" | "$OPENCODE_BIN" run -m "$MODEL" 2>&1) || {
    EXIT_CODE=$?
    log "OpenCode exited with code $EXIT_CODE"
    # Return structured error
    echo "{\"success\": false, \"error\": \"OpenCode exited with code $EXIT_CODE\", \"output\": $(echo "$ANALYSIS" | jq -Rs .)}"
    exit 1
}

# ------------------------------------------------------------------------------
# 5. RESPONSE FORMATTING
# ------------------------------------------------------------------------------

if [ -z "$ANALYSIS" ]; then
    echo '{"success": false, "error": "OpenCode returned empty response"}'
    exit 1
fi

log "OpenCode completed successfully"

# Return success JSON with the agent's full textual output.
# jq -Rs . escapes the entire string to be valid JSON value.
echo "{\"success\": true, \"analysis\": $(echo "$ANALYSIS" | jq -Rs .)}"