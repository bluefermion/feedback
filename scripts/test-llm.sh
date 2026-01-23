#!/bin/bash
# Test LLM API connection
# Usage: ./scripts/test-llm.sh

set -e

# Load .env if it exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Check for API key
if [ -z "$LLM_API_KEY" ]; then
    echo "ERROR: LLM_API_KEY not set"
    echo "Set it in .env or export it"
    exit 1
fi

API_URL="${LLM_BASE_URL:-https://api.demeterics.com/chat/v1}/chat/completions"
MODEL="${LLM_MODEL:-groq/llama-3.3-70b-versatile}"

echo "Testing LLM API..."
echo "URL: $API_URL"
echo "Model: $MODEL"
echo "API Key: ${LLM_API_KEY:0:10}..."
echo ""

# Build request
REQUEST=$(cat <<EOF
{
    "model": "$MODEL",
    "messages": [
        {"role": "user", "content": "Say 'Hello, API is working!' and nothing else."}
    ],
    "max_tokens": 50
}
EOF
)

echo "Sending request..."
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $LLM_API_KEY" \
    -d "$REQUEST")

# Extract HTTP code (last line)
HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo ""
echo "HTTP Status: $HTTP_CODE"
echo ""

if [ "$HTTP_CODE" = "200" ]; then
    echo "SUCCESS!"
    echo ""
    echo "Response:"
    echo "$BODY" | jq -r '.choices[0].message.content // .error // .'
else
    echo "FAILED!"
    echo ""
    echo "Response:"
    echo "$BODY" | jq . 2>/dev/null || echo "$BODY"
fi
