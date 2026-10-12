package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewSummaryRepository interface {
	ReviewSummary(context.Context, uint64, string) (reviewworkflow.SummaryCheckpoint, bool, error)
	SaveReviewSummary(context.Context, uint64, string, reviewworkflow.SummaryCheckpoint) error
}
