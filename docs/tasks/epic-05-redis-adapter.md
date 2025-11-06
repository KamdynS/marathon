## Epic 05: Redis Store Adapter

### Purpose
Durable Store backed by Redis for production.

### Scope
- Keying: `wf:<id>:state`, `wf:<id>:events` (ZSET by seq), `act:<id>:state`, `idx:status:<status>`.
- Idempotency keys: `idem:start:<key> → workflow_id`, `idem:act:<act_id>`.
- Atomic appends (LUA) to enforce monotonic sequence.

### Acceptance Criteria
- Save/Get state works; events append and read in order.
- `GetEventsSince` efficient via ZRANGEBYSCORE with seq.
- Idempotent Start returns same workflow_id without duplicate events.
- Concurrency-safe under load tests.

### Tech Notes
- Use pipelines/transactions where helpful; prefer LUA for seq increment + append.


