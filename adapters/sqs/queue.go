//go:build adapters_sqs
// +build adapters_sqs

package sqsqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/KamdynS/marathon/queue"
)

// Queue implements queue.Queue backed by AWS SQS.
type Queue struct {
	client  *sqs.Client
	cfg     Config
	mu      sync.Mutex
	handles map[string]string // taskID -> receiptHandle
}

// New constructs a new SQS-backed queue adapter using default AWS config chain.
func New(ctx context.Context, cfg Config) (*Queue, error) {
	if cfg.QueueURL == "" {
		return nil, fmt.Errorf("QueueURL is required")
	}
	base := DefaultConfig()
	// apply defaults
	if cfg.WaitTimeSeconds == 0 {
		cfg.WaitTimeSeconds = base.WaitTimeSeconds
	}
	if cfg.VisibilityTimeout == 0 {
		cfg.VisibilityTimeout = base.VisibilityTimeout
	}
	if cfg.MaxNumberOfMessages == 0 {
		cfg.MaxNumberOfMessages = base.MaxNumberOfMessages
	}

	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	awscfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := sqs.NewFromConfig(awscfg)
	return NewFromClient(client, cfg), nil
}

// NewFromClient constructs the adapter from an existing SQS client.
func NewFromClient(client *sqs.Client, cfg Config) *Queue {
	return &Queue{
		client:  client,
		cfg:     cfg,
		handles: make(map[string]string),
	}
}

// Enqueue sends a task to SQS. queueName is ignored; QueueURL controls the destination.
func (q *Queue) Enqueue(ctx context.Context, _ string, t *queue.Task) error {
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	msgAttributes := map[string]sqstypes.MessageAttributeValue{
		"ActivityName": {
			DataType:    aws.String("String"),
			StringValue: aws.String(t.ActivityName),
		},
		"Attempts": {
			DataType:    aws.String("Number"),
			StringValue: aws.String(strconv.Itoa(t.Attempts)),
		},
		"TaskType": {
			DataType:    aws.String("String"),
			StringValue: aws.String(string(t.Type)),
		},
	}
	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(q.cfg.QueueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: msgAttributes,
	}
	if q.cfg.FIFO {
		groupID := q.cfg.MessageGroupID
		if groupID == "" {
			groupID = t.WorkflowID
		}
		input.MessageGroupId = aws.String(groupID)
		// Deduplication window is 5 minutes for FIFO standard dedup
		input.MessageDeduplicationId = aws.String(t.ID)
	}
	_, err = q.client.SendMessage(ctx, input)
	if err != nil {
		return fmt.Errorf("sqs SendMessage: %w", err)
	}
	return nil
}

// Dequeue retrieves a task using the configured WaitTimeSeconds.
func (q *Queue) Dequeue(ctx context.Context, queueName string) (*queue.Task, error) {
	wait := time.Duration(q.cfg.WaitTimeSeconds) * time.Second
	return q.DequeueWithTimeout(ctx, queueName, wait)
}

// DequeueWithTimeout performs long-poll ReceiveMessage and returns a single task if available.
func (q *Queue) DequeueWithTimeout(ctx context.Context, _ string, timeout time.Duration) (*queue.Task, error) {
	waitSec := q.cfg.WaitTimeSeconds
	if timeout > 0 {
		if int(timeout/time.Second) < waitSec {
			waitSec = int(timeout / time.Second)
		}
	}
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > 20 {
		waitSec = 20
	}
	maxMsgs := q.cfg.MaxNumberOfMessages
	if maxMsgs < 1 {
		maxMsgs = 1
	}
	if maxMsgs > 10 {
		maxMsgs = 10
	}
	vis := q.cfg.VisibilityTimeout
	if vis < 0 {
		vis = 0
	}
	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(q.cfg.QueueURL),
		// Request both system attributes and custom attributes
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
		},
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{
			sqstypes.MessageSystemAttributeNameApproximateReceiveCount,
		},
		MessageAttributeNames: []string{"All"},
		MaxNumberOfMessages:   int32(maxMsgs),
		WaitTimeSeconds:       int32Ptr(int32(waitSec)),
	}
	// Apply visibility timeout if configured
	if vis > 0 {
		input.VisibilityTimeout = int32Ptr(int32(vis))
	}
	out, err := q.client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("sqs ReceiveMessage: %w", err)
	}
	if len(out.Messages) == 0 {
		return nil, nil
	}
	msg := out.Messages[0]

	var t queue.Task
	if msg.Body == nil {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(*msg.Body), &t); err != nil {
		return nil, fmt.Errorf("unmarshal task body: %w", err)
	}
	// Derive attempts from system attribute if present
	attempts := 0
	if msg.Attributes != nil {
		if rc, ok := msg.Attributes[string(sqstypes.MessageSystemAttributeNameApproximateReceiveCount)]; ok {
			if n, convErr := strconv.Atoi(rc); convErr == nil {
				attempts = n
			}
		}
	}
	if attempts <= 0 {
		if t.Attempts > 0 {
			attempts = t.Attempts
		} else {
			attempts = 1
		}
	}
	t.Attempts = attempts
	if t.Metadata == nil {
		t.Metadata = make(map[string]interface{})
	}
	if msg.ReceiptHandle != nil {
		t.Metadata["sqs_receipt_handle"] = *msg.ReceiptHandle
		q.mu.Lock()
		q.handles[t.ID] = *msg.ReceiptHandle
		q.mu.Unlock()
	}
	return &t, nil
}

// Ack deletes the message using the stored receipt handle.
func (q *Queue) Ack(ctx context.Context, _ string, taskID string) error {
	receipt, ok := q.lookupHandle(taskID)
	if !ok {
		return fmt.Errorf("ack: receipt handle not found for task %s", taskID)
	}
	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.cfg.QueueURL),
		ReceiptHandle: aws.String(receipt),
	})
	if err != nil {
		return fmt.Errorf("sqs DeleteMessage: %w", err)
	}
	q.mu.Lock()
	delete(q.handles, taskID)
	q.mu.Unlock()
	return nil
}

// Nack changes visibility; optionally deletes on non-requeue if configured.
func (q *Queue) Nack(ctx context.Context, _ string, taskID string, requeue bool) error {
	receipt, ok := q.lookupHandle(taskID)
	if !ok {
		return fmt.Errorf("nack: receipt handle not found for task %s", taskID)
	}
	if !requeue && q.cfg.DropOnNackNoRequeue {
		_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(q.cfg.QueueURL),
			ReceiptHandle: aws.String(receipt),
		})
		if err != nil {
			return fmt.Errorf("sqs DeleteMessage: %w", err)
		}
		q.mu.Lock()
		delete(q.handles, taskID)
		q.mu.Unlock()
		return nil
	}
	vis := int32(0)
	if requeue {
		vis = int32(q.cfg.RequeueBackoffSeconds)
		if vis < 0 {
			vis = 0
		}
	}
	_, err := q.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(q.cfg.QueueURL),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: vis,
	})
	if err != nil {
		return fmt.Errorf("sqs ChangeMessageVisibility: %w", err)
	}
	return nil
}

// Len returns ApproximateNumberOfMessages (ready only).
func (q *Queue) Len(ctx context.Context, _ string) (int, error) {
	out, err := q.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(q.cfg.QueueURL),
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("sqs GetQueueAttributes: %w", err)
	}
	if out.Attributes == nil {
		return 0, nil
	}
	s := out.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)]
	if s == "" {
		return 0, nil
	}
	n, convErr := strconv.Atoi(s)
	if convErr != nil {
		return 0, nil
	}
	return n, nil
}

func (q *Queue) Close() error {
	return nil
}

func (q *Queue) lookupHandle(taskID string) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	h, ok := q.handles[taskID]
	return h, ok
}

func int32Ptr(v int32) *int32 { return &v }


