package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type ReviewRetentionWorker struct {
	repository  outbound.ReviewRetentionRepository
	usage       outbound.UsageRepository
	clock       outbound.Clock
	logger      *slog.Logger
	retention   time.Duration
	interval    time.Duration
	batchSize   int
	usageBatch  int
	orphanAfter time.Duration
}

func NewReviewRetentionWorker(repository outbound.ReviewRetentionRepository, usage outbound.UsageRepository, clock outbound.Clock, logger *slog.Logger, retention time.Duration, reviewTimeout time.Duration) *ReviewRetentionWorker {
	return &ReviewRetentionWorker{
		repository:  repository,
		usage:       usage,
		clock:       clock,
		logger:      logger,
		retention:   retention,
		interval:    5 * time.Minute,
		batchSize:   10,
		usageBatch:  500,
		orphanAfter: max(3*time.Hour, 2*reviewTimeout),
	}
}

func (w *ReviewRetentionWorker) Run(ctx context.Context) {
	w.reconcileOrphans(ctx, w.clock.Now())
	w.cleanup(ctx)
	nextCleanup := w.clock.Now().Add(24 * time.Hour)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := w.clock.Now()
			w.reconcileOrphans(ctx, now)
			if !now.Before(nextCleanup) {
				w.cleanup(ctx)
				nextCleanup = now.Add(24 * time.Hour)
			}
		}
	}
}

func (w *ReviewRetentionWorker) cleanup(ctx context.Context) {
	now := w.clock.Now()
	legacyCutoff := now.Add(-w.retention)
	w.cleanupReviews(ctx, now, legacyCutoff)
	w.cleanupUsages(ctx, legacyCutoff)
}

func (w *ReviewRetentionWorker) reconcileOrphans(ctx context.Context, now time.Time) {
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
}

func (w *ReviewRetentionWorker) cleanupReviews(ctx context.Context, now time.Time, legacyCutoff time.Time) {
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
			break
		}
	}
}

func (w *ReviewRetentionWorker) cleanupUsages(ctx context.Context, legacyCutoff time.Time) {
	var deletedUsages int64
	for {
		deleted, err := w.usage.DeleteExpired(ctx, legacyCutoff, w.usageBatch)
		if err != nil {
			w.logger.Error("만료된 프로바이더 사용량을 제거하지 못했습니다", "error", err)
			return
		}
		deletedUsages += deleted
		if deleted < int64(w.usageBatch) {
			break
		}
	}
	if deletedUsages > 0 {
		w.logger.Info("만료된 프로바이더 사용량을 제거했습니다", "usages", deletedUsages)
	}
}
