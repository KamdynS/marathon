package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestInMemoryQueue_Visibility_Redelivery(t *testing.T) {
	q := NewInMemoryQueueWithOptions(Options{
		VisibilityTimeout: 100 * time.Millisecond,
	})
	defer q.Close()

	ctx := context.Background()
	task := NewTask(TaskTypeActivity, "wf-vis", "x")

	if err := q.Enqueue(ctx, "default", task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	got, err := q.DequeueWithTimeout(ctx, "default", time.Second)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("expected same task id")
	}
	if got.Attempts != 1 {
		t.Fatalf("expected attempts=1 got %d", got.Attempts)
	}

	// Do not Ack; wait for visibility to expire and task to be redelivered
	time.Sleep(150 * time.Millisecond)

	got2, err := q.DequeueWithTimeout(ctx, "default", time.Second)
	if err != nil {
		t.Fatalf("re-dequeue: %v", err)
	}
	if got2.ID != task.ID {
		t.Fatalf("expected redelivery of same task")
	}
	if got2.Attempts != 2 {
		t.Fatalf("expected attempts=2 on redelivery, got %d", got2.Attempts)
	}
	_ = q.Ack(ctx, "default", got2.ID)
}

func TestInMemoryQueue_Len_ExcludesInflight(t *testing.T) {
	q := NewInMemoryQueueWithOptions(Options{
		VisibilityTimeout: 5 * time.Second,
	})
	defer q.Close()

	ctx := context.Background()
	t1 := NewTask(TaskTypeActivity, "wf1", "a")
	t2 := NewTask(TaskTypeActivity, "wf1", "b")
	_ = q.Enqueue(ctx, "default", t1)
	_ = q.Enqueue(ctx, "default", t2)

	if l, _ := q.Len(ctx, "default"); l != 2 {
		t.Fatalf("len want 2 got %d", l)
	}
	got, _ := q.DequeueWithTimeout(ctx, "default", time.Second)
	if got == nil {
		t.Fatalf("expected a task")
	}
	if l, _ := q.Len(ctx, "default"); l != 1 {
		t.Fatalf("len want 1 got %d", l)
	}
	_ = q.Ack(ctx, "default", got.ID)
}

func TestInMemoryQueue_ConcurrentProducersConsumers(t *testing.T) {
	q := NewInMemoryQueueWithOptions(Options{
		VisibilityTimeout: 500 * time.Millisecond,
	})
	defer q.Close()

	ctx := context.Background()
	queueName := "default"

	producers := 5
	consumers := 5
	total := 200

	// Start consumers first; coordinate with an atomic counter to avoid deadlock.
	var consumed int64
	doneCons := make(chan struct{}, consumers)
	for c := 0; c < consumers; c++ {
		go func() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				// Stop when we've consumed all tasks
				if atomic.LoadInt64(&consumed) >= int64(total) {
					break
				}
				task, _ := q.DequeueWithTimeout(ctx, queueName, 50*time.Millisecond)
				if task == nil {
					continue
				}
				_ = q.Ack(ctx, queueName, task.ID)
				atomic.AddInt64(&consumed, 1)
			}
			doneCons <- struct{}{}
		}()
	}

	// Start producers
	doneProd := make(chan struct{}, producers)
	for p := 0; p < producers; p++ {
		go func() {
			for i := 0; i < total/producers; i++ {
				_ = q.Enqueue(ctx, queueName, NewTask(TaskTypeActivity, "wf", "x"))
			}
			doneProd <- struct{}{}
		}()
	}
	for p := 0; p < producers; p++ {
		<-doneProd
	}

	// Wait for consumers to finish or timeout
	waitDeadline := time.After(12 * time.Second)
	doneCount := 0
	for doneCount < consumers {
		select {
		case <-doneCons:
			doneCount++
		case <-waitDeadline:
			t.Fatalf("timeout waiting for consumers, consumed=%d", atomic.LoadInt64(&consumed))
		}
	}
	if got := atomic.LoadInt64(&consumed); got != int64(total) {
		t.Fatalf("consumed %d, expected %d", got, total)
	}
}

func TestInMemoryQueue_DLQ_NackNoRequeue(t *testing.T) {
	q := NewInMemoryQueueWithOptions(Options{
		VisibilityTimeout: 50 * time.Millisecond,
		EnableDLQ:         true,
	})
	defer q.Close()

	ctx := context.Background()
	queueName := "default"
	task := NewTask(TaskTypeActivity, "wf-dlq", "x")

	if err := q.Enqueue(ctx, queueName, task); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	got, err := q.DequeueWithTimeout(ctx, queueName, time.Second)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("expected same task id")
	}

	// Nack without requeue should drop to DLQ (not redeliver)
	if err := q.Nack(ctx, queueName, got.ID, false); err != nil {
		t.Fatalf("nack: %v", err)
	}

	// Should not be redelivered if we try to dequeue again (queue empty)
	_, err = q.DequeueWithTimeout(ctx, queueName, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("expected dequeue timeout, got nil error")
	}

	// Inspect DLQ (same package allows access)
	q.mu.RLock()
	defer q.mu.RUnlock()
	d := q.dlq[queueName]
	if len(d) != 1 || d[0].ID != task.ID {
		t.Fatalf("dlq want 1 with id=%s, got=%d", task.ID, len(d))
	}
}
