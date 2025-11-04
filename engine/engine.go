// Package engine provides the workflow execution engine and coordinator.
package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/workflow"
)

// Engine coordinates workflow execution
type Engine struct {
	stateStore       state.Store
	queue            queue.Queue
	workflowRegistry *workflow.Registry
	runningWorkflows sync.Map // workflowID -> *executionContext
	mu               sync.Mutex
}

// Config holds engine configuration
type Config struct {
	StateStore       state.Store
	Queue            queue.Queue
	WorkflowRegistry *workflow.Registry
}

// New creates a new workflow engine
func New(cfg Config) (*Engine, error) {
	if cfg.StateStore == nil {
		return nil, fmt.Errorf("state store is required")
	}
	if cfg.Queue == nil {
		return nil, fmt.Errorf("queue is required")
	}
	if cfg.WorkflowRegistry == nil {
		return nil, fmt.Errorf("workflow registry is required")
	}

	return &Engine{
		stateStore:       cfg.StateStore,
		queue:            cfg.Queue,
		workflowRegistry: cfg.WorkflowRegistry,
	}, nil
}

// StartWorkflow initiates a new workflow execution
func (e *Engine) StartWorkflow(ctx context.Context, workflowName string, input interface{}) (string, error) {
	// Get workflow definition
	def, err := e.workflowRegistry.Get(workflowName)
	if err != nil {
		return "", fmt.Errorf("workflow not found: %w", err)
	}

	// Generate workflow ID
	workflowID := generateWorkflowID()

	// Create initial state
	workflowState := &state.WorkflowState{
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		Status:       state.StatusPending,
		Input:        input,
		StartTime:    time.Now().UTC(),
		TaskQueue:    def.Options.TaskQueue,
	}

	// Save initial state
	if err := e.stateStore.SaveWorkflowState(ctx, workflowState); err != nil {
		return "", fmt.Errorf("failed to save workflow state: %w", err)
	}

	// Record workflow started event
	event := state.NewEvent(workflowID, state.EventWorkflowStarted, map[string]interface{}{
		"workflow_name": workflowName,
		"input":         input,
		"task_queue":    def.Options.TaskQueue,
	})

	if err := e.stateStore.AppendEvent(ctx, event); err != nil {
		return "", fmt.Errorf("failed to record start event: %w", err)
	}

	// Start execution asynchronously
	go e.executeWorkflow(context.Background(), workflowID, def, input)

	log.Printf("[Engine] Started workflow %s (%s)", workflowID, workflowName)

	return workflowID, nil
}

// GetWorkflowStatus retrieves the current status of a workflow
func (e *Engine) GetWorkflowStatus(ctx context.Context, workflowID string) (*state.WorkflowState, error) {
	return e.stateStore.GetWorkflowState(ctx, workflowID)
}

// GetWorkflowEvents retrieves all events for a workflow
func (e *Engine) GetWorkflowEvents(ctx context.Context, workflowID string) ([]*state.Event, error) {
	return e.stateStore.GetEvents(ctx, workflowID)
}

// CancelWorkflow cancels a running workflow
func (e *Engine) CancelWorkflow(ctx context.Context, workflowID string) error {
	// Get current state
	workflowState, err := e.stateStore.GetWorkflowState(ctx, workflowID)
	if err != nil {
		return err
	}

	if workflowState.IsComplete() {
		return fmt.Errorf("workflow already completed")
	}

	// Update state to canceled
	now := time.Now().UTC()
	workflowState.Status = state.StatusCanceled
	workflowState.EndTime = &now

	if err := e.stateStore.SaveWorkflowState(ctx, workflowState); err != nil {
		return err
	}

	// Record canceled event
	event := state.NewEvent(workflowID, state.EventWorkflowCanceled, nil)
	if err := e.stateStore.AppendEvent(ctx, event); err != nil {
		return err
	}

	log.Printf("[Engine] Canceled workflow %s", workflowID)

	return nil
}

// executeWorkflow runs a workflow to completion
func (e *Engine) executeWorkflow(ctx context.Context, workflowID string, def *workflow.Definition, input interface{}) {
	// Create execution context
	execCtx := newExecutionContext(workflowID, e.queue, e.stateStore, def.Options.TaskQueue)

	// Update state to running
	workflowState, _ := e.stateStore.GetWorkflowState(ctx, workflowID)
	workflowState.Status = state.StatusRunning
	e.stateStore.SaveWorkflowState(ctx, workflowState)

	// Set execution timeout if specified
	if def.Options.ExecutionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, def.Options.ExecutionTimeout)
		defer cancel()
	}

	// Execute the workflow
	output, err := def.Workflow.Execute(execCtx, input)

	// Update final state
	now := time.Now().UTC()
	workflowState.EndTime = &now

	if err != nil {
		// Workflow failed
		workflowState.Status = state.StatusFailed
		workflowState.Error = err.Error()

		event := state.NewEvent(workflowID, state.EventWorkflowFailed, map[string]interface{}{
			"error": err.Error(),
		})
		e.stateStore.AppendEvent(ctx, event)

		log.Printf("[Engine] Workflow %s failed: %v", workflowID, err)
	} else {
		// Workflow completed
		workflowState.Status = state.StatusCompleted
		workflowState.Output = output

		event := state.NewEvent(workflowID, state.EventWorkflowCompleted, map[string]interface{}{
			"output": output,
		})
		e.stateStore.AppendEvent(ctx, event)

		log.Printf("[Engine] Workflow %s completed successfully", workflowID)
	}

	e.stateStore.SaveWorkflowState(ctx, workflowState)
}

// generateWorkflowID generates a unique workflow ID
func generateWorkflowID() string {
	return fmt.Sprintf("wf-%d", time.Now().UnixNano())
}
