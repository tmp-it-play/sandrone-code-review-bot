package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
)

type ProgressAdmission interface {
	RecoverReviewProgress(ctx context.Context, task job.ReviewJob) error
}
