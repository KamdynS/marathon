## Epic 04: In-Memory Defaults Polish

### Purpose
Make local dev defaults robust and predictable.

### Scope
- InMemory Queue: visibility timeout, basic DLQ (optional), metrics hooks.
- InMemory Store: thread-safety, monotonic seq, list pagination.

### Acceptance Criteria
- Tasks not acked before visibility timeout are redelivered once.
- No data races under `-race`; tests simulate concurrent operations.
- Queue length, dequeue rate exposed via hooks (no-op default).


