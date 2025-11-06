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


