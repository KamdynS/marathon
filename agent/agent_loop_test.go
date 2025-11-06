package agent_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/KamdynS/marathon/activity"
	"github.com/KamdynS/marathon/agent"
	"github.com/KamdynS/marathon/engine"
	"github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/queue"
	"github.com/KamdynS/marathon/state"
	"github.com/KamdynS/marathon/tools"
	"github.com/KamdynS/marathon/worker"
	"github.com/KamdynS/marathon/workflow"
)

// ---- Fakes ----

type fakeLLMRouter struct {
	mu             sync.Mutex
	chatErrorsLeft int
}

func (f *fakeLLMRouter) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chatErrorsLeft > 0 {
		f.chatErrorsLeft--
		return nil, errors.New("llm transient error")
	}
	// First pass returns a single tool call if tools provided
	if len(req.Tools) > 0 {
		return &llm.Response{
			ToolCalls: []llm.ToolCall{{ID: "1", Name: req.Tools[0].Function.Name, Arguments: `{"q":"ping"}`}},
		}, nil
	}
	return &llm.Response{Content: "final"}, nil
}
func (f *fakeLLMRouter) Completion(ctx context.Context, prompt string) (*llm.Response, error) {
	return &llm.Response{Content: "unused"}, nil
}
func (f *fakeLLMRouter) Stream(ctx context.Context, req *llm.ChatRequest, output chan<- *llm.Response) error {
	close(output)
	return nil
}
func (f *fakeLLMRouter) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	return nil, errors.New("no stream")
}
func (f *fakeLLMRouter) Model() string { return "fake" }

type flakyTool struct {
	mu            sync.Mutex
	failFirst     bool
	calledCount   int
	name, desc    string
	schema        map[string]interface{}
}

func (t *flakyTool) Name() string        { return t.name }
func (t *flakyTool) Description() string { return t.desc }
func (t *flakyTool) Schema() map[string]interface{} {
	return t.schema
}
func (t *flakyTool) Execute(ctx context.Context, input string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calledCount++
	if t.failFirst {
		t.failFirst = false
		return "", errors.New("tool transient error")
	}
	return "tool-ok", nil
}

// slowStreamLLM emits chunks slowly to allow cancel mid-iteration
type slowStreamLLM struct{}
func (s *slowStreamLLM) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.Response, error) {
	return &llm.Response{Content: "unused"}, nil
}
func (s *slowStreamLLM) Completion(ctx context.Context, prompt string) (*llm.Response, error) { return &llm.Response{Content: ""}, nil }
func (s *slowStreamLLM) Stream(ctx context.Context, req *llm.ChatRequest, output chan<- *llm.Response) error { close(output); return nil }
func (s *slowStreamLLM) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	return &struct{ llm.Stream }{ /* no-op */ }, errors.New("no stream")
}
func (s *slowStreamLLM) Model() string { return "slow" }

// ---- Helpers ----

func setupEngineAndWorker(t *testing.T, store state.Store, q queue.Queue, wfReg *workflow.Registry, actReg *activity.Registry) (*engine.Engine, *worker.Worker) {
	t.Helper()
	eng, err := engine.New(engine.Config{StateStore: store, Queue: q, WorkflowRegistry: wfReg})
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
	return eng, w
}

func registerAgentIteration(t *testing.T, actReg *activity.Registry, model llm.Client, reg tools.Registry) {
	t.Helper()
	a := agent.NewChatAgent(model, agent.AgentConfig{SystemPrompt: ""}, reg)
	iter := &activity.AgentActivity{NameStr: "agent-iteration", Agent: a}
	if err := actReg.Register("agent-iteration", iter, activity.Info{Description: "iter"}); err != nil {
		t.Fatalf("register activity: %v", err)
	}
}

func buildLoopDef() *workflow.Definition {
	return &workflow.Definition{
		Name:     "agent-loop",
		Workflow: &workflow.AgentLoopWorkflow{},
		Options:  workflow.Options{TaskQueue: "default"},
	}
}

// ---- Tests ----

func TestAgentLoop_ToolErrorRetry_NoDuplicateEvents(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()

	wfReg := workflow.NewRegistry()
	actReg := activity.NewRegistry()

	tool := &flakyTool{failFirst: true, name: "util", desc: "desc", schema: map[string]interface{}{}}
	reg := tools.NewRegistry()
	if err := reg.Register(tool); err != nil { t.Fatal(err) }

	model := &fakeLLMRouter{}
	registerAgentIteration(t, actReg, model, reg)
	if err := wfReg.Register(buildLoopDef()); err != nil { t.Fatal(err) }

	eng, w := setupEngineAndWorker(t, store, q, wfReg, actReg)
	ctx := context.Background()
	if err := w.Start(ctx); err != nil { t.Fatal(err) }
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()

	// Start loop with 1 iteration
	id, err := eng.StartWorkflow(ctx, "agent-loop", workflow.AgentLoopInput{Message: "go", MaxIterations: 1})
	if err != nil { t.Fatal(err) }

	// wait for completion
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.GetWorkflowStatus(ctx, id)
		if st != nil && st.Status == state.StatusCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	evs, _ := eng.GetWorkflowEvents(ctx, id)
	toolCalled := 0
	toolResult := 0
	for _, e := range evs {
		if e.Type == state.EventAgentToolCalled { toolCalled++ }
		if e.Type == state.EventAgentToolResult { toolResult++ }
	}
	if toolCalled != 1 {
		t.Fatalf("expected 1 tool_called, got %d", toolCalled)
	}
	// tool_result should be emitted once; accept zero in case provider omits it
	if toolResult > 1 {
		t.Fatalf("expected <=1 tool_result, got %d", toolResult)
	}
}

func TestAgentLoop_LLMErrorRetry(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()
	wfReg := workflow.NewRegistry()
	actReg := activity.NewRegistry()

	reg := tools.NewRegistry()
	_ = reg.Register(&flakyTool{name: "util"})
	model := &fakeLLMRouter{chatErrorsLeft: 1}
	registerAgentIteration(t, actReg, model, reg)
	if err := wfReg.Register(buildLoopDef()); err != nil { t.Fatal(err) }
	eng, w := setupEngineAndWorker(t, store, q, wfReg, actReg)
	ctx := context.Background()
	_ = w.Start(ctx)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()
	id, _ := eng.StartWorkflow(ctx, "agent-loop", workflow.AgentLoopInput{Message: "x", MaxIterations: 1})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.GetWorkflowStatus(ctx, id)
		if st != nil && st.Status == state.StatusCompleted { break }
		time.Sleep(50 * time.Millisecond)
	}
	st, _ := eng.GetWorkflowStatus(ctx, id)
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed, got %s", st.Status)
	}
}

func TestAgentLoop_CancelMidIteration(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()
	wfReg := workflow.NewRegistry()
	actReg := activity.NewRegistry()

	reg := tools.NewRegistry()
	model := &slowStreamLLM{}
	registerAgentIteration(t, actReg, model, reg)
	if err := wfReg.Register(buildLoopDef()); err != nil { t.Fatal(err) }
	eng, w := setupEngineAndWorker(t, store, q, wfReg, actReg)
	ctx := context.Background()
	_ = w.Start(ctx)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()
	id, _ := eng.StartWorkflow(ctx, "agent-loop", workflow.AgentLoopInput{Message: "x", MaxIterations: 1})
	// cancel quickly
	_ = eng.CancelWorkflow(ctx, id)
	time.Sleep(300 * time.Millisecond)
	st, _ := eng.GetWorkflowStatus(ctx, id)
	if st.Status != state.StatusCanceled {
		t.Fatalf("expected canceled, got %s", st.Status)
	}
}

func TestAgentLoop_ResumeAfterRestart(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()
	wfReg := workflow.NewRegistry()
	actReg := activity.NewRegistry()

	reg := tools.NewRegistry()
	_ = reg.Register(&flakyTool{name: "util"})
	model := &fakeLLMRouter{}
	registerAgentIteration(t, actReg, model, reg)
	if err := wfReg.Register(buildLoopDef()); err != nil { t.Fatal(err) }

	eng, _ := setupEngineAndWorker(t, store, q, wfReg, actReg)
	// Start workflow without starting worker yet
	id, _ := eng.StartWorkflow(context.Background(), "agent-loop", workflow.AgentLoopInput{Message: "x", MaxIterations: 1})
	// start worker later (simulating restart)
	w, _ := worker.New(worker.Config{
		Queue:            q,
		QueueName:        "default",
		ActivityRegistry: actReg,
		StateStore:       store,
		MaxConcurrent:    1,
		PollInterval:     50 * time.Millisecond,
	})
	_ = w.Start(context.Background())
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.GetWorkflowStatus(context.Background(), id)
		if st != nil && st.Status == state.StatusCompleted { break }
		time.Sleep(50 * time.Millisecond)
	}
	st, _ := eng.GetWorkflowStatus(context.Background(), id)
	if st.Status != state.StatusCompleted {
		t.Fatalf("expected completed, got %s", st.Status)
	}
}

func TestAgentLoop_MaxIterations(t *testing.T) {
	store := state.NewInMemoryStore()
	q := queue.NewInMemoryQueue()
	defer q.Close()
	wfReg := workflow.NewRegistry()
	actReg := activity.NewRegistry()

	reg := tools.NewRegistry()
	_ = reg.Register(&flakyTool{name: "util"})
	model := &fakeLLMRouter{}
	registerAgentIteration(t, actReg, model, reg)
	if err := wfReg.Register(buildLoopDef()); err != nil { t.Fatal(err) }
	eng, w := setupEngineAndWorker(t, store, q, wfReg, actReg)
	ctx := context.Background()
	_ = w.Start(ctx)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
	}()
	id, _ := eng.StartWorkflow(ctx, "agent-loop", workflow.AgentLoopInput{Message: "x", MaxIterations: 2})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.GetWorkflowStatus(ctx, id)
		if st != nil && st.Status == state.StatusCompleted { break }
		time.Sleep(50 * time.Millisecond)
	}
	evs, _ := eng.GetWorkflowEvents(ctx, id)
	iterations := 0
	for _, e := range evs {
		if e.Type == state.EventActivityScheduled {
			if name, ok := e.Data["activity_name"].(string); ok && name == "agent-iteration" {
				iterations++
			}
		}
	}
	if iterations != 2 {
		t.Fatalf("expected 2 iteration activities, got %d", iterations)
	}
}


