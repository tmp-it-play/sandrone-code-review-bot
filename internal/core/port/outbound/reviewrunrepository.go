package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewRunRepository interface {
	ReviewRunLifecycleRepository
	CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error)
	HasRegisteredReviewContinuation(ctx context.Context, run reviewworkflow.Run, activityBoundary time.Time) (bool, error)
	ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error
	RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error
}
