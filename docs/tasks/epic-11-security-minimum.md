## Epic 11: Security Minimums

### Purpose
Ship sensible defaults without over-engineering.

### Scope
- Idempotency-Key enforcement for Start.
- Document auth middleware integration points.
- Rate limit guidance (proxy-level), secret management notes.

### Acceptance Criteria
- Requests with the same Idempotency-Key return the same workflow_id.
- Docs show where to plug auth/rate-limit into existing server.


