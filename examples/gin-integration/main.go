// Package main demonstrates integrating durable workflows with Gin web framework
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/engine"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/worker"
	"github.com/KamdynS/marathon/workflow"
)

// SummarizeRequest is the request body for summarization
type SummarizeRequest struct {
	Text string `json:"text" binding:"required"`
}

// TranslateRequest is the request body for translation
type TranslateRequest struct {
	Text       string `json:"text" binding:"required"`
	TargetLang string `json:"target_lang" binding:"required"`
}

// mockLLMClient simulates an LLM client
type mockLLMClient struct{}

func (m *mockLLMClient) Chat(ctx context.Context, req *activity.ChatRequest) (*activity.ChatResponse, error) {
	time.Sleep(500 * time.Millisecond) // Simulate LLM latency

	return &activity.ChatResponse{
		Content: fmt.Sprintf("Processed: %s", req.Messages[0].Content),
		Usage: activity.Usage{
			InputTokens:  10,
			OutputTokens: 15,
			TotalTokens:  25,
		},
	}, nil
}

func main() {
	// 1. Setup workflow infrastructure (background)
	eng, stateStore, taskQueue := setupWorkflowEngine()
	startWorkers(eng, stateStore, taskQueue)

	log.Println("✅ Workflow engine and workers started")

	// 2. Your own Gin web server
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Summarize endpoint - starts a workflow
	router.POST("/api/summarize", func(c *gin.Context) {
		var req SummarizeRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		// Start durable workflow from handler
		workflowID, err := eng.StartWorkflow(c.Request.Context(),
			"summarize-text", map[string]string{"text": req.Text})

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(202, gin.H{
			"workflow_id": workflowID,
			"message":     "Summarization started",
			"status_url":  fmt.Sprintf("/api/workflows/%s", workflowID),
		})
	})

	// Translate endpoint - starts another workflow
	router.POST("/api/translate", func(c *gin.Context) {
		var req TranslateRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		workflowID, err := eng.StartWorkflow(c.Request.Context(),
			"translate-text", req)

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(202, gin.H{
			"workflow_id": workflowID,
			"message":     "Translation started",
			"status_url":  fmt.Sprintf("/api/workflows/%s", workflowID),
		})
	})

	// Check workflow status
	router.GET("/api/workflows/:id", func(c *gin.Context) {
		workflowID := c.Param("id")

		status, err := eng.GetWorkflowStatus(c.Request.Context(), workflowID)
		if err != nil {
			c.JSON(404, gin.H{"error": "workflow not found"})
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

	// List all workflows
	router.GET("/api/workflows", func(c *gin.Context) {
		statusFilter := state.WorkflowStatus(c.Query("status"))

		workflows, err := stateStore.ListWorkflows(c.Request.Context(), statusFilter)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"count":     len(workflows),
			"workflows": workflows,
		})
	})

	// Cancel a workflow
	router.POST("/api/workflows/:id/cancel", func(c *gin.Context) {
		workflowID := c.Param("id")

		if err := eng.CancelWorkflow(c.Request.Context(), workflowID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{"message": "workflow canceled"})
	})

	// 3. Run your server
	log.Println("🚀 Starting Gin server on http://localhost:8080")
	log.Println("\nTry these commands:")
	log.Println("  curl -X POST http://localhost:8080/api/summarize \\")
	log.Println("    -H 'Content-Type: application/json' \\")
	log.Println("    -d '{\"text\": \"Long text here...\"}'")
	log.Println("\n  curl http://localhost:8080/api/workflows/{workflow-id}")

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func setupWorkflowEngine() (*engine.Engine, state.Store, queue.Queue) {
	// Create infrastructure
	stateStore := state.NewInMemoryStore()
	taskQueue := queue.NewInMemoryQueue()

	// Register workflows
	workflowRegistry := workflow.NewRegistry()

	summarizeWf := workflow.New("summarize-text").
		Description("Summarize text using LLM").
		Activity("llm-call", activity.ChatRequest{
			Messages: []activity.Message{
				{Role: "system", Content: "You are a helpful assistant that summarizes text."},
				{Role: "user", Content: "Please summarize this."},
			},
		}).
		Build()

	translateWf := workflow.New("translate-text").
		Description("Translate text to another language").
		Activity("llm-call", nil).
		Build()

	workflowRegistry.Register(summarizeWf)
	workflowRegistry.Register(translateWf)

	// Create engine
	eng, err := engine.New(engine.Config{
		StateStore:       stateStore,
		Queue:            taskQueue,
		WorkflowRegistry: workflowRegistry,
	})

	if err != nil {
		log.Fatal(err)
	}

	return eng, stateStore, taskQueue
}

func startWorkers(eng *engine.Engine, stateStore state.Store, taskQueue queue.Queue) {
	// Register activities
	activityRegistry := activity.NewRegistry()

	llmClient := &mockLLMClient{}
	llmActivity := activity.NewLLMActivity(llmClient)

	activityRegistry.Register("llm-call", llmActivity, activity.Info{
		Description: "Call LLM for text processing",
		Timeout:     30 * time.Second,
	})

	// Start worker pool
	w, err := worker.New(worker.Config{
		Queue:            taskQueue,
		QueueName:        "default",
		ActivityRegistry: activityRegistry,
		StateStore:       stateStore,
		MaxConcurrent:    5,
		PollInterval:     100 * time.Millisecond,
	})

	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := w.Start(context.Background()); err != nil {
			log.Printf("Worker error: %v", err)
		}
	}()
}
