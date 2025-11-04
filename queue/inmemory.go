package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemoryQueue is a channel-based in-memory queue implementation
type InMemoryQueue struct {
	mu      sync.RWMutex
	queues  map[string]chan *Task
	pending map[string]map[string]*Task // queueName -> taskID -> task
	closed  bool
}

// NewInMemoryQueue creates a new in-memory queue
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		queues:  make(map[string]chan *Task),
		pending: make(map[string]map[string]*Task),
		closed:  false,
	}
}

// getOrCreateQueue returns an existing queue channel or creates a new one
func (q *InMemoryQueue) getOrCreateQueue(queueName string) chan *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil
	}

	queue, exists := q.queues[queueName]
	if !exists {
		// Create buffered channel to prevent blocking on enqueue
		queue = make(chan *Task, 100)
		q.queues[queueName] = queue
		q.pending[queueName] = make(map[string]*Task)
	}

	return queue
}

// Enqueue implements Queue
func (q *InMemoryQueue) Enqueue(ctx context.Context, queueName string, task *Task) error {
	queue := q.getOrCreateQueue(queueName)
	if queue == nil {
		return fmt.Errorf("queue is closed")
	}

	select {
	case queue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Dequeue implements Queue
func (q *InMemoryQueue) Dequeue(ctx context.Context, queueName string) (*Task, error) {
	return q.DequeueWithTimeout(ctx, queueName, 0)
}

// DequeueWithTimeout implements Queue
func (q *InMemoryQueue) DequeueWithTimeout(ctx context.Context, queueName string, timeout time.Duration) (*Task, error) {
	queue := q.getOrCreateQueue(queueName)
	if queue == nil {
		return nil, fmt.Errorf("queue is closed")
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	select {
	case task := <-queue:
		if task != nil {
			task.Attempts++
			q.addPending(queueName, task)
		}
		return task, nil
	case <-timeoutCh:
		return nil, fmt.Errorf("dequeue timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// addPending adds a task to the pending map
func (q *InMemoryQueue) addPending(queueName string, task *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if pending, exists := q.pending[queueName]; exists {
		pending[task.ID] = task
	}
}

// removePending removes a task from the pending map
func (q *InMemoryQueue) removePending(queueName string, taskID string) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if pending, exists := q.pending[queueName]; exists {
		task := pending[taskID]
		delete(pending, taskID)
		return task
	}

	return nil
}

// Ack implements Queue
func (q *InMemoryQueue) Ack(ctx context.Context, queueName string, taskID string) error {
	task := q.removePending(queueName, taskID)
	if task == nil {
		return fmt.Errorf("task %s not found in pending", taskID)
	}
	return nil
}

// Nack implements Queue
func (q *InMemoryQueue) Nack(ctx context.Context, queueName string, taskID string, requeue bool) error {
	task := q.removePending(queueName, taskID)
	if task == nil {
		return fmt.Errorf("task %s not found in pending", taskID)
	}

	if requeue {
		return q.Enqueue(ctx, queueName, task)
	}

	return nil
}

// Len implements Queue
func (q *InMemoryQueue) Len(ctx context.Context, queueName string) (int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	queue, exists := q.queues[queueName]
	if !exists {
		return 0, nil
	}

	return len(queue), nil
}

// Close implements Queue
func (q *InMemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil
	}

	q.closed = true

	for _, queue := range q.queues {
		close(queue)
	}

	q.queues = make(map[string]chan *Task)
	q.pending = make(map[string]map[string]*Task)

	return nil
}
