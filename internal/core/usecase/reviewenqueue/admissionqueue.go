package reviewenqueue

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
)

type AdmissionQueue interface {
	EnqueueReviewAdmission(ctx context.Context, payload job.ReviewJob) (job.ReviewAdmission, error)
	EnqueueSummary(ctx context.Context, payload job.SummaryJob) error
	EnqueueReply(ctx context.Context, payload job.ReplyJob) error
}
