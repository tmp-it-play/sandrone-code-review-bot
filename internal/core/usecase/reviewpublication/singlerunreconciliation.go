package reviewpublication

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) reconcileRun(ctx context.Context, run reviewworkflow.Run) error {
	target := pullrequest.Target{
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		BaseSHA:        run.BaseSHA,
		HeadSHA:        run.HeadSHA,
	}
	now := r.deps.Clock.Now()
	lease, err := r.deps.Runs.AcquireRun(ctx, run.ID, now, now.Add(r.config.RunLease))
	if errors.Is(err, reviewworkflow.ErrRunLeased) || errors.Is(err, reviewworkflow.ErrPublicationLeased) || errors.Is(err, reviewworkflow.ErrProgressCommentLeased) {
		return nil
	}
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		invalidation := r.publicationInvalidation(run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다", nil)
		return r.finishSuperseded(ctx, run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다", invalidation)
	}
	if err != nil {
		return err
	}
	leaseToken := lease.Token
	if leaseToken == "" {
		return nil
	}
	defer r.releaseRun(ctx, run.ID, leaseToken)
	claimedAt := r.deps.Clock.Now()
	if err := r.deps.Publications.ClaimPublication(ctx, run.ID, leaseToken, claimedAt, claimedAt.Add(r.config.PublicationLease)); err != nil {
		return err
	}
	current, err := r.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	currentTarget, currentTargetMatches, currentTargetErr := r.rebindNoopHeadChange(ctx, target, current)
	if currentTargetErr != nil {
		return r.resolveUnverified(ctx, run, leaseToken, currentTargetErr)
	}
	if !currentTargetMatches {
		invalidation := r.publicationInvalidation(run, "게시 조정 전에 base 또는 head가 변경되었습니다", nil)
		return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "게시 조정 전에 base 또는 head가 변경되었습니다", changedHeadSHA(run.HeadSHA, current.HeadSHA), false, invalidation)
	}
	target = currentTarget
	marker := reviewworkflow.PublicationMarker(run.Key)
	storedPublication, storedPublicationFound, err := r.deps.Publications.ReviewPublication(ctx, run.ID, leaseToken)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if storedPublicationFound && storedPublication.Marker != marker {
		return r.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("저장된 리뷰 게시 marker가 실행과 일치하지 않습니다"))
	}
	published := false
	if !storedPublicationFound {
		published, err = r.deps.Publisher.PublicationExists(ctx, target, marker)
		if err != nil {
			return r.resolveUnverified(ctx, run, leaseToken, err)
		}
		if !published {
			if err := r.deps.Runs.ResumeRun(ctx, run.ID, leaseToken, r.deps.Clock.Now()); err != nil {
				return err
			}
			r.deps.Logger.Info("게시 payload가 없어 리뷰 실행 상태를 복구했습니다", "run", run.ID, "target", target.Reference())
			return nil
		}
	}
	_, found, err := r.deps.Reviews.ByRunID(ctx, run.ID)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if !found {
		return r.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("게시 marker는 있지만 내부 리뷰 이력이 없습니다"))
	}
	if storedPublicationFound && storedPublication.PayloadHash != "" {
		if err := r.publishPreparedPublication(ctx, target, run.ID, leaseToken, storedPublication); errors.Is(err, publication.ErrTargetChanged) {
			detail := "저장된 리뷰 게시 payload 재개 직전에 base 또는 head가 변경되었습니다"
			invalidation := r.publicationInvalidation(run, detail, nil)
			return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, detail, r.resolveSupersedingHeadSHA(ctx, target), false, invalidation)
		} else if err != nil {
			return r.resolveUnverified(ctx, run, leaseToken, err)
		}
	} else if published {
		expiresAt := run.StartedAt.Add(r.config.Retention)
		if err := r.completeReviewPublication(ctx, run.ID, leaseToken, marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, expiresAt); err != nil {
			return err
		}
	}
	confirmed, err := r.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	_, confirmedTargetMatches, confirmedTargetErr := r.rebindNoopHeadChange(ctx, target, confirmed)
	if confirmedTargetErr != nil {
		return r.resolveUnverified(ctx, run, leaseToken, confirmedTargetErr)
	}
	if !confirmedTargetMatches {
		invalidation := r.publicationInvalidation(run, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다", nil)
		return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다", changedHeadSHA(run.HeadSHA, confirmed.HeadSHA), false, invalidation)
	}
	finalization := reviewworkflow.ReviewPublicationFinalization{
		Status:           reviewworkflow.RunStatusPartial,
		Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
		AdvanceWatermark: false,
	}
	if storedPublicationFound && storedPublication.PayloadHash != "" {
		finalization = storedPublication.Finalization
	}
	return r.finishWithLease(ctx, run, leaseToken, finalization.Status, finalization.Detail, "", finalization.AdvanceWatermark, nil)
}

func (r *Reconciler) resolveSupersedingHeadSHA(ctx context.Context, target pullrequest.Target) string {
	current, err := r.deps.Source.PullRequest(ctx, target)
	if err != nil {
		r.deps.Logger.Warn("대체 head를 확인하지 못했습니다", "target", target.Reference(), "error", err)
		return ""
	}
	return changedHeadSHA(target.HeadSHA, current.HeadSHA)
}

func changedHeadSHA(previous string, current string) string {
	if current == "" || current == previous {
		return ""
	}
	return current
}
