package reviewpublication

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) resolveUnverified(ctx context.Context, run reviewworkflow.Run, leaseToken string, cause error) error {
	if run.HeartbeatAt.After(r.deps.Clock.Now().Add(-r.config.OrphanAfter)) {
		return cause
	}
	detail := "7일 동안 GitHub 게시 결과를 확정하지 못했습니다"
	invalidation := r.deferredInvalidation(run, invalidationBody(detail), cause)
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := r.finishWithLease(finishContext, run, leaseToken, reviewworkflow.RunStatusFailed, detail, "", false, invalidation); err != nil {
		return errors.Join(cause, err)
	}
	r.deps.Logger.Warn("GitHub 게시 결과를 확정하지 못해 리뷰 실행을 종료했습니다", "run", run.ID, "error", cause)
	return nil
}

func (r *Reconciler) releaseRun(ctx context.Context, runID uint64, leaseToken string) {
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := r.deps.Runs.ReleaseRun(releaseContext, runID, leaseToken, r.deps.Clock.Now()); err != nil {
		r.deps.Logger.Warn("게시 조정 run lease를 해제하지 못했습니다", "run", runID, "error", err)
	}
}

func (r *Reconciler) finishSuperseded(ctx context.Context, run reviewworkflow.Run, detail string, invalidation *reviewworkflow.PublicationInvalidation) error {
	now := r.deps.Clock.Now()
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := r.deps.Runs.FinishRun(finishContext, run.ID, reviewworkflow.RunResult{
		Status:                  reviewworkflow.RunStatusSuperseded,
		Error:                   detail,
		TerminalAt:              now,
		ExpiresAt:               now.Add(r.config.Retention),
		PublicationInvalidation: invalidation,
	})
	return err
}

func (r *Reconciler) finishWithLease(ctx context.Context, run reviewworkflow.Run, leaseToken string, status reviewworkflow.RunStatus, detail string, supersedingHeadSHA string, advanceWatermark bool, invalidation *reviewworkflow.PublicationInvalidation) error {
	now := r.deps.Clock.Now()
	finishContext, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	_, err := r.deps.Runs.FinishRun(finishContext, run.ID, reviewworkflow.RunResult{
		Status:                  status,
		Error:                   detail,
		SupersedingHeadSHA:      supersedingHeadSHA,
		TerminalAt:              now,
		ExpiresAt:               now.Add(r.config.Retention),
		AdvanceWatermark:        advanceWatermark,
		LeaseToken:              leaseToken,
		PublicationInvalidation: invalidation,
	})
	finishCancel()
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) && status != reviewworkflow.RunStatusSuperseded {
		target := pullrequest.Target{
			InstallationID: run.InstallationID,
			Owner:          run.Owner,
			Repository:     run.Repository,
			Number:         run.Number,
			BaseSHA:        run.BaseSHA,
			HeadSHA:        run.HeadSHA,
		}
		detail = "게시 결과 확정 중 더 최신인 리뷰 실행이 확인되었습니다"
		invalidation, invalidateErr := r.invalidatePublication(ctx, target, run, detail)
		if invalidateErr != nil {
			return invalidateErr
		}
		finalizeContext, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, err = r.deps.Runs.FinishRun(finalizeContext, run.ID, reviewworkflow.RunResult{
			Status:                  reviewworkflow.RunStatusSuperseded,
			Error:                   detail,
			TerminalAt:              now,
			ExpiresAt:               now.Add(r.config.Retention),
			LeaseToken:              leaseToken,
			PublicationInvalidation: invalidation,
		})
		finalizeCancel()
	}
	return err
}
