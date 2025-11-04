package activity

import (
	"context"
	"testing"
	"time"
)

func TestActivityFunc(t *testing.T) {
	activity := ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result", nil
	})

	result, err := activity.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result != "result" {
		t.Errorf("expected result, got %v", result)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	activity := ActivityFunc(func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result", nil
	})

	info := Info{
		Description: "test activity",
		Timeout:     5 * time.Second,
	}

	if err := registry.Register("test", activity, info); err != nil {
		t.Fatalf("failed to register activity: %v", err)
	}

	reg, err := registry.Get("test")
	if err != nil {
		t.Fatalf("failed to get activity: %v", err)
	}

	if reg.Info.Name != "test" {
		t.Errorf("expected name test, got %s", reg.Info.Name)
	}

	if reg.Info.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", reg.Info.Timeout)
	}

	// Test duplicate registration
	if err := registry.Register("test", activity, info); err == nil {
		t.Error("expected error when registering duplicate activity")
	}
}

func TestRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	// Test backoff calculation
	duration1 := policy.GetBackoffDuration(0)
	if duration1 != time.Second {
		t.Errorf("expected 1s for attempt 0, got %v", duration1)
	}

	duration2 := policy.GetBackoffDuration(1)
	if duration2 != 2*time.Second {
		t.Errorf("expected 2s for attempt 1, got %v", duration2)
	}

	duration3 := policy.GetBackoffDuration(2)
	if duration3 != 4*time.Second {
		t.Errorf("expected 4s for attempt 2, got %v", duration3)
	}
}

// LLMActivity removed — dropping tests that referenced old types

type mockTool struct{}

func (m *mockTool) Name() string {
	return "calculator"
}

func (m *mockTool) Description() string {
	return "performs calculations"
}

func (m *mockTool) Execute(ctx context.Context, input string) (string, error) {
	return "42", nil
}

// ToolActivity removed — dropping tests that referenced old type
