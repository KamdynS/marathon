# HTTP API Reference

The workflow API server provides RESTful endpoints for managing workflows.

## Base URL

```
http://localhost:8080
```

## Authentication

Currently no authentication is implemented. In production, add your own authentication middleware or use a reverse proxy.

## Endpoints

### Health Check

Check if the server is healthy.

```
GET /health
```

**Response**

```json
{
  "status": "ok"
}
```

---

### Start Workflow

Start a new workflow execution.

```
POST /workflows
```

**Request Body**

```json
{
  "workflow_name": "string",
  "input": any
}
```

**Example**

```bash
curl -X POST http://localhost:8080/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_name": "summarize-text",
    "input": {
      "text": "Long text to summarize..."
    }
  }'
```

**Response**

```json
{
  "workflow_id": "wf-1234567890"
}
```

**Status Codes**

- `200` - Workflow started successfully
- `400` - Invalid request (missing workflow_name)
- `500` - Internal error

---

### Get Workflow Status

Get the current status of a workflow.

```
GET /workflows/{workflow_id}
```

**Example**

```bash
curl http://localhost:8080/workflows/wf-1234567890
```

**Response**

```json
{
  "workflow_id": "wf-1234567890",
  "workflow_name": "summarize-text",
  "status": "completed",
  "input": {
    "text": "Long text..."
  },
  "output": {
    "summary": "Short summary..."
  },
  "start_time": "2024-01-01T12:00:00Z",
  "end_time": "2024-01-01T12:00:05Z",
  "duration": "5s"
}
```

**Status Values**

- `pending` - Workflow is queued
- `running` - Workflow is executing
- `completed` - Workflow finished successfully
- `failed` - Workflow failed (see `error` field)
- `canceled` - Workflow was canceled

**Status Codes**

- `200` - Success
- `404` - Workflow not found

---

### Get Workflow Events

Get the event log for a workflow.

```
GET /workflows/{workflow_id}/events
```

**Response (JSON)**

```bash
curl http://localhost:8080/workflows/wf-1234567890/events
```

```json
[
  {
    "id": "evt-1",
    "workflow_id": "wf-1234567890",
    "type": "workflow_started",
    "timestamp": "2024-01-01T12:00:00Z",
    "sequence_num": 1,
    "data": {
      "workflow_name": "summarize-text",
      "input": {...}
    }
  },
  {
    "id": "evt-2",
    "workflow_id": "wf-1234567890",
    "type": "activity_scheduled",
    "timestamp": "2024-01-01T12:00:01Z",
    "sequence_num": 2,
    "data": {
      "activity_id": "act-123",
      "activity_name": "llm-call"
    }
  }
]
```

**Event Types**

- `workflow_started`
- `workflow_completed`
- `workflow_failed`
- `workflow_canceled`
- `activity_scheduled`
- `activity_started`
- `activity_completed`
- `activity_failed`
- `activity_retrying`
- `timer_scheduled`
- `timer_fired`
- `signal_received`

---

### Stream Workflow Events (SSE)

Long-running workflows should use 202 + SSE instead of blocking requests.

```
GET /workflows/{workflow_id}/events
Accept: text/event-stream
```

Supports `Last-Event-ID` for resume. Server emits:

```
id: <sequence>
event: <type>
data: {json event}

: keep-alive
```

**Example**

```bash
curl -N -H "Accept: text/event-stream" \
  http://localhost:8080/workflows/wf-1234567890/events
```

See `server/agenthttp/sse.go` for a reusable helper.

---

### Cancel Workflow

Cancel a running workflow.

```
POST /workflows/{workflow_id}/cancel
```

**Example**

```bash
curl -X POST http://localhost:8080/workflows/wf-1234567890/cancel
```

**Response**

`204 No Content` on success

**Status Codes**

- `204` - Workflow canceled successfully
- `404` - Workflow not found
- `500` - Internal error (e.g., workflow already completed)

---

## Error Responses

All errors return a JSON object:

```json
{
  "error": "human-readable error message"
}
```

**Common Error Codes**

- `400` - Bad Request (invalid input)
- `404` - Not Found (workflow doesn't exist)
- `405` - Method Not Allowed
- `500` - Internal Server Error

## Rate Limiting

Not currently implemented. Consider adding a reverse proxy (nginx, traefik) for rate limiting in production.

## CORS

CORS is not enabled by default. Configure it in your application or reverse proxy.

## Webhook Integration

To get notified when workflows complete, poll the status endpoint or use the SSE stream. Webhook support is planned for a future release.

## SDK Usage

Instead of using HTTP directly, you can use the Go SDK:

```go
// Start workflow
workflowID, err := engine.StartWorkflow(ctx, "workflow-name", input)

// Get status
status, err := engine.GetWorkflowStatus(ctx, workflowID)

// Get events
events, err := engine.GetWorkflowEvents(ctx, workflowID)

// Cancel
err = engine.CancelWorkflow(ctx, workflowID)
```

## OpenAPI Specification

Coming soon: Full OpenAPI 3.0 spec for API documentation and code generation.

## Versioning

The API is currently unversioned. Once stable, we'll use `/v1/` prefix for versioned endpoints.

