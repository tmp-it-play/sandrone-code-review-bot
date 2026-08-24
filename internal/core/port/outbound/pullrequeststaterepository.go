package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PullRequestStateRepository interface {
	LastReviewedSHA(ctx context.Context, target pullrequest.Target, activeAfter time.Time) (string, error)
	SetLastReviewedSHA(ctx context.Context, target pullrequest.Target, sha string) error
}
