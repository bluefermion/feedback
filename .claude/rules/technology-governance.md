# Technology Governance

Best practices for core technologies used in this project.

## Go Web Server

### HTTP Server Configuration
```go
server := &http.Server{
    Addr:         ":8080",
    Handler:      handler,
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

### Best Practices
- **Graceful shutdown**: Handle SIGINT/SIGTERM with `signal.NotifyContext`
- **Context propagation**: Pass `r.Context()` through all layers
- **Middleware order**: Logging → Recovery → Auth → Handler
- **Connection limiting**: Buffered channel as semaphore for rate limiting

### Go 1.22+ Routing
- Uses stdlib `http.ServeMux` with method-based routing
- Path parameters: `GET /api/feedback/{id}`
- **CRITICAL**: Panics on duplicate route patterns — always check before adding

## SQLite

### Configuration
- WAL mode enabled for concurrent reads
- Pure Go driver (`modernc.org/sqlite`) — no CGO required
- Auto-migration on startup

### Best Practices
- **Parameterized queries**: Never interpolate user input
- **Connection pooling**: Set `SetMaxOpenConns()` appropriately
- **Backup strategy**: Include `.db-wal` and `.db-shm` files
- **Safe migrations**: CREATE → QA → DELETE → RENAME pattern

## Docker

### Multi-Stage Build Pattern
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/server ./cmd/server

FROM alpine:3.19
RUN adduser -D -u 1000 appuser
COPY --from=builder /app/server /server
USER appuser
EXPOSE 8080
CMD ["/server"]
```

### Key Rules
- `CGO_ENABLED=0` for static binary
- `-trimpath -ldflags="-s -w"` strips debug info
- Non-root user (`appuser`) for security
- Multi-stage: builder (300MB+) → final (5-20MB)

### The Killswitch (Self-Healing Container)
`Dockerfile.selfhealing` mounts the whole repo read-write at `/workspace` for
the OpenCode agent — that access is real and intentional (it needs to fix
files anywhere in the tree). Two layers keep it safe; keep both.

**Layer 1 — the Watchdog (`internal/watchdog`).** A second LLM reads the
agent's live stderr (teed from `trigger.runOpenCode`) and judges it against
`policy.go` — THE LAWS, plain English, written by a human. Rules:
- The laws are text. Change behavior by editing the policy, not by tuning
  the judge prompt. Anything a reviewer should be able to argue about in a
  PR belongs in the policy.
- The agent's output goes in the *user* turn as data; the laws and task go
  in the *system* turn. The worker never writes the Watchdog's instructions.
- Fail **closed**: judge unreachable ⇒ `Kill("watchdog blind: …")`. A
  Watchdog that can't see is not a Watchdog. `WATCHDOG_FAIL_OPEN=true`
  exists for local dev only.
- `Kill` = `docker kill` + write `KILLSWITCH`. `CanTrigger` checks that
  file **first**, before admin/container/cooldown — a human's (or the
  Watchdog's) kill outranks everything. Only a human clears it
  (`make re-arm`).
- Prefer a different `WATCHDOG_MODEL` than the worker's model: two models
  are harder to fool the same way.

**Layer 2 — the tripwire (`scripts/guard/pre-commit`).** A deterministic
git hook for when the Watchdog itself is fooled. Bake it at `COPY`-time into
a path **outside** the bind mount (`/etc/opencode-guard/hooks`); point
`core.hooksPath` there and re-assert it on every run (`scripts/analyze.sh`);
refuse to run the agent if the hook is missing. Prove it fires with
`make guard-canary` after any change to it, `Dockerfile.selfhealing`, or
`scripts/analyze.sh`.

See README "The Killswitch" for the rationale.

## HTMX

### Integration Pattern
- `hx-boost="true"` for AJAX navigation
- Detect HTMX: `r.Header.Get("HX-Request") == "true"`
- Return fragments for HTMX, full pages otherwise

### Best Practices
- **Security**: Use `html/template` auto-escaping — never raw HTML
- **Authentication**: Cookies with secure flags, not localStorage
- **No PRG pattern**: Return HTML directly from POST handlers
- **Lazy loading**: Expensive operations in separate endpoints
- **Debounce**: `hx-trigger="keyup changed delay:300ms"`

## Vanilla JavaScript (Widget)

### Patterns
- IIFE for scope isolation
- Dynamic CSS injection
- Fetch API for HTTP requests
- Canvas API for screenshots (html2canvas)

### Best Practices
- No build step — direct browser loading
- CDN for dependencies (html2canvas)
- Progressive enhancement — works without JS
