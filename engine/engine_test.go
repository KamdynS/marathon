package engine

import (
	"context"
	"testing"
	"time"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/worker"
	"github.com/KamdynS/marathon/workflow"
)

func TestEngine_StartWorkflow(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()

	workflowRegistry := workflow.NewRegistry()

	// Register a simple workflow
	def := workflow.New("test-workflow").Build()
	workflowRegistry.Register(def)

	engine, err := New(Config{
		StateStore:       store,
		Queue:            q,
		WorkflowRegistry: workflowRegistry,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()

	// Start workflow
	workflowID, err := engine.StartWorkflow(ctx, "test-workflow", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	if workflowID == "" {
		t.Error("expected workflow ID to be generated")
	}

	// Wait a bit for async execution
	time.Sleep(100 * time.Millisecond)

	// Get workflow status
	workflowState, err := engine.GetWorkflowStatus(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if workflowState.WorkflowName != "test-workflow" {
		t.Errorf("expected workflow name test-workflow, got %s", workflowState.WorkflowName)
	}

	// Get events
	events, err := engine.GetWorkflowEvents(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow events: %v", err)
	}

	if len(events) < 1 {
		t.Errorf("expected at least 1 event, got %d", len(events))
	}

	if events[0].Type != state.EventWorkflowStarted {
		t.Errorf("expected first event to be workflow started, got %s", events[0].Type)
	}
}

func TestEngine_WithActivity(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()

	workflowRegistry := workflow.NewRegistry()
	activityRegistry := activity.NewRegistry()

	// Register a test activity
	testActivity := activity.ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		return "activity-output", nil
	})

	activityRegistry.Register("test-activity", testActivity, activity.Info{
		Description: "test activity",
		Timeout:     5 * time.Second,
	})

	// Register a workflow that uses the activity
	def := workflow.New("test-workflow-with-activity").
		Activity("test-activity", "activity-input").
		Build()
	workflowRegistry.Register(def)

	engine, err := New(Config{
		StateStore:       store,
		Queue:            q,
		WorkflowRegistry: workflowRegistry,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Start a worker to process activities
	workerCfg := worker.Config{
		Queue:            q,
		QueueName:        "default",
		ActivityRegistry: activityRegistry,
		StateStore:       store,
		MaxConcurrent:    1,
		PollInterval:     100 * time.Millisecond,
	}

	w, err := worker.New(workerCfg)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	ctx := context.Background()
	w.Start(ctx)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.Stop(stopCtx)
	}()

	// Start workflow
	workflowID, err := engine.StartWorkflow(ctx, "test-workflow-with-activity", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	// Wait for workflow and activity to complete
	time.Sleep(2 * time.Second)

	// Get workflow status
	workflowState, err := engine.GetWorkflowStatus(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	// Check that workflow completed
	if workflowState.Status != state.StatusCompleted {
		t.Errorf("expected workflow to be completed, got %s", workflowState.Status)
	}

	// Get events
	events, err := engine.GetWorkflowEvents(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow events: %v", err)
	}

	// Should have: started, activity scheduled, activity started, activity completed, workflow completed
	if len(events) < 3 {
		t.Errorf("expected at least 3 events, got %d", len(events))
	}
}

func TestEngine_CancelWorkflow(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()

	workflowRegistry := workflow.NewRegistry()

	def := workflow.New("test-workflow").Build()
	workflowRegistry.Register(def)

	engine, err := New(Config{
		StateStore:       store,
		Queue:            q,
		WorkflowRegistry: workflowRegistry,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()

	// Start workflow
	workflowID, err := engine.StartWorkflow(ctx, "test-workflow", "test-input")
	if err != nil {
		t.Fatalf("failed to start workflow: %v", err)
	}

	// Cancel immediately
	if err := engine.CancelWorkflow(ctx, workflowID); err != nil {
		t.Fatalf("failed to cancel workflow: %v", err)
	}

	// Check status
	workflowState, err := engine.GetWorkflowStatus(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}

	if workflowState.Status != state.StatusCanceled {
		t.Errorf("expected status canceled, got %s", workflowState.Status)
	}

	// Check events
	events, err := engine.GetWorkflowEvents(ctx, workflowID)
	if err != nil {
		t.Fatalf("failed to get workflow events: %v", err)
	}

	foundCancel := false
	for _, event := range events {
		if event.Type == state.EventWorkflowCanceled {
			foundCancel = true
			break
		}
	}

	if !foundCancel {
		t.Error("expected workflow canceled event")
	}
}
