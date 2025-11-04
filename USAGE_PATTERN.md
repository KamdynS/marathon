# Usage Pattern

## Core Design Philosophy

This library is designed to integrate **into your existing web application**, not replace it.

## The Pattern

```
┌─────────────────────────────────────────┐
│      Your Application (main.go)        │
│                                         │
│  1. Setup (once in main())              │
│     ├─ Create engine                    │
│     ├─ Start workers                    │
│     └─ Setup your web framework         │
│                                         │
│  2. Your Routes                         │
│     ├─ POST /your/route                 │
│     │   └─ eng.StartWorkflow()          │
│     └─ GET /your/route/:id              │
│         └─ eng.GetWorkflowStatus()      │
└─────────────────────────────────────────┘
```

## Example Flow

### 1. Setup in main()

```go
func main() {
    // Create workflow infrastructure
    eng := setupWorkflowEngine()
    startWorkers(eng)
    
    // Use YOUR web framework
    router := gin.Default()
    
    // Define YOUR routes
    router.POST("/api/process", func(c *gin.Context) {
        workflowID, _ := eng.StartWorkflow(c, "my-workflow", input)
        c.JSON(202, gin.H{"id": workflowID})
    })
    
    router.Run(":8080")
}
```

### 2. User makes request to YOUR endpoint

```
POST /api/process
```

### 3. Your handler starts a workflow

```go
workflowID, _ := eng.StartWorkflow(ctx, "process-order", orderData)
```

### 4. Return immediately (async)

```go
c.JSON(202, gin.H{"workflow_id": workflowID})
```

### 5. Workers process in background

Workers poll the queue and execute activities.

### 6. User polls status from YOUR endpoint

```
GET /api/workflows/{id}
```

```go
status, _ := eng.GetWorkflowStatus(ctx, workflowID)
c.JSON(200, status)
```

## Why This Pattern?

### ✅ You Control Everything

- **Your web framework** - Gin, Echo, Chi, Fiber, net/http
- **Your routes** - Design your API however you want
- **Your middleware** - Auth, logging, CORS, rate limiting
- **Your serialization** - JSON, protobuf, msgpack
- **Your deployment** - Docker, K8s, Lambda, EC2

### ✅ Workflows Are Durable

Even though YOU control the web layer:
- Workflows survive server restarts
- State is persisted
- Workers can be separate processes
- Full event sourcing and replay

### ✅ Clean Separation

```
┌──────────────┐
│ Your Web App │ ← You control this
├──────────────┤
│ Engine       │ ← We provide this
│ Workers      │ ← We provide this
│ State Store  │ ← We provide this
└──────────────┘
```

## What About the `server/` Package?

The `server/` package is an **optional reference implementation**. You can use it if you want a quick API, but most users will:

1. Use their own web framework
2. Call `engine.StartWorkflow()` from their handlers
3. Call `engine.GetWorkflowStatus()` from their handlers

## Common Patterns

### Pattern 1: Same Process (Development)

Workers run in the same process as your web server:

```go
func main() {
    eng := setupEngine()
    go startWorkers(eng)  // Background goroutine
    
    router := gin.Default()
    // your routes...
    router.Run(":8080")
}
```

### Pattern 2: Separate Workers (Production)

Workers run in different processes/containers:

```go
// web-server/main.go
func main() {
    eng := setupEngine()
    // Don't start workers
    
    router := gin.Default()
    router.Run(":8080")
}

// worker/main.go
func main() {
    eng := setupEngine()
    startWorkers(eng)
    select {} // Keep alive
}
```

Deploy:
- Web servers scale horizontally
- Workers scale based on queue depth
- Use Redis for state, SQS for queue

### Pattern 3: Serverless (Lambda)

API runs on Lambda, workers on Lambda:

```go
// api/main.go
func handler(ctx context.Context, req events.APIGatewayProxyRequest) {
    workflowID, _ := globalEngine.StartWorkflow(ctx, "workflow", input)
    return response(202, workflowID)
}

// worker/main.go
func handler(ctx context.Context, sqsEvent events.SQSEvent) {
    // Worker processes SQS messages
}
```

## Framework Examples

### Gin

```go
router := gin.Default()
router.POST("/api/task", func(c *gin.Context) {
    workflowID, _ := eng.StartWorkflow(c, "task", input)
    c.JSON(202, gin.H{"id": workflowID})
})
```

### Echo

```go
e := echo.New()
e.POST("/api/task", func(c echo.Context) error {
    workflowID, _ := eng.StartWorkflow(c.Request().Context(), "task", input)
    return c.JSON(202, map[string]string{"id": workflowID})
})
```

### Chi

```go
r := chi.NewRouter()
r.Post("/api/task", func(w http.ResponseWriter, r *http.Request) {
    workflowID, _ := eng.StartWorkflow(r.Context(), "task", input)
    json.NewEncoder(w).Encode(map[string]string{"id": workflowID})
})
```

### net/http

```go
http.HandleFunc("/api/task", func(w http.ResponseWriter, r *http.Request) {
    workflowID, _ := eng.StartWorkflow(r.Context(), "task", input)
    json.NewEncoder(w).Encode(map[string]string{"id": workflowID})
})
http.ListenAndServe(":8080", nil)
```

## Key Takeaways

1. **Setup once** - Engine and workers initialized in `main()`
2. **Use your framework** - Gin, Echo, whatever you prefer
3. **Start workflows from handlers** - Just call `eng.StartWorkflow()`
4. **Workers run in background** - Same process or separate
5. **You own the API design** - Complete control over routes and responses

The library provides **durability and orchestration**, you provide **the web layer**.

