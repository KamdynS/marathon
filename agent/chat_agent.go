package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/tools"
)

// ChatAgent implements a simple ReAct-like loop with tool calls.
type ChatAgent struct {
	Model      llm.Client
	Config     AgentConfig
	Tools      tools.Registry
	middleware []Middleware
	processors []MemoryProcessor
	resolver   ConfigResolver
}

// NewChatAgent constructs a ChatAgent.
func NewChatAgent(model llm.Client, cfg AgentConfig, reg tools.Registry) *ChatAgent {
	return &ChatAgent{Model: model, Config: cfg, Tools: reg}
}

// UseMiddleware adds middleware hooks.
func (a *ChatAgent) UseMiddleware(m ...Middleware) { a.middleware = append(a.middleware, m...) }

// UseProcessors adds memory processors.
func (a *ChatAgent) UseProcessors(p ...MemoryProcessor) { a.processors = append(a.processors, p...) }

// WithResolver sets a dynamic resolver.
func (a *ChatAgent) WithResolver(r ConfigResolver) { a.resolver = r }

// Run executes one iteration of LLM call and optional tool calls.
func (a *ChatAgent) Run(ctx context.Context, input Message) (Message, error) {
	outCh := make(chan Message, 1)
	if err := a.RunStream(ctx, input, outCh); err != nil {
		return Message{}, err
	}
	var final Message
	for m := range outCh {
		final = m
	}
	return final, nil
}

// RunStream streams assistant output. Concatenates chunks into a final message.
func (a *ChatAgent) RunStream(ctx context.Context, input Message, output chan<- Message) error {
	defer close(output)

	// Resolve final config and tools
	effectiveConfig := a.Config
	effectiveTools := a.Tools
	if a.resolver != nil {
		rc, rt := a.resolver.Resolve(ctx, input, a.Config)
		if (rc != AgentConfig{}) {
			effectiveConfig = rc
		}
		if rt != nil {
			effectiveTools = rt
		}
	}

	// Build request messages
	msgs := []llm.Message{{Role: "system", Content: effectiveConfig.SystemPrompt}}
	msgs = append(msgs, llm.Message{Role: input.Role, Content: input.Content})

	// Prepare tool defs
	var toolDefs []llm.Tool
	if effectiveTools != nil {
		for _, name := range effectiveTools.List() {
			if t, ok := effectiveTools.Get(name); ok {
				toolDefs = append(toolDefs, llm.Tool{Type: "function", Function: llm.ToolFunction{Name: t.Name(), Description: t.Description(), Parameters: t.Schema()}})
			}
		}
	}

	// Issue LLM call (non-stream first to discover tool calls)
	req := &llm.ChatRequest{Messages: msgs, Tools: toolDefs, Model: effectiveConfig.ModelOverride}
	for _, m := range a.middleware {
		if err := m.BeforeLLMCall(ctx, req); err != nil {
			return err
		}
	}
	resp, err := a.Model.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("llm call failed: %w", err)
	}
	for _, m := range a.middleware {
		_ = m.AfterLLMResponse(ctx, resp)
	}

	// Handle at most one tool call per turn (multi-tool deferred)
	if len(resp.ToolCalls) > 0 && effectiveTools != nil {
		tc := resp.ToolCalls[0]
		if t, ok := effectiveTools.Get(tc.Name); ok {
			for _, m := range a.middleware {
				_ = m.BeforeToolExecute(ctx, tc.Name, tc.Arguments)
			}
			var inputStr = tc.Arguments
			var tmp map[string]interface{}
			_ = json.Unmarshal([]byte(tc.Arguments), &tmp)
			result, execErr := t.Execute(ctx, inputStr)
			for _, m := range a.middleware {
				_ = m.AfterToolExecute(ctx, tc.Name, result, execErr)
			}
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}
			msgs = append(msgs, llm.Message{Role: "tool", Content: result})
		}
		// Second pass after single tool result
		req2 := &llm.ChatRequest{Messages: msgs, Tools: toolDefs, Model: effectiveConfig.ModelOverride}
		resp2, err := a.Model.Chat(ctx, req2)
		if err != nil {
			return fmt.Errorf("llm call failed: %w", err)
		}
		for _, m := range a.middleware {
			_ = m.AfterLLMResponse(ctx, resp2)
		}
		if resp2.Content != "" {
			output <- Message{Role: "assistant", Content: resp2.Content}
		}
		for _, m := range a.middleware {
			_ = m.AfterRun(ctx, Message{Role: "assistant", Content: resp2.Content})
		}
		return nil
	}

	// If no tools, stream the response using provider-neutral ChatStream
	s, err := a.Model.ChatStream(ctx, req)
	if err != nil {
		// Fallback to legacy streaming if ChatStream is unavailable
		inner := make(chan *llm.Response, 16)
		go func() { _ = a.Model.Stream(ctx, req, inner) }()
		var buffer string
		for chunk := range inner {
			if chunk == nil {
				continue
			}
			if chunk.Content != "" {
				buffer += chunk.Content
				output <- Message{Role: "assistant", Content: chunk.Content, Meta: map[string]string{"streaming": "true"}}
			}
		}
		if buffer != "" {
			output <- Message{Role: "assistant", Content: buffer}
			for _, m := range a.middleware {
				_ = m.AfterRun(ctx, Message{Role: "assistant", Content: buffer})
			}
		}
		return nil
	}
	defer func() { _ = s.Close() }()
	var buffer string
	for {
		delta, derr := s.Recv(ctx)
		if derr != nil {
			break
		}
		switch delta.Type {
		case llm.DeltaTypeText:
			if delta.Text != "" {
				buffer += delta.Text
				output <- Message{Role: "assistant", Content: delta.Text, Meta: map[string]string{"streaming": "true"}}
			}
		case llm.DeltaTypeDone:
			// End of stream
			if buffer != "" {
				output <- Message{Role: "assistant", Content: buffer}
				for _, m := range a.middleware {
					_ = m.AfterRun(ctx, Message{Role: "assistant", Content: buffer})
				}
			}
			return nil
		default:
			// Ignore tool-call deltas here; tool execution is handled via non-stream path above.
		}
	}
	if buffer != "" {
		output <- Message{Role: "assistant", Content: buffer}
		for _, m := range a.middleware {
			_ = m.AfterRun(ctx, Message{Role: "assistant", Content: buffer})
		}
	}
	return nil
}
