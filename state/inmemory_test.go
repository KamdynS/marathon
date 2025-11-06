package state

import (
	"context"
	"sort"
	"testing"
	"time"
)

func TestInMemoryStore_IdempotencyKey_Table(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	type step struct {
		key         string
		workflowID  string
		wantCreated bool
		wantExists  string
	}

	steps := []step{
		{key: "a", workflowID: "wf-1", wantCreated: true, wantExists: ""},
		{key: "a", workflowID: "wf-2", wantCreated: false, wantExists: "wf-1"},
		{key: "b", workflowID: "wf-3", wantCreated: true, wantExists: ""},
	}

	for i, st := range steps {
		created, existing, err := store.MapIdempotencyKeyToWorkflow(ctx, st.key, st.workflowID)
		if err != nil {
			t.Fatalf("step %d: map error: %v", i, err)
		}
		if created != st.wantCreated {
			t.Fatalf("step %d: created=%v want %v", i, created, st.wantCreated)
		}
		if existing != st.wantExists {
			t.Fatalf("step %d: existing=%q want %q", i, existing, st.wantExists)
		}
	}

	if got, ok, _ := store.GetWorkflowIDByIdempotencyKey(ctx, "a"); !ok || got != "wf-1" {
		t.Fatalf("lookup a: got (%v,%v) want (true,wf-1)", ok, got)
	}
}

func TestInMemoryStore_Timers_Table(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	wf := "wf-timer"

	cases := []struct {
		name       string
		offset     time.Duration
		expectDue  int
		waitThen   time.Duration
		expectFire bool
	}{
		{name: "due_now", offset: -10 * time.Millisecond, expectDue: 1, waitThen: 0, expectFire: true},
		{name: "future_then_due", offset: 200 * time.Millisecond, expectDue: 0, waitThen: 250 * time.Millisecond, expectFire: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			timerID := "tm-" + tc.name
			fireAt := time.Now().Add(tc.offset).UTC()
			if err := store.ScheduleTimer(ctx, wf, timerID, fireAt); err != nil {
				t.Fatalf("schedule: %v", err)
			}

			due, err := store.ListDueTimers(ctx, time.Now())
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(due) != tc.expectDue {
				t.Fatalf("expect due=%d got %d", tc.expectDue, len(due))
			}

			if tc.waitThen > 0 {
				time.Sleep(tc.waitThen)
			}

			transitioned, err := store.MarkTimerFired(ctx, wf, timerID)
			if err != nil {
				t.Fatalf("mark: %v", err)
			}
			if transitioned != tc.expectFire {
				t.Fatalf("expectFire %v got %v", tc.expectFire, transitioned)
			}

			transitioned2, err := store.MarkTimerFired(ctx, wf, timerID)
			if err != nil {
				t.Fatalf("mark2: %v", err)
			}
			if transitioned2 {
				t.Fatalf("second mark should not transition")
			}
		})
	}
}

func TestEventSequence_MonotonicAndAtomic(t *testing.T) {
	store := NewInMemoryStore()
	wf := "wf-seq"

	_ = store.SaveWorkflowState(context.Background(), &WorkflowState{WorkflowID: wf, WorkflowName: "seq", Status: StatusRunning, StartTime: time.Now().UTC()})

	n := 50
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			_ = store.AppendEvent(context.Background(), NewEvent(wf, EventSignalReceived, map[string]interface{}{"i": i}))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	evs, err := store.GetEvents(context.Background(), wf)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}

	if len(evs) != n {
		t.Fatalf("expected %d events, got %d", n, len(evs))
	}

	seqs := make([]int64, len(evs))
	for i, e := range evs {
		seqs[i] = e.SequenceNum
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for i, s := range seqs {
		expected := int64(i + 1)
		if s != expected {
			t.Fatalf("expected seq %d, got %d", expected, s)
		}
	}
}
