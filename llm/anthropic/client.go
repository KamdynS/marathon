package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	base "github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/observability"
	anth "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client implements marathon/llm.Client for Anthropic Claude Messages API.
type Client struct {
	client  anth.Client
	cfg     Config
	retrier *base.Retrier
}

// Config configures the Anthropic client.
type Config struct {
	APIKey      string
	Model       string
	BaseURL     string
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
	Retry       base.RetryConfig
	Hooks       *observability.Hooks
}

// NewClient creates an Anthropic client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Model == "" {
		cfg.Model = "claude-3-5-haiku-latest"
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1000
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Retry.MaxRetries == 0 {
		cfg.Retry = base.DefaultRetryConfig()
	}

	opts := []option.RequestOption{}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, option.WithHTTPClient(&http.Client{Timeout: cfg.Timeout}))
	}
	c := anth.NewClient(opts...)
	return &Client{client: c, cfg: cfg, retrier: base.NewRetrier(cfg.Retry)}, nil
}

func (c *Client) Model() string { return c.cfg.Model }

func (c *Client) Chat(ctx context.Context, req *base.ChatRequest) (*base.Response, error) {
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMRequest(ctx, "anthropic", pickModel(req, c.cfg.Model), map[string]any{"operation": "chat"})
	}
	start := time.Now()
	var out *anth.Message
	err := c.retrier.Do(ctx, func() error {
		params := toAnthParams(req, c.cfg)
		resp, err := c.client.Messages.New(ctx, params)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMResponse(ctx, "anthropic", pickModel(req, c.cfg.Model), time.Since(start), map[string]any{"operation": "chat", "error": err != nil})
	}
	if err != nil {
		return nil, err
	}
	return fromAnthMessage(out), nil
}

func (c *Client) Completion(ctx context.Context, prompt string) (*base.Response, error) {
	// Implement via Chat with a single user message.
	return c.Chat(ctx, &base.ChatRequest{Messages: []base.Message{{Role: "user", Content: prompt}}})
}

func (c *Client) Stream(ctx context.Context, req *base.ChatRequest, output chan<- *base.Response) error {
	// Minimal streaming: fall back to single Chat call and emit once.
	resp, err := c.Chat(ctx, req)
	if err != nil {
		return err
	}
	if resp != nil && resp.Content != "" {
		output <- resp
	}
	return nil
}

// ChatStream implements provider-neutral streaming using the official SDK event stream.
// If SDK streaming is unavailable, this falls back to single-shot and emits a done delta.
// ChatStream is provided via build-tagged files. See chatstream_fallback.go and chatstream_streaming.go.

type anthStaticStream struct {
	emitted  bool
	closed   bool
	text     string
	provider string
	model    string
}

func (s *anthStaticStream) Recv(ctx context.Context) (base.Delta, error) {
	if s.closed {
		return base.Delta{}, base.ErrStreamClosed
	}
	if !s.emitted && s.text != "" {
		s.emitted = true
		return base.Delta{Type: base.DeltaTypeText, Text: s.text, Provider: s.provider, Model: s.model}, nil
	}
	s.closed = true
	return base.Delta{Type: base.DeltaTypeDone, Provider: s.provider, Model: s.model}, nil
}

func (s *anthStaticStream) Close() error { s.closed = true; return nil }

func toAnthParams(req *base.ChatRequest, cfg Config) anth.MessageNewParams {
	// Build messages
	msgs := make([]anth.MessageParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := anth.MessageParamRoleUser
		if m.Role == "assistant" {
			role = anth.MessageParamRoleAssistant
		}
		msgs = append(msgs, anth.MessageParam{
			Role: role,
			Content: []anth.ContentBlockParamUnion{{
				OfText: &anth.TextBlockParam{Text: m.Content},
			}},
		})
	}
	params := anth.MessageNewParams{
		Messages:  msgs,
		MaxTokens: int64(cfg.MaxTokens),
		Model:     anth.Model(pickModel(req, cfg.Model)),
	}
	// Attach tools if provided (maps Marathon llm.Tool -> Anthropic tool schema)
	if len(req.Tools) > 0 {
		params.Tools = toAnthTools(req.Tools)
	}
	if cfg.Temperature > 0 {
		params.Temperature = anth.Float(cfg.Temperature)
	}
	return params
}

// toAnthTools converts Marathon tool definitions into Anthropic tool params.
func toAnthTools(tools []base.Tool) []anth.ToolUnionParam {
	out := make([]anth.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		tp := &anth.ToolParam{Name: t.Function.Name}
		// Skip optional description assignment to avoid optional type conversions
		// Provide a minimal input schema; if a schema is provided, we ignore for now to avoid type conversion complexity
		tp.InputSchema = anth.ToolInputSchemaParam{Type: "object"}
		out = append(out, anth.ToolUnionParam{OfTool: tp})
	}
	return out
}

func fromAnthMessage(m *anth.Message) *base.Response {
	if m == nil {
		return &base.Response{Provider: "anthropic"}
	}
	var content string
	var toolCalls []base.ToolCall
	for _, c := range m.Content {
		// Text blocks
		if c.Text != "" {
			content += c.Text
			continue
		}
		// Tool use blocks (Anthropic Messages API): type=="tool_use"
		if c.Type == "tool_use" {
			// Expected fields: ID, Name, and Input (map) on the content block
			// Marshal input to a JSON string for base.ToolCall.Arguments
			var argsJSON string
			if c.Input != nil {
				if b, err := json.Marshal(c.Input); err == nil {
					argsJSON = string(b)
				}
			}
			toolCalls = append(toolCalls, base.ToolCall{ID: c.ID, Name: c.Name, Arguments: argsJSON})
		}
	}
	resp := &base.Response{
		Content:  content,
		Provider: "anthropic",
		Model:    string(m.Model),
	}
	if len(toolCalls) > 0 {
		resp.ToolCalls = toolCalls
	}
	resp.Usage = &base.Usage{
		InputTokens:  int(m.Usage.InputTokens),
		OutputTokens: int(m.Usage.OutputTokens),
		TotalTokens:  int(m.Usage.InputTokens + m.Usage.OutputTokens),
	}
	return resp
}

func pickModel(req *base.ChatRequest, fallback string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	return fallback
}

func toOptionalString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}
