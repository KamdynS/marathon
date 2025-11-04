package openai

import (
	"context"
	"net/http"
	"time"

	base "github.com/KamdynS/marathon/llm"
	"github.com/KamdynS/marathon/observability"
	oa "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// Client implements marathon/llm.Client for OpenAI official SDK.
type Client struct {
	client  oa.Client
	cfg     Config
	retrier *base.Retrier
}

// Config configures the OpenAI client.
type Config struct {
	APIKey       string
	Model        string
	BaseURL      string
	Temperature  float64
	MaxTokens    int
	Timeout      time.Duration
	Retry        base.RetryConfig
	Organization string
	Hooks        *observability.Hooks
}

// NewClient creates an OpenAI client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Retry.MaxRetries == 0 {
		cfg.Retry = base.DefaultRetryConfig()
	}

	httpClient := &http.Client{Timeout: cfg.Timeout}
	opts := []option.RequestOption{option.WithHTTPClient(httpClient)}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.Organization != "" {
		opts = append(opts, option.WithOrganization(cfg.Organization))
	}
	c := oa.NewClient(opts...)
	return &Client{client: c, cfg: cfg, retrier: base.NewRetrier(cfg.Retry)}, nil
}

func (c *Client) Model() string { return c.cfg.Model }

func (c *Client) Chat(ctx context.Context, req *base.ChatRequest) (*base.Response, error) {
	start := time.Now()
	model := pickModel(req, c.cfg.Model)
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMRequest(ctx, "openai", model, map[string]any{"operation": "chat"})
	}
	var resp *oa.ChatCompletion
	err := c.retrier.Do(ctx, func() error {
		messages := toOAMessages(req)
		params := oa.ChatCompletionNewParams{Messages: messages}
		if m := model; m != "" {
			params.Model = shared.ChatModel(m)
		}
		if c.cfg.MaxTokens > 0 {
			params.MaxTokens = oa.Int(int64(c.cfg.MaxTokens))
		}
		if c.cfg.Temperature > 0 {
			params.Temperature = oa.Float(float64(c.cfg.Temperature))
		}
		// Attach tools if provided
		if len(req.Tools) > 0 {
			params.Tools = toOATools(req.Tools)
		}
		r, err := c.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return err
		}
		resp = r
		return nil
	})
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMResponse(ctx, "openai", model, time.Since(start), map[string]any{"operation": "chat", "error": err != nil})
	}
	if err != nil {
		return nil, err
	}
	return fromOAResponse(resp), nil
}

func (c *Client) Completion(ctx context.Context, prompt string) (*base.Response, error) {
	return c.Chat(ctx, &base.ChatRequest{Messages: []base.Message{{Role: "user", Content: prompt}}})
}

func (c *Client) Stream(ctx context.Context, req *base.ChatRequest, output chan<- *base.Response) error {
	start := time.Now()
	model := pickModel(req, c.cfg.Model)
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMRequest(ctx, "openai", model, map[string]any{"operation": "stream"})
	}
	messages := toOAMessages(req)
	params := oa.ChatCompletionNewParams{Messages: messages}
	if m := model; m != "" {
		params.Model = shared.ChatModel(m)
	}
	if c.cfg.MaxTokens > 0 {
		params.MaxTokens = oa.Int(int64(c.cfg.MaxTokens))
	}
	if c.cfg.Temperature > 0 {
		params.Temperature = oa.Float(float64(c.cfg.Temperature))
	}
	// Attach tools if provided
	if len(req.Tools) > 0 {
		params.Tools = toOATools(req.Tools)
	}
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	for stream.Next() {
		ev := stream.Current()
		if len(ev.Choices) == 0 {
			continue
		}
		for _, ch := range ev.Choices {
			if ch.Delta.Content != "" {
				output <- &base.Response{Content: ch.Delta.Content, Provider: "openai", Model: string(params.Model)}
			}
		}
	}
	err := stream.Err()
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMResponse(ctx, "openai", model, time.Since(start), map[string]any{"operation": "stream", "error": err != nil})
	}
	return err
}

// ChatStream implements provider-neutral delta streaming for OpenAI.
func (c *Client) ChatStream(ctx context.Context, req *base.ChatRequest) (base.Stream, error) {
	start := time.Now()
	messages := toOAMessages(req)
	params := oa.ChatCompletionNewParams{Messages: messages}
	if m := pickModel(req, c.cfg.Model); m != "" {
		params.Model = shared.ChatModel(m)
	}
	if c.cfg.MaxTokens > 0 {
		params.MaxTokens = oa.Int(int64(c.cfg.MaxTokens))
	}
	if c.cfg.Temperature > 0 {
		params.Temperature = oa.Float(float64(c.cfg.Temperature))
	}
	// Attach tools if provided for streaming as well
	if len(req.Tools) > 0 {
		params.Tools = toOATools(req.Tools)
	}
	s := c.client.Chat.Completions.NewStreaming(ctx, params)
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMRequest(ctx, "openai", string(params.Model), map[string]any{"operation": "chat_stream"})
		// We cannot know when the caller will finish; best-effort log now with zero duration and rely on caller to log completion if desired.
		c.cfg.Hooks.SafeLLMResponse(ctx, "openai", string(params.Model), time.Since(start), map[string]any{"operation": "chat_stream", "started": true})
	}
	// Wrap into base.Stream implementation
	return &oaStreamWrapper{inner: s, provider: "openai", model: string(params.Model)}, nil
}

type oaStreamWrapper struct {
	inner    oaStreamCore
	provider string
	model    string
	closed   bool
}

// oaStreamCore matches the subset of the OpenAI stream API we use.
type oaStreamCore interface {
	Next() bool
	Current() oa.ChatCompletionChunk
	Err() error
	Close() error
}

func (w *oaStreamWrapper) Recv(ctx context.Context) (base.Delta, error) {
	if w.closed {
		return base.Delta{}, base.ErrStreamClosed
	}
	if !w.inner.Next() {
		if err := w.inner.Err(); err != nil {
			return base.Delta{}, err
		}
		w.closed = true
		return base.Delta{Type: base.DeltaTypeDone, Provider: w.provider, Model: w.model}, nil
	}
	ev := w.inner.Current()
	// Map each choice delta to a text delta. Tool calling mapping will be added when available from SDK.
	for _, ch := range ev.Choices {
		if ch.Delta.Content != "" {
			return base.Delta{Type: base.DeltaTypeText, Text: ch.Delta.Content, Provider: w.provider, Model: w.model}, nil
		}
		// If tool call deltas exist in future SDK updates, map accordingly here.
	}
	// If no content in this event, emit empty and continue on next Recv.
	return base.Delta{}, nil
}

func (w *oaStreamWrapper) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.inner.Close()
}

func toOAMessages(req *base.ChatRequest) []oa.ChatCompletionMessageParamUnion {
	msgs := make([]oa.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, oa.ChatCompletionMessageParamUnion{OfSystem: &oa.ChatCompletionSystemMessageParam{Content: oa.ChatCompletionSystemMessageParamContentUnion{OfString: oa.String(req.SystemPrompt)}}})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			msgs = append(msgs, oa.ChatCompletionMessageParamUnion{OfAssistant: &oa.ChatCompletionAssistantMessageParam{Content: oa.ChatCompletionAssistantMessageParamContentUnion{OfString: oa.String(m.Content)}}})
		default:
			msgs = append(msgs, oa.ChatCompletionMessageParamUnion{OfUser: &oa.ChatCompletionUserMessageParam{Content: oa.ChatCompletionUserMessageParamContentUnion{OfString: oa.String(m.Content)}}})
		}
	}
	return msgs
}

func fromOAResponse(r *oa.ChatCompletion) *base.Response {
	if r == nil || len(r.Choices) == 0 {
		return &base.Response{Provider: "openai", Model: string(r.Model)}
	}
	choice := r.Choices[0]
	content := choice.Message.Content
	resp := &base.Response{
		Content:      content,
		Provider:     "openai",
		Model:        string(r.Model),
		FinishReason: string(choice.FinishReason),
	}
	// Try to map tool calls when present in response
	if len(choice.Message.ToolCalls) > 0 {
		calls := make([]base.ToolCall, 0, len(choice.Message.ToolCalls))
		for _, tc := range choice.Message.ToolCalls {
			calls = append(calls, base.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
		}
		resp.ToolCalls = calls
	}
	resp.Usage = &base.Usage{
		InputTokens:  int(r.Usage.PromptTokens),
		OutputTokens: int(r.Usage.CompletionTokens),
		TotalTokens:  int(r.Usage.TotalTokens),
	}
	return resp
}

// TODO: Map tool calls when SDK types verified. Currently not populating ToolCalls.

func pickModel(req *base.ChatRequest, fallback string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	return fallback
}
