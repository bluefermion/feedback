# CLAUDE.md - Claude Code Context

This file provides context for Claude Code when working with this repository.

## Project Overview

**feedback** is a standalone feedback collection and analysis system built by [Blue Fermion Labs](https://bluefermionlabs.com). It provides:

1. **Frontend Widget** - A yellow floating button with "!" icon that captures user feedback
2. **Go Backend** - HTTP handlers for feedback submission and storage in SQLite
3. **LLM Analysis** - Optional automatic error analysis and categorization
4. **OpenCode Integration** - When an admin submits feedback, can trigger automated code analysis via [OpenCode.ai](https://opencode.ai)

## Repository Structure

```
feedback/
├── cmd/
│   └── server/
│       └── main.go            # Entry point, HTTP server setup
├── internal/
│   ├── handler/
│   │   └── feedback.go        # HTTP handlers for /api/feedback
│   ├── model/
│   │   └── feedback.go        # Feedback data model
│   └── repository/
│       └── repository.go      # SQLite CRUD operations
├── widget/
│   └── js/
│       └── feedback-widget.js # Frontend widget
├── go.mod
├── README.md
├── CLAUDE.md
├── GEMINI.md
├── LICENSE
└── .gitignore
```

## Key Components

### Feedback Widget (widget/js/)
- Floating yellow "!" button (Material Orange #FF9800)
- Type selection: Bug, Feature, Improvement, Other
- Console log interception (stores last 50 entries)
- Device/browser metadata collection
- Auto-init on page load

### Backend Server (cmd/server/)
- HTTP server with routing
- Static file serving for widget
- Demo page at /demo
- Logging middleware

### HTTP Handlers (internal/handler/)
- `POST /api/feedback` - Submit feedback
- `GET /api/feedback` - List all feedback (paginated)
- `GET /api/feedback/{id}` - Get specific feedback
- Input validation and sanitization

### Data Model (internal/model/)
- Feedback struct with all fields
- Request/Response types
- Error response type

### Repository (internal/repository/)
- SQLite storage with WAL mode
- Auto-migration on startup
- CRUD operations with proper error handling

## Environment Variables

```bash
# Server config
PORT=8080                      # HTTP port (default: 8080)
FEEDBACK_DB_PATH=feedback.db   # SQLite database path

# Optional LLM Analysis
LLM_API_KEY=your-api-key
LLM_BASE_URL=https://api.groq.com/openai/v1
LLM_MODEL=qwen/qwen3-32b

# Debug
COMMON_DEBUG=true              # Enable debug logging
```

## Common Commands

```bash
# Run locally
go run ./cmd/server

# Run tests
go test ./...

# Build
go build -o bin/feedback ./cmd/server

# Access demo
open http://localhost:8080/demo
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | / | Service info (JSON) |
| GET | /health | Health check |
| GET | /demo | Demo HTML page |
| POST | /api/feedback | Submit feedback |
| GET | /api/feedback | List feedback |
| GET | /api/feedback/{id} | Get feedback by ID |
| GET | /static/* | Static files (widget) |

## Code Style

- Standard Go formatting (gofmt/goimports)
- Error wrapping with context
- JSON for all API responses
- Internal packages for implementation details

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/handler/...
```

## Dependencies

- `modernc.org/sqlite` - Pure Go SQLite driver (no CGO)

## Production Notes

For production deployment, consider:
1. Adding authentication middleware
2. Switching to PostgreSQL for high concurrency
3. Enabling content moderation with LLM guards
4. Adding rate limiting

See [demeterics.ai](https://demeterics.ai) for a production implementation.

## Related Projects

- [demeterics.com](https://demeterics.com) - AI Chat Widget (sister project)
- [github.com/patdeg/common](https://github.com/patdeg/common) - Shared Go utilities

## Attribution

Built by [Blue Fermion Labs](https://bluefermionlabs.com)
