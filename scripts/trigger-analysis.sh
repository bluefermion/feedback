#!/bin/bash
# trigger-analysis.sh - Host-side bridge to the Docker container.
#
# EDUCATIONAL CONTEXT:
# This script runs on the Host machine (where the Go server is).
# Its job is to:
# 1. Verify the worker container is running.
# 2. Serialize the input data (Feedback JSON).
# 3. "Teleport" the data into the container safely (Base64 encoding).
# 4. Invoke the internal script (`analyze.sh`) inside the container namespace.
#
# This allows the Go application to remain lightweight and platform-independent,
# delegating the heavy, environment-dependent analysis task to a Docker container.
#
# Usage: trigger-analysis.sh <feedback-json>

set -e # Stop on any error

# Configuration with defaults
CONTAINER_NAME="${OPENCODE_CONTAINER:-opencode-selfhealing}"

# ------------------------------------------------------------------------------
# 1. PRE-FLIGHT CHECKS
# ------------------------------------------------------------------------------

# Check if the worker container is active.
# `docker ps` lists running containers. We filter by exact name match.
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    # Return JSON error format expected by the calling Go code.
    echo '{"error": "OpenCode container not running"}'
    exit 1
fi

# ------------------------------------------------------------------------------
# 2. INPUT PREPARATION
# ------------------------------------------------------------------------------

# Accept input either as a command-line argument OR via Standard Input (stdin).
if [ -n "$1" ]; then
    FEEDBACK="$1"
else
    FEEDBACK=$(cat)
fi

# SAFETY: Base64 encoding.
# Passing complex JSON (with spaces, quotes, special chars) as a command argument
# to `docker exec` is brittle and prone to escaping issues.
# Encoding it to Base64 ensures it's a safe, unbroken string of ASCII characters.
# -w 0: Disable line wrapping (output must be a single line).
ENCODED=$(echo "$FEEDBACK" | base64 -w 0)

# ------------------------------------------------------------------------------
# 3. REMOTE EXECUTION
# ------------------------------------------------------------------------------

# `docker exec`: Run a command in a running container.
# We pass the Base64 string as the argument to the internal script.
# The internal script (/app/analyze.sh) will decode it back to JSON.
docker exec "$CONTAINER_NAME" /app/analyze.sh "$ENCODED"