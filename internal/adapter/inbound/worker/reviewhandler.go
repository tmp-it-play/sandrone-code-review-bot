package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpullrequest"
)

type ReviewHandler struct {
	usecase *reviewpullrequest.UseCase
	metrics JobMetrics
}

func NewReviewHandler(usecase *reviewpullrequest.UseCase, metrics JobMetrics) *ReviewHandler {
	return &ReviewHandler{usecase: usecase, metrics: metrics}
}

func (h *ReviewHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload job.ReviewJob
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("작업을 해석하지 못했습니다: %w", asynq.SkipRetry)
	}
	payload.Attempt = attemptNumber(ctx)
	payload.FinalAttempt = isFinalAttempt(ctx)
	startedAt := time.Now()
	outcome, err := h.usecase.ExecuteWithOutcome(ctx, payload)
	label := outcomeLabel(err)
	if err == nil {
		label = string(outcome)
	}
	h.metrics.ObserveJob("review", label, time.Since(startedAt))
	return err
}
