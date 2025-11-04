package workflow

import (
	"fmt"
	"sync"
)

// Registry stores workflow definitions
type Registry struct {
	mu        sync.RWMutex
	workflows map[string]*Definition
}

// NewRegistry creates a new workflow registry
func NewRegistry() *Registry {
	return &Registry{
		workflows: make(map[string]*Definition),
	}
}

// Register adds a workflow definition to the registry
func (r *Registry) Register(def *Definition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if def.Name == "" {
		return fmt.Errorf("workflow name cannot be empty")
	}

	if _, exists := r.workflows[def.Name]; exists {
		return fmt.Errorf("workflow %s already registered", def.Name)
	}

	r.workflows[def.Name] = def
	return nil
}

// Get retrieves a workflow definition by name
func (r *Registry) Get(name string) (*Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, exists := r.workflows[name]
	if !exists {
		return nil, fmt.Errorf("workflow %s not found", name)
	}

	return def, nil
}

// List returns all registered workflow names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.workflows))
	for name := range r.workflows {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is a global registry for workflows
var DefaultRegistry = NewRegistry()

// Register registers a workflow in the default registry
func Register(def *Definition) error {
	return DefaultRegistry.Register(def)
}

// Get retrieves a workflow from the default registry
func Get(name string) (*Definition, error) {
	return DefaultRegistry.Get(name)
}

// List returns all workflows in the default registry
func List() []string {
	return DefaultRegistry.List()
}
