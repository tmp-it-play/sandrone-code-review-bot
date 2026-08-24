package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) finishWorkflow(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowWithOutcome(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, advanceWatermark)
}

func (u *UseCase) finishWorkflowWithOutcome(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	terminalAt := u.deps.Clock.Now()
	return u.deps.Workflows.FinishRun(ctx, runID, reviewworkflow.RunResult{
		Status:           status,
		ReviewOutcome:    outcome,
		Error:            detail,
		TerminalAt:       terminalAt,
		ExpiresAt:        terminalAt.Add(u.deps.Retention),
		AdvanceWatermark: advanceWatermark,
		LeaseToken:       leaseToken,
	})
}

func reviewOutcomeOf(status reviewworkflow.RunStatus) review.Outcome {
	switch status {
	case reviewworkflow.RunStatusComplete:
		return review.OutcomeSucceeded
	case reviewworkflow.RunStatusPartial:
		return review.OutcomePartial
	case reviewworkflow.RunStatusSuperseded:
		return review.OutcomeSuperseded
	case reviewworkflow.RunStatusSkipped:
		return review.OutcomeSkipped
	default:
		return review.OutcomeUnavailable
	}
}
