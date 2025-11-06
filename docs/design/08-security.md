## Security & Safety (V1 minimum)

### AuthN/AuthZ
- Library is server-agnostic; integrate your middleware.
- Protect Start/Cancel endpoints; status/events can be read-scoped.

### Idempotency Keys
- Header: `Idempotency-Key` on Start.
- Activities: use `activity_id` as key; dedupe in Store.

### Rate Limiting
- Recommend reverse proxy (nginx/traefik) or gateway for rate limits.

### Secrets
- K8s: mount secrets for Redis/SQS.
- never log secrets; prefer environment variables handled by platform.


