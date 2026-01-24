# Code Review Rules

## Security Checklist

- [ ] No SQL injection (use parameterized queries)
- [ ] No XSS (escape HTML output, use Content-Type headers)
- [ ] No secrets in code (use environment variables)
- [ ] Input validation on all user data
- [ ] Rate limiting on public endpoints

## Go-Specific Rules

- [ ] All errors are handled or explicitly ignored with `_`
- [ ] Context passed through for cancellation support
- [ ] No goroutine leaks (use WaitGroups or done channels)
- [ ] Defer used for cleanup (Close, Unlock, etc.)
- [ ] Interfaces defined where they're used, not where implemented

## API Design Rules

- [ ] RESTful conventions (verbs match HTTP methods)
- [ ] Consistent error response format
- [ ] Pagination on list endpoints
- [ ] Appropriate status codes (201 for create, 204 for delete)
- [ ] Content-Type headers set correctly

## Testing Rules

- [ ] Unit tests for business logic
- [ ] Integration tests for handlers
- [ ] Table-driven tests for multiple cases
- [ ] Test error paths, not just happy paths
