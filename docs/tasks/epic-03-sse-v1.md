## Epic 03: SSE v1

### Purpose
Deliver first-class streaming for workflow events with resume support.

### Scope
- Library subscription API by workflow id and since-seq.
- HTTP SSE helper with `Last-Event-ID` and heartbeats.
- Docs and examples.

### Acceptance Criteria
- Client can start stream mid-flight and receive only new events.
- On terminal events, stream emits `event: done` then closes.
- Heartbeat every 15s; disconnects detected within 30s.

### Tech Notes
- Use `GetEventsSince` and maintain last seq per client.


