## Roadmap (focused for V1)

### V1 (foundations)
- Idempotency (Start + Activities), durable timers, replay guarantees.
- Agent Loop (Full): tools + LLM router, streaming, retries, stop conditions.
- SSE v1 with resume and heartbeats.
- Local DX: single binary with in-process workers.

### V1.1 (production adapters)
- Redis Store adapter.
- SQS Queue adapter.

### V1.2 (K8s Jobs)
- K8s Jobs runner + minimal deploy manifests.

### Later
- Signals/Queries; Child workflows.
- Observability hooks & docs.
- More examples; migration guides.


