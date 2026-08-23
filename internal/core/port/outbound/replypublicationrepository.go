package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ReplyPublicationRepository interface {
	ClaimReplyPublication(ctx context.Context, target pullrequest.Target, operationKey string, commentID int64, inThread bool, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error)
	RenewReplyPublication(ctx context.Context, operationKey string, leaseToken string, leaseExpiresAt time.Time) error
	CompleteReplyPublication(ctx context.Context, operationKey string, leaseToken string, completedAt time.Time, expiresAt time.Time) error
	ReleaseReplyPublication(ctx context.Context, operationKey string, leaseToken string) error
}
