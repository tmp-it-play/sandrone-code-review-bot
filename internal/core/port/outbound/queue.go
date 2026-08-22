package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
)

type Queue interface {
	EnqueueReview(ctx context.Context, payload job.ReviewJob) error
	EnqueueSummary(ctx context.Context, payload job.SummaryJob) error
	EnqueueReply(ctx context.Context, payload job.ReplyJob) error
}
