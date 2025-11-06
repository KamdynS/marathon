package worker

import (
	"context"
	"testing"
	"time"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
)

func TestWorker_New(t *testing.T) {
	q := queue.NewInMemoryQueue()
	defer q.Close()

	registry := activity.NewRegistry()
	store := state.NewInMemoryStore()

	cfg := Config{
		Queue:            q,
		QueueName:        "test-queue",
		ActivityRegistry: registry,
		StateStore:       store,
	}

	worker, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	if worker.queueName != "test-queue" {
		t.Errorf("expected queue name test-queue, got %s", worker.queueName)
	}

	if worker.maxConcurrent != DefaultConfig().MaxConcurrent {
		t.Errorf("expected default max concurrent, got %d", worker.maxConcurrent)
	}
}

func TestWorker_ExecuteActivity(t *testing.T) {
	q := queue.NewInMemoryQueue()
	defer q.Close()

	registry := activity.NewRegistry()
	store := state.NewInMemoryStore()

	// Register a test activity
	testActivity := activity.ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		return "test-output", nil
	})

	registry.Register("test-activity", testActivity, activity.Info{
		Description: "test activity",
		Timeout:     5 * time.Second,
	})

	cfg := Config{
		Queue:            q,
		QueueName:        "test-queue",
		ActivityRegistry: registry,
		StateStore:       store,
		MaxConcurrent:    1,
		PollInterval:     100 * time.Millisecond,
	}

	worker, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	ctx := context.Background()

	// Start worker
	if err := worker.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	// Enqueue a task
	task := queue.NewTask(queue.TaskTypeActivity, "wf-123", "test-input")
	task.ActivityID = "act-123"
	task.ActivityName = "test-activity"

	if err := q.Enqueue(ctx, "test-queue", task); err != nil {
		t.Fatalf("failed to enqueue task: %v", err)
	}

	// Wait for task to be processed
	time.Sleep(500 * time.Millisecond)

	// Stop worker
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("failed to stop worker: %v", err)
	}

	// Verify activity state was saved
	activityState, err := store.GetActivityState(ctx, "act-123")
	if err != nil {
		t.Fatalf("failed to get activity state: %v", err)
	}

	if activityState.Status != state.StatusCompleted {
		t.Errorf("expected status completed, got %s", activityState.Status)
	}

	if activityState.Output != "test-output" {
		t.Errorf("expected output test-output, got %v", activityState.Output)
	}

	// Verify events were recorded
	events, err := store.GetEvents(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events (started, completed), got %d", len(events))
	}
}

func TestWorker_ActivityFailure(t *testing.T) {
	q := queue.NewInMemoryQueue()
	defer q.Close()

	registry := activity.NewRegistry()
	store := state.NewInMemoryStore()

	// Register a failing activity
	failingActivity := activity.ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		return nil, context.DeadlineExceeded
	})

	registry.Register("failing-activity", failingActivity, activity.Info{
		Description: "failing activity",
		Timeout:     5 * time.Second,
	})

	cfg := Config{
		Queue:            q,
		QueueName:        "test-queue",
		ActivityRegistry: registry,
		StateStore:       store,
		MaxConcurrent:    1,
		PollInterval:     100 * time.Millisecond,
	}

	worker, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	ctx := context.Background()

	// Start worker
	worker.Start(ctx)

	// Enqueue a task
	task := queue.NewTask(queue.TaskTypeActivity, "wf-456", "test-input")
	task.ActivityID = "act-456"
	task.ActivityName = "failing-activity"

	q.Enqueue(ctx, "test-queue", task)

	// Wait for task to be processed
	time.Sleep(500 * time.Millisecond)

	// Stop worker
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Stop(stopCtx)

	// Verify activity state shows failure
	activityState, err := store.GetActivityState(ctx, "act-456")
	if err != nil {
		t.Fatalf("failed to get activity state: %v", err)
	}

	if activityState.Status != state.StatusFailed {
		t.Errorf("expected status failed, got %s", activityState.Status)
	}

	if activityState.Error == "" {
		t.Error("expected error to be recorded")
	}

	// Verify failure event was recorded
	events, err := store.GetEvents(ctx, "wf-456")
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	foundFailure := false
	for _, event := range events {
		if event.Type == state.EventActivityFailed {
			foundFailure = true
			break
		}
	}

	if !foundFailure {
		t.Error("expected activity failure event to be recorded")
	}
}

func TestWorker_Activity_Idempotency_Table(t *testing.T) {
    actReg := activity.NewRegistry()
    actReg.Register("echo", activity.ActivityFunc(func(ctx context.Context, in interface{}) (interface{}, error) {
        return in, nil
    }), activity.Info{})

    cases := []struct{
        name            string
        preState        *state.ActivityState
        expectStarted   int
        expectCompleted int
        expectOutput    interface{}
    }{
        {name: "fresh", preState: nil, expectStarted: 1, expectCompleted: 1, expectOutput: "in"},
        {name: "already_started", preState: &state.ActivityState{ActivityID: "act-fixed", ActivityName: "echo", WorkflowID: "wf-1", Status: state.StatusRunning, Input: "in", StartTime: time.Now().UTC()}, expectStarted: 0, expectCompleted: 1, expectOutput: "in"},
        {name: "already_completed", preState: &state.ActivityState{ActivityID: "act-fixed", ActivityName: "echo", WorkflowID: "wf-1", Status: state.StatusCompleted, Input: "in", Output: "cached", StartTime: time.Now().UTC()}, expectStarted: 0, expectCompleted: 0, expectOutput: "cached"},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            store := state.NewInMemoryStore()
            q := queue.NewInMemoryQueue()
            defer q.Close()

            w, err := New(Config{ID: "w1", Queue: q, QueueName: "default", ActivityRegistry: actReg, StateStore: store, MaxConcurrent: 1, PollInterval: 50 * time.Millisecond})
            if err != nil { t.Fatalf("new worker: %v", err) }

            if tc.preState != nil {
                if err := store.SaveActivityState(context.Background(), tc.preState); err != nil { t.Fatalf("pre save: %v", err) }
            }

            tsk := queue.NewTask(queue.TaskTypeActivity, "wf-1", "in")
            tsk.ActivityID = "act-fixed"
            tsk.ActivityName = "echo"

            res := w.executeActivity(context.Background(), tsk)
            if !res.Success { t.Fatalf("exec failed: %v", res.Error) }
            if res.Output != tc.expectOutput { t.Fatalf("output: got %v want %v", res.Output, tc.expectOutput) }

            evs, err := store.GetEvents(context.Background(), "wf-1")
            if err != nil { t.Fatalf("get events: %v", err) }
            started, completed := 0, 0
            for _, e := range evs {
                if e.Type == state.EventActivityStarted { started++ }
                if e.Type == state.EventActivityCompleted { completed++ }
            }
            if started != tc.expectStarted { t.Fatalf("started: got %d want %d", started, tc.expectStarted) }
            if completed != tc.expectCompleted { t.Fatalf("completed: got %d want %d", completed, tc.expectCompleted) }
        })
    }
}
