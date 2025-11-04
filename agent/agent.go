package agent

import (
	"context"

	"github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/tools"
)

// Message represents a conversation message with role and content.
type Message struct {
	Role    string            `json:"role"`
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Agent defines the core interface for AI agents.
type Agent interface {
	Run(ctx context.Context, input Message) (Message, error)
	RunStream(ctx context.Context, input Message, output chan<- Message) error
}

// AgentConfig controls agent execution.
type AgentConfig struct {
	MaxIterations int
	SystemPrompt  string
	ModelOverride string
}

// Middleware allows hooks around key lifecycle events.
type Middleware interface {
	BeforeLLMCall(ctx context.Context, req *llm.ChatRequest) error
	AfterLLMResponse(ctx context.Context, resp *llm.Response) error
	BeforeToolExecute(ctx context.Context, toolName string, input string) error
	AfterToolExecute(ctx context.Context, toolName string, result string, execErr error) error
	AfterRun(ctx context.Context, final Message) error
}

// MemoryProcessor can transform/prune conversation history before sending to LLM.
type MemoryProcessor interface {
	Process(ctx context.Context, history []Message) []Message
}

// ConfigResolver can adjust configuration and tools at runtime based on input.
type ConfigResolver interface {
	Resolve(ctx context.Context, input Message, base AgentConfig) (AgentConfig, tools.Registry)
}
