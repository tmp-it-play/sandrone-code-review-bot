package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/refreshprogresscomment"
)

type ProgressCommentWorker struct {
	interval   time.Duration
	reconciler *refreshprogresscomment.Reconciler
}

const progressCommentMutationTimeout = 20 * time.Second
const progressCommentStoreTimeout = 10 * time.Second

func NewProgressCommentWorker(refreshes outbound.ProgressCommentRefreshRepository, publisher outbound.ReviewPublisher, renderer outbound.Renderer, clock outbound.Clock, logger *slog.Logger) *ProgressCommentWorker {
	return &ProgressCommentWorker{
		interval: 6 * time.Second,
		reconciler: refreshprogresscomment.New(refreshprogresscomment.Dependencies{
			Refreshes: refreshes,
			Publisher: publisher,
			Renderer:  renderer,
			Clock:     clock,
			Logger:    logger,
		}, refreshprogresscomment.Config{
			Lease:       reviewworkflow.PublicationInvalidationFenceDelay + progressCommentMutationTimeout + progressCommentStoreTimeout,
			Timeout:     progressCommentMutationTimeout,
			Limit:       4,
			Concurrency: 4,
		}),
	}
}

func (w *ProgressCommentWorker) Run(ctx context.Context) {
	w.reconciler.Reconcile(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.reconciler.Reconcile(ctx)
		}
	}
}
