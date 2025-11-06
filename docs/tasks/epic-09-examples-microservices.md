## Epic 09: Example Microservices

### Purpose
Deliver two end-to-end examples showcasing best practices.

### Scope
- ReAct Agent microservice.
- Multi-Agent microservice.
- Each with `cmd/api`, `cmd/worker`, Dockerfile, k8s templates.

### Acceptance Criteria
- `docker compose up` runs local examples with in-memory defaults.
- K8s manifests applied result in a working demo (Redis+SQS stubs optional).
- Readme with `curl` commands and SSE demo.


