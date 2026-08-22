package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
)

type Client struct {
	client *asynq.Client
}

func NewClient(client *asynq.Client) *Client {
	return &Client{client: client}
}

func (c *Client) EnqueueReview(ctx context.Context, payload job.ReviewJob) error {
	return c.enqueue(ctx, TaskReview, payload)
}

func (c *Client) EnqueueSummary(ctx context.Context, payload job.SummaryJob) error {
	return c.enqueue(ctx, TaskSummary, payload)
}

func (c *Client) EnqueueReply(ctx context.Context, payload job.ReplyJob) error {
	return c.enqueue(ctx, TaskReply, payload)
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) enqueue(ctx context.Context, taskType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("작업을 직렬화하지 못했습니다: %w", err)
	}
	task := asynq.NewTask(taskType, encoded)
	options := []asynq.Option{
		asynq.MaxRetry(6),
		asynq.Timeout(20 * time.Minute),
		asynq.Retention(48 * time.Hour),
	}
	if _, err := c.client.EnqueueContext(ctx, task, options...); err != nil {
		return fmt.Errorf("작업을 큐에 넣지 못했습니다: %w", err)
	}
	return nil
}
