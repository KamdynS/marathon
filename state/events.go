// Package state provides workflow state persistence and event sourcing capabilities.
package state

import (
	"encoding/json"
	"time"
)

// EventType represents the type of workflow event
type EventType string

const (
	EventWorkflowStarted   EventType = "workflow_started"
	EventWorkflowCompleted EventType = "workflow_completed"
	EventWorkflowFailed    EventType = "workflow_failed"
	EventWorkflowCanceled  EventType = "workflow_canceled"
	EventActivityScheduled EventType = "activity_scheduled"
	EventActivityStarted   EventType = "activity_started"
	EventActivityCompleted EventType = "activity_completed"
	EventActivityFailed    EventType = "activity_failed"
	EventActivityRetrying  EventType = "activity_retrying"
	EventTimerScheduled    EventType = "timer_scheduled"
	EventTimerFired        EventType = "timer_fired"
	EventSignalReceived    EventType = "signal_received"
)

// Event represents a workflow execution event
type Event struct {
	ID          string                 `json:"id"`
	WorkflowID  string                 `json:"workflow_id"`
	Type        EventType              `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	SequenceNum int64                  `json:"sequence_num"`
	Data        map[string]interface{} `json:"data"`
}

// WorkflowStartedData contains data for workflow started event
type WorkflowStartedData struct {
	WorkflowName string      `json:"workflow_name"`
	Input        interface{} `json:"input"`
	TaskQueue    string      `json:"task_queue"`
}

// WorkflowCompletedData contains data for workflow completed event
type WorkflowCompletedData struct {
	Output interface{} `json:"output"`
}

// WorkflowFailedData contains data for workflow failed event
type WorkflowFailedData struct {
	Error string `json:"error"`
}

// ActivityScheduledData contains data for activity scheduled event
type ActivityScheduledData struct {
	ActivityID   string      `json:"activity_id"`
	ActivityName string      `json:"activity_name"`
	Input        interface{} `json:"input"`
	Timeout      string      `json:"timeout"`
}

// ActivityCompletedData contains data for activity completed event
type ActivityCompletedData struct {
	ActivityID string      `json:"activity_id"`
	Output     interface{} `json:"output"`
}

// ActivityFailedData contains data for activity failed event
type ActivityFailedData struct {
	ActivityID string `json:"activity_id"`
	Error      string `json:"error"`
	Attempt    int    `json:"attempt"`
}

// NewEvent creates a new event with generated ID
func NewEvent(workflowID string, eventType EventType, data map[string]interface{}) *Event {
	return &Event{
		ID:         generateID(),
		WorkflowID: workflowID,
		Type:       eventType,
		Timestamp:  time.Now().UTC(),
		Data:       data,
	}
}

// ToJSON serializes the event to JSON
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON deserializes an event from JSON
func FromJSON(data []byte) (*Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// generateID generates a unique ID for events
func generateID() string {
	return time.Now().Format("20060102150405.000000")
}
