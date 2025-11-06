## Kubernetes Jobs Runner (Design)

### Goal
Allow the server (same binary) to spawn worker capacity in-cluster via `Job` objects. Workers execute activities by polling SQS and writing state to Redis.

### Container & Entry Points
- Single image: `/app` supports `--mode=server` and `--mode=worker`.
- Server runs HTTP + Engine; Worker runs the worker pool.

### Job Spec (conceptual)
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: marathon-worker-<ts>
spec:
  parallelism: 5              # 5 pods (each can run N concurrent tasks)
  ttlSecondsAfterFinished: 60
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: ghcr.io/you/marathon:TAG
        args: ["--mode=worker", "--queue=default", "--max-concurrent=10"]
        env:
        - name: REDIS_ADDR
          valueFrom: { secretKeyRef: { name: redis, key: addr } }
        - name: SQS_QUEUE_URL
          valueFrom: { secretKeyRef: { name: sqs, key: url } }
```

### Server-Side Submission
- Use client-go; minimal RBAC for `create/get/list` Jobs in a namespace.
- Label Jobs with `app=marathon, role=worker, queue=<name>` for tracking.
- Optional: Reconcile desired worker count from queue depth, or integrate with KEDA.

### Lifecycle
1) Server observes queue depth or receives a scale request.
2) Server submits a Job; K8s schedules worker Pods.
3) Pods start, connect to SQS/Redis, execute activities, exit when idle/timeout.
4) TTL cleans up Job resources.

### Configuration
- Max concurrent per Pod, parallelism per Job, idle timeout, queue name.
- Image tag, service account, namespace.

### Failure Modes
- Pod OOM/crash → Job backoffLimit governs retries; SQS visibility timeout redelivers.
- Redis outage → workers fail fast; engine cannot proceed (expected). Restart when Redis recovers.


