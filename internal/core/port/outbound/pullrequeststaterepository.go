package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PullRequestStateRepository interface {
	LastReviewedSHA(ctx context.Context, target pullrequest.Target) (string, error)
	SetLastReviewedSHA(ctx context.Context, target pullrequest.Target, sha string) error
}
