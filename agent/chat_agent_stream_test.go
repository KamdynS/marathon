package agent

import (
	"context"
	"testing"

	"github.com/KamdynS/marathon/llm"
)

type fakeStream struct {
	idx    int
	closed bool
	deltas []llm.Delta
}

func (s *fakeStream) Recv(ctx context.Context) (llm.Delta, error) {
	if s.closed {
		return llm.Delta{}, llm.ErrStreamClosed
	}
	if s.idx >= len(s.deltas) {
		s.closed = true
		return llm.Delta{Type: llm.DeltaTypeDone, Provider: "fake", Model: "fake"}, nil
	}
	d := s.deltas[s.idx]
	s.idx++
	return d, nil
}

func (s *fakeStream) Close() error { s.closed = true; return nil }

type fakeLLM struct{}

func (f *fakeLLM) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.Response, error) {
	return &llm.Response{Content: ""}, nil
}
func (f *fakeLLM) Completion(ctx context.Context, prompt string) (*llm.Response, error) {
	return &llm.Response{Content: ""}, nil
}
func (f *fakeLLM) Stream(ctx context.Context, req *llm.ChatRequest, output chan<- *llm.Response) error {
	close(output)
	return nil
}
func (f *fakeLLM) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	return &fakeStream{deltas: []llm.Delta{
		{Type: llm.DeltaTypeText, Text: "Hello"},
		{Type: llm.DeltaTypeText, Text: " world"},
		{Type: llm.DeltaTypeDone},
	}}, nil
}
func (f *fakeLLM) Model() string { return "fake" }

func TestChatAgent_Stream_ProviderNeutral(t *testing.T) {
	a := NewChatAgent(&fakeLLM{}, AgentConfig{SystemPrompt: ""}, nil)
	out := make(chan Message, 8)
	err := a.RunStream(context.Background(), Message{Role: "user", Content: "hi"}, out)
	if err != nil {
		t.Fatalf("RunStream error: %v", err)
	}
	var seen []Message
	for m := range out {
		seen = append(seen, m)
	}
	if len(seen) == 0 {
		t.Fatalf("no messages streamed")
	}
	last := seen[len(seen)-1]
	if last.Content != "Hello world" {
		t.Fatalf("unexpected final content: %q", last.Content)
	}
}
