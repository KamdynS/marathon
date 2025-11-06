## SSE: First-Class Support

### Requirements
- Long-running workflows must not block requests.
- Clients can resume streams using `Last-Event-ID` or sequence number.
- Heartbeats keep connections alive and detect disconnects.

### Event Shape
```json
{
  "id": "evt-123",
  "workflow_id": "wf-1",
  "type": "activity_completed",
  "timestamp": "2025-01-01T00:00:00Z",
  "sequence_num": 42,
  "data": { "activity_id": "act-1" }
}
```

### Transport
- HTTP headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`.
- Messages:
```
id: <sequence_num>
event: <type>
data: {json}

: ping
```
- On terminal events, emit `event: done` and close.

### Library-Level Subscription
- Provide a subscription API that yields new events since a given sequence.
- HTTP layer is a thin wrapper for framework-agnostic usage.


