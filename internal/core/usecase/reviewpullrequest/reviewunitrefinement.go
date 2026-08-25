package reviewpullrequest

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (e *reviewUnitExecutor) refineUnit(ctx context.Context, input reviewUnitExecution, planner *reviewUnitWorkPlanner, work reviewUnitWork, unitLease string, inputHash string, parentResult reviewworkflow.UnitResult, refinedAt time.Time) ([]reviewUnitWork, bool, error) {
	refinement, available, err := planner.Split(work, input.planner.RoutePolicy)
	if err != nil || !available {
		return nil, available, err
	}
	parentResult.Status = reviewworkflow.UnitStatusFailed
	parentResult.InputHash = inputHash
	parentResult.FinishedAt = refinedAt
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	defer checkpointCancel()
	stored, err := e.deps.execution.SplitUnit(checkpointContext, input.runID, input.runLease, reviewworkflow.UnitSplit{
		ParentHash:       work.unit.Hash,
		ParentLeaseToken: unitLease,
		ParentInputHash:  inputHash,
		ParentResult:     parentResult,
		Children:         refinement.children,
		Assignments:      refinement.assignments,
		RefinedAt:        refinedAt,
	})
	if err != nil {
		return nil, true, err
	}
	planner.ApplySplit(refinement)
	childWorks, err := planner.Build(stored)
	if err != nil {
		return nil, true, err
	}
	return childWorks, true, nil
}

func reviewUnitRefinementFitsBudget(queued []reviewUnitWork, externalCalls int, reviewerCallLimit int, reservedRetryCalls int) bool {
	minimumCalls := 2 + reservedRetryCalls
	for _, work := range queued {
		if work.reservesCall() {
			minimumCalls++
		}
	}
	return externalCalls+minimumCalls <= reviewerCallLimit
}

func prependReviewUnitWorks(current []reviewUnitWork, added []reviewUnitWork) []reviewUnitWork {
	combined := make([]reviewUnitWork, 0, len(added)+len(current))
	combined = append(combined, added...)
	combined = append(combined, current...)
	return combined
}
