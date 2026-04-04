---
name: tavily-extract
description: Extract and parse content from specific URLs using the Tavily Extract API. Use when you need the full content of a web page, article, or document.
argument-hint: "[url(s)] [optional query for relevance]"
allowed-tools: Bash, Read
---

# Tavily URL Extract

Extract content from: **$ARGUMENTS**

## Instructions

Use the Tavily Extract API to fetch and parse web page content. The API key is in `~/.bash-env` as `TAVILY_API_KEY`.

Parse the arguments: the first argument(s) that look like URLs are the targets. Any remaining text is the relevance query.

### Single URL

```bash
source ~/.bash_env && curl -s 'https://api.tavily.com/extract' \
  -H 'Content-Type: application/json' \
  -d "$(cat <<ENDJSON
{
  "api_key": "$TAVILY_API_KEY",
  "urls": "<url>",
  "format": "markdown",
  "extract_depth": "advanced",
  "chunks_per_source": 5
}
ENDJSON
)"
```

### Multiple URLs

```bash
source ~/.bash_env && curl -s 'https://api.tavily.com/extract' \
  -H 'Content-Type: application/json' \
  -d "$(cat <<ENDJSON
{
  "api_key": "$TAVILY_API_KEY",
  "urls": ["<url1>", "<url2>"],
  "format": "markdown",
  "extract_depth": "advanced",
  "chunks_per_source": 5
}
ENDJSON
)"
```

## Parameter guidance

- Use `"format": "markdown"` for readable formatted output (default)
- Use `"format": "text"` for plain text only
- Use `"extract_depth": "advanced"` for JavaScript-rendered pages (costs more credits)
- Use `"query"` to rerank extracted chunks by relevance to a specific question
- Use `"chunks_per_source"` (1-5) to control how much content per URL
- Use `"include_images": true` to also extract images from the page

## Output

1. Present the extracted content in clean markdown
2. If a relevance query was provided, highlight the most relevant sections
3. Note any failed URLs from the `failed_results` array
