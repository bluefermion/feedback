# Prime Directives — Non-Negotiable Rules

These rules override all other instructions and are learned from production incidents across Blue Fermion Labs projects.

## Universal Safety Rules

### Deployment Safety
- **NEVER** deploy to production without explicit user permission
- **NEVER** run multiple deployments in a single request — stop and wait for confirmation
- Git is for **version control only** — deployment happens via `make prod` or explicit commands
- Always ask: "Should I deploy to dev or production?"

### Data Safety
- **NEVER** delete database tables directly
- Safe migration pattern: CREATE AS SELECT → QA → DELETE old → RENAME new
- No PII in logs at INFO/WARN/ERROR levels — DEBUG only
- Parameterized queries only — never string concatenation for SQL

### Code Safety
- **NEVER** run `git checkout .` or `git reset --hard` without explicit permission
- **NEVER** commit secrets, API keys, or credentials
- **ALWAYS** run `go fmt` and `go vet` before commits — vet MUST pass

## Incident Log

| Date | Issue | Root Cause | Prevention |
|------|-------|------------|------------|
| — | Accidental deployment | Auto-ran `make prod` | Always ask user first |
| — | Lost uncommitted work | Ran `git checkout .` | Never without permission |
| — | Data loss | Deleted table directly | Use safe migration pattern |
| — | Route 404 | Middleware whitelist missing | Check all middleware layers |
| — | Template crash | Struct field mismatch | Verify template-struct alignment |
| — | Secrets leaked | Logged auth token | Only log PII at DEBUG level |

## Enforcement

When in doubt:
1. Ask the user before destructive operations
2. Prefer safe defaults
3. Log decisions for auditability
