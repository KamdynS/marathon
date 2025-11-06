package activity

import (
	"context"

	"github.com/KamdynS/marathon/state"
)

// EventEmitter is a function that appends an event with provided type and data.
type EventEmitter func(eventType state.EventType, data map[string]interface{}) error

type eventEmitterKey struct{}
type activityCtxKey struct{}

// WithEventEmitter attaches an EventEmitter to a context.
func WithEventEmitter(ctx context.Context, emit EventEmitter) context.Context {
	return context.WithValue(ctx, eventEmitterKey{}, emit)
}

// GetEventEmitter extracts an EventEmitter from context if present.
func GetEventEmitter(ctx context.Context) (EventEmitter, bool) {
	v := ctx.Value(eventEmitterKey{})
	if v == nil {
		return nil, false
	}
	emitter, ok := v.(EventEmitter)
	return emitter, ok
}

// ActivityEventContext carries store and workflowID for idempotent event/tool behaviors.
type ActivityEventContext struct {
	Store      state.Store
	WorkflowID string
}

// WithActivityEventContext attaches ActivityEventContext to context.
func WithActivityEventContext(ctx context.Context, aec ActivityEventContext) context.Context {
	return context.WithValue(ctx, activityCtxKey{}, aec)
}

// GetActivityEventContext retrieves ActivityEventContext if present.
func GetActivityEventContext(ctx context.Context) (ActivityEventContext, bool) {
	v := ctx.Value(activityCtxKey{})
	if v == nil {
		return ActivityEventContext{}, false
	}
	aec, ok := v.(ActivityEventContext)
	return aec, ok
}
