package activity

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/KamdynS/marathon/state"
)

func TestEventEmitter_ContextRoundTripAndCall(t *testing.T) {
	var called int32
	emitter := func(eventType state.EventType, data map[string]interface{}) error {
		atomic.AddInt32(&called, 1)
		if eventType != state.EventAgentMessage {
			t.Fatalf("unexpected event type: %s", eventType)
		}
		if data["k"] != "v" {
			t.Fatalf("unexpected data: %#v", data)
		}
		return nil
	}
	ctx := WithEventEmitter(context.Background(), emitter)
	got, ok := GetEventEmitter(ctx)
	if !ok || got == nil {
		t.Fatalf("expected emitter present")
	}
	_ = got(state.EventAgentMessage, map[string]interface{}{"k": "v"})
	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected emitter to be called once")
	}
}

func TestActivityEventContext_RoundTrip(t *testing.T) {
	store := state.NewInMemoryStore()
	aec := ActivityEventContext{Store: store, WorkflowID: "wf-1"}
	ctx := WithActivityEventContext(context.Background(), aec)
	got, ok := GetActivityEventContext(ctx)
	if !ok {
		t.Fatalf("expected activity event context present")
	}
	if got.WorkflowID != "wf-1" || got.Store == nil {
		t.Fatalf("unexpected activity event context: %+v", got)
	}
}


