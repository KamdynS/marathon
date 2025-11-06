package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// pendingRecord tracks an inflight task and its visibility deadline.
type pendingRecord struct {
	task     *Task
	deadline time.Time
}

// Hooks provides optional callbacks for queue operations. All are optional no-ops by default.
type Hooks struct {
	OnEnqueue   func(queueName string, task *Task)
	OnDequeue   func(queueName string, task *Task)
	OnAck       func(queueName string, task *Task)
	OnNack      func(queueName string, task *Task, requeue bool)
	OnRedeliver func(queueName string, task *Task)
}

// Options configures the in-memory queue behavior.
type Options struct {
	// VisibilityTimeout controls how long a dequeued task stays invisible
	// before it is eligible for redelivery if not Ack'ed.
	VisibilityTimeout time.Duration
	// EnableDLQ routes Nack'ed (requeue=false) tasks to an in-memory DLQ.
	EnableDLQ bool
	// Hooks are optional callbacks for instrumentation; all nil means no-op.
	Hooks Hooks
}

// InMemoryQueue is a channel-based in-memory queue implementation
type InMemoryQueue struct {
	mu      sync.RWMutex
	queues  map[string]chan *Task
	pending map[string]map[string]*pendingRecord // queueName -> taskID -> record
	dlq     map[string][]*Task                   // optional DLQ per queue
	closed  bool
	opts    Options
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewInMemoryQueue creates a new in-memory queue
func NewInMemoryQueue() *InMemoryQueue {
	return NewInMemoryQueueWithOptions(Options{
		VisibilityTimeout: 30 * time.Second,
		EnableDLQ:         false,
	})
}

// NewInMemoryQueueWithOptions creates a new in-memory queue with options.
func NewInMemoryQueueWithOptions(opts Options) *InMemoryQueue {
	q := &InMemoryQueue{
		queues:  make(map[string]chan *Task),
		pending: make(map[string]map[string]*pendingRecord),
		dlq:     make(map[string][]*Task),
		closed:  false,
		opts:    opts,
		stopCh:  make(chan struct{}),
	}
	q.wg.Add(1)
	go q.scanLoop()
	return q
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
		q.pending[queueName] = make(map[string]*pendingRecord)
		if _, ok := q.dlq[queueName]; !ok {
			q.dlq[queueName] = make([]*Task, 0)
		}
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
		if q.opts.Hooks.OnEnqueue != nil {
			q.opts.Hooks.OnEnqueue(queueName, task)
		}
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
			vis := q.opts.VisibilityTimeout
			if vis <= 0 {
				vis = 30 * time.Second
			}
			q.addPending(queueName, task, time.Now().Add(vis))
			if q.opts.Hooks.OnDequeue != nil {
				q.opts.Hooks.OnDequeue(queueName, task)
			}
		}
		return task, nil
	case <-timeoutCh:
		return nil, fmt.Errorf("dequeue timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// addPending adds a task to the inflight/pending map with a visibility deadline.
func (q *InMemoryQueue) addPending(queueName string, task *Task, deadline time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if pending, exists := q.pending[queueName]; exists {
		pending[task.ID] = &pendingRecord{task: task, deadline: deadline}
	}
}

// removePending removes a task from the pending map
func (q *InMemoryQueue) removePending(queueName string, taskID string) *pendingRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	if pending, exists := q.pending[queueName]; exists {
		rec := pending[taskID]
		delete(pending, taskID)
		return rec
	}

	return nil
}

// Ack implements Queue
func (q *InMemoryQueue) Ack(ctx context.Context, queueName string, taskID string) error {
	rec := q.removePending(queueName, taskID)
	if rec == nil {
		return fmt.Errorf("task %s not found in pending", taskID)
	}
	if q.opts.Hooks.OnAck != nil {
		q.opts.Hooks.OnAck(queueName, rec.task)
	}
	return nil
}

// Nack implements Queue
func (q *InMemoryQueue) Nack(ctx context.Context, queueName string, taskID string, requeue bool) error {
	rec := q.removePending(queueName, taskID)
	if rec == nil {
		return fmt.Errorf("task %s not found in pending", taskID)
	}

	if requeue {
		if q.opts.Hooks.OnNack != nil {
			q.opts.Hooks.OnNack(queueName, rec.task, true)
		}
		return q.Enqueue(ctx, queueName, rec.task)
	}

	if q.opts.Hooks.OnNack != nil {
		q.opts.Hooks.OnNack(queueName, rec.task, false)
	}
	if q.opts.EnableDLQ {
		q.mu.Lock()
		q.dlq[queueName] = append(q.dlq[queueName], rec.task)
		q.mu.Unlock()
	}
	return nil
}

// Len implements Queue.
// Len returns the number of READY (not inflight) tasks for the named queue.
// Inflight tasks (dequeued but not yet Ack'ed) are NOT included.
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
	// Stop scanner
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.stopCh)
	q.mu.Unlock()

	q.wg.Wait()

	q.mu.Lock()
	for _, queue := range q.queues {
		close(queue)
	}

	q.queues = make(map[string]chan *Task)
	q.pending = make(map[string]map[string]*pendingRecord)
	q.dlq = make(map[string][]*Task)
	q.mu.Unlock()

	return nil
}

// scanLoop periodically scans inflight tasks and redelivers those past visibility deadline.
func (q *InMemoryQueue) scanLoop() {
	defer q.wg.Done()
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case now := <-t.C:
			q.scanOnce(now)
		}
	}
}

func (q *InMemoryQueue) scanOnce(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for queueName, inflight := range q.pending {
		ch := q.queues[queueName]
		for id, rec := range inflight {
			if now.After(rec.deadline) {
				// Redeliver exactly once per expiry: move back to ready and remove from inflight
				select {
				case ch <- rec.task:
					if q.opts.Hooks.OnRedeliver != nil {
						q.opts.Hooks.OnRedeliver(queueName, rec.task)
					}
					delete(inflight, id)
				default:
					// Queue is full; push deadline forward a bit and try again later.
					rec.deadline = now.Add(200 * time.Millisecond)
				}
			}
		}
	}
}
