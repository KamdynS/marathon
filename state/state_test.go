package state

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStore_WorkflowState(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	state := &WorkflowState{
		WorkflowID:   "wf-123",
		WorkflowName: "test-workflow",
		Status:       StatusRunning,
		Input:        map[string]string{"key": "value"},
		StartTime:    time.Now(),
		TaskQueue:    "default",
	}

	// Test Save
	if err := store.SaveWorkflowState(ctx, state); err != nil {
		t.Fatalf("failed to save workflow state: %v", err)
	}

	// Test Get
	retrieved, err := store.GetWorkflowState(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to get workflow state: %v", err)
	}

	if retrieved.WorkflowID != "wf-123" {
		t.Errorf("expected workflow ID wf-123, got %s", retrieved.WorkflowID)
	}

	if retrieved.Status != StatusRunning {
		t.Errorf("expected status running, got %s", retrieved.Status)
	}

	// Test Get non-existent
	_, err = store.GetWorkflowState(ctx, "non-existent")
	if err == nil {
		t.Error("expected error when getting non-existent workflow")
	}
}

func TestInMemoryStore_Events(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	event1 := NewEvent("wf-123", EventWorkflowStarted, map[string]interface{}{
		"workflow_name": "test",
	})

	event2 := NewEvent("wf-123", EventActivityScheduled, map[string]interface{}{
		"activity_name": "llm-call",
	})

	// Test AppendEvent
	if err := store.AppendEvent(ctx, event1); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	if err := store.AppendEvent(ctx, event2); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	// Test GetEvents
	events, err := store.GetEvents(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	if events[0].SequenceNum != 1 {
		t.Errorf("expected sequence 1, got %d", events[0].SequenceNum)
	}

	if events[1].SequenceNum != 2 {
		t.Errorf("expected sequence 2, got %d", events[1].SequenceNum)
	}

	// Test GetEventsSince
	recent, err := store.GetEventsSince(ctx, "wf-123", 1)
	if err != nil {
		t.Fatalf("failed to get events since: %v", err)
	}

	if len(recent) != 1 {
		t.Errorf("expected 1 event, got %d", len(recent))
	}

	if recent[0].Type != EventActivityScheduled {
		t.Errorf("expected activity scheduled, got %s", recent[0].Type)
	}
}

func TestInMemoryStore_ActivityState(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	state := &ActivityState{
		ActivityID:   "act-123",
		ActivityName: "llm-call",
		WorkflowID:   "wf-123",
		Status:       StatusRunning,
		Input:        "test input",
		StartTime:    time.Now(),
		Attempt:      1,
	}

	// Test Save
	if err := store.SaveActivityState(ctx, state); err != nil {
		t.Fatalf("failed to save activity state: %v", err)
	}

	// Test Get
	retrieved, err := store.GetActivityState(ctx, "act-123")
	if err != nil {
		t.Fatalf("failed to get activity state: %v", err)
	}

	if retrieved.ActivityID != "act-123" {
		t.Errorf("expected activity ID act-123, got %s", retrieved.ActivityID)
	}

	if retrieved.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", retrieved.Attempt)
	}
}

func TestInMemoryStore_ListWorkflows(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	state1 := &WorkflowState{
		WorkflowID:   "wf-1",
		WorkflowName: "test-1",
		Status:       StatusRunning,
		StartTime:    time.Now(),
	}

	state2 := &WorkflowState{
		WorkflowID:   "wf-2",
		WorkflowName: "test-2",
		Status:       StatusCompleted,
		StartTime:    time.Now(),
	}

	store.SaveWorkflowState(ctx, state1)
	store.SaveWorkflowState(ctx, state2)

	// List all
	all, err := store.ListWorkflows(ctx, "")
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}

	if len(all) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(all))
	}

	// List by status
	running, err := store.ListWorkflows(ctx, StatusRunning)
	if err != nil {
		t.Fatalf("failed to list running workflows: %v", err)
	}

	if len(running) != 1 {
		t.Errorf("expected 1 running workflow, got %d", len(running))
	}

	if running[0].WorkflowID != "wf-1" {
		t.Errorf("expected workflow wf-1, got %s", running[0].WorkflowID)
	}
}

func TestInMemoryStore_DeleteWorkflow(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	state := &WorkflowState{
		WorkflowID:   "wf-123",
		WorkflowName: "test",
		Status:       StatusCompleted,
		StartTime:    time.Now(),
	}

	event := NewEvent("wf-123", EventWorkflowStarted, nil)

	activity := &ActivityState{
		ActivityID: "act-123",
		WorkflowID: "wf-123",
		StartTime:  time.Now(),
	}

	store.SaveWorkflowState(ctx, state)
	store.AppendEvent(ctx, event)
	store.SaveActivityState(ctx, activity)

	// Delete workflow
	if err := store.DeleteWorkflow(ctx, "wf-123"); err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}

	// Verify deletion
	_, err := store.GetWorkflowState(ctx, "wf-123")
	if err == nil {
		t.Error("expected error when getting deleted workflow")
	}

	events, _ := store.GetEvents(ctx, "wf-123")
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}

	_, err = store.GetActivityState(ctx, "act-123")
	if err == nil {
		t.Error("expected error when getting deleted activity")
	}
}

func TestWorkflowStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   WorkflowStatus
		terminal bool
	}{
		{StatusPending, false},
		{StatusRunning, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusCanceled, true},
	}

	for _, tt := range tests {
		if tt.status.IsTerminal() != tt.terminal {
			t.Errorf("status %s: expected IsTerminal=%v, got %v",
				tt.status, tt.terminal, tt.status.IsTerminal())
		}
	}
}
