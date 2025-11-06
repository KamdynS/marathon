## Execution Model & Durability

### Workflows (deterministic)
- Define orchestration logic. Same inputs → same path.
- Access to `ExecuteActivity`, `Sleep`, `Now`, `WorkflowID`, `Logger`.
- Long-running logic relies on persisted timers and event replay.

### Activities (non-deterministic)
- Perform I/O (LLM calls, tools, HTTP, DB, etc.).
- Configurable timeout and retry policy (attempts, backoff, max interval).
- Must be idempotent; use idempotency keys and safe upserts.

### Events & State
```
WorkflowStarted → ActivityScheduled → ActivityStarted → ActivityCompleted/Failed → ... → WorkflowCompleted/Failed/Canceled
```
- Append-only event log with monotonic sequence numbers.
- `WorkflowState` and `ActivityState` are derived from events and stored for fast reads.

### Idempotency
- Start: caller may send `Idempotency-Key` to dedupe StartWorkflow.
- Activities: engine sets `activity_id` as idempotency key; activity implementations should be safe to retry with the same key.
- Store maintains a `keys` map to dedupe repeated Start/Activity attempts.

### At-Least-Once Delivery
- Queue delivers tasks at least once; workers may see duplicates.
- Activities must be idempotent; store dedupes `ActivityStarted`/`ActivityCompleted` by `activity_id`.

### Retries & Backoff
- Activity retry policy with exponential backoff and max attempts.
- Nack + requeue if not terminal; route to DLQ (optional) after max attempts.

### Timers & Sleep
- `Sleep(d)` schedules a durable timer; on resume/replay, engine fires `timer_fired` events and continues.
- Implemented via Store timer wheels or Queue delayed tasks; persisted across restarts.

### Cancellation
- `CancelWorkflow` sets terminal status and appends `workflow_canceled`.
- Workers should check context cancellation to stop long activities.


