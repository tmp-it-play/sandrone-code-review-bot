package reviewpublication

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReceiptRepository interface {
	outbound.ReviewPublicationAttemptRepository
	ReviewPublication(ctx context.Context, runID uint64, token string) (reviewworkflow.ReviewPublication, bool, error)
	CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error
}
