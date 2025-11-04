// Package queue provides task queue interfaces and implementations for
// distributing work to workers.
package queue

import (
	"context"
	"time"
)

// TaskType represents the type of task
type TaskType string

const (
	TaskTypeActivity TaskType = "activity"
	TaskTypeWorkflow TaskType = "workflow"
)

// Task represents a unit of work to be executed
type Task struct {
	ID           string                 `json:"id"`
	Type         TaskType               `json:"type"`
	WorkflowID   string                 `json:"workflow_id"`
	ActivityID   string                 `json:"activity_id,omitempty"`
	ActivityName string                 `json:"activity_name,omitempty"`
	Input        interface{}            `json:"input"`
	Metadata     map[string]interface{} `json:"metadata"`
	EnqueueTime  time.Time              `json:"enqueue_time"`
	Attempts     int                    `json:"attempts"`
}

// TaskResult represents the result of task execution
type TaskResult struct {
	TaskID     string        `json:"task_id"`
	WorkflowID string        `json:"workflow_id"`
	ActivityID string        `json:"activity_id,omitempty"`
	Success    bool          `json:"success"`
	Output     interface{}   `json:"output,omitempty"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// Queue defines the interface for task distribution
type Queue interface {
	// Enqueue adds a task to the queue
	Enqueue(ctx context.Context, queueName string, task *Task) error

	// Dequeue retrieves a task from the queue (blocking)
	Dequeue(ctx context.Context, queueName string) (*Task, error)

	// DequeueWithTimeout retrieves a task with a timeout
	DequeueWithTimeout(ctx context.Context, queueName string, timeout time.Duration) (*Task, error)

	// Ack acknowledges successful task completion
	Ack(ctx context.Context, queueName string, taskID string) error

	// Nack indicates task failure and potentially requeues
	Nack(ctx context.Context, queueName string, taskID string, requeue bool) error

	// Len returns the number of tasks in the queue
	Len(ctx context.Context, queueName string) (int, error)

	// Close closes the queue and releases resources
	Close() error
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return time.Now().Format("20060102150405.000000")
}

// NewTask creates a new task with generated ID
func NewTask(taskType TaskType, workflowID string, input interface{}) *Task {
	return &Task{
		ID:          generateTaskID(),
		Type:        taskType,
		WorkflowID:  workflowID,
		Input:       input,
		Metadata:    make(map[string]interface{}),
		EnqueueTime: time.Now().UTC(),
		Attempts:    0,
	}
}
