// Package main demonstrates a simple LLM workflow
package main

import (
	"context"
	"log"
	"time"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/agent"
	"github.com/KamdynS/marathon/engine"
	mllm "github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/server"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/tools"
	"github.com/KamdynS/marathon/worker"
	"github.com/KamdynS/marathon/workflow"
)

// mockLLM implements marathon/llm.Client for the example
type mockLLM struct{}

func (m *mockLLM) Model() string { return "mock-llm" }

func (m *mockLLM) Chat(ctx context.Context, req *mllm.ChatRequest) (*mllm.Response, error) {
	time.Sleep(200 * time.Millisecond)
	// Echo back the last user message
	var content string
	if len(req.Messages) > 0 {
		content = "Hello! You said: " + req.Messages[len(req.Messages)-1].Content
	}
	return &mllm.Response{Content: content, Provider: "mock", Model: m.Model()}, nil
}

func (m *mockLLM) Completion(ctx context.Context, prompt string) (*mllm.Response, error) {
	return &mllm.Response{Content: "Completion: " + prompt, Provider: "mock", Model: m.Model()}, nil
}

func (m *mockLLM) Stream(ctx context.Context, req *mllm.ChatRequest, output chan<- *mllm.Response) error {
	defer close(output)
	r, _ := m.Chat(ctx, req)
	output <- r
	return nil
}

// ChatStream implements the provider-neutral streaming interface for the mock.
func (m *mockLLM) ChatStream(ctx context.Context, req *mllm.ChatRequest) (mllm.Stream, error) {
	r, _ := m.Chat(ctx, req)
	return &mockStream{text: r.Content}, nil
}

type mockStream struct {
	emitted bool
	closed  bool
	text    string
}

func (s *mockStream) Recv(ctx context.Context) (mllm.Delta, error) {
	if s.closed {
		return mllm.Delta{}, mllm.ErrStreamClosed
	}
	if !s.emitted {
		s.emitted = true
		if s.text != "" {
			return mllm.Delta{Type: mllm.DeltaTypeText, Text: s.text, Provider: "mock", Model: "mock-llm"}, nil
		}
	}
	s.closed = true
	return mllm.Delta{Type: mllm.DeltaTypeDone, Provider: "mock", Model: "mock-llm"}, nil
}

func (s *mockStream) Close() error { s.closed = true; return nil }

func main() {
	ctx := context.Background()

	// Create infrastructure components
	stateStore := state.NewInMemoryStore()
	taskQueue := queue.NewInMemoryQueue()
	defer taskQueue.Close()

	// Setup activity registry
	activityRegistry := activity.NewRegistry()
	// Build an agent backed by mock LLM and no tools
	ag := agent.NewChatAgent(&mockLLM{}, agent.AgentConfig{SystemPrompt: "You are helpful."}, tools.NewRegistry())
	if err := activityRegistry.Register("llm-call", &activity.AgentActivity{NameStr: "llm-call", Agent: ag}, activity.Info{
		Description: "Call LLM with a prompt",
		Timeout:     30 * time.Second,
	}); err != nil {
		log.Fatalf("Failed to register LLM activity: %v", err)
	}

	// Setup workflow registry
	workflowRegistry := workflow.NewRegistry()

	// Define a simple greeting workflow
	greetingWorkflow := workflow.New("greeting-workflow").
		Description("A simple workflow that greets the user").
		TaskQueue("default").
		Activity("llm-call", "Hello, AI!").
		Build()

	if err := workflowRegistry.Register(greetingWorkflow); err != nil {
		log.Fatalf("Failed to register workflow: %v", err)
	}

	// Create workflow engine
	eng, err := engine.New(engine.Config{
		StateStore:       stateStore,
		Queue:            taskQueue,
		WorkflowRegistry: workflowRegistry,
	})
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	// Start worker
	w, err := worker.New(worker.Config{
		Queue:            taskQueue,
		QueueName:        "default",
		ActivityRegistry: activityRegistry,
		StateStore:       stateStore,
		MaxConcurrent:    5,
		PollInterval:     100 * time.Millisecond,
	})
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	if err := w.Start(ctx); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		w.Stop(stopCtx)
	}()

	log.Println("✅ Worker started")

	// Start API server in background
	srv, err := server.New(server.Config{
		Engine: eng,
		Port:   8080,
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	go func() {
		log.Println("✅ API server starting on http://localhost:8080")
		if err := srv.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Give server a moment to start
	time.Sleep(500 * time.Millisecond)

	// Start a workflow programmatically
	log.Println("\n🚀 Starting greeting workflow...")
	workflowID, err := eng.StartWorkflow(ctx, "greeting-workflow", nil)
	if err != nil {
		log.Fatalf("Failed to start workflow: %v", err)
	}

	log.Printf("✅ Workflow started with ID: %s", workflowID)

	// Poll for completion
	log.Println("\n⏳ Waiting for workflow to complete...")
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)

		workflowState, err := eng.GetWorkflowStatus(ctx, workflowID)
		if err != nil {
			log.Printf("Error getting status: %v", err)
			continue
		}

		log.Printf("Status: %s (duration: %v)", workflowState.Status, workflowState.Duration())

		if workflowState.IsComplete() {
			if workflowState.Status == state.StatusCompleted {
				log.Printf("\n✅ Workflow completed successfully!")
				log.Printf("Output: %v", workflowState.Output)
			} else {
				log.Printf("\n❌ Workflow failed: %s", workflowState.Error)
			}
			break
		}
	}

	// Show events
	log.Println("\n📋 Workflow events:")
	events, err := eng.GetWorkflowEvents(ctx, workflowID)
	if err != nil {
		log.Printf("Error getting events: %v", err)
	} else {
		for _, event := range events {
			log.Printf("  %d. %s at %s", event.SequenceNum, event.Type, event.Timestamp.Format(time.RFC3339))
		}
	}

	log.Println("\n🔗 Try the API:")
	log.Println("  curl http://localhost:8080/health")
	log.Printf("  curl http://localhost:8080/workflows/%s\n", workflowID)
	log.Println("\n📝 Start a new workflow:")
	log.Println(`  curl -X POST http://localhost:8080/workflows \`)
	log.Println(`    -H "Content-Type: application/json" \`)
	log.Println(`    -d '{"workflow_name": "greeting-workflow", "input": null}'`)
	log.Println("\nPress Ctrl+C to stop...")

	// Keep running
	select {}
}
