# CLAUDE.md - Feedback Service

> Standalone feedback widget + Go backend by [Blue Fermion Labs](https://bluefermionlabs.com)

## ⚠️ PRIME DIRECTIVES

1. **NEVER** deploy without explicit user permission — always ask first
2. **NEVER** run `git checkout`/`git reset --hard` that destroys uncommitted work
3. **NEVER** delete database tables directly — use safe migration pattern
4. **NEVER** commit secrets or credentials to version control
5. **ALWAYS** run `go fmt` and `go vet` before commits — vet MUST pass

## Commands

```bash
go run ./cmd/server           # Run locally (:8080)
go test ./...                 # Run all tests
go build -o bin/feedback ./cmd/server  # Build binary
go fmt ./... && go vet ./...  # Quality gates (required before commit)
```

## Code Style

- **IMPORTANT**: `gofmt` and `goimports` required for all Go code
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- API routes (`/api/*`) return JSON; Web routes (`/feedback/*`) return HTML
- Internal packages (`internal/`) not importable externally

## Architecture

```
cmd/server/main.go        → Entry point, routing
internal/handler/         → HTTP handlers
internal/model/           → Data structures
internal/repository/      → SQLite CRUD (WAL mode)
widget/js/                → Frontend widget (auto-init)
```

## Environment

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | 8080 | HTTP server port |
| `FEEDBACK_DB_PATH` | feedback.db | SQLite path |
| `LLM_API_KEY` | — | Optional LLM analysis |
| `OPENCODE_ENABLED` | false | OpenCode integration |

## Key Behaviors

- Widget auto-initializes on page load
- SQLite WAL mode — also backs up `.db-wal` and `.db-shm` files
- Database auto-migrates on startup
- Pure Go SQLite (`modernc.org/sqlite`) — no CGO

## Gotchas

- **YOU MUST** validate feedback type: `Bug`, `Feature`, `Improvement`, `Other`
- Widget color is Material Orange (`#FF9800`)
- Go 1.22+ panics on duplicate route patterns — check before adding
- Template-struct mismatches crash at runtime — verify before deploy
- Check `github.com/patdeg/common` before adding new utilities

## Deployment

- Git is version control only, NOT deployment
- One deployment per request — stop and confirm between deploys
- Ask: "Deploy to dev or production?" — never assume
