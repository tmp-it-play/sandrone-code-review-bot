package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type FindingRepository interface {
	SaveAll(ctx context.Context, reviewID uint64, target pullrequest.Target, findings []review.Finding) error
	Fingerprints(ctx context.Context, target pullrequest.Target) (map[string]struct{}, error)
	ByReview(ctx context.Context, reviewID uint64) ([]review.Finding, error)
}
