//go:build adapters_sqs
// +build adapters_sqs

package sqsqueue

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/KamdynS/marathon/queue"
)

func sqsClientAndURL(t *testing.T) (*sqs.Client, string) {
	t.Helper()
	ctx := context.Background()
	// Prefer explicit queue URL if provided
	if qurl := os.Getenv("SQS_QUEUE_URL"); qurl != "" {
		awscfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			t.Fatalf("aws config: %v", err)
		}
		return sqs.NewFromConfig(awscfg), qurl
	}
	// Localstack mode
	endpoint := os.Getenv("LOCALSTACK_URL")
	if endpoint == "" {
		t.Skip("neither SQS_QUEUE_URL nor LOCALSTACK_URL set; skipping SQS integration tests")
	}
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == sqs.ServiceID {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	awscfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		t.Fatalf("aws config localstack: %v", err)
	}
	client := sqs.NewFromConfig(awscfg)
	// create a throwaway queue
	qname := "marathon-test-" + time.Now().UTC().Format("20060102150405")
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qname),
		Attributes: map[string]string{
			"VisibilityTimeout": "3",
		},
	})
	if err != nil {
		t.Fatalf("create localstack queue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: out.QueueUrl})
	})
	return client, aws.ToString(out.QueueUrl)
}

func newAdapterForTest(t *testing.T, maxBatch int, backoff int) *Queue {
	client, url := sqsClientAndURL(t)
	cfg := DefaultConfig()
	cfg.QueueURL = url
	cfg.WaitTimeSeconds = 2
	cfg.VisibilityTimeout = 3
	cfg.MaxNumberOfMessages = maxBatch
	cfg.RequeueBackoffSeconds = backoff
	return NewFromClient(client, cfg)
}

func TestEnqueueDequeueAck(t *testing.T) {
	q := newAdapterForTest(t, 1, 0)
	ctx := context.Background()

	task := queue.NewTask(queue.TaskTypeActivity, "wf-1", map[string]interface{}{"x": 1})
	task.ActivityID = "act-1"
	task.ActivityName = "do"
	buf, _ := json.Marshal(task.Input)
	_ = buf // silence linters about unused variable for Input type

	if err := q.Enqueue(ctx, "ignored", task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.DequeueWithTimeout(ctx, "ignored", 3*time.Second)
	if err != nil {
		t.Fatalf("DequeueWithTimeout: %v", err)
	}
	if got == nil {
		t.Fatalf("expected task, got nil")
	}
	if got.ID != task.ID || got.WorkflowID != task.WorkflowID {
		t.Fatalf("task mismatch: got %+v want %+v", got, task)
	}
	if err := q.Ack(ctx, "ignored", got.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	// should be gone
	got2, err := q.DequeueWithTimeout(ctx, "ignored", 2*time.Second)
	if err != nil {
		t.Fatalf("DequeueWithTimeout 2: %v", err)
	}
	if got2 != nil {
		t.Fatalf("expected no message after ack")
	}
}

func TestVisibilityRedelivery(t *testing.T) {
	q := newAdapterForTest(t, 1, 0)
	ctx := context.Background()
	task := queue.NewTask(queue.TaskTypeActivity, "wf-v", map[string]interface{}{"x": 2})
	task.ActivityID = "act-v"
	task.ActivityName = "vis"
	if err := q.Enqueue(ctx, "ignored", task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got1, err := q.DequeueWithTimeout(ctx, "ignored", 2*time.Second)
	if err != nil || got1 == nil {
		t.Fatalf("first dequeue: %v %v", got1, err)
	}
	// don't ack; wait for visibility timeout to expire
	time.Sleep(4 * time.Second)
	got2, err := q.DequeueWithTimeout(ctx, "ignored", 3*time.Second)
	if err != nil || got2 == nil {
		t.Fatalf("second dequeue: %v %v", got2, err)
	}
	if got2.ID != task.ID {
		t.Fatalf("expected same task redelivered, got %s", got2.ID)
	}
	_ = q.Ack(ctx, "ignored", got2.ID)
}

func TestNackRequeueImmediate(t *testing.T) {
	q := newAdapterForTest(t, 1, 0)
	ctx := context.Background()
	task := queue.NewTask(queue.TaskTypeActivity, "wf-n", map[string]interface{}{"x": 3})
	task.ActivityID = "act-n"
	task.ActivityName = "nack"
	if err := q.Enqueue(ctx, "ignored", task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.DequeueWithTimeout(ctx, "ignored", 2*time.Second)
	if err != nil || got == nil {
		t.Fatalf("dequeue: %v %v", got, err)
	}
	if err := q.Nack(ctx, "ignored", got.ID, true); err != nil {
		t.Fatalf("Nack requeue: %v", err)
	}
	got2, err := q.DequeueWithTimeout(ctx, "ignored", 3*time.Second)
	if err != nil || got2 == nil {
		t.Fatalf("dequeue again: %v %v", got2, err)
	}
	if got2.ID != got.ID {
		t.Fatalf("expected same task after requeue")
	}
	_ = q.Ack(ctx, "ignored", got2.ID)
}

func TestLenApproximate(t *testing.T) {
	q := newAdapterForTest(t, 2, 0) // exercise MaxNumberOfMessages>1 path
	ctx := context.Background()
	// Just ensure it doesn't error and returns >=0
	n, err := q.Len(ctx, "ignored")
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n < 0 {
		t.Fatalf("Len should be non-negative")
	}
}
