package reviewpublication

import (
	"context"
	"time"
)

type ReceiptRepository interface {
	CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error
}
