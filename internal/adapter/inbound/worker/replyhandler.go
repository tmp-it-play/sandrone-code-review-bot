package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/replythread"
)

type ReplyHandler struct {
	usecase *replythread.UseCase
	metrics JobMetrics
}

func NewReplyHandler(usecase *replythread.UseCase, metrics JobMetrics) *ReplyHandler {
	return &ReplyHandler{usecase: usecase, metrics: metrics}
}

func (h *ReplyHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload job.ReplyJob
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("작업을 해석하지 못했습니다: %w", asynq.SkipRetry)
	}
	payload.Attempt = attemptNumber(ctx)
	payload.FinalAttempt = isFinalAttempt(ctx)
	startedAt := time.Now()
	err := h.usecase.Execute(ctx, payload)
	h.metrics.ObserveJob("reply", outcomeLabel(err), time.Since(startedAt))
	return err
}
