## Adapters: Store and Queue

### Interfaces
```go
type Store interface {
  SaveWorkflowState(ctx context.Context, s *WorkflowState) error
  GetWorkflowState(ctx context.Context, id string) (*WorkflowState, error)
  AppendEvent(ctx context.Context, e *Event) error
  GetEvents(ctx context.Context, id string) ([]*Event, error)
  GetEventsSince(ctx context.Context, id string, since int64) ([]*Event, error)
  SaveActivityState(ctx context.Context, s *ActivityState) error
  GetActivityState(ctx context.Context, id string) (*ActivityState, error)
  ListWorkflows(ctx context.Context, status WorkflowStatus) ([]*WorkflowState, error)
  DeleteWorkflow(ctx context.Context, id string) error
}

type Queue interface {
  Enqueue(ctx context.Context, queueName string, t *Task) error
  DequeueWithTimeout(ctx context.Context, queueName string, timeout time.Duration) (*Task, error)
  Ack(ctx context.Context, queueName string, taskID string) error
  Nack(ctx context.Context, queueName string, taskID string, requeue bool) error
  Len(ctx context.Context, queueName string) (int, error)
  Close() error
}
```

### Default Implementations (local)
- InMemory Store: event log + derived state in RAM.
- InMemory Queue: channel-backed, visibility timeout simulated in worker loop.

### Production Adapters
- Redis Store: hash/maps for `WorkflowState` / `ActivityState`, list/sorted-set for events, Lua/transactions for monotonic seq & idempotency keys.
- SQS Queue: `Enqueue` → SendMessage; `Dequeue` → ReceiveMessage with visibility timeout; `Ack` → DeleteMessage; `Nack` → ChangeMessageVisibility/Requeue.

### Config Surface
- Minimal constructor config structs; no global env reliance.
- Build tags optional (e.g., `adapters_redis`, `adapters_sqs`) to keep base deps light.

### SQS Notes
- At-least-once delivery; duplicates are possible and must be handled by higher layers.
- `Len` uses `ApproximateNumberOfMessages` and is approximate by design.
- Visibility timeout controls redelivery; messages not Acked before timeout will reappear.
- Nack behavior:
  - `requeue=true` sets visibility to a configured backoff (or 0 for immediate retry).
  - `requeue=false` sets visibility to 0 so redrive policy (DLQ) can apply; optionally drop via delete.
- Optional FIFO support via `MessageGroupId` and deduplication by `task.ID`.


### Redis Store Notes
- Key prefix (default `marathon`), overridable.
- Keys:
  - Workflow state JSON: `{prefix}:wf:{id}:state`
  - Workflow events ZSET (score=sequence): `{prefix}:wf:{id}:events`
  - Workflow seq counter: `{prefix}:wf:{id}:seq` (used by Lua)
  - Activity state JSON: `{prefix}:act:{id}:state`
  - Status index SET: `{prefix}:idx:status:{status}` (members: workflow IDs)
  - Idempotency:
    - Start: `{prefix}:idem:start:{key}` → `workflow_id`
    - Activity: `{prefix}:idem:act:{activity_id}`
  - Timers:
    - Due ZSET: `{prefix}:timers:due` (member=`{workflowID}:{timerID}`, score=`fireAt ms`)
    - Record JSON: `{prefix}:timer:{workflowID}:{timerID}`
- Lua:
  - AppendEvent: INCR seq, embed `sequence_num` into event JSON, ZADD, update workflow `last_event_seq` in state JSON atomically.
  - MarkTimerFired: set `fired=true` in record and ZREM from due set atomically.

#### Construction
- First-class Redis support while keeping connection management user-owned:
  - Preferred: pass your `redis.UniversalClient` (single, cluster, or ring) via `NewFromClient(ctx, client, prefix)`. The adapter will not close it.
  - Convenience: use `New(Config)` if you want the adapter to create/manage a basic client for you.
  - Both constructors load the Lua script and cache the SHA (best-effort).


