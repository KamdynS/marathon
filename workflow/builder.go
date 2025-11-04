package workflow

import (
	"context"
	"fmt"
	"time"
)

// Builder provides a fluent API for constructing workflows
type Builder struct {
	name        string
	description string
	version     string
	steps       []Step
	options     Options
}

// Step represents a single step in a workflow
type Step interface {
	Execute(ctx Context) (interface{}, error)
}

// ActivityStep executes an activity
type ActivityStep struct {
	ActivityName string
	Input        interface{}
	Timeout      time.Duration
}

// Execute implements Step
func (s *ActivityStep) Execute(ctx Context) (interface{}, error) {
	// Create a context for timeout if needed
	execCtx := context.Context(ctx)
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, s.Timeout)
		defer cancel()
	}

	future := ctx.ExecuteActivity(execCtx, s.ActivityName, s.Input)
	var result interface{}
	if err := future.Get(execCtx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ParallelStep executes multiple steps in parallel
type ParallelStep struct {
	Steps []Step
}

// Execute implements Step
func (s *ParallelStep) Execute(ctx Context) (interface{}, error) {
	results := make([]interface{}, len(s.Steps))
	errors := make([]error, len(s.Steps))

	type result struct {
		index int
		value interface{}
		err   error
	}

	resultCh := make(chan result, len(s.Steps))

	for i, step := range s.Steps {
		go func(idx int, st Step) {
			val, err := st.Execute(ctx)
			resultCh <- result{index: idx, value: val, err: err}
		}(i, step)
	}

	for i := 0; i < len(s.Steps); i++ {
		res := <-resultCh
		results[res.index] = res.value
		errors[res.index] = res.err
	}

	// Check if any step failed
	for _, err := range errors {
		if err != nil {
			return results, fmt.Errorf("parallel execution failed: %w", err)
		}
	}

	return results, nil
}

// SequenceStep executes steps in sequence
type SequenceStep struct {
	Steps []Step
}

// Execute implements Step
func (s *SequenceStep) Execute(ctx Context) (interface{}, error) {
	var lastResult interface{}
	for _, step := range s.Steps {
		result, err := step.Execute(ctx)
		if err != nil {
			return nil, err
		}
		lastResult = result
	}
	return lastResult, nil
}

// New creates a new workflow builder
func New(name string) *Builder {
	return &Builder{
		name:    name,
		version: "1.0",
		steps:   make([]Step, 0),
		options: Options{
			TaskQueue:        "default",
			ExecutionTimeout: 10 * time.Minute,
			RetryPolicy:      DefaultRetryPolicy(),
		},
	}
}

// Description sets the workflow description
func (b *Builder) Description(desc string) *Builder {
	b.description = desc
	return b
}

// Version sets the workflow version
func (b *Builder) Version(version string) *Builder {
	b.version = version
	return b
}

// TaskQueue sets the task queue name
func (b *Builder) TaskQueue(queue string) *Builder {
	b.options.TaskQueue = queue
	return b
}

// Timeout sets the execution timeout
func (b *Builder) Timeout(timeout time.Duration) *Builder {
	b.options.ExecutionTimeout = timeout
	return b
}

// Activity adds an activity step
func (b *Builder) Activity(name string, input interface{}) *Builder {
	b.steps = append(b.steps, &ActivityStep{
		ActivityName: name,
		Input:        input,
	})
	return b
}

// ActivityWithTimeout adds an activity step with a timeout
func (b *Builder) ActivityWithTimeout(name string, input interface{}, timeout time.Duration) *Builder {
	b.steps = append(b.steps, &ActivityStep{
		ActivityName: name,
		Input:        input,
		Timeout:      timeout,
	})
	return b
}

// Parallel adds a parallel execution step
func (b *Builder) Parallel(steps ...Step) *Builder {
	b.steps = append(b.steps, &ParallelStep{Steps: steps})
	return b
}

// Sequence adds a sequence execution step
func (b *Builder) Sequence(steps ...Step) *Builder {
	b.steps = append(b.steps, &SequenceStep{Steps: steps})
	return b
}

// Build creates the final workflow definition
func (b *Builder) Build() *Definition {
	return &Definition{
		Name:        b.name,
		Description: b.description,
		Version:     b.version,
		Workflow:    b.buildWorkflow(),
		Options:     b.options,
	}
}

func (b *Builder) buildWorkflow() Workflow {
	steps := b.steps
	return WorkflowFunc(func(ctx Context, input interface{}) (interface{}, error) {
		var lastResult interface{}
		for _, step := range steps {
			result, err := step.Execute(ctx)
			if err != nil {
				return nil, err
			}
			lastResult = result
		}
		return lastResult, nil
	})
}
