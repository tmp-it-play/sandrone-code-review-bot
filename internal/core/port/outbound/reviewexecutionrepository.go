package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewExecutionRepository interface {
	ReserveExternalCall(ctx context.Context, reservation reviewworkflow.ExternalCallReservation) (bool, error)
	SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) (reviewworkflow.PlanSnapshot, error)
	SplitUnit(ctx context.Context, runID uint64, runLeaseToken string, split reviewworkflow.UnitSplit) ([]reviewworkflow.Unit, error)
	StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, inputHash string, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.UnitClaim, error)
	FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, leaseToken string, result reviewworkflow.UnitResult) error
}
