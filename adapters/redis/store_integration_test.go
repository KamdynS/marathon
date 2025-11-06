package redisstore

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/KamdynS/marathon/state"
)

func redisAddrFromEnv(t *testing.T) string {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping Redis integration tests")
	}
	return addr
}

func newTestStore(t *testing.T) *Store {
	addr := redisAddrFromEnv(t)
	cfg := Config{
		Addr:   addr,
		Prefix: "marathon-test-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New redis store: %v", err)
	}
	t.Cleanup(func() {
		// cleanup keys with this prefix
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rdb := redis.NewClient(&redis.Options{Addr: addr})
		defer rdb.Close()
		var cursor uint64
		for {
			keys, cur, err := rdb.Scan(ctx, cursor, cfg.Prefix+"*", 200).Result()
			if err != nil {
				break
			}
			cursor = cur
			if len(keys) > 0 {
				_ = rdb.Del(ctx, keys...).Err()
			}
			if cursor == 0 {
				break
			}
		}
		_ = s.Close()
	})
	return s
}

func TestStateRoundTripAndStatusIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	wf := &state.WorkflowState{
		WorkflowID:   "wf-1",
		WorkflowName: "demo",
		Status:       state.StatusPending,
		Input:        map[string]interface{}{"x": 1},
		StartTime:    now,
		TaskQueue:    "q",
	}
	if err := s.SaveWorkflowState(ctx, wf); err != nil {
		t.Fatalf("SaveWorkflowState: %v", err)
	}
	got, err := s.GetWorkflowState(ctx, wf.WorkflowID)
	if err != nil {
		t.Fatalf("GetWorkflowState: %v", err)
	}
	if got.WorkflowID != wf.WorkflowID || got.Status != wf.Status {
		t.Fatalf("workflow state mismatch: got %+v", got)
	}
	// status index inclusion
	list, err := s.ListWorkflows(ctx, state.StatusPending)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(list) != 1 || list[0].WorkflowID != wf.WorkflowID {
		t.Fatalf("expected 1 pending workflow, got %d", len(list))
	}
	// change status
	wf.Status = state.StatusRunning
	if err := s.SaveWorkflowState(ctx, wf); err != nil {
		t.Fatalf("SaveWorkflowState 2: %v", err)
	}
	list2, err := s.ListWorkflows(ctx, state.StatusPending)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(list2) != 0 {
		t.Fatalf("expected 0 pending after status change, got %d", len(list2))
	}
	list3, err := s.ListWorkflows(ctx, state.StatusRunning)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(list3) != 1 || list3[0].WorkflowID != wf.WorkflowID {
		t.Fatalf("expected 1 running workflow, got %d", len(list3))
	}
}

func TestActivityStateRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	act := &state.ActivityState{
		ActivityID:   "act-1",
		ActivityName: "foo",
		WorkflowID:   "wf-2",
		Status:       state.StatusRunning,
		Input:        map[string]interface{}{"y": 2},
		StartTime:    now,
		Attempt:      1,
	}
	if err := s.SaveActivityState(ctx, act); err != nil {
		t.Fatalf("SaveActivityState: %v", err)
	}
	got, err := s.GetActivityState(ctx, act.ActivityID)
	if err != nil {
		t.Fatalf("GetActivityState: %v", err)
	}
	if got.ActivityID != act.ActivityID || got.WorkflowID != act.WorkflowID {
		t.Fatalf("activity state mismatch: got %+v", got)
	}
}

func TestAppendEventConcurrentSequencing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	wfID := "wf-evt"
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			ev := state.NewEvent(wfID, state.EventAgentMessage, map[string]interface{}{"i": i})
			_ = s.AppendEvent(ctx, ev)
		}(i)
	}
	wg.Wait()
	evs, err := s.GetEvents(ctx, wfID)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(evs) != N {
		t.Fatalf("expected %d events, got %d", N, len(evs))
	}
	seen := make(map[int64]bool, N)
	for idx, e := range evs {
		if e.SequenceNum <= 0 {
			t.Fatalf("event has non-positive sequence: %+v", e)
		}
		if idx > 0 && evs[idx-1].SequenceNum >= e.SequenceNum {
			t.Fatalf("events not strictly increasing: %d >= %d", evs[idx-1].SequenceNum, e.SequenceNum)
		}
		seen[e.SequenceNum] = true
	}
	for i := 1; i <= N; i++ {
		if !seen[int64(i)] {
			t.Fatalf("missing sequence %d", i)
		}
	}
}

func TestGetEventsSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	wfID := "wf-since"
	for i := 0; i < 10; i++ {
		ev := state.NewEvent(wfID, state.EventAgentMessage, map[string]interface{}{"i": i})
		if err := s.AppendEvent(ctx, ev); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	evs, err := s.GetEventsSince(ctx, wfID, 5)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(evs) != 4 {
		t.Fatalf("expected 4 events > 5, got %d", len(evs))
	}
	for _, e := range evs {
		if e.SequenceNum <= 5 {
			t.Fatalf("expected seq > 5, got %d", e.SequenceNum)
		}
	}
}

func TestIdempotencyHelpers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	created, existing, err := s.MapIdempotencyKeyToWorkflow(ctx, "idem-1", "wf-a")
	if err != nil {
		t.Fatalf("MapIdempotencyKeyToWorkflow: %v", err)
	}
	if !created || existing != "" {
		t.Fatalf("expected created first time")
	}
	created2, existing2, err := s.MapIdempotencyKeyToWorkflow(ctx, "idem-1", "wf-b")
	if err != nil {
		t.Fatalf("MapIdempotencyKeyToWorkflow 2: %v", err)
	}
	if created2 || existing2 != "wf-a" {
		t.Fatalf("expected existing wf-a, got %v %s", created2, existing2)
	}
	id, ok, err := s.GetWorkflowIDByIdempotencyKey(ctx, "idem-1")
	if err != nil {
		t.Fatalf("GetWorkflowIDByIdempotencyKey: %v", err)
	}
	if !ok || id != "wf-a" {
		t.Fatalf("expected wf-a, got %s %v", id, ok)
	}
	// Activity idempotency helper
	ok1, err := s.MarkActivityIfFirst(ctx, "act-x")
	if err != nil || !ok1 {
		t.Fatalf("MarkActivityIfFirst 1: %v %v", ok1, err)
	}
	ok2, err := s.MarkActivityIfFirst(ctx, "act-x")
	if err != nil || ok2 {
		t.Fatalf("MarkActivityIfFirst 2 expected false: %v %v", ok2, err)
	}
}

func TestTimersFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	wf := "wf-timer"
	tid := "t-1"
	fireAt := now.Add(200 * time.Millisecond)
	if err := s.ScheduleTimer(ctx, wf, tid, fireAt); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	// no due yet
	due0, err := s.ListDueTimers(ctx, now)
	if err != nil {
		t.Fatalf("ListDueTimers: %v", err)
	}
	if len(due0) != 0 {
		t.Fatalf("expected 0 due timers initially")
	}
	time.Sleep(250 * time.Millisecond)
	due, err := s.ListDueTimers(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListDueTimers: %v", err)
	}
	if len(due) != 1 || due[0].WorkflowID != wf || due[0].TimerID != tid {
		t.Fatalf("expected 1 due timer, got %+v", due)
	}
	// fire
	changed, err := s.MarkTimerFired(ctx, wf, tid)
	if err != nil || !changed {
		t.Fatalf("MarkTimerFired: %v %v", changed, err)
	}
	// idempotent
	changed2, err := s.MarkTimerFired(ctx, wf, tid)
	if err != nil || changed2 {
		t.Fatalf("MarkTimerFired idempotent expected false: %v %v", changed2, err)
	}
}


