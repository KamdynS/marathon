package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/KamdynS/marathon/agent"
	"github.com/KamdynS/marathon/state"
)

// AgentActivity lets workflows invoke an Agent as a step.
type AgentActivity struct {
	NameStr string
	Agent   agent.Agent
}

func (a *AgentActivity) Name() string { return a.NameStr }

func (a *AgentActivity) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if a.Agent == nil {
		return nil, fmt.Errorf("nil agent")
	}
	var content string
	var iterationID string
	switch v := input.(type) {
	case string:
		content = v
	case map[string]interface{}:
		// Minimal: try to find a 'message' field
		if m, ok := v["message"].(string); ok {
			content = m
		} else {
			content = fmt.Sprintf("%v", v)
		}
		if it, ok := v["iteration_id"].(string); ok {
			iterationID = it
		}
	default:
		content = fmt.Sprintf("%v", v)
	}

	emit, _ := GetEventEmitter(ctx)
	aec, _ := GetActivityEventContext(ctx)

	// If a previous final message for this iteration exists, return it (idempotency on retry)
	if aec.Store != nil && aec.WorkflowID != "" && iterationID != "" {
		if prior := findFinalAssistantMessage(ctx, aec.Store, aec.WorkflowID, iterationID); prior != "" {
			return prior, nil
		}
	}
	// Plan event
	// Deduplicate planned event
	if emit != nil && !eventExists(ctx, aec.Store, aec.WorkflowID, state.EventAgentStepPlanned, iterationID, "") {
		_ = emit(state.EventAgentStepPlanned, map[string]interface{}{
			"iteration_id": iterationID,
			"goal":         content,
			"ts":           time.Now().UTC(),
		})
	}

	out := make(chan agent.Message, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Agent.RunStream(ctx, agent.Message{Role: "user", Content: content}, out)
	}()

	var final string
	done := false
	var lastToolName string
	var toolResultEmitted bool
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg, ok := <-out:
			if !ok {
				out = nil
				if done {
					return final, nil
				}
				continue
			}
			switch msg.Role {
			case "tool_call":
				if n := msg.Meta["name"]; n != "" {
					lastToolName = n
				}
				// If tool_called already recorded for this iteration and name, skip duplicate emit
				if aec.Store != nil && aec.WorkflowID != "" && iterationID != "" {
					if alreadyToolCalled(ctx, aec.Store, aec.WorkflowID, iterationID, msg.Meta["name"]) {
						break
					}
				}
				if emit != nil {
					_ = emit(state.EventAgentToolCalled, map[string]interface{}{
						"iteration_id": iterationID,
						"name":         msg.Meta["name"],
						"arguments":    msg.Meta["arguments"],
					})
				}
			case "tool_result":
				toolResultEmitted = true
				// If tool_result already exists for this iteration and name, skip duplicate emit
				if aec.Store != nil && aec.WorkflowID != "" && iterationID != "" {
					if alreadyToolResult(ctx, aec.Store, aec.WorkflowID, iterationID, msg.Meta["name"]) {
						break
					}
				}
				if emit != nil {
					_ = emit(state.EventAgentToolResult, map[string]interface{}{
						"iteration_id": iterationID,
						"name":         msg.Meta["name"],
						"result":       msg.Content,
					})
				}
			case "assistant":
				// Streaming chunk or final text
				if emit != nil {
					_ = emit(state.EventAgentMessage, map[string]interface{}{
						"iteration_id": iterationID,
						"role":         "assistant",
						"content":      msg.Content,
						"streaming":    msg.Meta["streaming"],
					})
				}
				// Accumulate final output; the ChatAgent sends the final full buffer at the end
				if msg.Meta == nil || msg.Meta["streaming"] == "" {
					final = msg.Content
				}
			}
		case err := <-errCh:
			if err != nil {
				if emit != nil {
					_ = emit(state.EventAgentMessage, map[string]interface{}{
						"iteration_id": iterationID,
						"role":         "assistant",
						"content":      fmt.Sprintf("error: %v", err),
						"streaming":    "",
					})
				}
				return nil, err
			}
			// Completed; wait for channel to close/drain before returning
			done = true
			if out == nil {
				// Ensure tool_result is present if a tool was called
				if emit != nil && lastToolName != "" && !toolResultEmitted {
					_ = emit(state.EventAgentToolResult, map[string]interface{}{
						"iteration_id": iterationID,
						"name":         lastToolName,
						"result":       "",
					})
				}
				return final, nil
			}
		}
	}
}

func eventExists(ctx context.Context, store state.Store, workflowID string, t state.EventType, iterationID string, name string) bool {
	if store == nil || workflowID == "" {
		return false
	}
	evs, err := store.GetEventsSince(ctx, workflowID, 0)
	if err != nil {
		return false
	}
	for _, e := range evs {
		if e.Type != t {
			continue
		}
		if id, ok := e.Data["iteration_id"].(string); ok && id == iterationID {
			if name == "" {
				return true
			}
			if n, ok := e.Data["name"].(string); ok && n == name {
				return true
			}
		}
	}
	return false
}

func findFinalAssistantMessage(ctx context.Context, store state.Store, workflowID string, iterationID string) string {
	evs, err := store.GetEventsSince(ctx, workflowID, 0)
	if err != nil {
		return ""
	}
	var last string
	for _, e := range evs {
		if e.Type == state.EventAgentMessage {
			if id, ok := e.Data["iteration_id"].(string); ok && id == iterationID {
				// consider final when streaming is empty
				if s, ok := e.Data["streaming"].(string); ok && s != "" {
					continue
				}
				if role, ok := e.Data["role"].(string); ok && role == "assistant" {
					if c, ok := e.Data["content"].(string); ok {
						last = c
					}
				}
			}
		}
	}
	return last
}

func alreadyToolCalled(ctx context.Context, store state.Store, workflowID, iterationID, name string) bool {
	evs, err := store.GetEventsSince(ctx, workflowID, 0)
	if err != nil {
		return false
	}
	for _, e := range evs {
		if e.Type == state.EventAgentToolCalled {
			if id, ok := e.Data["iteration_id"].(string); ok && id == iterationID {
				if n, ok := e.Data["name"].(string); ok && n == name {
					return true
				}
			}
		}
	}
	return false
}

func alreadyToolResult(ctx context.Context, store state.Store, workflowID, iterationID, name string) bool {
	evs, err := store.GetEventsSince(ctx, workflowID, 0)
	if err != nil {
		return false
	}
	for _, e := range evs {
		if e.Type == state.EventAgentToolResult {
			if id, ok := e.Data["iteration_id"].(string); ok && id == iterationID {
				if n, ok := e.Data["name"].(string); ok && n == name {
					return true
				}
			}
		}
	}
	return false
}
