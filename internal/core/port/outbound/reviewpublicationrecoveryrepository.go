package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationRecoveryRepository interface {
	AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (string, error)
	ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error
	ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error
	ReviewPublication(ctx context.Context, runID uint64, runLeaseToken string) (reviewworkflow.ReviewPublication, bool, error)
	CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error
	FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error)
	PublicationCandidateHighWatermark(ctx context.Context, before time.Time) (uint64, error)
	PublicationCandidates(ctx context.Context, before time.Time, afterID uint64, throughID uint64, limit int) ([]reviewworkflow.Run, error)
	ClaimPublicationInvalidations(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]reviewworkflow.PublicationInvalidation, error)
	CompletePublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, resolvedAt time.Time) error
	RetryPublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, failedAt time.Time, nextAttemptAt time.Time, failure string) error
}
