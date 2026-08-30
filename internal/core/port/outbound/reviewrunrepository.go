package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewRunRepository interface {
	ReviewRunLifecycleRepository
	ProgressCommentRefreshRepository
	CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error)
	FinishDetachedProgressFailure(ctx context.Context, run reviewworkflow.Run, result reviewworkflow.RunResult) (uint64, reviewworkflow.RunStatus, error)
	LatestRunAnchor(ctx context.Context, target pullrequest.Target) (reviewworkflow.RunAnchor, bool, error)
	HasRegisteredReviewContinuation(ctx context.Context, run reviewworkflow.Run, activityBoundary time.Time) (bool, error)
	OwnsProgressMarker(ctx context.Context, runID uint64, marker string) (bool, error)
	ClaimProgressCommentMutation(ctx context.Context, runID uint64, marker string, claimedAt time.Time, leaseExpiresAt time.Time) (string, bool, error)
	CompleteProgressCommentMutation(ctx context.Context, marker string, leaseToken string, completedAt time.Time) error
	FenceProgressCommentMutation(ctx context.Context, marker string, leaseToken string, failedAt time.Time, uncertainUntil time.Time) error
	RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error
}
