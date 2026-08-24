package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type ReviewRetentionWorker struct {
	repository  outbound.ReviewWorkflowRepository
	clock       outbound.Clock
	logger      *slog.Logger
	retention   time.Duration
	interval    time.Duration
	batchSize   int
	orphanAfter time.Duration
}

func NewReviewRetentionWorker(repository outbound.ReviewWorkflowRepository, clock outbound.Clock, logger *slog.Logger, retention time.Duration) *ReviewRetentionWorker {
	return &ReviewRetentionWorker{
		repository:  repository,
		clock:       clock,
		logger:      logger,
		retention:   retention,
		interval:    24 * time.Hour,
		batchSize:   10,
		orphanAfter: 7 * 24 * time.Hour,
	}
}

func (w *ReviewRetentionWorker) Run(ctx context.Context) {
	w.cleanup(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanup(ctx)
		}
	}
}

func (w *ReviewRetentionWorker) cleanup(ctx context.Context) {
	now := w.clock.Now()
	legacyCutoff := now.Add(-w.retention)
	for {
		orphans, err := w.repository.ReconcileOrphans(ctx, now.Add(-w.orphanAfter), now, now.Add(w.retention), w.batchSize)
		if err != nil {
			w.logger.Error("중단된 리뷰 실행을 정리하지 못했습니다", "error", err)
			return
		}
		if orphans.Reconciled > 0 {
			w.logger.Info("중단된 리뷰 실행을 실패로 종료했습니다", "runs", orphans.Reconciled)
		}
		if orphans.Selected < w.batchSize {
			break
		}
	}
	for {
		result, err := w.repository.DeleteExpired(ctx, now, legacyCutoff, w.batchSize)
		if err != nil {
			w.logger.Error("만료된 리뷰 데이터를 제거하지 못했습니다", "error", err)
			return
		}
		if result.Runs+result.Reviews+result.Findings+result.Occurrences+result.States+result.Publications > 0 {
			w.logger.Info("만료된 리뷰 데이터를 제거했습니다", "runs", result.Runs, "reviews", result.Reviews, "findings", result.Findings, "occurrences", result.Occurrences, "states", result.States, "publications", result.Publications)
		}
		if result.Runs < int64(w.batchSize) && result.Reviews < int64(w.batchSize) && result.Findings < int64(w.batchSize) && result.Occurrences < int64(w.batchSize) && result.States < int64(w.batchSize) && result.Publications < int64(w.batchSize) {
			return
		}
	}
}
