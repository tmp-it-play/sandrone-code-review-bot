package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewRunLifecycleRepository interface {
	AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.RunLease, error)
	ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error
	ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error
	FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error)
}
