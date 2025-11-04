# Marathon Architecture

This document describes the architecture of the Marathon durable workflow system.

## Overview

The system is designed around **event sourcing** and **distributed task execution**, specifically optimized for AI workloads (LLM calls, tool execution, agentic loops).

## High-Level Architecture

```
┌──────────────────────────────────────────────────────────┐
│                   Client Application                      │
│  - Defines workflows and activities                       │
│  - Starts workflows via API or SDK                        │
└────────────────────┬─────────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────────┐
│                 Workflow API Server                       │
│  POST /workflows        - Start workflow                  │
│  GET  /workflows/{id}   - Get status                     │
│  GET  /workflows/{id}/events - Stream events (SSE)       │
└────────────────────┬─────────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────────┐
│                 Workflow Engine                           │
│  - Orchestrates workflow execution                        │
│  - Manages state machine                                  │
│  - Schedules activities                                   │
│  - Handles retries and failures                           │
└────┬─────────────┬─────────────────┬────────────────────┘
     │             │                 │
     ▼             ▼                 ▼
┌─────────┐  ┌──────────┐     ┌────────────────┐
│  State  │  │  Queue   │     │ Worker Pool    │
│  Store  │  │          │     │                │
│ (Redis) │  │  (SQS)   │     │ (K8s/Lambda)   │
└─────────┘  └──────────┘     └────────────────┘
     ▲                               │
     │                               ▼
     │                        ┌──────────────┐
     └────────────────────────│  Activities  │
                              │ - LLM calls  │
                              │ - Tool exec  │
                              └──────────────┘
```

## Core Components

### 1. Workflow Definition

Workflows are defined using a builder pattern:

```go
workflow := workflow.New("my-workflow").
    Activity("step1", input1).
    Activity("step2", input2).
    Build()
```

**Key Properties:**
- Deterministic: Same input always produces same execution path
- Composable: Can include linear, parallel, and branching logic
- Versioned: Each workflow has a version for evolution

### 2. Activity

Activities are the actual work units:

```go
type Activity interface {
    Name() string
    Execute(ctx context.Context, input interface{}) (interface{}, error)
}
```

**Built-in Activities:**
- `LLMActivity`: Wraps LLM client for chat completions
- `ToolActivity`: Wraps tools for execution
- Custom: Users can implement their own

**Properties:**
- Non-deterministic: Can have side effects
- Retryable: Automatic retry with exponential backoff
- Timeout-bound: Each activity has execution timeout

### 3. Workflow Engine

The engine coordinates workflow execution:

```go
engine.StartWorkflow(ctx, "workflow-name", input)
```

**Responsibilities:**
1. Instantiate workflow execution context
2. Schedule activities to task queue
3. Track workflow state
4. Handle workflow completion/failure
5. Enable workflow cancellation

**State Machine:**
```
Pending → Running → [Completed | Failed | Canceled]
```

### 4. Worker Pool

Workers poll tasks and execute activities:

```go
worker.New(Config{
    Queue:            queue,
    ActivityRegistry: registry,
    StateStore:       store,
    MaxConcurrent:    10,
})
```

**Execution Flow:**
1. Poll task from queue
2. Retrieve activity from registry
3. Execute activity with timeout
4. Update state store with result
5. Record event log
6. Ack/Nack task

### 5. State Store

Persists workflow state using event sourcing:

```go
type Store interface {
    SaveWorkflowState(ctx, state)
    GetWorkflowState(ctx, workflowID)
    AppendEvent(ctx, event)
    GetEvents(ctx, workflowID)
}
```

**Event Types:**
- `WorkflowStarted`
- `ActivityScheduled`
- `ActivityStarted`
- `ActivityCompleted`
- `ActivityFailed`
- `WorkflowCompleted`
- `WorkflowFailed`
- `WorkflowCanceled`

**Why Event Sourcing?**
- Complete audit trail
- Enables replay for recovery
- Supports time travel debugging
- Natural fit for distributed systems

### 6. Task Queue

Distributes work to workers:

```go
type Queue interface {
    Enqueue(ctx, queueName, task)
    Dequeue(ctx, queueName) (*Task, error)
    Ack(ctx, queueName, taskID)
    Nack(ctx, queueName, taskID, requeue bool)
}
```

**Properties:**
- FIFO ordering
- At-least-once delivery
- Visibility timeout for task safety
- Dead letter queue for failed tasks

## Data Flow

### Starting a Workflow

```
Client → API Server → Engine
                        │
                        ├─→ Create WorkflowState
                        ├─→ Append WorkflowStarted event
                        ├─→ Execute workflow function
                        └─→ Schedule activities to queue
```

### Processing an Activity

```
Queue ← Activity Task
  │
  ▼
Worker polls task
  │
  ├─→ Update ActivityState (running)
  ├─→ Append ActivityStarted event
  ├─→ Execute activity
  ├─→ Update ActivityState (completed/failed)
  ├─→ Append ActivityCompleted/Failed event
  └─→ Ack/Nack task
```

### Completing a Workflow

```
All activities done
  │
  ▼
Engine detects completion
  │
  ├─→ Update WorkflowState (completed)
  ├─→ Append WorkflowCompleted event
  └─→ Return final output
```

## Durability Guarantees

### Workflow Crashes

If the workflow execution process crashes:
1. State is already persisted in state store
2. Engine can be restarted
3. Running workflows continue from last checkpoint
4. Activities already in queue are safe

### Worker Crashes

If a worker crashes mid-activity:
1. Task visibility timeout expires
2. Task returns to queue
3. Another worker picks it up
4. Activity retries with exponential backoff
5. Event log tracks all attempts

### State Store Failures

If Redis/state store fails:
1. Workflows cannot proceed (by design)
2. All state is in durable storage
3. Once store recovers, system resumes
4. No data loss (assuming persistent storage)

## Scalability

### Horizontal Scaling

- **API Servers**: Stateless, can scale infinitely
- **Workers**: Scale based on queue depth
- **State Store**: Use Redis Cluster for large scale
- **Queue**: SQS supports unlimited throughput

### Performance Characteristics

- **Workflow Throughput**: Limited by state store writes
- **Activity Throughput**: Limited by worker count
- **Latency**: ~100ms for simple workflows (in-memory)
- **Latency**: ~500ms-1s for workflows with external state/queue

## Design Decisions

### Why Event Sourcing?

1. **Auditability**: Full history of what happened
2. **Debuggability**: Can replay to understand failures
3. **Resilience**: Can reconstruct state from events
4. **Temporal queries**: "What was state at time X?"

### Why Separate Workers?

1. **Isolation**: Activity failures don't affect orchestration
2. **Scalability**: Scale workers independently of API
3. **Flexibility**: Run workers on different infrastructure
4. **Resource control**: CPU/memory limits per worker

### Why Task Queue?

1. **Decoupling**: Engine doesn't need to know about workers
2. **Reliability**: At-least-once delivery guarantees
3. **Load leveling**: Queue absorbs traffic spikes
4. **Priority**: Can prioritize urgent workflows

## Comparison with Temporal

| Feature | Durable Workflows | Temporal |
|---------|-------------------|----------|
| Language | Go only | Multi-language |
| Deployment | Simple (single binary) | Complex (multiple services) |
| Learning curve | Low | High |
| AI-specific features | LLM activities, tool execution | Generic |
| Default dependencies | None (in-memory) | Requires Cassandra/PostgreSQL |
| Workflow versioning | Planned | Full support |
| Child workflows | Planned | Full support |
| Signal/Query | Partial | Full support |
| Multi-cluster | No | Yes |

## Future Enhancements

### Workflow Signals

Allow external events to influence running workflows:

```go
engine.SignalWorkflow(workflowID, "user_approved", data)
```

### Workflow Queries

Query workflow state without side effects:

```go
result := engine.QueryWorkflow(workflowID, "get_current_step")
```

### Child Workflows

Workflows can start sub-workflows:

```go
childID := ctx.ExecuteChildWorkflow("child-workflow", input)
```

### Workflow Search

Query workflows by status, metadata, etc:

```go
results := engine.SearchWorkflows(SearchQuery{
    Status: StatusRunning,
    WorkflowName: "my-workflow",
})
```

## Best Practices

### Workflow Design

1. Keep workflows deterministic
2. Use activities for non-deterministic operations
3. Design for idempotency
4. Handle failures gracefully

### Activity Design

1. Make activities idempotent
2. Use appropriate timeouts
3. Return structured errors
4. Don't access external state directly

### Operational

1. Monitor queue depth
2. Set up alerting on failed workflows
3. Use retry policies appropriate for your workload
4. Archive completed workflows periodically

## Troubleshooting

### Workflow Stuck

1. Check worker logs for activity failures
2. Verify queue is being consumed
3. Check state store for events
4. Look for deadlocks in workflow logic

### High Latency

1. Check state store latency
2. Monitor worker CPU/memory
3. Look for slow activities
4. Consider increasing worker pool size

### Data Loss

- Should never happen if state store is durable
- Check backup/replication of state store
- Verify event log is complete

