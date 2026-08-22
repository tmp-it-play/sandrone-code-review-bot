package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewRepository interface {
	Save(ctx context.Context, record review.Record) (uint64, error)
	Recent(ctx context.Context, limit int) ([]review.Record, error)
	ByID(ctx context.Context, id uint64) (review.Record, error)
}
