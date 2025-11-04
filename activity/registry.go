package activity

import (
	"fmt"
	"sync"
	"time"
)

// Registration holds an activity and its metadata
type Registration struct {
	Activity Activity
	Info     Info
}

// Registry stores activity registrations
type Registry struct {
	mu         sync.RWMutex
	activities map[string]*Registration
}

// NewRegistry creates a new activity registry
func NewRegistry() *Registry {
	return &Registry{
		activities: make(map[string]*Registration),
	}
}

// Register adds an activity to the registry
func (r *Registry) Register(name string, activity Activity, info Info) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return fmt.Errorf("activity name cannot be empty")
	}

	if _, exists := r.activities[name]; exists {
		return fmt.Errorf("activity %s already registered", name)
	}

	info.Name = name
	if info.Timeout == 0 {
		info.Timeout = 30 * time.Second
	}
	if info.RetryPolicy == nil {
		info.RetryPolicy = DefaultRetryPolicy()
	}

	r.activities[name] = &Registration{
		Activity: activity,
		Info:     info,
	}

	return nil
}

// Get retrieves an activity by name
func (r *Registry) Get(name string) (*Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, exists := r.activities[name]
	if !exists {
		return nil, fmt.Errorf("activity %s not found", name)
	}

	return reg, nil
}

// List returns all registered activity names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.activities))
	for name := range r.activities {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is a global registry for activities
var DefaultRegistry = NewRegistry()

// Register registers an activity in the default registry
func Register(name string, activity Activity, info Info) error {
	return DefaultRegistry.Register(name, activity, info)
}

// Get retrieves an activity from the default registry
func Get(name string) (*Registration, error) {
	return DefaultRegistry.Get(name)
}

// List returns all activities in the default registry
func List() []string {
	return DefaultRegistry.List()
}
