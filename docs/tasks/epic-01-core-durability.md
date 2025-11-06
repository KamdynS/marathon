## Epic 01: Core Durability & Idempotency

### Purpose
Lock down correctness: idempotent Start/Activities, durable timers, clean replay.

### Scope
- StartWorkflow idempotency keys.
- Activity idempotency + dedupe in Store.
- Durable timers (`Sleep`) across restarts.
- Event seq monotonicity & atomic append.

### Acceptance Criteria
- StartWorkflow with same Idempotency-Key returns same workflow_id and does not duplicate events.
- Activity with same `activity_id` does not duplicate `ActivityStarted/Completed` events.
- Kill/restart engine and worker: timers still fire at/after intended times; workflow resumes.
- Event sequence numbers strictly increase; concurrent appends are serialized.
- Tests cover crashes during activity execution and timer windows.

### Tech Notes
- Store: maintain `idempotency:<key> → workflow_id` and `activity_dedupe:<activity_id>`.
- Timers: persisted schedule list (sorted set) + worker that enqueues `timer_fired` tasks.


