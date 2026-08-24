package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type FindingRepository interface {
	Fingerprints(ctx context.Context, target pullrequest.Target) (map[string]struct{}, error)
	OpenOccurrences(ctx context.Context, target pullrequest.Target, paths []string, limit int) ([]review.OpenOccurrence, error)
	RevalidateOccurrences(ctx context.Context, target pullrequest.Target, revalidations []review.OccurrenceRevalidation) error
	ByReview(ctx context.Context, reviewID uint64) ([]review.Finding, error)
}
