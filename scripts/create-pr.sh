#!/bin/bash
# Create PR with LLM-generated title and description
# Uses Demeterics API to analyze commits and generate PR content
#
# Usage: ./scripts/create-pr.sh [--no-preview]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check for required tools
command -v gh >/dev/null 2>&1 || { echo -e "${RED}Error: gh CLI not installed${NC}"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo -e "${RED}Error: jq not installed${NC}"; exit 1; }

# Check for API key
if [ -z "$DEMETERICS_API_KEY" ]; then
    # Try to load from .env
    if [ -f .env ]; then
        export $(grep -E '^DEMETERICS_API_KEY=' .env | xargs)
    fi
fi

if [ -z "$DEMETERICS_API_KEY" ]; then
    echo -e "${RED}Error: DEMETERICS_API_KEY not set${NC}"
    echo "Set it in .env or export DEMETERICS_API_KEY=your_key"
    exit 1
fi

# Get current branch
CURRENT_BRANCH=$(git branch --show-current)
DEFAULT_BRANCH=$(git remote show origin | grep 'HEAD branch' | cut -d' ' -f5)

# Ensure not on main/master
if [ "$CURRENT_BRANCH" = "$DEFAULT_BRANCH" ] || [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
    echo -e "${RED}Error: Cannot create PR from $CURRENT_BRANCH${NC}"
    echo "Create a feature branch first:"
    echo "  git checkout -b feature/your-feature-name"
    exit 1
fi

echo -e "${BLUE}Creating PR for branch: ${YELLOW}$CURRENT_BRANCH${NC}"
echo ""

# Check if PR already exists
EXISTING_PR=$(gh pr list --head "$CURRENT_BRANCH" --json number --jq '.[0].number' 2>/dev/null || echo "")
if [ -n "$EXISTING_PR" ]; then
    echo -e "${YELLOW}PR #$EXISTING_PR already exists for this branch${NC}"
    echo "View it at: $(gh pr view "$EXISTING_PR" --json url --jq '.url')"
    exit 0
fi

# Get commits since branching from default branch
echo -e "${BLUE}Analyzing commits...${NC}"
MERGE_BASE=$(git merge-base "$DEFAULT_BRANCH" HEAD)
COMMITS=$(git log "$MERGE_BASE"..HEAD --pretty=format:"%s" --reverse)
COMMIT_COUNT=$(git rev-list "$MERGE_BASE"..HEAD --count)

if [ "$COMMIT_COUNT" -eq 0 ]; then
    echo -e "${RED}Error: No commits to create PR from${NC}"
    echo "Make some commits first, then run this again."
    exit 1
fi

echo -e "Found ${GREEN}$COMMIT_COUNT${NC} commit(s)"

# Get diff stats
DIFF_STATS=$(git diff "$MERGE_BASE"..HEAD --stat | tail -1)
FILES_CHANGED=$(git diff "$MERGE_BASE"..HEAD --name-only)

# Get repo name
REPO_NAME=$(basename "$(git rev-parse --show-toplevel)")

echo -e "${BLUE}Generating PR title and description with AI...${NC}"

# Build the LLM request
JSON_PAYLOAD=$(jq -n \
    --arg model "openai/gpt-oss-20b" \
    --arg repo "$REPO_NAME" \
    --arg branch "$CURRENT_BRANCH" \
    --arg commits "$COMMITS" \
    --arg files "$FILES_CHANGED" \
    --arg stats "$DIFF_STATS" \
    '{
      model: $model,
      messages: [
        {
          role: "system",
          content: ("/// APP: GitHub-PR-Creator\n/// FLOW: " + $repo + "\n/// ENV: production\n\nYou are a PR description generator. Based on the commits and changed files, generate a PR title and description.\n\nRespond in this exact format:\n\nTITLE: <short imperative title, max 72 chars>\n\nDESCRIPTION:\n## Summary\n<1-3 bullet points summarizing the changes>\n\n## Changes\n<list of specific changes made>\n\n## Test Plan\n<how to test these changes>\n\nBe concise. Use markdown formatting.")
        },
        {
          role: "user",
          content: ("Generate PR title and description for:\n\nBranch: " + $branch + "\n\nCommits:\n" + $commits + "\n\nFiles changed:\n" + $files + "\n\nStats: " + $stats)
        }
      ],
      temperature: 0.3,
      max_tokens: 1000
    }')

# Call Demeterics API
RESPONSE=$(curl -s https://api.demeterics.com/groq/v1/chat/completions \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $DEMETERICS_API_KEY" \
    -d "$JSON_PAYLOAD")

# Check for errors
if echo "$RESPONSE" | jq -e '.error' >/dev/null 2>&1; then
    ERROR_MSG=$(echo "$RESPONSE" | jq -r '.error.message // "Unknown error"')
    echo -e "${RED}LLM API error: $ERROR_MSG${NC}"
    exit 1
fi

# Extract content
CONTENT=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // ""')

if [ -z "$CONTENT" ]; then
    echo -e "${RED}Failed to generate PR content${NC}"
    exit 1
fi

# Parse title and description
PR_TITLE=$(echo "$CONTENT" | grep -E "^TITLE:" | sed 's/^TITLE:[[:space:]]*//' | head -1)
PR_DESCRIPTION=$(echo "$CONTENT" | sed -n '/^DESCRIPTION:/,$p' | tail -n +2)

# Fallback if parsing fails
if [ -z "$PR_TITLE" ]; then
    PR_TITLE="$CURRENT_BRANCH"
fi

if [ -z "$PR_DESCRIPTION" ]; then
    PR_DESCRIPTION="$CONTENT"
fi

# Add footer
PR_DESCRIPTION="$PR_DESCRIPTION

---
*PR generated with [Demeterics AI](https://demeterics.ai)*"

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Title:${NC} $PR_TITLE"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Description:${NC}"
echo "$PR_DESCRIPTION"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Preview mode (default) or direct create
if [ "$1" = "--no-preview" ]; then
    CREATE_PR="y"
else
    echo -e "${YELLOW}Create this PR? [Y/n/e(dit)]${NC} "
    read -r CREATE_PR
    CREATE_PR=${CREATE_PR:-y}
fi

case "$CREATE_PR" in
    [Yy]*)
        echo -e "${BLUE}Creating PR...${NC}"
        PR_URL=$(gh pr create \
            --title "$PR_TITLE" \
            --body "$PR_DESCRIPTION" \
            --base "$DEFAULT_BRANCH" \
            --head "$CURRENT_BRANCH")
        echo ""
        echo -e "${GREEN}PR created successfully!${NC}"
        echo -e "URL: ${BLUE}$PR_URL${NC}"
        ;;
    [Ee]*)
        echo -e "${YELLOW}Opening editor...${NC}"
        gh pr create --base "$DEFAULT_BRANCH" --head "$CURRENT_BRANCH"
        ;;
    *)
        echo -e "${YELLOW}PR creation cancelled${NC}"
        exit 0
        ;;
esac
