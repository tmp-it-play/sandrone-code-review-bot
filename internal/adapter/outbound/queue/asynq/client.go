package asynq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
)

const taskRetention = 30 * 24 * time.Hour
const defaultTaskTimeout = 20 * time.Minute
const reviewTaskTimeout = 3 * time.Hour
const conversationalTaskTimeout = 50 * time.Minute

type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewClient(client *asynq.Client, inspector *asynq.Inspector) *Client {
	return &Client{client: client, inspector: inspector}
}

func (c *Client) EnqueueReview(ctx context.Context, payload job.ReviewJob) error {
	_, err := c.EnqueueReviewAdmission(ctx, payload)
	return err
}

func (c *Client) EnqueueReviewAdmission(ctx context.Context, payload job.ReviewJob) (job.ReviewAdmission, error) {
	accepted, err := c.enqueue(ctx, TaskReview, payload)
	if err != nil {
		return job.ReviewAdmission{}, err
	}
	if accepted {
		return job.ReviewAdmission{Accepted: true, ProgressMessageTheme: progresscomment.NormalizeTheme(payload.ProgressMessageTheme)}, nil
	}
	admission, err := c.progressAdmission(payload)
	if err != nil {
		return job.ReviewAdmission{}, err
	}
	return admission, nil
}

func (c *Client) EnqueueSummary(ctx context.Context, payload job.SummaryJob) error {
	_, err := c.enqueue(ctx, TaskSummary, payload)
	return err
}

func (c *Client) progressAdmission(payload job.ReviewJob) (job.ReviewAdmission, error) {
	id := deterministicTaskID(TaskReview, payload)
	if id == "" || c.inspector == nil {
		return job.ReviewAdmission{}, nil
	}
	info, err := c.inspector.GetTaskInfo("default", id)
	if err != nil {
		return job.ReviewAdmission{}, fmt.Errorf("기존 리뷰 작업을 확인하지 못했습니다: %w", err)
	}
	if info.State == asynq.TaskStateCompleted || info.State == asynq.TaskStateArchived {
		return job.ReviewAdmission{}, nil
	}
	var stored job.ReviewJob
	if err := json.Unmarshal(info.Payload, &stored); err != nil {
		return job.ReviewAdmission{}, fmt.Errorf("기존 리뷰 작업을 읽지 못했습니다: %w", err)
	}
	recoverable := stored.ProgressMarker != "" && stored.ProgressMarker == payload.ProgressMarker
	return job.ReviewAdmission{
		ProgressRecoverable:  recoverable,
		ProgressMessageTheme: progresscomment.NormalizeTheme(stored.ProgressMessageTheme),
	}, nil
}

func (c *Client) EnqueueReply(ctx context.Context, payload job.ReplyJob) error {
	_, err := c.enqueue(ctx, TaskReply, payload)
	return err
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) enqueue(ctx context.Context, taskType string, payload any) (bool, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("작업을 직렬화하지 못했습니다: %w", err)
	}
	task := asynq.NewTask(taskType, encoded)
	options := []asynq.Option{
		asynq.MaxRetry(6),
		asynq.Timeout(timeoutFor(taskType)),
		asynq.Retention(taskRetention),
	}
	if id := deterministicTaskID(taskType, payload); id != "" {
		options = append(options, asynq.TaskID(id))
	}
	if _, err := c.client.EnqueueContext(ctx, task, options...); err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return false, nil
		}
		return false, fmt.Errorf("작업을 큐에 넣지 못했습니다: %w", err)
	}
	return true, nil
}

func timeoutFor(taskType string) time.Duration {
	switch taskType {
	case TaskReview:
		return reviewTaskTimeout
	case TaskSummary, TaskReply:
		return conversationalTaskTimeout
	default:
		return defaultTaskTimeout
	}
}
