package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type SummaryPublicationRepository interface {
	ClaimSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, orderKey string, observedAt time.Time, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error)
	RenewSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, leaseExpiresAt time.Time) error
	CompleteSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string, completedAt time.Time, expiresAt time.Time) error
	ReleaseSummaryPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error
}
