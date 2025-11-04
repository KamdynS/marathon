package llm

import (
	"context"
	"time"
)

// Client is the provider-agnostic LLM interface used by Marathon agents.
type Client interface {
	Chat(ctx context.Context, req *ChatRequest) (*Response, error)
	Completion(ctx context.Context, prompt string) (*Response, error)
	Stream(ctx context.Context, req *ChatRequest, output chan<- *Response) error
	// ChatStream provides provider-neutral delta streaming. Implementations should prefer this over Stream.
	ChatStream(ctx context.Context, req *ChatRequest) (Stream, error)
	Model() string
}

// ClientOptions allows injecting observability hooks without core dependencies.
type ClientOptions struct {
	// Hooks can be nil; when set, providers should invoke callbacks around requests.
	Hooks interface{}
}

// Message represents a single role/content entry in a chat.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool defines a callable function made available to the model.
type Tool struct {
	Type     string       `json:"type"` // typically "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function signature exposed to the model.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolCall is a model-initiated function call.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// Usage contains token usage accounting when provided by the model.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ChatRequest is the normalized chat request sent to providers.
type ChatRequest struct {
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
	Model        string    `json:"model,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
}

// Response is the normalized provider response.
type Response struct {
	Content      string     `json:"content"`
	Provider     string     `json:"provider,omitempty"`
	Model        string     `json:"model,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Usage        *Usage     `json:"usage,omitempty"`
}

// RetryConfig controls retry behavior for network/provider errors.
type RetryConfig struct {
	MaxRetries    int           `json:"max_retries"`
	InitialDelay  time.Duration `json:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	BackoffFactor float64       `json:"backoff_factor"`
}

// DefaultRetryConfig returns sane defaults for provider retries.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}
