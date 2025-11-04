package anthropic

import (
	"context"
	"time"

	base "github.com/KamdynS/marathon/llm"
)

// ChatStream provides a fallback implementation using a single Chat call
// and then emitting one text delta followed by done. This is used when
// true SDK streaming is not available in this build.
func (c *Client) ChatStream(ctx context.Context, req *base.ChatRequest) (base.Stream, error) {
	// Log start even for fallback to maintain parity with OpenAI
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMRequest(ctx, "anthropic", pickModel(req, c.cfg.Model), map[string]any{"operation": "chat_stream", "fallback": true})
	}
	start := time.Now()
	resp, err := c.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.cfg.Hooks != nil {
		c.cfg.Hooks.SafeLLMResponse(ctx, "anthropic", pickModel(req, c.cfg.Model), time.Since(start), map[string]any{"operation": "chat_stream", "fallback": true, "error": false})
	}
	text := ""
	if resp != nil {
		text = resp.Content
	}
	return &anthStaticStream{emitted: false, closed: false, text: text, provider: "anthropic", model: c.Model()}, nil
}
