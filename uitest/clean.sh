#!/bin/bash
# Clean up generated test artifacts (screenshots, JSON reports, content, pycache)
# Keeps: .md reports, .gitkeep files, browser_state sessions

set -e
cd "$(dirname "$0")"

echo "Cleaning uitest artifacts..."

# Screenshots (PNGs)
count=$(find screenshots -name "*.png" 2>/dev/null | wc -l)
find screenshots -name "*.png" -delete 2>/dev/null
echo "  Removed $count screenshots"

# JSON reports (keep .md reports)
count=$(find reports -name "*.json" 2>/dev/null | wc -l)
find reports -name "*.json" -delete 2>/dev/null
echo "  Removed $count JSON reports"

# Extracted content
count=$(find content -name "*.md" 2>/dev/null | wc -l)
find content -name "*.md" -delete 2>/dev/null
echo "  Removed $count content files"

# Python cache
if [ -d __pycache__ ]; then
    rm -rf __pycache__
    echo "  Removed __pycache__"
fi

echo "Done. Kept: .md reports, browser sessions, .gitkeep files"
