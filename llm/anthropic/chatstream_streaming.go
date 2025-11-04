//go:build anthropicstream
// +build anthropicstream

package anthropic

import (
	"context"

	base "github.com/KamdynS/marathon/llm"
	anth "github.com/anthropics/anthropic-sdk-go"
)

type anthStreamWrapper struct {
	inner  *anth.MessageStream
	model  string
	closed bool
}

func (c *Client) ChatStream(ctx context.Context, req *base.ChatRequest) (base.Stream, error) {
	params := toAnthParams(req, c.cfg)
	s := c.client.Messages.NewStreaming(ctx, params)
	return &anthStreamWrapper{inner: s, model: string(params.Model)}, nil
}

func (w *anthStreamWrapper) Recv(ctx context.Context) (base.Delta, error) {
	if w.closed {
		return base.Delta{}, base.ErrStreamClosed
	}
	ev, err := w.inner.Recv()
	if err != nil {
		w.closed = true
		return base.Delta{Type: base.DeltaTypeDone, Provider: "anthropic", Model: w.model}, nil
	}
	// Map a subset of common event types to text deltas
	switch ev.Type {
	case "content_block_delta":
		if ev.Delta != nil && ev.Delta.Text != "" {
			return base.Delta{Type: base.DeltaTypeText, Text: ev.Delta.Text, Provider: "anthropic", Model: w.model}, nil
		}
	case "message_stop":
		w.closed = true
		return base.Delta{Type: base.DeltaTypeDone, Provider: "anthropic", Model: w.model}, nil
	}
	return base.Delta{}, nil
}

func (w *anthStreamWrapper) Close() error { w.closed = true; w.inner.Close(); return nil }
