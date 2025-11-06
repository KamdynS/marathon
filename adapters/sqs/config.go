//go:build adapters_sqs
// +build adapters_sqs

package sqsqueue

// Config controls the SQS adapter behavior.
type Config struct {
	// Required: fully qualified SQS queue URL
	QueueURL string

	// Optional: AWS region; falls back to default chain if empty
	Region string

	// ReceiveMessage long polling seconds (0..20). If DequeueWithTimeout supplies
	// a shorter timeout, that value is used instead for that call.
	WaitTimeSeconds int

	// Visibility timeout in seconds for received messages.
	VisibilityTimeout int

	// Number of messages to request per ReceiveMessage (1..10). Adapter returns only one for now.
	MaxNumberOfMessages int

	// FIFO mode. If true, MessageGroupID is required (or derived from Task.WorkflowID).
	FIFO bool
	// Message group ID to use for FIFO queues when not derivable from task.
	MessageGroupID string

	// Backoff in seconds when Nack with requeue=true. 0 makes it immediately available.
	RequeueBackoffSeconds int

	// If true and Nack with requeue=false, drop the message (DeleteMessage) instead of exposing it.
	DropOnNackNoRequeue bool
}

// DefaultConfig provides sensible defaults.
func DefaultConfig() Config {
	return Config{
		WaitTimeSeconds:       20,
		VisibilityTimeout:     30,
		MaxNumberOfMessages:   1,
		RequeueBackoffSeconds: 0,
	}
}


