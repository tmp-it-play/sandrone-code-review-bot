package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) finishWorkflow(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResult(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, "", advanceWatermark, nil)
}

func (u *UseCase) finishWorkflowSupersededByHeadWithInvalidation(ctx context.Context, runID uint64, leaseToken string, supersedingHeadSHA string, detail string, invalidation *reviewworkflow.PublicationInvalidation) (reviewworkflow.RunStatus, error) {
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), reactionTimeout)
	defer cancel()
	terminalAt := u.deps.Clock.Now()
	return u.deps.Runs.FinishRun(finishContext, runID, reviewworkflow.RunResult{
		Status:                  reviewworkflow.RunStatusSuperseded,
		ReviewOutcome:           review.OutcomeSuperseded,
		Error:                   detail,
		SupersedingHeadSHA:      supersedingHeadSHA,
		TerminalAt:              terminalAt,
		ExpiresAt:               terminalAt.Add(u.deps.Retention),
		LeaseToken:              leaseToken,
		PublicationInvalidation: invalidation,
	})
}

func (u *UseCase) finishWorkflowWithOutcome(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResult(ctx, runID, leaseToken, status, outcome, detail, "", advanceWatermark, nil)
}

func (u *UseCase) finishWorkflowWithProgress(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool, progress *progressSession) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResultWithProgress(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, "", advanceWatermark, progress)
}

func (u *UseCase) finishWorkflowSupersededByHeadWithProgress(ctx context.Context, runID uint64, leaseToken string, supersedingHeadSHA string, detail string, progress *progressSession) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResultWithProgress(ctx, runID, leaseToken, reviewworkflow.RunStatusSuperseded, review.OutcomeSuperseded, detail, supersedingHeadSHA, false, progress)
}

func (u *UseCase) finishWorkflowWithOutcomeAndProgress(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool, progress *progressSession) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowResultWithProgress(ctx, runID, leaseToken, status, outcome, detail, "", advanceWatermark, progress)
}

func (u *UseCase) finishWorkflowResultWithProgress(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, supersedingHeadSHA string, advanceWatermark bool, progress *progressSession) (reviewworkflow.RunStatus, error) {
	progress.Stop()
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), reactionTimeout)
	defer cancel()
	return u.finishWorkflowResult(finishContext, runID, leaseToken, status, outcome, detail, supersedingHeadSHA, advanceWatermark, progress.TerminalFinalization())
}

func (u *UseCase) finishWorkflowResult(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, supersedingHeadSHA string, advanceWatermark bool, terminalProgress *reviewworkflow.TerminalProgressFinalization) (reviewworkflow.RunStatus, error) {
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
		TerminalProgress:   terminalProgress,
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
