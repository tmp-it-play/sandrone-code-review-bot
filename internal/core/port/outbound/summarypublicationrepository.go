package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type SummaryPublicationRepository interface {
	ClaimSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, orderKey string, observedAt time.Time, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error)
	ReserveSummaryExternalCall(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, limit int) (bool, error)
	SummaryCompletion(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, inputHash string, currentAt time.Time) (publication.CompletionCheckpoint, bool, error)
	RecordSummaryCompletionAttempt(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, response llm.Response, recordedAt time.Time) (publication.CompletionCheckpoint, error)
	SaveSummaryCompletion(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, checkpoint publication.CompletionCheckpoint) (publication.CompletionCheckpoint, error)
	RenewSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, leaseExpiresAt time.Time) error
	CompleteSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, completedAt time.Time) error
	ReleaseSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error
}
