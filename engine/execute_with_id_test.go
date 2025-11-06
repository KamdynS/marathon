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

func TestExecuteActivityWithID_Idempotent(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()

	actReg := activity.NewRegistry()
	var executedCount int
	testAct := activity.ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		executedCount++
		return "ok", nil
	})
	if err := actReg.Register("id-act", testAct, activity.Info{Description: "id act"}); err != nil {
		t.Fatalf("register activity: %v", err)
	}

	wfReg := workflow.NewRegistry()
	wf := workflow.WorkflowFunc(func(ctx workflow.Context, in interface{}) (interface{}, error) {
		f1 := ctx.ExecuteActivityWithID(context.Background(), "id-act", nil, "fixed")
		var out1 interface{}
		if err := f1.Get(context.Background(), &out1); err != nil {
			return nil, err
		}
		// Call again with the same ID; should not re-execute the activity
		f2 := ctx.ExecuteActivityWithID(context.Background(), "id-act", nil, "fixed")
		var out2 interface{}
		if err := f2.Get(context.Background(), &out2); err != nil {
			return nil, err
		}
		return out2, nil
	})
	def := &workflow.Definition{Name: "wf-id-act", Workflow: wf, Options: workflow.Options{TaskQueue: "default"}}
	if err := wfReg.Register(def); err != nil {
		t.Fatalf("register wf: %v", err)
	}

	eng, err := New(Config{StateStore: store, Queue: q, WorkflowRegistry: wfReg})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	w, err := worker.New(worker.Config{
		Queue:            q,
		QueueName:        "default",
		ActivityRegistry: actReg,
		StateStore:       store,
		MaxConcurrent:    1,
		PollInterval:     50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("worker: %v", err)
	}
	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()

	id, err := eng.StartWorkflow(ctx, "wf-id-act", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// wait for completion
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.GetWorkflowStatus(ctx, id)
		if st != nil && st.Status == state.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Verify executed once
	if executedCount != 1 {
		t.Fatalf("expected executed once, got %d", executedCount)
	}
	// Verify events recorded once
	evs, _ := eng.GetWorkflowEvents(ctx, id)
	started := 0
	completed := 0
	for _, e := range evs {
		if e.Type == state.EventActivityStarted {
			started++
		}
		if e.Type == state.EventActivityCompleted {
			completed++
		}
	}
	if started != 1 || completed != 1 {
		t.Fatalf("expected 1 started and 1 completed, got started=%d completed=%d", started, completed)
	}
}


