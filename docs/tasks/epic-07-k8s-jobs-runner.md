## Epic 07: Kubernetes Jobs Runner

### Purpose
Server can scale workers by submitting K8s Jobs using client-go.

### Scope
- CLI flags: `--mode=server|worker`, `--queue`, `--max-concurrent`.
- Server-side Job submitter library with typed Job spec builder.
- RBAC, namespace selection, labels/owner references.

### Acceptance Criteria
- From server, create a Job that spawns N worker Pods; each Pod runs worker mode and processes tasks.
- Jobs are cleaned by TTL; backoffLimit=0 avoids retries; failures are visible.
- E2E test (kind or fake client) validates Job creation request shape.

### Tech Notes
- Keep dependency footprint minimal; gate client-go usage behind a build tag if needed.


