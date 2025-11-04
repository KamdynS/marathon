package llm

import (
	"context"
	"errors"
	"time"
)

// RoutePolicy decides which client/model to use for a given request.
type RoutePolicy interface {
	// Select returns the target client and optional model override.
	Select(req *ChatRequest) (Client, string, error)
}

// StaticPolicy routes by req.Model if present, otherwise uses Default.
type StaticPolicy struct {
	Default Client
	ByModel map[string]Client
}

// Select picks a client based on explicit model or defaults.
func (p StaticPolicy) Select(req *ChatRequest) (Client, string, error) {
	if req != nil && req.Model != "" {
		if c, ok := p.ByModel[req.Model]; ok && c != nil {
			return c, req.Model, nil
		}
		if p.Default != nil {
			return p.Default, req.Model, nil
		}
		return nil, "", errors.New("no default client configured")
	}
	if p.Default == nil {
		return nil, "", errors.New("no default client configured")
	}
	return p.Default, "", nil
}

// RouterClient implements Client and delegates via RoutePolicy.
type RouterClient struct {
	policy RoutePolicy
	cfg    RouterConfig
}

// NewRouterClient creates a router client with the given policy.
func NewRouterClient(policy RoutePolicy) *RouterClient {
	return &RouterClient{policy: policy}
}

// RouterConfig controls router behavior like timeouts and fallback.
type RouterConfig struct {
	// Timeout applies when the incoming context has no deadline.
	Timeout time.Duration
	// Fallback is used once when the primary selection errors.
	Fallback Client
}

// WithConfig sets optional router config.
func (r *RouterClient) WithConfig(cfg RouterConfig) *RouterClient {
	r.cfg = cfg
	return r
}

// Chat delegates to the selected client.
func (r *RouterClient) Chat(ctx context.Context, req *ChatRequest) (*Response, error) {
	c, modelOverride, err := r.policy.Select(req)
	if err != nil {
		return nil, err
	}
	if modelOverride != "" {
		// Shallow clone to avoid mutating caller's struct
		clone := *req
		clone.Model = modelOverride
		req = &clone
	}
	ctx = r.ensureTimeout(ctx)
	resp, err := c.Chat(ctx, req)
	if err != nil && r.cfg.Fallback != nil && c != r.cfg.Fallback {
		return r.cfg.Fallback.Chat(ctx, req)
	}
	return resp, err
}

// Completion delegates to default selection.
func (r *RouterClient) Completion(ctx context.Context, prompt string) (*Response, error) {
	c, _, err := r.policy.Select(&ChatRequest{})
	if err != nil {
		return nil, err
	}
	ctx = r.ensureTimeout(ctx)
	resp, err := c.Completion(ctx, prompt)
	if err != nil && r.cfg.Fallback != nil && c != r.cfg.Fallback {
		return r.cfg.Fallback.Completion(ctx, prompt)
	}
	return resp, err
}

// Stream delegates to the selected client and forwards stream.
func (r *RouterClient) Stream(ctx context.Context, req *ChatRequest, output chan<- *Response) error {
	c, modelOverride, err := r.policy.Select(req)
	if err != nil {
		return err
	}
	if modelOverride != "" {
		clone := *req
		clone.Model = modelOverride
		req = &clone
	}
	ctx = r.ensureTimeout(ctx)
	if err := c.Stream(ctx, req, output); err != nil && r.cfg.Fallback != nil && c != r.cfg.Fallback {
		return r.cfg.Fallback.Stream(ctx, req, output)
	} else {
		return err
	}
}

// ChatStream delegates to the selected client and returns a provider-neutral stream.
func (r *RouterClient) ChatStream(ctx context.Context, req *ChatRequest) (Stream, error) {
	c, modelOverride, err := r.policy.Select(req)
	if err != nil {
		return nil, err
	}
	if modelOverride != "" {
		clone := *req
		clone.Model = modelOverride
		req = &clone
	}
	ctx = r.ensureTimeout(ctx)
	s, err := c.ChatStream(ctx, req)
	if err != nil && r.cfg.Fallback != nil && c != r.cfg.Fallback {
		return r.cfg.Fallback.ChatStream(ctx, req)
	}
	return s, err
}

func (r *RouterClient) ensureTimeout(ctx context.Context) context.Context {
	if _, ok := ctx.Deadline(); ok {
		return ctx
	}
	if r.cfg.Timeout <= 0 {
		return ctx
	}
	c, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	// Best-effort: let callers close over ctx; we add a finalizer-like cancel when the parent is done
	go func() { <-c.Done(); cancel() }()
	return c
}

// Model returns an identifier for this client.
func (r *RouterClient) Model() string { return "router" }
