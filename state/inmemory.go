package state

import (
    "context"
    "fmt"
    "sync"
    "time"
)

// InMemoryStore is an in-memory implementation of Store
type InMemoryStore struct {
	mu         sync.RWMutex
	workflows  map[string]*WorkflowState
	events     map[string][]*Event
	activities map[string]*ActivityState
    idemKeys   map[string]string                    // idempotency key -> workflowID
    timers     map[string]map[string]*TimerRecord   // workflowID -> timerID -> record
}

// NewInMemoryStore creates a new in-memory state store
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		workflows:  make(map[string]*WorkflowState),
		events:     make(map[string][]*Event),
        activities: make(map[string]*ActivityState),
        idemKeys:   make(map[string]string),
        timers:     make(map[string]map[string]*TimerRecord),
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

    // Delete associated timers
    delete(s.timers, workflowID)

	return nil
}

// MapIdempotencyKeyToWorkflow implements Store
func (s *InMemoryStore) MapIdempotencyKeyToWorkflow(ctx context.Context, key string, workflowID string) (bool, string, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if existing, ok := s.idemKeys[key]; ok {
        return false, existing, nil
    }
    s.idemKeys[key] = workflowID
    return true, "", nil
}

// GetWorkflowIDByIdempotencyKey implements Store
func (s *InMemoryStore) GetWorkflowIDByIdempotencyKey(ctx context.Context, key string) (string, bool, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    id, ok := s.idemKeys[key]
    return id, ok, nil
}

// ScheduleTimer implements Store
func (s *InMemoryStore) ScheduleTimer(ctx context.Context, workflowID string, timerID string, fireAt time.Time) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if _, ok := s.timers[workflowID]; !ok {
        s.timers[workflowID] = make(map[string]*TimerRecord)
    }
    if _, exists := s.timers[workflowID][timerID]; exists {
        return nil
    }
    rec := &TimerRecord{WorkflowID: workflowID, TimerID: timerID, FireAt: fireAt, Fired: false}
    s.timers[workflowID][timerID] = rec
    return nil
}

// ListDueTimers implements Store
func (s *InMemoryStore) ListDueTimers(ctx context.Context, now time.Time) ([]TimerRecord, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    due := make([]TimerRecord, 0)
    for _, wfTimers := range s.timers {
        for _, rec := range wfTimers {
            if !rec.Fired && !rec.FireAt.After(now) {
                // append a copy
                due = append(due, *rec)
            }
        }
    }
    return due, nil
}

// MarkTimerFired implements Store
func (s *InMemoryStore) MarkTimerFired(ctx context.Context, workflowID string, timerID string) (bool, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    wfTimers, ok := s.timers[workflowID]
    if !ok {
        return false, fmt.Errorf("no timers for workflow %s", workflowID)
    }
    rec, ok := wfTimers[timerID]
    if !ok {
        return false, fmt.Errorf("timer %s not found", timerID)
    }
    if rec.Fired {
        return false, nil
    }
    rec.Fired = true
    return true, nil
}
