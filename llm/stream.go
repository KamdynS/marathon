package llm

import (
	"context"
	"errors"
)

// DeltaType identifies the kind of streaming event emitted by a provider.
type DeltaType string

const (
	DeltaTypeText          DeltaType = "text"
	DeltaTypeToolCallStart DeltaType = "tool_call_start"
	DeltaTypeToolCallDelta DeltaType = "tool_call_delta"
	DeltaTypeToolCallEnd   DeltaType = "tool_call_end"
	DeltaTypeDone          DeltaType = "done"
)

// ToolCallChunk represents an incremental tool call payload.
type ToolCallChunk struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"` // chunked JSON string
}

// Delta is a provider-neutral streaming event.
type Delta struct {
	Type      DeltaType      `json:"type"`
	Text      string         `json:"text,omitempty"`
	ToolChunk *ToolCallChunk `json:"tool_chunk,omitempty"`
	// Provider/model are optional hints for observability
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Stream provides a pull-based API over provider event streams.
// Implementations should return (Delta{Type: DeltaTypeDone}, nil) or io.EOF when complete.
type Stream interface {
	Recv(ctx context.Context) (Delta, error)
	Close() error
}

// ErrStreamClosed indicates Recv was called after Close or terminal event.
var ErrStreamClosed = errors.New("stream closed")
