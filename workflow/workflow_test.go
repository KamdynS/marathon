package workflow

import (
	"testing"
	"time"
)

func TestWorkflowBuilder(t *testing.T) {
	def := New("test-workflow").
		Description("A test workflow").
		Version("1.0").
		TaskQueue("test-queue").
		Timeout(5*time.Minute).
		Activity("llm-call", map[string]string{"prompt": "hello"}).
		Build()

	if def.Name != "test-workflow" {
		t.Errorf("expected name test-workflow, got %s", def.Name)
	}

	if def.Description != "A test workflow" {
		t.Errorf("expected description, got %s", def.Description)
	}

	if def.Options.TaskQueue != "test-queue" {
		t.Errorf("expected task queue test-queue, got %s", def.Options.TaskQueue)
	}

	if def.Options.ExecutionTimeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", def.Options.ExecutionTimeout)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	def := New("test-workflow").Build()

	if err := registry.Register(def); err != nil {
		t.Fatalf("failed to register workflow: %v", err)
	}

	retrieved, err := registry.Get("test-workflow")
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}

	if retrieved.Name != "test-workflow" {
		t.Errorf("expected name test-workflow, got %s", retrieved.Name)
	}

	// Test duplicate registration
	if err := registry.Register(def); err == nil {
		t.Error("expected error when registering duplicate workflow")
	}

	// Test list
	names := registry.List()
	if len(names) != 1 || names[0] != "test-workflow" {
		t.Errorf("expected [test-workflow], got %v", names)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxAttempts != 3 {
		t.Errorf("expected max attempts 3, got %d", policy.MaxAttempts)
	}

	if policy.InitialInterval != time.Second {
		t.Errorf("expected initial interval 1s, got %v", policy.InitialInterval)
	}

	if policy.BackoffCoefficient != 2.0 {
		t.Errorf("expected backoff coefficient 2.0, got %f", policy.BackoffCoefficient)
	}
}
