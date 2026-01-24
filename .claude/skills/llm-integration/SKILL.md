---
name: llm-integration
description: LLM analysis, OpenCode, self-healing, AI features, error categorization
---

# LLM Integration

## Overview

The feedback service supports optional LLM-powered features:
1. **Error Analysis** — Automatic categorization and severity assessment
2. **OpenCode Integration** — Trigger automated code fixes via OpenCode.ai

## Configuration

```bash
# LLM Analysis (optional)
LLM_API_KEY=your-api-key
LLM_BASE_URL=https://api.groq.com/openai/v1
LLM_MODEL=llama-3.3-70b-versatile

# OpenCode Self-Healing (optional)
OPENCODE_ENABLED=true
SELFHEALING_MODE=analyze    # "analyze" or "opencode"
GIT_USER_NAME=OpenCode Bot
GIT_USER_EMAIL=opencode@example.com
```

## Self-Healing Modes

| Mode | Behavior |
|------|----------|
| `analyze` | LLM analyzes feedback and suggests fixes (no auto-commit) |
| `opencode` | Triggers OpenCode.ai to analyze and potentially commit fixes |

## Analysis Flow

```
User Feedback → Validation → Storage → [Optional] LLM Analysis
                                              ↓
                                        Categorization
                                              ↓
                                        Severity Rating
                                              ↓
                                   [If enabled] OpenCode Trigger
```

## API Status Endpoint

Check self-healing status:

```bash
curl http://localhost:8080/api/selfhealing/status
```

Response:
```json
{
  "enabled": true,
  "mode": "analyze",
  "llm_configured": true
}
```

## Security Considerations

- NEVER commit LLM_API_KEY to version control
- Use environment variables or secret management
- OpenCode commits should require review in production
- Consider rate limiting LLM calls to control costs
