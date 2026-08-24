package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewWorkflowRepository interface {
	CreateOrGetRun(ctx context.Context, run reviewworkflow.Run) (reviewworkflow.Run, error)
	AcquireRun(ctx context.Context, runID uint64, startedAt time.Time, leaseExpiresAt time.Time) (string, error)
	ResumeRun(ctx context.Context, runID uint64, leaseToken string, resumedAt time.Time) error
	RenewRun(ctx context.Context, runID uint64, leaseToken string, heartbeatAt time.Time, leaseExpiresAt time.Time) error
	ReleaseRun(ctx context.Context, runID uint64, leaseToken string, releasedAt time.Time) error
	ReserveExternalCall(ctx context.Context, runID uint64, runLeaseToken string, limit int) (bool, error)
	SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) ([]reviewworkflow.Unit, error)
	StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, inputHash string, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.UnitClaim, error)
	FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, leaseToken string, result reviewworkflow.UnitResult) error
	ReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, inputHash string) (reviewworkflow.VerificationCheckpoint, bool, error)
	RecordReviewVerificationAttempt(ctx context.Context, runID uint64, runLeaseToken string, inputHash string, response llm.Response, finishedAt time.Time) (reviewworkflow.VerificationCheckpoint, error)
	SaveReviewVerification(ctx context.Context, runID uint64, runLeaseToken string, checkpoint reviewworkflow.VerificationCheckpoint) (reviewworkflow.VerificationCheckpoint, error)
	ClaimPublication(ctx context.Context, runID uint64, runLeaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) error
	ReviewPublication(ctx context.Context, runID uint64, runLeaseToken string) (reviewworkflow.ReviewPublication, bool, error)
	PrepareReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, payload reviewworkflow.ReviewPublicationPayload, finalization reviewworkflow.ReviewPublicationFinalization, preparedAt time.Time, expiresAt time.Time) (reviewworkflow.ReviewPublication, error)
	CompleteReviewPublication(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, completedAt time.Time, expiresAt time.Time) error
	FinishRun(ctx context.Context, runID uint64, result reviewworkflow.RunResult) (reviewworkflow.RunStatus, error)
}
