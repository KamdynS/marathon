// Package activity provides interfaces and implementations for executable
// units of work within workflows.
package activity

import (
	"context"
	"time"
)

// Activity represents a unit of work that can be executed within a workflow.
// Activities are non-deterministic operations that interact with external systems.
type Activity interface {
	// Name returns the unique name of this activity
	Name() string

	// Execute runs the activity with the given context and input
	Execute(ctx context.Context, input interface{}) (interface{}, error)
}

// ActivityFunc is a function-based activity implementation
type ActivityFunc func(ctx context.Context, input interface{}) (interface{}, error)

// Execute implements Activity
func (f ActivityFunc) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	return f(ctx, input)
}

// Name implements Activity
func (f ActivityFunc) Name() string {
	return "anonymous"
}

// Info holds metadata about an activity
type Info struct {
	Name        string
	Description string
	Timeout     time.Duration
	RetryPolicy *RetryPolicy
}

// RetryPolicy defines retry behavior for an activity
type RetryPolicy struct {
	MaxAttempts        int
	InitialInterval    time.Duration
	BackoffCoefficient float64
	MaxInterval        time.Duration
	NonRetryableErrors []string
}

// DefaultRetryPolicy returns a sensible default retry policy for activities
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:        3,
		InitialInterval:    time.Second,
		BackoffCoefficient: 2.0,
		MaxInterval:        time.Minute,
		NonRetryableErrors: []string{},
	}
}

// IsRetryable checks if an error is retryable based on the policy
func (p *RetryPolicy) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	for _, nonRetryable := range p.NonRetryableErrors {
		if errStr == nonRetryable {
			return false
		}
	}

	return true
}

// GetBackoffDuration calculates the backoff duration for a given attempt
func (p *RetryPolicy) GetBackoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return p.InitialInterval
	}

	duration := p.InitialInterval
	for i := 0; i < attempt; i++ {
		duration = time.Duration(float64(duration) * p.BackoffCoefficient)
		if duration > p.MaxInterval {
			return p.MaxInterval
		}
	}

	return duration
}
