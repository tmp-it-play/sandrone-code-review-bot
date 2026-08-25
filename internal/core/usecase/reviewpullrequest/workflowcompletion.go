package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) finishWorkflow(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResult(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, "", advanceWatermark)
}

func (u *UseCase) finishWorkflowSupersededByHead(ctx context.Context, runID uint64, leaseToken string, supersedingHeadSHA string, detail string) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResult(ctx, runID, leaseToken, reviewworkflow.RunStatusSuperseded, review.OutcomeSuperseded, detail, supersedingHeadSHA, false)
}

func (u *UseCase) finishWorkflowWithOutcome(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResult(ctx, runID, leaseToken, status, outcome, detail, "", advanceWatermark)
}

func (u *UseCase) finishWorkflowResult(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, supersedingHeadSHA string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	terminalAt := u.deps.Clock.Now()
	return u.deps.Runs.FinishRun(ctx, runID, reviewworkflow.RunResult{
		Status:             status,
		ReviewOutcome:      outcome,
		Error:              detail,
		SupersedingHeadSHA: supersedingHeadSHA,
		TerminalAt:         terminalAt,
		ExpiresAt:          terminalAt.Add(u.deps.Retention),
		AdvanceWatermark:   advanceWatermark,
		LeaseToken:         leaseToken,
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

func changedHeadSHA(previous string, current string) string {
	if current == "" || current == previous {
		return ""
	}
	return current
}
