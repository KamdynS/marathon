package tools

import (
	"github.com/KamdynS/marathon/llm"
)

// ToolDefinition is a simple schema for defining tools exposed to LLMs.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToLLMTool converts a ToolDefinition into an llm.Tool.
func (d ToolDefinition) ToLLMTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: d.Name, Description: d.Description, Parameters: d.Parameters}}
}

// FromRegistry builds llm.Tool definitions from a Registry.
func FromRegistry(reg Registry) []llm.Tool {
	if reg == nil {
		return nil
	}
	names := reg.List()
	out := make([]llm.Tool, 0, len(names))
	for _, n := range names {
		if t, ok := reg.Get(n); ok {
			out = append(out, llm.Tool{Type: "function", Function: llm.ToolFunction{Name: t.Name(), Description: t.Description(), Parameters: t.Schema()}})
		}
	}
	return out
}
