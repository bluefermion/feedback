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

### Deterministic Guardrails (Self-Healing Container)
`Dockerfile.selfhealing` mounts the whole repo read-write at `/workspace` for
the OpenCode agent — that access is real and intentional (it needs to fix
files anywhere in the tree). The pattern that keeps it safe:

- Bake anything the agent must not be able to reach at `COPY`-time, into a
  path **outside** the bind mount (e.g. `/etc/opencode-guard/hooks`). A file
  living inside `/workspace` can be edited or deleted by the process running
  there, no matter what permissions say — the boundary has to be structural,
  not a runtime check.
- Point `git config core.hooksPath` at that outside-the-mount path, and
  re-assert it on every run (`scripts/analyze.sh`) rather than once at build
  time, so a reset doesn't silently disable it.
- Fail **closed**: if the guard file isn't present and executable, refuse to
  run the agent at all rather than proceed unguarded.
- Never trust that a guard fires just because it's installed — prove it with
  a live-fire test (`scripts/guard/guard-canary.sh`, `make guard-canary`)
  that deliberately attempts a forbidden action and asserts it's rejected.
  Run it in CI or after any change to `Dockerfile.selfhealing` /
  `scripts/guard/**`.

See README "Guardrails You Can't Talk Your Way Past" for the full rationale.

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
