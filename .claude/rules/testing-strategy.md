# Testing Strategy

## Pre-Commit Checklist

```bash
go fmt ./...          # Format (required)
go vet ./...          # Static analysis (MUST PASS)
go test ./...         # Unit tests
go mod tidy           # Clean dependencies
```

## Unit Tests

### Standards
- Table-driven tests for multiple cases
- Test both success AND error paths
- Mock external dependencies via interfaces
- Target >80% coverage

### Example Pattern
```go
func TestCreateFeedback(t *testing.T) {
    tests := []struct {
        name    string
        input   model.FeedbackRequest
        wantErr bool
    }{
        {"valid bug", model.FeedbackRequest{Type: "Bug", Message: "test"}, false},
        {"empty message", model.FeedbackRequest{Type: "Bug", Message: ""}, true},
        {"invalid type", model.FeedbackRequest{Type: "Invalid", Message: "test"}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Integration Tests

```bash
# Start server in background
go run ./cmd/server &
SERVER_PID=$!

# Wait for startup
sleep 2

# Test health endpoint
curl -f http://localhost:8080/health || exit 1

# Test feedback submission
curl -f -X POST http://localhost:8080/api/feedback \
  -H "Content-Type: application/json" \
  -d '{"type":"Bug","message":"Integration test"}' || exit 1

# Cleanup
kill $SERVER_PID
```

## Pre-Deployment Verification

Before any deployment, verify:

1. [ ] `go vet ./...` passes with no errors
2. [ ] `go test ./...` passes
3. [ ] Health endpoint returns 200: `curl http://localhost:8080/health`
4. [ ] Feedback submission works (POST /api/feedback)
5. [ ] Admin views render correctly (/feedback, /feedback/{id})
6. [ ] Widget loads on /demo page
7. [ ] No secrets in committed code

## Race Detection

Run periodically:
```bash
go test -race ./...
```

## Coverage Report

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```
