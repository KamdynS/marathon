// Package worker provides worker pool implementation for executing workflow tasks.
package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
)

// Worker polls tasks from a queue and executes activities
type Worker struct {
	id               string
	queue            queue.Queue
	queueName        string
	activityRegistry *activity.Registry
	stateStore       state.Store
	pollInterval     time.Duration
	maxConcurrent    int
	stopCh           chan struct{}
	wg               sync.WaitGroup
	running          bool
	mu               sync.Mutex
}

// Config holds worker configuration
type Config struct {
	ID               string
	Queue            queue.Queue
	QueueName        string
	ActivityRegistry *activity.Registry
	StateStore       state.Store
	PollInterval     time.Duration
	MaxConcurrent    int
}

// DefaultConfig returns a default worker configuration
func DefaultConfig() Config {
	return Config{
		ID:            fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		PollInterval:  time.Second,
		MaxConcurrent: 5,
	}
}

// New creates a new worker
func New(cfg Config) (*Worker, error) {
	if cfg.Queue == nil {
		return nil, fmt.Errorf("queue is required")
	}
	if cfg.ActivityRegistry == nil {
		return nil, fmt.Errorf("activity registry is required")
	}
	if cfg.StateStore == nil {
		return nil, fmt.Errorf("state store is required")
	}
	if cfg.QueueName == "" {
		cfg.QueueName = "default"
	}
	if cfg.ID == "" {
		cfg.ID = DefaultConfig().ID
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = DefaultConfig().MaxConcurrent
	}

	return &Worker{
		id:               cfg.ID,
		queue:            cfg.Queue,
		queueName:        cfg.QueueName,
		activityRegistry: cfg.ActivityRegistry,
		stateStore:       cfg.StateStore,
		pollInterval:     cfg.PollInterval,
		maxConcurrent:    cfg.MaxConcurrent,
		stopCh:           make(chan struct{}),
		running:          false,
	}, nil
}

// Start begins polling for and executing tasks
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("worker already running")
	}
	w.running = true
	w.mu.Unlock()

	log.Printf("[Worker %s] Starting worker on queue %s with %d max concurrent tasks",
		w.id, w.queueName, w.maxConcurrent)

	// Start worker goroutines
	for i := 0; i < w.maxConcurrent; i++ {
		w.wg.Add(1)
		go w.pollLoop(ctx, i)
	}

	return nil
}

// Stop gracefully stops the worker
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	log.Printf("[Worker %s] Stopping worker...", w.id)

	// Signal all goroutines to stop
	close(w.stopCh)

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[Worker %s] Worker stopped gracefully", w.id)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker stop timeout: %w", ctx.Err())
	}
}

// pollLoop continuously polls for tasks
func (w *Worker) pollLoop(ctx context.Context, workerNum int) {
	defer w.wg.Done()

	log.Printf("[Worker %s-%d] Poll loop started", w.id, workerNum)

	for {
		select {
		case <-w.stopCh:
			log.Printf("[Worker %s-%d] Poll loop stopping", w.id, workerNum)
			return
		case <-ctx.Done():
			log.Printf("[Worker %s-%d] Context canceled", w.id, workerNum)
			return
		default:
			w.pollOnce(ctx, workerNum)
		}
	}
}

// pollOnce polls for a single task and executes it
func (w *Worker) pollOnce(ctx context.Context, workerNum int) {
	// Poll with timeout to allow checking stop signal
	task, err := w.queue.DequeueWithTimeout(ctx, w.queueName, w.pollInterval)
	if err != nil {
		// Timeout or context canceled - this is normal
		return
	}

	if task == nil {
		return
	}

	log.Printf("[Worker %s-%d] Received task %s for workflow %s",
		w.id, workerNum, task.ID, task.WorkflowID)

	// Execute the task
	result := w.executeTask(ctx, task)

	// Ack or Nack based on result
	if result.Success {
		if err := w.queue.Ack(ctx, w.queueName, task.ID); err != nil {
			log.Printf("[Worker %s-%d] Failed to ack task %s: %v",
				w.id, workerNum, task.ID, err)
		}
	} else {
		// Nack and requeue if not max attempts
		requeue := task.Attempts < 3 // TODO: make configurable
		if err := w.queue.Nack(ctx, w.queueName, task.ID, requeue); err != nil {
			log.Printf("[Worker %s-%d] Failed to nack task %s: %v",
				w.id, workerNum, task.ID, err)
		}
	}
}

// executeTask executes a single task
func (w *Worker) executeTask(ctx context.Context, task *queue.Task) *queue.TaskResult {
	startTime := time.Now()

	result := &queue.TaskResult{
		TaskID:     task.ID,
		WorkflowID: task.WorkflowID,
		ActivityID: task.ActivityID,
		Success:    false,
	}

	switch task.Type {
	case queue.TaskTypeActivity:
		result = w.executeActivity(ctx, task)
	case queue.TaskTypeWorkflow:
		// Workflow execution would be handled differently
		result.Error = "workflow tasks not yet implemented"
	default:
		result.Error = fmt.Sprintf("unknown task type: %s", task.Type)
	}

	result.Duration = time.Since(startTime)

	log.Printf("[Worker %s] Task %s completed: success=%v, duration=%v",
		w.id, task.ID, result.Success, result.Duration)

	return result
}

// executeActivity executes an activity task
func (w *Worker) executeActivity(ctx context.Context, task *queue.Task) *queue.TaskResult {
	result := &queue.TaskResult{
		TaskID:     task.ID,
		WorkflowID: task.WorkflowID,
		ActivityID: task.ActivityID,
		Success:    false,
	}

	// Get activity from registry
	reg, err := w.activityRegistry.Get(task.ActivityName)
	if err != nil {
		result.Error = fmt.Sprintf("activity not found: %v", err)
		return result
	}

	// Load existing activity state if any for idempotency
	var activityState *state.ActivityState
	existing, getErr := w.stateStore.GetActivityState(ctx, task.ActivityID)
	if getErr == nil {
		// Existing record
		activityState = existing
		// If already completed, return cached result without emitting duplicate events
		if activityState.Status == state.StatusCompleted {
			result.Success = true
			result.Output = activityState.Output
			return result
		}
		// Already started previously: do not emit duplicate ActivityStarted event
	} else {
		// No existing record; create running state and emit ActivityStarted once
		activityState = &state.ActivityState{
			ActivityID:   task.ActivityID,
			ActivityName: task.ActivityName,
			WorkflowID:   task.WorkflowID,
			Status:       state.StatusRunning,
			Input:        task.Input,
			StartTime:    time.Now().UTC(),
			Attempt:      task.Attempts,
		}
		w.stateStore.SaveActivityState(ctx, activityState)

		// Record activity started event once
		event := state.NewEvent(task.WorkflowID, state.EventActivityStarted, map[string]interface{}{
			"activity_id":   task.ActivityID,
			"activity_name": task.ActivityName,
		})
		w.stateStore.AppendEvent(ctx, event)
	}

	// Create execution context with timeout
	execCtx := ctx
	if reg.Info.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, reg.Info.Timeout)
		defer cancel()
	}

	// Execute the activity
	// Inject an event emitter into the activity context so it can stream agent events
	execCtx = activity.WithEventEmitter(execCtx, func(eventType state.EventType, data map[string]interface{}) error {
		evt := state.NewEvent(task.WorkflowID, eventType, data)
		return w.stateStore.AppendEvent(ctx, evt)
	})
	// Inject ActivityEventContext for idempotency-aware activities
	execCtx = activity.WithActivityEventContext(execCtx, activity.ActivityEventContext{
		Store:      w.stateStore,
		WorkflowID: task.WorkflowID,
	})

	// Derive a cancelable context that cancels when workflow is canceled
	wfCtx, cancel := context.WithCancel(execCtx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-wfCtx.Done():
				return
			case <-ticker.C:
				st, err := w.stateStore.GetWorkflowState(context.Background(), task.WorkflowID)
				if err == nil && st.Status == state.StatusCanceled {
					cancel()
					return
				}
			}
		}
	}()

	output, err := reg.Activity.Execute(wfCtx, task.Input)

	now := time.Now().UTC()
	activityState.EndTime = &now

	if err != nil {
		// Activity failed
		result.Error = err.Error()
		activityState.Status = state.StatusFailed
		activityState.Error = err.Error()

		// Record failure event
		event := state.NewEvent(task.WorkflowID, state.EventActivityFailed, map[string]interface{}{
			"activity_id": task.ActivityID,
			"error":       err.Error(),
			"attempt":     task.Attempts,
		})
		w.stateStore.AppendEvent(ctx, event)

		log.Printf("[Worker %s] Activity %s failed: %v", w.id, task.ActivityName, err)
	} else {
		// Activity succeeded
		result.Success = true
		result.Output = output
		if activityState.Status != state.StatusCompleted {
			activityState.Status = state.StatusCompleted
			activityState.Output = output
			// Record completion event once
			event := state.NewEvent(task.WorkflowID, state.EventActivityCompleted, map[string]interface{}{
				"activity_id": task.ActivityID,
				"output":      output,
			})
			w.stateStore.AppendEvent(ctx, event)
		}

		log.Printf("[Worker %s] Activity %s completed successfully", w.id, task.ActivityName)
	}

	// Save final activity state
	w.stateStore.SaveActivityState(ctx, activityState)

	return result
}
