// Package workflow provides the core workflow definition and execution interfaces
// for building durable, resumable AI workflows.
package workflow

import (
	"context"
	"time"
)

// Workflow represents a durable workflow definition that can be executed,
// paused, and resumed. Workflows should be deterministic for replay purposes.
type Workflow interface {
	// Name returns the unique name of this workflow
	Name() string

	// Execute runs the workflow with the given context and input
	Execute(ctx Context, input interface{}) (interface{}, error)
}

// Context provides workflow execution context with access to activities,
// timers, and state management. It's similar to Temporal's workflow.Context.
type Context interface {
	context.Context

	// ExecuteActivity schedules an activity for execution
	ExecuteActivity(ctx context.Context, activity string, input interface{}) Future

	// Sleep pauses workflow execution for the specified duration
	Sleep(duration time.Duration) Future

	// Now returns the current workflow time (for determinism)
	Now() time.Time

	// WorkflowID returns the unique identifier for this workflow execution
	WorkflowID() string

	// Logger returns a workflow-aware logger
	Logger() Logger
}

// Future represents the result of an asynchronous operation
type Future interface {
	// Get blocks until the result is available
	Get(ctx context.Context, valuePtr interface{}) error

	// IsReady returns true if the result is available
	IsReady() bool
}

// Logger provides structured logging for workflows
type Logger interface {
	Debug(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Warn(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
}

// WorkflowFunc is a function-based workflow implementation
type WorkflowFunc func(ctx Context, input interface{}) (interface{}, error)

// Name implements Workflow
func (f WorkflowFunc) Name() string {
	return "anonymous"
}

// Execute implements Workflow
func (f WorkflowFunc) Execute(ctx Context, input interface{}) (interface{}, error) {
	return f(ctx, input)
}

// Definition holds metadata about a workflow
type Definition struct {
	Name        string
	Description string
	Version     string
	Workflow    Workflow
	Options     Options
}

// Options configure workflow execution behavior
type Options struct {
	// TaskQueue specifies which queue to send activities to
	TaskQueue string

	// ExecutionTimeout is the maximum time a workflow can run
	ExecutionTimeout time.Duration

	// RetryPolicy defines how to retry failed workflows
	RetryPolicy *RetryPolicy
}

// RetryPolicy defines retry behavior for workflows and activities
type RetryPolicy struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int

	// InitialInterval is the backoff duration for the first retry
	InitialInterval time.Duration

	// BackoffCoefficient is the rate at which backoff increases
	BackoffCoefficient float64

	// MaxInterval caps the backoff duration
	MaxInterval time.Duration
}

// DefaultRetryPolicy returns a sensible default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:        3,
		InitialInterval:    time.Second,
		BackoffCoefficient: 2.0,
		MaxInterval:        time.Minute,
	}
}
