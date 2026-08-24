package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) finishUnitCheckpoint(ctx context.Context, runID uint64, runLease string, unitHash string, unitLease string, result reviewworkflow.UnitResult) error {
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	defer checkpointCancel()
	return u.deps.Workflows.FinishUnit(checkpointContext, runID, runLease, unitHash, unitLease, result)
}
