package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpublication"
)

type ReviewPublicationWorker struct {
	interval   time.Duration
	reconciler *reviewpublication.Reconciler
}

func NewReviewPublicationWorker(runs outbound.ReviewRunRepository, publications outbound.ReviewPublicationLifecycleRepository, candidates outbound.ReviewPublicationCandidateRepository, invalidations outbound.ReviewPublicationInvalidationRepository, source outbound.PullRequestSource, publisher outbound.ReviewPublisher, checks reviewpublication.ProgressCheck, reviews outbound.ReviewRepository, clock outbound.Clock, logger *slog.Logger, retention time.Duration) *ReviewPublicationWorker {
	return &ReviewPublicationWorker{
		interval: 10 * time.Minute,
		reconciler: reviewpublication.NewReconciler(
			reviewpublication.ReconcilerDependencies{
				Runs:          runs,
				Publications:  publications,
				Candidates:    candidates,
				Invalidations: invalidations,
				Source:        source,
				Publisher:     publisher,
				Checks:        checks,
				Reviews:       reviews,
				Clock:         clock,
				Logger:        logger,
			},
			reviewpublication.ReconcilerConfig{
				Retention:         retention,
				RunLease:          20 * time.Minute,
				PublicationLease:  5 * time.Minute,
				OrphanAfter:       7 * 24 * time.Hour,
				ReconcileTimeout:  90 * time.Second,
				BatchSize:         10,
				InvalidationLimit: 10,
			},
		),
	}
}

func (w *ReviewPublicationWorker) Run(ctx context.Context) {
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
