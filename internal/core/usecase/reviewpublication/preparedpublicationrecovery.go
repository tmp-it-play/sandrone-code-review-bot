package reviewpublication

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) publishPreparedPublication(ctx context.Context, target pullrequest.Target, runID uint64, leaseToken string, prepared reviewworkflow.ReviewPublication) error {
	return r.publications.Publish(ctx, target, runID, leaseToken, prepared)
}

func (r *Reconciler) completeReviewPublication(ctx context.Context, runID uint64, leaseToken string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	return r.publications.Complete(ctx, runID, leaseToken, marker, channel, externalID, expiresAt)
}
