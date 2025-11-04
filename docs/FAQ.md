# Frequently Asked Questions

## General

### What is this project?

Marathon is a Temporal-inspired durable workflow orchestration system specifically designed for AI agents and LLM workloads in Go.

### Why not just use Temporal?

Temporal is excellent but:
- Complex to deploy and operate
- Not AI-specific
- Requires Cassandra/PostgreSQL
- Steeper learning curve

This project is simpler, AI-focused, and works with no external dependencies (in-memory defaults).

### Is this production-ready?

The core system is functional and tested, but it's new. Use with caution in production. Redis and SQS adapters are still in development.

### What's the performance like?

- **In-memory**: ~100ms latency for simple workflows
- **Redis/SQS**: ~500ms-1s latency
- **Throughput**: Thousands of workflows/second with proper infrastructure

## Architecture

### How does durability work?

We use event sourcing. Every state change is recorded as an event in the state store. If a process crashes, we can replay events to reconstruct state.

### What happens if a worker crashes?

The task returns to the queue after a visibility timeout. Another worker picks it up and retries the activity.

### Can workflows span multiple days?

Yes! Workflows can run indefinitely. State is persisted and workers can be restarted without losing progress.

### How do you handle retries?

Each activity has a retry policy with exponential backoff. Failed activities are automatically retried until max attempts or success.

## Development

### Do I need Redis/SQS to develop?

No! Use in-memory implementations for development:

```go
store := state.NewInMemoryStore()
queue := queue.NewInMemoryQueue()
```

### How do I test workflows?

Write unit tests for activities, then integration tests for workflows:

```go
// Example test
func TestMyWorkflow(t *testing.T) { /* ... */ }
```

## Operations

### How do I scale workers?

Increase worker replicas or processes. Workers are stateless and safe to scale horizontally.

### How do I monitor workflows?

Use the event log from the state store and add metrics/tracing where needed.

## Getting Help

1. Read the [documentation](.)
2. Search [GitHub Issues](https://github.com/KamdynS/marathon/issues)
3. Ask in [GitHub Discussions](https://github.com/KamdynS/marathon/discussions)
4. File a bug report

