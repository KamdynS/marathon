## Architecture Overview

High-level view, optimized for local DX with a clean path to Redis+SQS+K8s Jobs.

```
┌──────────────────────────────────────────────────────────────────┐
│                      Your Web Application                        │
│  Routes/Handlers ─── calls Engine.StartWorkflow()                │
│  SSE endpoint     ─── streams events from Store                  │
└───────────────┬──────────────────────────────────────────────────┘
                │
                ▼
┌──────────────────────────────────────────────────────────────────┐
│                            Engine                                 │
│  - Event-sourced state (Store)                                    │
│  - Schedules Activity tasks to Queue                              │
│  - Deterministic workflow context                                 │
│  - Idempotent Start/Activities                                    │
└───────────────┬───────────────────────────┬──────────────────────┘
                │                           │
                ▼                           ▼
         ┌─────────────┐             ┌─────────────┐
         │  Store      │             │   Queue     │
         │ InMemory    │             │ InMemory    │   (local default)
         │ Redis (prod)│             │ SQS (prod)  │
         └─────────────┘             └─────────────┘
                ▲                           │
                │                           ▼
                │                    ┌───────────────┐
                │                    │ Workers       │
                │                    │ - Local go-rs │ (default)
                │                    │ - K8s Jobs    │ (prod)
                │                    └───────────────┘
                │                           │
                └───────────────────────────▼
                                     Activities
                               (LLM calls, tools, I/O)
```

### Data Flow
1) Handler starts a workflow with input. Engine persists `WorkflowStarted` and initial state.
2) Engine evaluates workflow, enqueues `Activity` tasks to the Queue.
3) Workers poll the queue, execute activities, update `Activity*` state/events.
4) Engine completes the workflow on final step, writes `WorkflowCompleted/Failed`.
5) SSE endpoint streams events from Store (JSON, Last-Event-ID supported).

### Durability & Semantics
- Event sourcing for auditability and replay.
- At-least-once delivery from Queue to Workers.
- Idempotency keys for workflow start and activities to prevent duplicates.
- Exponential backoff retries with policy per activity.

### Runtime Modes
- Local (single binary): in-memory Store/Queue, in-process workers.
- Split (two processes): same Store/Queue, workers run separately.
- K8s: server submits Jobs to spawn worker Pods (same image), workers poll SQS.


