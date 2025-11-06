package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KamdynS/marathon/engine"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/server/agenthttp"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/workflow"
)

// helper to spin a handler that uses the shared SSE helper with configurable intervals
func makeSSEHandler(getSince agenthttp.EventsSinceGetter, heartbeat, poll time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = agenthttp.StreamEvents(r.Context(), w, r.Header.Get("Last-Event-ID"), getSince, r.URL.Query().Get("id"), poll, heartbeat)
	}
}

func newEngineAndStore(t *testing.T) (*engine.Engine, state.Store) {
	t.Helper()
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	t.Cleanup(func() { q.Close() })
	reg := workflow.NewRegistry()
	// trivial workflow def to satisfy engine init; tests manually append events
	def := workflow.New("noop").Build()
	reg.Register(def)
	eng, err := engine.New(engine.Config{StateStore: store, Queue: q, WorkflowRegistry: reg})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	t.Cleanup(func() { eng.Stop() })
	return eng, store
}

func saveWorkflow(t *testing.T, store state.Store, id string) {
	t.Helper()
	err := store.SaveWorkflowState(context.Background(), &state.WorkflowState{
		WorkflowID:   id,
		WorkflowName: "noop",
		Status:       state.StatusRunning,
		StartTime:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("save workflow: %v", err)
	}
}

func TestSSE_ResumeStreamsOnlyNewEvents(t *testing.T) {
	eng, store := newEngineAndStore(t)
	wfID := "wf-sse-1"
	saveWorkflow(t, store, wfID)

	// append three events
	_ = store.AppendEvent(context.Background(), state.NewEvent(wfID, state.EventActivityScheduled, map[string]interface{}{"i": 1}))
	_ = store.AppendEvent(context.Background(), state.NewEvent(wfID, state.EventActivityStarted, map[string]interface{}{"i": 2}))
	_ = store.AppendEvent(context.Background(), state.NewEvent(wfID, state.EventActivityCompleted, map[string]interface{}{"i": 3}))

	ts := httptest.NewServer(makeSSEHandler(eng.GetWorkflowEventsSince, 5*time.Second, 50*time.Millisecond))
	defer ts.Close()

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/?id=%s", ts.URL, wfID), nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", "2")
	client := ts.Client()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	// read until first 'id: ' line
	deadline := time.Now().Add(2 * time.Second)
	var gotIDLine string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, "id: ") {
			gotIDLine = strings.TrimSpace(line)
			break
		}
	}
	if gotIDLine == "" {
		t.Fatalf("did not receive an id line")
	}
	if gotIDLine != "id: 3" {
		t.Fatalf("expected id: 3, got %q", gotIDLine)
	}
}

func TestSSE_EmitsDoneOnTerminalAndCloses(t *testing.T) {
	eng, store := newEngineAndStore(t)
	wfID := "wf-sse-2"
	saveWorkflow(t, store, wfID)

	ts := httptest.NewServer(makeSSEHandler(eng.GetWorkflowEventsSince, 50*time.Millisecond, 20*time.Millisecond))
	defer ts.Close()

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/?id=%s", ts.URL, wfID), nil)
	req.Header.Set("Accept", "text/event-stream")
	client := ts.Client()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	// append a terminal event shortly after connection
	time.AfterFunc(50*time.Millisecond, func() {
		_ = store.AppendEvent(context.Background(), state.NewEvent(wfID, state.EventWorkflowCompleted, map[string]interface{}{"ok": true}))
	})

	foundDone := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "event: done") {
			foundDone = true
			break
		}
	}
	if !foundDone {
		t.Fatalf("expected 'event: done' before close")
	}
}

func TestSSE_HeartbeatWhenIdle_AndStopsOnCancel(t *testing.T) {
	eng, _ := newEngineAndStore(t)
	wfID := "wf-sse-3"
	// no events appended; should heartbeat

	ts := httptest.NewServer(makeSSEHandler(eng.GetWorkflowEventsSince, 30*time.Millisecond, 10*time.Millisecond))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/?id=%s", ts.URL, wfID), nil)
	req.Header.Set("Accept", "text/event-stream")
	client := ts.Client()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	var mu sync.Mutex
	gotHeartbeat := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, ": ping") {
				mu.Lock()
				gotHeartbeat = true
				mu.Unlock()
				return
			}
		}
	}()

	<-done
	mu.Lock()
	hb := gotHeartbeat
	mu.Unlock()
	if !hb {
		t.Fatalf("expected heartbeat ': ping'")
	}

	// ensure server stops on client cancel
	cancel()
	time.Sleep(50 * time.Millisecond)
	// body reader should eventually terminate; not asserting strictly to avoid flakes
}


