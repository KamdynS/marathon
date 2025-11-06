## Runtime Modes

### 1) Local Single Binary (default)
- Store: InMemory, Queue: InMemory.
- Workers: goroutines in the same process.
- Great DX, lowest latency, simple debugging.

### 2) Split Processes (same infra)
- Server process runs API and Engine.
- Worker process runs workers only (same codebase, different entrypoint/flag).
- Store/Queue can still be in-memory for dev or Redis/SQS for prod.

### 3) Kubernetes (first external target)
- Server runs as a Deployment.
- Server submits `Job` objects (same container image) to spawn worker Pods.
- Workers connect to SQS/Redis via env/secret.
- Scale workers by queue depth (HPA on custom metrics or KEDA).

#### Single Binary → K8s Jobs
- Build a single image with both modes, gated by `--mode=server|worker`.
- Server process (in-cluster) uses client-go to create Jobs whose `command: ["/app", "--mode=worker"]`.
- Job spec sets `parallelism` for number of concurrent workers and `backoffLimit`.
- Each worker Pod runs N concurrent activities (configurable), polling SQS.


