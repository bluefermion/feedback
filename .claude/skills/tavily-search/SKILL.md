---
name: tavily-search
description: Search the web using the Tavily API for real-time information, news, and research. Use when you need current facts, recent events, or to verify claims.
argument-hint: "[query]"
allowed-tools: Bash, Read
---

# Tavily Web Search

Search the web for: **$ARGUMENTS**

## Instructions

Use the Tavily Search API to find current information. The API key is in `~/.bash-env` as `TAVILY_API_KEY`.

Run a curl command like this:

```bash
source ~/.bash_env && curl -s 'https://api.tavily.com/search' \
  -H 'Content-Type: application/json' \
  -d "$(cat <<ENDJSON
{
  "api_key": "$TAVILY_API_KEY",
  "query": "<the search query>",
  "search_depth": "advanced",
  "max_results": 5,
  "include_answer": "advanced",
  "include_raw_content": "markdown",
  "topic": "news"
}
ENDJSON
)"
```

## Parameter guidance

- Use `"topic": "news"` for recent events, `"topic": "general"` for broader research, `"topic": "finance"` for financial data
- Use `"time_range": "day"` or `"week"` or `"month"` to filter by recency
- Use `"include_answer": "advanced"` to get an LLM-generated summary answer
- Use `"include_raw_content": "markdown"` to get full page content in markdown
- Use `"include_domains"` to restrict to specific sources (e.g., `["forbes.com", "reuters.com"]`)
- Use `"exact_match": true` for precise phrase matching
- Increase `"max_results"` up to 20 if you need broader coverage

## Output

1. Present the `answer` field first (if available) as a summary
2. Then list each result with title, URL, and key content
3. Flag any conflicting information between sources
4. If the search is for fact-checking, explicitly state what the sources confirm or contradict
