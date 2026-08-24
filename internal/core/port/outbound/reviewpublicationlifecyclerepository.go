package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationLifecycleRepository interface {
	ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error
	ReviewPublication(ctx context.Context, runID uint64, runLeaseToken string) (reviewworkflow.ReviewPublication, bool, error)
	CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error
}
