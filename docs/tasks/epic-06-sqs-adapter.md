## Epic 06: SQS Queue Adapter

### Purpose
At-least-once task delivery via SQS.

### Scope
- Enqueue → SendMessage with group/attributes.
- DequeueWithTimeout → ReceiveMessage with VisibilityTimeout.
- Ack → DeleteMessage; Nack → ChangeMessageVisibility or re-send.
- Optional DLQ via redrive policy (infra-level).

### Acceptance Criteria
- Tasks delivered at least once; duplicates possible by design.
- Visibility timeout respected; unacked tasks reappear.
- Backpressure and batch sizes configurable.

### Tech Notes
- Map `Task` to message body JSON; store Attempts in attributes.


