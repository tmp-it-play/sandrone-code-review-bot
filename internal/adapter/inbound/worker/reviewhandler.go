package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpullrequest"
)

type ReviewHandler struct {
	usecase *reviewpullrequest.UseCase
	metrics *observability.Metrics
}

func NewReviewHandler(usecase *reviewpullrequest.UseCase, metrics *observability.Metrics) *ReviewHandler {
	return &ReviewHandler{usecase: usecase, metrics: metrics}
}

func (h *ReviewHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload job.ReviewJob
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("작업을 해석하지 못했다: %w", asynq.SkipRetry)
	}
	payload.FinalAttempt = isFinalAttempt(ctx)
	startedAt := time.Now()
	err := h.usecase.Execute(ctx, payload)
	h.metrics.ObserveJob("review", outcomeLabel(err), time.Since(startedAt))
	return err
}
