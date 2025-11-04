package observability

import (
	"context"
	"time"
)

// Hooks provides optional callbacks for logging, metrics, and tracing without
// introducing dependencies in the core library. All functions are optional.
type Hooks struct {
	// Logf logs a structured message with a severity level and key-value fields.
	Logf func(ctx context.Context, level string, msg string, fields map[string]any)

	// OnLLMRequest is called before a provider request is sent.
	OnLLMRequest func(ctx context.Context, provider string, model string, meta map[string]any)
	// OnLLMResponse is called after a provider response is received.
	OnLLMResponse func(ctx context.Context, provider string, model string, latency time.Duration, meta map[string]any)
	// OnLLMRetry is called when a retry is attempted.
	OnLLMRetry func(ctx context.Context, provider string, model string, attempt int, err error)
}

// SafeLog logs if Logf is configured.
func (h *Hooks) SafeLog(ctx context.Context, level string, msg string, fields map[string]any) {
	if h != nil && h.Logf != nil {
		h.Logf(ctx, level, msg, fields)
	}
}

// SafeLLMRequest invokes OnLLMRequest if configured.
func (h *Hooks) SafeLLMRequest(ctx context.Context, provider string, model string, meta map[string]any) {
	if h != nil && h.OnLLMRequest != nil {
		h.OnLLMRequest(ctx, provider, model, meta)
	}
}

// SafeLLMResponse invokes OnLLMResponse if configured.
func (h *Hooks) SafeLLMResponse(ctx context.Context, provider string, model string, latency time.Duration, meta map[string]any) {
	if h != nil && h.OnLLMResponse != nil {
		h.OnLLMResponse(ctx, provider, model, latency, meta)
	}
}

// SafeLLMRetry invokes OnLLMRetry if configured.
func (h *Hooks) SafeLLMRetry(ctx context.Context, provider string, model string, attempt int, err error) {
	if h != nil && h.OnLLMRetry != nil {
		h.OnLLMRetry(ctx, provider, model, attempt, err)
	}
}
