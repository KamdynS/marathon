package state

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryStore is an in-memory implementation of Store
type InMemoryStore struct {
	mu         sync.RWMutex
	workflows  map[string]*WorkflowState
	events     map[string][]*Event
	activities map[string]*ActivityState
}

// NewInMemoryStore creates a new in-memory state store
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		workflows:  make(map[string]*WorkflowState),
		events:     make(map[string][]*Event),
		activities: make(map[string]*ActivityState),
	}
}

// SaveWorkflowState implements Store
func (s *InMemoryStore) SaveWorkflowState(ctx context.Context, state *WorkflowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external mutations
	stateCopy := *state
	s.workflows[state.WorkflowID] = &stateCopy
	return nil
}

// GetWorkflowState implements Store
func (s *InMemoryStore) GetWorkflowState(ctx context.Context, workflowID string) (*WorkflowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.workflows[workflowID]
	if !exists {
		return nil, fmt.Errorf("workflow %s not found", workflowID)
	}

	// Return a copy to avoid external mutations
	stateCopy := *state
	return &stateCopy, nil
}

// AppendEvent implements Store
func (s *InMemoryStore) AppendEvent(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.events[event.WorkflowID]

	// Set sequence number
	event.SequenceNum = int64(len(events)) + 1

	// Create a copy to avoid external mutations
	eventCopy := *event
	s.events[event.WorkflowID] = append(events, &eventCopy)

	return nil
}

// GetEvents implements Store
func (s *InMemoryStore) GetEvents(ctx context.Context, workflowID string) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.events[workflowID]
	if !exists {
		return []*Event{}, nil
	}

	// Return copies to avoid external mutations
	result := make([]*Event, len(events))
	for i, event := range events {
		eventCopy := *event
		result[i] = &eventCopy
	}

	return result, nil
}

// GetEventsSince implements Store
func (s *InMemoryStore) GetEventsSince(ctx context.Context, workflowID string, since int64) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.events[workflowID]
	if !exists {
		return []*Event{}, nil
	}

	result := make([]*Event, 0)
	for _, event := range events {
		if event.SequenceNum > since {
			eventCopy := *event
			result = append(result, &eventCopy)
		}
	}

	return result, nil
}

// SaveActivityState implements Store
func (s *InMemoryStore) SaveActivityState(ctx context.Context, state *ActivityState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external mutations
	stateCopy := *state
	s.activities[state.ActivityID] = &stateCopy
	return nil
}

// GetActivityState implements Store
func (s *InMemoryStore) GetActivityState(ctx context.Context, activityID string) (*ActivityState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.activities[activityID]
	if !exists {
		return nil, fmt.Errorf("activity %s not found", activityID)
	}

	// Return a copy to avoid external mutations
	stateCopy := *state
	return &stateCopy, nil
}

// ListWorkflows implements Store
func (s *InMemoryStore) ListWorkflows(ctx context.Context, status WorkflowStatus) ([]*WorkflowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*WorkflowState, 0)
	for _, state := range s.workflows {
		if status == "" || state.Status == status {
			stateCopy := *state
			result = append(result, &stateCopy)
		}
	}

	return result, nil
}

// DeleteWorkflow implements Store
func (s *InMemoryStore) DeleteWorkflow(ctx context.Context, workflowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.workflows, workflowID)
	delete(s.events, workflowID)

	// Delete associated activities
	for activityID, activity := range s.activities {
		if activity.WorkflowID == workflowID {
			delete(s.activities, activityID)
		}
	}

	return nil
}
