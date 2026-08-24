package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewRepository interface {
	Save(ctx context.Context, record review.Record) (uint64, error)
	SaveWithFindings(ctx context.Context, record review.Record, target pullrequest.Target, findings []review.Finding, lifecycleExpiresAt time.Time) (uint64, error)
	Recent(ctx context.Context, limit int) ([]review.Record, error)
	ByID(ctx context.Context, id uint64) (review.Record, error)
	ByRunID(ctx context.Context, runID uint64) (review.Record, bool, error)
}
