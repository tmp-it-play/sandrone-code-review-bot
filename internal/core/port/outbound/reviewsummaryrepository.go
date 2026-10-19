package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/textrevision"
)

type ReviewSummaryRepository interface {
	ReviewSummary(context.Context, uint64, string) (reviewworkflow.SummaryCheckpoint, bool, error)
	SaveReviewSummary(context.Context, uint64, string, reviewworkflow.SummaryCheckpoint) error
	ReviewTextRevision(context.Context, uint64, string) (textrevision.Checkpoint, bool, error)
	SaveReviewTextRevision(context.Context, uint64, string, textrevision.Checkpoint) error
}
