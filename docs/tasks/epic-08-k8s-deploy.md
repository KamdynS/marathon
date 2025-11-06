## Epic 08: Kubernetes Deployment Manifests

### Purpose
Provide minimal manifests to run server and workers in K8s.

### Scope
- `Deployment` for server (`--mode=server`).
- Service, Ingress (optional), ConfigMap/Secrets.
- Job template for workers (`--mode=worker`).
- RBAC for Job creation from server.

### Acceptance Criteria
- `kubectl apply -f k8s/` starts server; `POST /scale` (optional) or CLI submits a worker Job.
- Secrets mount properly; server and workers can reach Redis/SQS.


