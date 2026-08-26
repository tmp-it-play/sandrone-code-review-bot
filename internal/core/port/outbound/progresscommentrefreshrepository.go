package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
)

type ProgressCommentRefreshRepository interface {
	EnsureProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh) (bool, error)
	ClaimProgressCommentRefreshes(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]progresscomment.Refresh, error)
	CompleteProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh, completedAt time.Time, nextRefreshAt time.Time) error
	RetryProgressCommentRefresh(ctx context.Context, refresh progresscomment.Refresh, failedAt time.Time, nextAttemptAt time.Time, uncertainUntil time.Time) error
}
