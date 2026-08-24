package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationInvalidationRepository interface {
	ClaimPublicationInvalidations(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]reviewworkflow.PublicationInvalidation, error)
	CompletePublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, resolvedAt time.Time) error
	RetryPublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, failedAt time.Time, nextAttemptAt time.Time, failure string) error
}
