package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewWorkflowRepository interface {
	CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error)
	AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (string, error)
	ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error
	RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error
	ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error
	SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) ([]reviewworkflow.Unit, error)
	StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, startedAt time.Time, leaseExpiresAt time.Time) (string, error)
	FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, leaseToken string, result reviewworkflow.UnitResult) error
	ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error
	FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error)
	PublicationCandidateHighWatermark(ctx context.Context, before time.Time) (uint64, error)
	PublicationCandidates(ctx context.Context, before time.Time, afterID uint64, throughID uint64, limit int) ([]reviewworkflow.Run, error)
	ClaimPublicationInvalidations(ctx context.Context, claimedAt time.Time, leaseExpiresAt time.Time, limit int) ([]reviewworkflow.PublicationInvalidation, error)
	CompletePublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, resolvedAt time.Time) error
	RetryPublicationInvalidation(ctx context.Context, invalidationID uint64, leaseToken string, failedAt time.Time, nextAttemptAt time.Time, failure string) error
	ReconcileOrphans(ctx context.Context, staleBefore time.Time, terminalAt time.Time, expiresAt time.Time, limit int) (reviewworkflow.OrphanReconciliation, error)
	DeleteExpired(ctx context.Context, now time.Time, legacyCutoff time.Time, limit int) (reviewworkflow.CleanupResult, error)
}
