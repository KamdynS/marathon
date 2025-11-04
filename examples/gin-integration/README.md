# Marathon + Gin Integration Example

This example shows how to integrate Marathon durable workflows with your own Gin web server.

## Pattern

The key pattern is:

1. **Setup in `main()`**: Create engine and start workers
2. **Your routes**: Use your own web framework (Gin, Echo, net/http, etc.)
3. **Start workflows in handlers**: Call `engine.StartWorkflow()` from HTTP handlers

## Architecture

```
┌─────────────────────────────────────────┐
│         Your Gin Application            │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │  POST /api/summarize             │  │
│  │  → eng.StartWorkflow()           │  │
│  │  ← Returns workflow_id           │  │
│  └──────────────────────────────────┘  │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │  GET /api/workflows/:id          │  │
│  │  → eng.GetWorkflowStatus()       │  │
│  │  ← Returns status, output        │  │
│  └──────────────────────────────────┘  │
└─────────────────────────────────────────┘
              ↓
    ┌─────────────────┐
    │ Workflow Engine │
    └─────────────────┘
              ↓
    ┌─────────────────┐
    │  Worker Pool    │
    │  (background)   │
    └─────────────────┘
```

## Running

```bash
# Install Gin
go get github.com/gin-gonic/gin

# Run the example
go run main.go
```

## Try It

### Start a summarization workflow

```bash
curl -X POST http://localhost:8080/api/summarize \
  -H "Content-Type: application/json" \
  -d '{"text": "This is a long article about artificial intelligence and machine learning..."}'
```

Response:
```json
{
  "workflow_id": "wf-1234567890",
  "message": "Summarization started",
  "status_url": "/api/workflows/wf-1234567890"
}
```

### Check workflow status

```bash
curl http://localhost:8080/api/workflows/wf-1234567890
```

Response:
```json
{
  "workflow_id": "wf-1234567890",
  "status": "completed",
  "output": {"summary": "..."},
  "duration": "523ms"
}
```

### List all workflows

```bash
# All workflows
curl http://localhost:8080/api/workflows

# Filter by status
curl http://localhost:8080/api/workflows?status=running
curl http://localhost:8080/api/workflows?status=completed
```

### Cancel a workflow

```bash
curl -X POST http://localhost:8080/api/workflows/wf-1234567890/cancel
```

## Code Walkthrough

### 1. Setup Engine (in main)

```go
func main() {
    // Create engine with state store and queue
    eng := setupWorkflowEngine()
    
    // Start background workers
    startWorkers(eng)
    
    // Your Gin routes
    router := gin.Default()
    // ...
}
```

### 2. Define Your Routes

```go
router.POST("/api/summarize", func(c *gin.Context) {
    var req SummarizeRequest
    c.BindJSON(&req)
    
    // Start durable workflow
    workflowID, _ := eng.StartWorkflow(c.Request.Context(), 
        "summarize-text", req)
    
    c.JSON(202, gin.H{"workflow_id": workflowID})
})
```

### 3. Check Status in Another Route

```go
router.GET("/api/workflows/:id", func(c *gin.Context) {
    status, _ := eng.GetWorkflowStatus(c.Request.Context(), 
        c.Param("id"))
    
    c.JSON(200, status)
})
```

## Why This Pattern?

✅ **You control the web framework** - Use Gin, Echo, Chi, or net/http  
✅ **You control the routes** - Design your API however you want  
✅ **You control authentication** - Add your own auth middleware  
✅ **You control serialization** - JSON, protobuf, whatever  
✅ **Workflows are durable** - Even if your server restarts  

## Scaling

### Same Process (Development)

Workers run in the same process as your web server:

```go
func main() {
    eng := setupWorkflowEngine()
    startWorkers(eng)  // go routine
    
    router.Run(":8080")
}
```

### Separate Workers (Production)

Workers run in separate processes/containers:

```go
// web-server.go
func main() {
    eng := setupWorkflowEngine()
    // Don't start workers here
    
    router.Run(":8080")
}

// worker.go
func main() {
    // Same setup, but only start workers
    eng := setupWorkflowEngine()
    startWorkers(eng)
    
    select {} // Keep alive
}
```

Deploy:
- Web servers: Scale horizontally (stateless)
- Workers: Scale based on queue depth
- State/Queue: Use Redis + SQS

## Other Frameworks

### Echo

```go
e := echo.New()

e.POST("/api/summarize", func(c echo.Context) error {
    workflowID, _ := eng.StartWorkflow(c.Request().Context(), 
        "summarize-text", input)
    
    return c.JSON(202, map[string]string{"workflow_id": workflowID})
})
```

### net/http

```go
http.HandleFunc("/api/summarize", func(w http.ResponseWriter, r *http.Request) {
    workflowID, _ := eng.StartWorkflow(r.Context(), 
        "summarize-text", input)
    
    json.NewEncoder(w).Encode(map[string]string{"workflow_id": workflowID})
})

http.ListenAndServe(":8080", nil)
```

## Next Steps

- Add authentication middleware
- Add request validation
- Use Redis state store for production
- Deploy workers to Lambda/K8s
- Add monitoring and alerting

