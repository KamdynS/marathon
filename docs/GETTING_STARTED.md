# Getting Started with Marathon

This guide shows you how to integrate Marathon durable workflows into your web application.

## Prerequisites

- Go 1.21 or later
- A web framework (Gin, Echo, or net/http)
- Basic understanding of Go

## Installation

```bash
go get github.com/KamdynS/marathon
```

## The Pattern

The typical pattern for using Marathon durable workflows:

1. **Setup once in `main()`**: Create engine and start workers
2. **Use your own web server**: Gin, Echo, Chi, net/http, etc.
3. **Start workflows from handlers**: Call `engine.StartWorkflow()` in your routes
4. **Poll status from other handlers**: Check workflow progress

## Your First Workflow

Let's build an API that summarizes text using a durable LLM workflow.

### Step 1: Define an Activity

Activities are the actual work units. Let's create an LLM activity:

```go
package main

import (
    "context"
    "github.com/KamdynS/marathon/activity"
    "github.com/KamdynS/go-agents/llm"
    "github.com/KamdynS/go-agents/llm/openai"
)

func setupActivities() *activity.Registry {
    registry := activity.NewRegistry()
    
    // Create LLM client (use your actual implementation)
    llmClient, _ := openai.NewClient(openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  llm.ModelGPT4o,
    })
    
    // Wrap as activity
    llmActivity := activity.NewLLMActivity(llmClient)
    
    // Register
    registry.Register("llm-call", llmActivity, activity.Info{
        Description: "Call OpenAI API",
        Timeout:     30 * time.Second,
    })
    
    return registry
}
```

### Step 2: Define a Workflow

Workflows orchestrate activities:

```go
func setupWorkflows() *workflow.Registry {
    registry := workflow.NewRegistry()
    
    // Define workflow
    summarizeWorkflow := workflow.New("summarize-text").
        Description("Summarize a piece of text").
        TaskQueue("default").
        Activity("llm-call", activity.ChatRequest{
            Messages: []activity.Message{
                {
                    Role:    "system",
                    Content: "You are a helpful assistant that summarizes text.",
                },
                {
                    Role:    "user",
                    Content: "Summarize the following...",
                },
            },
        }).
        Build()
    
    registry.Register(summarizeWorkflow)
    
    return registry
}
```

### Step 3: Setup Engine and Workers

Create the engine and start workers in `main()`:

```go
func main() {
    // 1. Setup workflow infrastructure (background)
    eng := setupWorkflowEngine()
    startWorkers(eng)
    
    // 2. Your web server
    router := gin.Default()
    
    // 3. Your routes (see next step)
    // ...
    
    router.Run(":8080")
}

func setupWorkflowEngine() *engine.Engine {
    // Create infrastructure
    stateStore := state.NewInMemoryStore()
    taskQueue := queue.NewInMemoryQueue()
    
    // Register workflows
    workflowRegistry := setupWorkflows()
    
    // Create engine
    eng, _ := engine.New(engine.Config{
        StateStore:       stateStore,
        Queue:            taskQueue,
        WorkflowRegistry: workflowRegistry,
    })
    
    return eng
}

func startWorkers(eng *engine.Engine) {
    activityRegistry := setupActivities()
    
    w, _ := worker.New(worker.Config{
        Queue:            eng.Queue,
        QueueName:        "default",
        ActivityRegistry: activityRegistry,
        StateStore:       eng.StateStore,
        MaxConcurrent:    5,
    })
    
    go w.Start(context.Background())
}
```

### Step 4: Create Your Routes

Add routes that start and check workflows:

```go
// Start a workflow from a handler
router.POST("/api/summarize", func(c *gin.Context) {
    var req struct {
        Text string `json:"text"`
    }
    c.BindJSON(&req)
    
    // Start durable workflow
    workflowID, err := eng.StartWorkflow(c.Request.Context(), 
        "summarize-text", req)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // Return workflow ID immediately (async)
    c.JSON(202, gin.H{
        "workflow_id": workflowID,
        "status_url":  fmt.Sprintf("/api/workflows/%s", workflowID),
    })
})

// Check workflow status from another handler
router.GET("/api/workflows/:id", func(c *gin.Context) {
    workflowID := c.Param("id")
    
    status, err := eng.GetWorkflowStatus(c.Request.Context(), workflowID)
    if err != nil {
        c.JSON(404, gin.H{"error": "not found"})
        return
    }
    
    c.JSON(200, gin.H{
        "workflow_id": status.WorkflowID,
        "status":      status.Status,
        "output":      status.Output,
        "error":       status.Error,
        "duration":    status.Duration().String(),
    })
})
```

## Running the Example

```bash
go run main.go
```

Then in another terminal:

```bash
# Start a workflow
curl -X POST http://localhost:8080/api/summarize \
  -H "Content-Type: application/json" \
  -d '{"text": "Long article..."}'

# Response: {"workflow_id": "wf-123", "status_url": "/api/workflows/wf-123"}

# Check status
curl http://localhost:8080/api/workflows/wf-123

# Response: {"workflow_id": "wf-123", "status": "completed", "output": {...}}
```

## Next Steps

### Add More Activities

Create custom activities for your use case:

```go
type MyCustomActivity struct {
    // your dependencies
}

func (a *MyCustomActivity) Name() string {
    return "my-activity"
}

func (a *MyCustomActivity) Execute(ctx context.Context, input interface{}) (interface{}, error) {
    // your logic here
    return result, nil
}

// Register it
registry.Register("my-activity", &MyCustomActivity{}, activity.Info{
    Timeout: 10 * time.Second,
})
```

### Use with HTTP API

Add the API server for remote workflow management:

```go
srv, err := server.New(server.Config{
    Engine: eng,
    Port:   8080,
})

go srv.Start()

// Now you can use HTTP API
// curl -X POST http://localhost:8080/workflows \
//   -d '{"workflow_name": "summarize-text", "input": {...}}'
```

### Parallel Execution

Execute multiple activities in parallel:

```go
workflow := workflow.New("parallel-workflow").
    Parallel(
        workflow.ActivityStep{ActivityName: "task1", Input: input1},
        workflow.ActivityStep{ActivityName: "task2", Input: input2},
        workflow.ActivityStep{ActivityName: "task3", Input: input3},
    ).
    Build()
```

### Error Handling

Configure retry policies:

```go
registry.Register("flaky-activity", activity, activity.Info{
    Timeout: 10 * time.Second,
    RetryPolicy: &activity.RetryPolicy{
        MaxAttempts:        5,
        InitialInterval:    time.Second,
        BackoffCoefficient: 2.0,
        MaxInterval:        time.Minute,
    },
})
```

### Production Deployment

For production, use external state and queue:

```go
// +build adapters_redis,adapters_sqs

import (
    "github.com/KamdynS/marathon/adapters/redis"
    "github.com/KamdynS/marathon/adapters/sqs"
)

stateStore := redis.NewStateStore(redis.Config{
    Addr: "redis:6379",
})

taskQueue := sqs.NewQueue(sqs.Config{
    QueueURL: "https://sqs.us-east-1.amazonaws.com/...",
})
```

Build with tags:
```bash
go build -tags adapters_redis,adapters_sqs
```

## Common Patterns

### ReAct Agent Loop

```go
workflow := workflow.New("react-agent").
    Activity("plan", planInput).      // LLM plans action
    Activity("execute", toolInput).   // Execute tool
    Activity("reflect", reflectInput). // LLM reflects
    Build()
```

### Multi-Agent Collaboration

```go
workflow := workflow.New("multi-agent").
    Parallel(
        agentStep("researcher", researchQuery),
        agentStep("analyst", analysisQuery),
        agentStep("writer", writingQuery),
    ).
    Activity("synthesize", synthesisInput). // Combine results
    Build()
```

### Long-Running Tasks

```go
workflow := workflow.New("batch-processing").
    Timeout(24 * time.Hour).
    Activity("process-batch", batchInput).
    Build()
```

## Troubleshooting

### Workflow Not Starting

Check:
- Workflow is registered: `workflow.Register(def)`
- Engine is configured correctly
- Worker is running

### Activity Timing Out

Increase timeout:
```go
activity.Info{
    Timeout: 60 * time.Second,
}
```

### Worker Not Processing

Check:
- Worker is started: `worker.Start(ctx)`
- Queue name matches workflow task queue
- Activity is registered in worker's registry

## Testing and Coverage

We prefer table-driven tests and aim for 80%+ coverage.

Run the suite:

```bash
go test ./... -cover
```

Generate a coverage report:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## Resources

- [Architecture](ARCHITECTURE.md) - Deep dive into system design
- [API Reference](API.md) - HTTP API documentation
- [Examples](../examples/) - Complete working examples
- [Deployment](../deploy/) - Docker, K8s, and Terraform guides

