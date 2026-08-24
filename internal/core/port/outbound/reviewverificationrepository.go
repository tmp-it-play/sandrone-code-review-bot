package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewVerificationRepository interface {
	ReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, inputHash string) (reviewworkflow.VerificationCheckpoint, bool, error)
	RecordReviewVerificationAttempt(ctx context.Context, runID uint64, runLeaseToken string, inputHash string, response llm.Response, finishedAt time.Time) (reviewworkflow.VerificationCheckpoint, error)
	SaveReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, checkpoint reviewworkflow.VerificationCheckpoint) (reviewworkflow.VerificationCheckpoint, error)
}
