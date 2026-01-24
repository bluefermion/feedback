---
name: go-backend
description: Go HTTP handlers, SQLite repository, API design, backend patterns, middleware
---

# Go Backend Development

## Handler Pattern

All handlers follow this structure:

```go
func (h *Handler) HandleFeedback(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    var req model.FeedbackRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // 2. Validate input
    if err := validateFeedback(&req); err != nil {
        respondError(w, http.StatusBadRequest, err.Error())
        return
    }

    // 3. Business logic via repository
    feedback, err := h.repo.Create(r.Context(), &req)
    if err != nil {
        respondError(w, http.StatusInternalServerError, "failed to create feedback")
        return
    }

    // 4. Return JSON response
    respondJSON(w, http.StatusCreated, feedback)
}
```

## Web vs API Separation

| Route Pattern | Handler Location | Returns | Content-Type |
|---------------|------------------|---------|--------------|
| `/api/*` | `internal/handler/api.go` | JSON | `application/json` |
| `/feedback/*` | `internal/handler/web.go` | HTML | `text/html` |

## Repository Pattern

```go
type Repository struct {
    db *sql.DB
}

func (r *Repository) Create(ctx context.Context, f *model.Feedback) error {
    query := `INSERT INTO feedback (type, message, metadata, created_at) VALUES (?, ?, ?, ?)`
    _, err := r.db.ExecContext(ctx, query, f.Type, f.Message, f.Metadata, time.Now())
    if err != nil {
        return fmt.Errorf("failed to create feedback: %w", err)
    }
    return nil
}
```

## Error Handling

```go
// Good - wrapped with context
return fmt.Errorf("failed to create feedback: %w", err)

// Bad - bare error
return err

// Sentinel errors for known conditions
var ErrNotFound = errors.New("not found")

if errors.Is(err, ErrNotFound) {
    respondError(w, http.StatusNotFound, "feedback not found")
    return
}
```

## Import Order

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "net/http"

    // 2. External dependencies
    "modernc.org/sqlite"

    // 3. Internal packages
    "feedback/internal/model"
    "feedback/internal/repository"
)
```

## Middleware Chain

```go
// Order matters: outermost runs first
handler := loggingMiddleware(
    recoveryMiddleware(
        authMiddleware(
            router,
        ),
    ),
)
```

## Context Propagation

Always pass context through:
```go
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Pass to repository
    result, err := h.repo.Get(ctx, id)

    // Pass to external calls
    resp, err := h.client.Do(req.WithContext(ctx))
}
```

## JSON Response Helpers

```go
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
    respondJSON(w, status, model.ErrorResponse{Error: message})
}
```

## Validation

```go
func validateFeedback(req *model.FeedbackRequest) error {
    validTypes := map[string]bool{
        "Bug": true, "Feature": true, "Improvement": true, "Other": true,
    }

    if !validTypes[req.Type] {
        return fmt.Errorf("invalid feedback type: %s", req.Type)
    }

    if strings.TrimSpace(req.Message) == "" {
        return errors.New("message is required")
    }

    return nil
}
```
