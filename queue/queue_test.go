package queue

import (
	"context"
	"testing"
	"time"
)

func TestNewTask(t *testing.T) {
	task := NewTask(TaskTypeActivity, "wf-123", "input-data")

	if task.Type != TaskTypeActivity {
		t.Errorf("expected type activity, got %s", task.Type)
	}

	if task.WorkflowID != "wf-123" {
		t.Errorf("expected workflow ID wf-123, got %s", task.WorkflowID)
	}

	if task.ID == "" {
		t.Error("expected task ID to be generated")
	}

	if task.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", task.Attempts)
	}
}

func TestInMemoryQueue_EnqueueDequeue(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()
	task := NewTask(TaskTypeActivity, "wf-123", "test-input")

	// Test Enqueue
	if err := queue.Enqueue(ctx, "default", task); err != nil {
		t.Fatalf("failed to enqueue task: %v", err)
	}

	// Test Len
	length, err := queue.Len(ctx, "default")
	if err != nil {
		t.Fatalf("failed to get queue length: %v", err)
	}
	if length != 1 {
		t.Errorf("expected length 1, got %d", length)
	}

	// Test Dequeue
	dequeued, err := queue.Dequeue(ctx, "default")
	if err != nil {
		t.Fatalf("failed to dequeue task: %v", err)
	}

	if dequeued.ID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, dequeued.ID)
	}

	if dequeued.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", dequeued.Attempts)
	}

	// Queue should be empty now
	length, _ = queue.Len(ctx, "default")
	if length != 0 {
		t.Errorf("expected length 0, got %d", length)
	}
}

func TestInMemoryQueue_DequeueTimeout(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()

	// Test timeout when queue is empty
	_, err := queue.DequeueWithTimeout(ctx, "default", 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestInMemoryQueue_AckNack(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()
	task := NewTask(TaskTypeActivity, "wf-123", "test-input")

	// Enqueue and dequeue
	queue.Enqueue(ctx, "default", task)
	dequeued, _ := queue.Dequeue(ctx, "default")

	// Test Ack
	if err := queue.Ack(ctx, "default", dequeued.ID); err != nil {
		t.Fatalf("failed to ack task: %v", err)
	}

	// Acking again should fail
	if err := queue.Ack(ctx, "default", dequeued.ID); err == nil {
		t.Error("expected error when acking non-pending task")
	}
}

func TestInMemoryQueue_Nack_Requeue(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()
	task := NewTask(TaskTypeActivity, "wf-123", "test-input")

	// Enqueue and dequeue
	queue.Enqueue(ctx, "default", task)
	dequeued, _ := queue.Dequeue(ctx, "default")

	// Queue should be empty
	length, _ := queue.Len(ctx, "default")
	if length != 0 {
		t.Errorf("expected length 0, got %d", length)
	}

	// Nack with requeue
	if err := queue.Nack(ctx, "default", dequeued.ID, true); err != nil {
		t.Fatalf("failed to nack task: %v", err)
	}

	// Queue should have the task again
	length, _ = queue.Len(ctx, "default")
	if length != 1 {
		t.Errorf("expected length 1, got %d", length)
	}

	// Dequeue again and verify attempts incremented
	requeued, _ := queue.Dequeue(ctx, "default")
	if requeued.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", requeued.Attempts)
	}
}

func TestInMemoryQueue_Nack_NoRequeue(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()
	task := NewTask(TaskTypeActivity, "wf-123", "test-input")

	// Enqueue and dequeue
	queue.Enqueue(ctx, "default", task)
	dequeued, _ := queue.Dequeue(ctx, "default")

	// Nack without requeue
	if err := queue.Nack(ctx, "default", dequeued.ID, false); err != nil {
		t.Fatalf("failed to nack task: %v", err)
	}

	// Queue should still be empty
	length, _ := queue.Len(ctx, "default")
	if length != 0 {
		t.Errorf("expected length 0, got %d", length)
	}
}

func TestInMemoryQueue_MultipleQueues(t *testing.T) {
	queue := NewInMemoryQueue()
	defer queue.Close()

	ctx := context.Background()
	task1 := NewTask(TaskTypeActivity, "wf-1", "input-1")
	task2 := NewTask(TaskTypeActivity, "wf-2", "input-2")

	// Enqueue to different queues
	queue.Enqueue(ctx, "queue1", task1)
	queue.Enqueue(ctx, "queue2", task2)

	// Verify lengths
	len1, _ := queue.Len(ctx, "queue1")
	len2, _ := queue.Len(ctx, "queue2")

	if len1 != 1 || len2 != 1 {
		t.Errorf("expected both queues to have length 1, got %d and %d", len1, len2)
	}

	// Dequeue from queue1
	dequeued1, _ := queue.Dequeue(ctx, "queue1")
	if dequeued1.WorkflowID != "wf-1" {
		t.Errorf("expected workflow wf-1, got %s", dequeued1.WorkflowID)
	}

	// Dequeue from queue2
	dequeued2, _ := queue.Dequeue(ctx, "queue2")
	if dequeued2.WorkflowID != "wf-2" {
		t.Errorf("expected workflow wf-2, got %s", dequeued2.WorkflowID)
	}
}

func TestInMemoryQueue_Close(t *testing.T) {
	queue := NewInMemoryQueue()
	ctx := context.Background()

	task := NewTask(TaskTypeActivity, "wf-123", "test")
	queue.Enqueue(ctx, "default", task)

	// Close the queue
	if err := queue.Close(); err != nil {
		t.Fatalf("failed to close queue: %v", err)
	}

	// Operations after close should fail
	if err := queue.Enqueue(ctx, "default", task); err == nil {
		t.Error("expected error when enqueueing to closed queue")
	}
}
