package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/summarizepullrequest"
)

type SummaryHandler struct {
	usecase *summarizepullrequest.UseCase
	metrics *observability.Metrics
}

func NewSummaryHandler(usecase *summarizepullrequest.UseCase, metrics *observability.Metrics) *SummaryHandler {
	return &SummaryHandler{usecase: usecase, metrics: metrics}
}

func (h *SummaryHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload job.SummaryJob
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("작업을 해석하지 못했다: %w", asynq.SkipRetry)
	}
	payload.FinalAttempt = isFinalAttempt(ctx)
	startedAt := time.Now()
	err := h.usecase.Execute(ctx, payload)
	h.metrics.ObserveJob("summary", outcomeLabel(err), time.Since(startedAt))
	return err
}
