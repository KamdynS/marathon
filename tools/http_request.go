package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPRequestTool performs simple HTTP requests.
type HTTPRequestTool struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPRequestTool creates a new HTTP tool with an optional timeout.
func NewHTTPRequestTool(timeout time.Duration) *HTTPRequestTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &HTTPRequestTool{client: &http.Client{Timeout: timeout}, timeout: timeout}
}

func (t *HTTPRequestTool) Name() string { return "http_request" }
func (t *HTTPRequestTool) Description() string {
	return "Perform HTTP requests: METHOD|URL|BODY (BODY optional)"
}
func (t *HTTPRequestTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": "METHOD|URL|BODY (optional)",
	}
}

func (t *HTTPRequestTool) Execute(ctx context.Context, input string) (string, error) {
	parts := strings.SplitN(input, "|", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("expected 'METHOD|URL|BODY?' got %q", input)
	}
	method := strings.TrimSpace(parts[0])
	url := strings.TrimSpace(parts[1])
	var body io.Reader
	if len(parts) == 3 {
		body = strings.NewReader(parts[2])
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
