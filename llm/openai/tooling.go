package openai

import (
	base "github.com/KamdynS/marathon/llm"
	oa "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// toOATools converts Marathon tool definitions to OpenAI function tools.
func toOATools(tools []base.Tool) []oa.ChatCompletionToolUnionParam {
	out := make([]oa.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		fn := shared.FunctionDefinitionParam{Name: t.Function.Name}
		if t.Function.Description != "" {
			fn.Description = oa.String(t.Function.Description)
		}
		if t.Function.Parameters != nil {
			fn.Parameters = t.Function.Parameters
		}
		out = append(out, oa.ChatCompletionFunctionTool(fn))
	}
	return out
}
