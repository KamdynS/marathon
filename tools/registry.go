package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Tool defines a callable utility for agents.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
	Schema() map[string]interface{}
}

// Registry manages a set of tools available to agents.
type Registry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	List() []string
	Execute(ctx context.Context, name string, input string) (string, error)
}

// DefaultRegistry is an in-memory implementation of Registry.
type DefaultRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry constructs an empty registry.
func NewRegistry() *DefaultRegistry {
	return &DefaultRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool by its Name().
func (r *DefaultRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Get retrieves a tool by name.
func (r *DefaultRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tool names.
func (r *DefaultRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}

// Execute runs a tool by name with the given input.
func (r *DefaultRegistry) Execute(ctx context.Context, name string, input string) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}
	start := time.Now()
	_ = start // placeholder for potential metrics later
	return t.Execute(ctx, input)
}
