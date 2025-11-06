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

// executionContext implements workflow.Context
type executionContext struct {
	context.Context
	workflowID string
	queue      queue.Queue
	stateStore state.Store
	taskQueue  string
	futures    map[string]*futureImpl
	mu         sync.Mutex
}

// newExecutionContext creates a new execution context
func newExecutionContext(workflowID string, q queue.Queue, store state.Store, taskQueue string) *executionContext {
	return &executionContext{
		Context:    context.Background(),
		workflowID: workflowID,
		queue:      q,
		stateStore: store,
		taskQueue:  taskQueue,
		futures:    make(map[string]*futureImpl),
	}
}

// ExecuteActivity implements workflow.Context
func (ctx *executionContext) ExecuteActivity(activityCtx context.Context, activityName string, input interface{}) workflow.Future {
	// Generate activity ID
	activityID := generateActivityID()

	return ctx.ExecuteActivityWithID(activityCtx, activityName, input, activityID)
}

// ExecuteActivityWithID implements workflow.Context with stable id support
func (ctx *executionContext) ExecuteActivityWithID(activityCtx context.Context, activityName string, input interface{}, activityID string) workflow.Future {
	if activityID == "" {
		activityID = generateActivityID()
	}

	// If activity already completed, return cached result
	if st, err := ctx.stateStore.GetActivityState(activityCtx, activityID); err == nil && st != nil {
		if st.Status == state.StatusCompleted {
			future := newFuture(activityID)
			future.setValue(st.Output)
			return future
		}
	}

	// Create task
	task := queue.NewTask(queue.TaskTypeActivity, ctx.workflowID, input)
	task.ActivityID = activityID
	task.ActivityName = activityName

	// Record activity scheduled event only if not previously scheduled
	if _, err := ctx.stateStore.GetActivityState(activityCtx, activityID); err != nil {
		event := state.NewEvent(ctx.workflowID, state.EventActivityScheduled, map[string]interface{}{
			"activity_id":   activityID,
			"activity_name": activityName,
			"input":         input,
		})
		ctx.stateStore.AppendEvent(activityCtx, event)
	}

	// Enqueue task
	if err := ctx.queue.Enqueue(activityCtx, ctx.taskQueue, task); err != nil {
		// Return a future that will fail immediately
		future := newFuture(activityID)
		future.setError(fmt.Errorf("failed to enqueue activity: %w", err))
		return future
	}

	log.Printf("[Context] Scheduled activity %s (%s) for workflow %s",
		activityID, activityName, ctx.workflowID)

	// Create and return future
	future := newFuture(activityID)
	ctx.mu.Lock()
	ctx.futures[activityID] = future
	ctx.mu.Unlock()

	// Start polling for result in background
	go ctx.pollActivityResult(activityCtx, activityID, future)

	return future
}

// pollActivityResult polls for activity completion
func (ctx *executionContext) pollActivityResult(activityCtx context.Context, activityID string, future *futureImpl) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-activityCtx.Done():
			future.setError(activityCtx.Err())
			return
		case <-ticker.C:
			// Check activity state
			activityState, err := ctx.stateStore.GetActivityState(activityCtx, activityID)
			if err != nil {
				// Activity not found yet, continue polling
				continue
			}

			if activityState.Status == state.StatusCompleted {
				future.setValue(activityState.Output)
				return
			} else if activityState.Status == state.StatusFailed {
				future.setError(fmt.Errorf("activity failed: %s", activityState.Error))
				return
			}
			// Otherwise, continue polling
		}
	}
}

// Sleep implements workflow.Context
func (ctx *executionContext) Sleep(duration time.Duration) workflow.Future {
	timerID := fmt.Sprintf("tm-%d", time.Now().UnixNano())
	fireAt := time.Now().Add(duration).UTC()

	// persist timer schedule (idempotent)
	_ = ctx.stateStore.ScheduleTimer(context.Background(), ctx.workflowID, timerID, fireAt)

	// record timer scheduled event
	evt := state.NewEvent(ctx.workflowID, state.EventTimerScheduled, map[string]interface{}{
		"timer_id": timerID,
		"fire_at":  fireAt,
		"duration": duration.String(),
	})
	_ = ctx.stateStore.AppendEvent(context.Background(), evt)

	future := newFuture(timerID)

	// poll for TimerFired event durably
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				future.setError(ctx.Err())
				return
			case <-ticker.C:
				events, err := ctx.stateStore.GetEventsSince(context.Background(), ctx.workflowID, 0)
				if err != nil {
					continue
				}
				for _, e := range events {
					if e.Type == state.EventTimerFired {
						if idAny, ok := e.Data["timer_id"]; ok {
							if idStr, ok := idAny.(string); ok && idStr == timerID {
								future.setValue(nil)
								return
							}
						}
					}
				}
			}
		}
	}()

	return future
}

// Now implements workflow.Context
func (ctx *executionContext) Now() time.Time {
	// For determinism, this should use workflow time, not system time
	// For now, we'll use system time as a simple implementation
	return time.Now()
}

// WorkflowID implements workflow.Context
func (ctx *executionContext) WorkflowID() string {
	return ctx.workflowID
}

// Logger implements workflow.Context
func (ctx *executionContext) Logger() workflow.Logger {
	return &defaultLogger{workflowID: ctx.workflowID}
}

// futureImpl implements workflow.Future
type futureImpl struct {
	id      string
	value   interface{}
	err     error
	ready   bool
	readyCh chan struct{}
	mu      sync.Mutex
}

// newFuture creates a new future
func newFuture(id string) *futureImpl {
	return &futureImpl{
		id:      id,
		ready:   false,
		readyCh: make(chan struct{}),
	}
}

// Get implements workflow.Future
func (f *futureImpl) Get(ctx context.Context, valuePtr interface{}) error {
	// Wait for result
	select {
	case <-f.readyCh:
		// Result is ready
	case <-ctx.Done():
		return ctx.Err()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	// Set value if pointer provided
	if valuePtr != nil {
		// Simple assignment - in production, use reflection for proper type handling
		switch v := valuePtr.(type) {
		case *interface{}:
			*v = f.value
		default:
			// For now, just assign directly
			*valuePtr.(*interface{}) = f.value
		}
	}

	return nil
}

// IsReady implements workflow.Future
func (f *futureImpl) IsReady() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready
}

// setValue sets the future's value
func (f *futureImpl) setValue(value interface{}) {
	f.mu.Lock()
	f.value = value
	f.ready = true
	f.mu.Unlock()
	close(f.readyCh)
}

// setError sets the future's error
func (f *futureImpl) setError(err error) {
	f.mu.Lock()
	f.err = err
	f.ready = true
	f.mu.Unlock()
	close(f.readyCh)
}

// defaultLogger implements workflow.Logger
type defaultLogger struct {
	workflowID string
}

func (l *defaultLogger) Debug(msg string, keyvals ...interface{}) {
	log.Printf("[DEBUG] [Workflow %s] %s %v", l.workflowID, msg, keyvals)
}

func (l *defaultLogger) Info(msg string, keyvals ...interface{}) {
	log.Printf("[INFO] [Workflow %s] %s %v", l.workflowID, msg, keyvals)
}

func (l *defaultLogger) Warn(msg string, keyvals ...interface{}) {
	log.Printf("[WARN] [Workflow %s] %s %v", l.workflowID, msg, keyvals)
}

func (l *defaultLogger) Error(msg string, keyvals ...interface{}) {
	log.Printf("[ERROR] [Workflow %s] %s %v", l.workflowID, msg, keyvals)
}

// generateActivityID generates a unique activity ID
func generateActivityID() string {
	return fmt.Sprintf("act-%d", time.Now().UnixNano())
}
