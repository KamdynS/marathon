## Vision & Principles

Marathon is a Temporal-inspired library for durable AI workflows in Go. It is library-first, server-agnostic, and focused on LLM/agent workloads. Users keep their existing web servers and infra, and add durable workflows with minimal glue.

### Core Principles
- Opinionated defaults, pluggable adapters.
- Single-binary DX locally; easy swap to external infra.
- Event-sourced durability; deterministic workflows, non-deterministic activities.
- At-least-once delivery; idempotency first.
- First-class SSE for long-running UX.
- K8s Jobs as the first external worker fabric.
- Production adapters: Redis (state) + SQS (queue).

### Target Developer Experience
- Start a normal web server, initialize the engine once, start local workers in-process.
- Start workflows from handlers; poll or stream updates via SSE.
- When ready for production: switch state to Redis, queue to SQS, and enable K8s Jobs runner to spawn ephemeral worker Pods per demand.

### What We Are Not
- We do not provide a general-purpose database. Your application owns long-term data. We persist workflow state/events, not business records.
- We do not embed a web framework or enforce CORS/auth. Integrate with your server.

### Non-Goals (V1)
- Cross-cluster replication and advanced multi-tenancy.
- Full Temporal parity (signals/queries/child workflows can come later).
- Deep observability out of the box (hooks reserved, docs later).


