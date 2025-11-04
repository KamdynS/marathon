package state

import (
	"context"
	"time"
)

// WorkflowStatus represents the current status of a workflow
type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
	StatusCanceled  WorkflowStatus = "canceled"
)

// WorkflowState represents the current state of a workflow execution
type WorkflowState struct {
	WorkflowID   string         `json:"workflow_id"`
	WorkflowName string         `json:"workflow_name"`
	Status       WorkflowStatus `json:"status"`
	Input        interface{}    `json:"input"`
	Output       interface{}    `json:"output"`
	Error        string         `json:"error,omitempty"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      *time.Time     `json:"end_time,omitempty"`
	LastEventSeq int64          `json:"last_event_seq"`
	TaskQueue    string         `json:"task_queue"`
}

// ActivityState represents the state of an activity execution
type ActivityState struct {
	ActivityID   string         `json:"activity_id"`
	ActivityName string         `json:"activity_name"`
	WorkflowID   string         `json:"workflow_id"`
	Status       WorkflowStatus `json:"status"`
	Input        interface{}    `json:"input"`
	Output       interface{}    `json:"output"`
	Error        string         `json:"error,omitempty"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      *time.Time     `json:"end_time,omitempty"`
	Attempt      int            `json:"attempt"`
}

// Store defines the interface for persisting workflow state
type Store interface {
	// SaveWorkflowState saves the current state of a workflow
	SaveWorkflowState(ctx context.Context, state *WorkflowState) error

	// GetWorkflowState retrieves the current state of a workflow
	GetWorkflowState(ctx context.Context, workflowID string) (*WorkflowState, error)

	// AppendEvent appends an event to the workflow's event log
	AppendEvent(ctx context.Context, event *Event) error

	// GetEvents retrieves all events for a workflow
	GetEvents(ctx context.Context, workflowID string) ([]*Event, error)

	// GetEventsSince retrieves events since a specific sequence number
	GetEventsSince(ctx context.Context, workflowID string, since int64) ([]*Event, error)

	// SaveActivityState saves the state of an activity
	SaveActivityState(ctx context.Context, state *ActivityState) error

	// GetActivityState retrieves the state of an activity
	GetActivityState(ctx context.Context, activityID string) (*ActivityState, error)

	// ListWorkflows lists all workflows, optionally filtered by status
	ListWorkflows(ctx context.Context, status WorkflowStatus) ([]*WorkflowState, error)

	// DeleteWorkflow removes workflow state and events (for cleanup)
	DeleteWorkflow(ctx context.Context, workflowID string) error
}

// IsTerminal returns true if the status is terminal (workflow is done)
func (s WorkflowStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCanceled
}

// IsRunning returns true if the workflow is currently running
func (w *WorkflowState) IsRunning() bool {
	return w.Status == StatusRunning || w.Status == StatusPending
}

// IsComplete returns true if the workflow has finished
func (w *WorkflowState) IsComplete() bool {
	return w.Status.IsTerminal()
}

// Duration returns the workflow execution duration
func (w *WorkflowState) Duration() time.Duration {
	if w.EndTime != nil {
		return w.EndTime.Sub(w.StartTime)
	}
	return time.Since(w.StartTime)
}
