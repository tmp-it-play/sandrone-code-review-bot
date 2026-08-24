package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

const findingOccurrenceBootstrapInterval = 5 * time.Second
const findingOccurrenceBootstrapTimeout = 30 * time.Second

type FindingOccurrenceBootstrapWorker struct {
	bootstrap outbound.FindingOccurrenceBootstrap
	logger    *slog.Logger
}

func NewFindingOccurrenceBootstrapWorker(bootstrap outbound.FindingOccurrenceBootstrap, logger *slog.Logger) *FindingOccurrenceBootstrapWorker {
	return &FindingOccurrenceBootstrapWorker{bootstrap: bootstrap, logger: logger}
}

func (w *FindingOccurrenceBootstrapWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(findingOccurrenceBootstrapInterval)
	defer ticker.Stop()
	for {
		batchContext, cancel := context.WithTimeout(ctx, findingOccurrenceBootstrapTimeout)
		completed, err := w.bootstrap.BackfillBatch(batchContext)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				w.logger.Error("기존 지적 발생 이력 migration batch를 처리하지 못했습니다", "error", err)
			}
		} else if completed {
			w.logger.Info("기존 지적 발생 이력 migration을 완료했습니다")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
