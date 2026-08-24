package reviewpullrequest

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type reviewUnitWorkflowRepository interface {
	SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) ([]reviewworkflow.Unit, error)
	StartUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, inputHash string, startedAt time.Time, leaseExpiresAt time.Time) (reviewworkflow.UnitClaim, error)
	FinishUnit(ctx context.Context, runID uint64, runLeaseToken string, unitHash string, unitLeaseToken string, result reviewworkflow.UnitResult) error
}
