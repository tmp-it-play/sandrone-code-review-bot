package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationRepository interface {
	ReviewPublicationLifecycleRepository
	PrepareReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, payload reviewworkflow.ReviewPublicationPayload, finalization reviewworkflow.ReviewPublicationFinalization, preparedAt time.Time, expiresAt time.Time) (reviewworkflow.ReviewPublication, error)
}
