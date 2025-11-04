package activity

import (
	"context"
	"fmt"

	"github.com/KamdynS/marathon/agent"
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
	default:
		content = fmt.Sprintf("%v", v)
	}
	resp, err := a.Agent.Run(ctx, agent.Message{Role: "user", Content: content})
	if err != nil {
		return nil, err
	}
	return resp.Content, nil
}
