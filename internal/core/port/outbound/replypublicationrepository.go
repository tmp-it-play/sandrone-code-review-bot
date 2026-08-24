package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ReplyPublicationRepository interface {
	ClaimReplyPublication(ctx context.Context, target pullrequest.Target, operationKey string, commentID int64, inThread bool, claimedAt time.Time, leaseExpiresAt time.Time, expiresAt time.Time) (publication.Claim, error)
	ReserveReplyExternalCall(ctx context.Context, operationKey string, leaseToken string, limit int) (bool, error)
	ReplyCompletion(ctx context.Context, operationKey string, leaseToken string, inputHash string, currentAt time.Time) (publication.CompletionCheckpoint, bool, error)
	RecordReplyCompletionAttempt(ctx context.Context, operationKey string, leaseToken string, response llm.Response, recordedAt time.Time) (publication.CompletionCheckpoint, error)
	SaveReplyCompletion(ctx context.Context, operationKey string, leaseToken string, checkpoint publication.CompletionCheckpoint) (publication.CompletionCheckpoint, error)
	RenewReplyPublication(ctx context.Context, operationKey string, leaseToken string, leaseExpiresAt time.Time) error
	CompleteReplyPublication(ctx context.Context, operationKey string, leaseToken string, completedAt time.Time) error
	ReleaseReplyPublication(ctx context.Context, operationKey string, leaseToken string) error
}
