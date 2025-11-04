# Marathon Simple LLM Workflow Example

This example demonstrates a basic Marathon durable workflow that calls an LLM.

## What It Does

1. Defines a workflow that makes a single LLM call
2. Starts a worker to process activities
3. Starts an API server for workflow management
4. Executes the workflow and shows the results

## Running the Example

```bash
cd examples/simple-llm
go run main.go
```

The example will:
- Start a worker and API server
- Execute a greeting workflow
- Show the workflow status and events
- Keep the server running for you to try the API

## Try the API

While the example is running, try these commands in another terminal:

### Health Check
```bash
curl http://localhost:8080/health
```

### Get Workflow Status
```bash
curl http://localhost:8080/workflows/{workflow-id}
```

### Get Workflow Events
```bash
curl http://localhost:8080/workflows/{workflow-id}/events
```

### Start a New Workflow
```bash
curl -X POST http://localhost:8080/workflows \
  -H "Content-Type: application/json" \
  -d '{"workflow_name": "greeting-workflow", "input": null}'
```

### Stream Events (SSE)
```bash
curl -H "Accept: text/event-stream" \
  http://localhost:8080/workflows/{workflow-id}/events
```

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  API Server │────│   Engine    │────│    Queue    │
└─────────────┘    └─────────────┘    └─────────────┘
                          │                    │
                          ▼                    ▼
                   ┌─────────────┐      ┌─────────────┐
                   │ State Store │      │   Worker    │
                   └─────────────┘      └─────────────┘
                                              │
                                              ▼
                                        ┌──────────────┐
                                        │  Activities  │
                                        │  (LLM Call)  │
                                        └──────────────┘
```

## Key Concepts

- **Workflow**: Defines the sequence of activities (LLM call)
- **Activity**: The actual LLM execution (mocked in this example)
- **Worker**: Polls queue and executes activities
- **Engine**: Coordinates workflow execution
- **State Store**: Persists workflow state and events (in-memory for this example)

## Next Steps

- See `agent-loop/` for a more complex example with ReAct agents
- See `multi-agent/` for parallel multi-agent workflows
- Check the main README for production deployment options

