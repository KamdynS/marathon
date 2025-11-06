package workflow

import (
	"context"
	"fmt"
)

// AgentLoopInput configures the agent loop workflow.
type AgentLoopInput struct {
	Message       string `json:"message"`
	MaxIterations int    `json:"max_iterations"`
}

// AgentLoopWorkflow runs agent iterations as activities with stable IDs.
type AgentLoopWorkflow struct{}

func (w *AgentLoopWorkflow) Name() string { return "agent-loop" }

func (w *AgentLoopWorkflow) Execute(ctx Context, input interface{}) (interface{}, error) {
	in := AgentLoopInput{Message: "", MaxIterations: 1}
	switch v := input.(type) {
	case AgentLoopInput:
		in = v
	case map[string]interface{}:
		if m, ok := v["message"].(string); ok {
			in.Message = m
		}
		if mi, ok := v["max_iterations"].(int); ok {
			in.MaxIterations = mi
		}
		if mi64, ok := v["max_iterations"].(int64); ok && in.MaxIterations == 0 {
			in.MaxIterations = int(mi64)
		}
	}
	if in.MaxIterations <= 0 {
		in.MaxIterations = 1
	}

	current := in.Message
	var final interface{}
	for i := 1; i <= in.MaxIterations; i++ {
		iterID := fmt.Sprintf("iteration-%d", i)
		payload := map[string]interface{}{
			"iteration_id": iterID,
			"message":      current,
		}
		fut := ctx.ExecuteActivityWithID(context.Background(), "agent-iteration", payload, iterID)
		var out interface{}
		if err := fut.Get(context.Background(), &out); err != nil {
			return nil, err
		}
		// Feed assistant output back as next turn input; simplest loop
		text, _ := out.(string)
		final = text
		current = text
	}
	return final, nil
}


