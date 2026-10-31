package reviewpullrequest

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (e *reviewUnitExecutor) finishPrefix(ctx context.Context, input reviewUnitExecution, planner *reviewUnitWorkPlanner, work reviewUnitWork, count int, lease string, result reviewworkflow.UnitResult) ([]reviewUnitWork, string, error) {
	split, err := planner.splitGroups(work, work.files[:count], work.files[count:])
	if err != nil {
		return nil, "", fmt.Errorf("검토한 범위를 나눌 수 없습니다: %w", err)
	}
	parentHash := result.InputHash
	prefix := split.children[0].Hash
	result.InputHash, err = hashJSON([]string{"review-prefix-v1", parentHash, prefix})
	if err != nil {
		return nil, "", err
	}
	checkpoint, cancel := durableCheckpointContext(ctx)
	defer cancel()
	stored, err := e.deps.execution.SplitUnit(checkpoint, input.runID, input.runLease, reviewworkflow.UnitSplit{
		ParentHash: work.unit.Hash, ParentLeaseToken: lease, ParentInputHash: parentHash,
		ParentResult:    reviewworkflow.UnitResult{Status: reviewworkflow.UnitStatusSplit, InputHash: parentHash, FinishedAt: result.FinishedAt},
		CompletedPrefix: &result, Children: split.children, Assignments: split.assignments, RefinedAt: result.FinishedAt,
	})
	if err != nil {
		return nil, "", err
	}
	planner.ApplySplit(split)
	pending := make([]reviewworkflow.Unit, 0, len(stored))
	found := false
	for _, child := range stored {
		if child.Hash != prefix {
			pending = append(pending, child)
		} else if child.Status != reviewworkflow.UnitStatusSucceeded || child.InputHash != result.InputHash {
			return nil, "", reviewworkflow.ErrPlanChanged
		} else {
			found = true
		}
	}
	if !found || len(pending) == 0 {
		return nil, "", reviewworkflow.ErrPlanChanged
	}
	rest, err := planner.Build(pending)
	return rest, prefix, err
}
