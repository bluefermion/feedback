# Add New API Endpoint

Guide for adding a new API endpoint to the feedback service.

## Checklist

1. **Define the model** in `internal/model/`
   - Request struct with JSON tags
   - Response struct with JSON tags
   - Validation rules

2. **Add repository method** in `internal/repository/repository.go`
   - SQL query with parameterized inputs
   - Proper error wrapping
   - Context support for cancellation

3. **Create handler** in `internal/handler/`
   - Parse and validate request
   - Call repository
   - Return JSON response
   - Handle errors consistently

4. **Register route** in `cmd/server/main.go`
   - Use standard library router or your chosen mux
   - Apply middleware (logging, auth if needed)

5. **Write tests** in `internal/handler/*_test.go`
   - Happy path
   - Validation errors
   - Not found cases
   - Server errors

## Example: Adding DELETE /api/feedback/{id}

```go
// internal/repository/repository.go
func (r *Repository) Delete(ctx context.Context, id string) error {
    result, err := r.db.ExecContext(ctx, "DELETE FROM feedback WHERE id = ?", id)
    if err != nil {
        return fmt.Errorf("failed to delete feedback: %w", err)
    }
    rows, _ := result.RowsAffected()
    if rows == 0 {
        return ErrNotFound
    }
    return nil
}

// internal/handler/feedback.go
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id") // or your router's param extraction
    if err := h.repo.Delete(r.Context(), id); err != nil {
        if errors.Is(err, repository.ErrNotFound) {
            respondError(w, http.StatusNotFound, "feedback not found")
            return
        }
        respondError(w, http.StatusInternalServerError, "failed to delete")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```
