## API Usage (Library-first)

### Setup in main()

```go
// Initialize engine and start local workers
eng := setupWorkflowEngine()
go startWorkers(eng)

// Your web server
r := gin.Default()

// Start workflow
r.POST("/api/process", func(c *gin.Context) {
    var input map[string]any
    _ = c.BindJSON(&input)
    id, err := eng.StartWorkflow(c.Request.Context(), "my-workflow", input)
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    c.JSON(202, gin.H{"workflow_id": id})
})

// Poll status
r.GET("/api/workflows/:id", func(c *gin.Context) {
    st, err := eng.GetWorkflowStatus(c.Request.Context(), c.Param("id"))
    if err != nil { c.JSON(404, gin.H{"error": "not found"}); return }
    c.JSON(200, st)
})

// SSE stream
r.GET("/api/workflows/:id/events", sseHandler(eng))

_ = r.Run(":8080")
```

### SSE Handler (server-agnostic helper)
```go
func sseHandler(eng *engine.Engine) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Content-Type", "text/event-stream")
        flusher, ok := c.Writer.(http.Flusher)
        if !ok { c.Status(500); return }

        id := c.Param("id")
        var last int64
        for {
            select {
            case <-c.Request.Context().Done():
                return
            case <-time.After(500 * time.Millisecond):
                evts, _ := eng.GetWorkflowEvents(c, id)
                for _, e := range evts {
                    if e.SequenceNum <= last { continue }
                    data, _ := json.Marshal(e)
                    fmt.Fprintf(c.Writer, "data: %s\n\n", data)
                    flusher.Flush()
                    last = e.SequenceNum
                    if e.Type.IsTerminal() { fmt.Fprint(c.Writer, "event: done\n\n"); return }
                }
            }
        }
    }
}
```

### Swapping Adapters
```go
// Local
stateStore := state.NewInMemoryStore()
taskQueue  := queue.NewInMemoryQueue()

// Production
// stateStore := redisstore.New(...)
// taskQueue  := squeue.New(...)
```


