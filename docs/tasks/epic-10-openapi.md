## Epic 10: OpenAPI Specification

### Purpose
Provide a formal API spec for Start/Status/Events/Cancel.

### Scope
- OpenAPI 3.0 yaml under `docs/api/openapi.yaml`.
- Schemas for WorkflowState, Event, Error.
- Generate examples; no codegen required yet.

### Acceptance Criteria
- `GET /health`, `POST /workflows`, `GET /workflows/{id}`, `GET /workflows/{id}/events`, `POST /workflows/{id}/cancel` defined with request/response shapes.
- Validated by `openapi-cli` or similar linter.


