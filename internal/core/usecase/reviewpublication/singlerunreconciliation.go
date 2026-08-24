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
	leaseToken, err := r.deps.Runs.AcquireRun(ctx, run.ID, now, now.Add(r.config.RunLease))
	if errors.Is(err, reviewworkflow.ErrRunLeased) || errors.Is(err, reviewworkflow.ErrPublicationLeased) {
		return nil
	}
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		invalidation, invalidateErr := r.invalidatePublication(ctx, target, run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다")
		if invalidateErr != nil {
			return invalidateErr
		}
		return r.finishSuperseded(ctx, run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다", invalidation)
	}
	if err != nil {
		return err
	}
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
	if current.BaseSHA != run.BaseSHA || current.HeadSHA != run.HeadSHA {
		invalidation, err := r.invalidatePublication(ctx, target, run, "게시 조정 전에 base 또는 head가 변경되었습니다")
		if err != nil {
			return err
		}
		return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "게시 조정 전에 base 또는 head가 변경되었습니다", false, invalidation)
	}
	marker := reviewworkflow.PublicationMarker(run.Key)
	storedPublication, storedPublicationFound, err := r.deps.Publications.ReviewPublication(ctx, run.ID, leaseToken)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if storedPublicationFound && storedPublication.Marker != marker {
		return r.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("저장된 리뷰 게시 marker가 실행과 일치하지 않습니다"))
	}
	published, err := r.deps.Publisher.PublicationExists(ctx, target, marker)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if !published && !storedPublicationFound {
		if run.HeartbeatAt.After(r.deps.Clock.Now().Add(-r.config.OrphanAfter)) {
			return nil
		}
		return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusFailed, "7일 동안 GitHub 게시 marker를 확인하지 못했습니다", false, nil)
	}
	_, found, err := r.deps.Reviews.ByRunID(ctx, run.ID)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if !found {
		return r.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("게시 marker는 있지만 내부 리뷰 이력이 없습니다"))
	}
	if published {
		expiresAt := run.StartedAt.Add(r.config.Retention)
		if storedPublicationFound {
			expiresAt = storedPublication.ExpiresAt
		}
		if err := r.completeReviewPublication(ctx, run.ID, leaseToken, marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, expiresAt); err != nil {
			return err
		}
	} else if storedPublication.Status == reviewworkflow.ReviewPublicationStatusPrepared {
		if err := r.publishPreparedPublication(ctx, target, run.ID, leaseToken, storedPublication); errors.Is(err, publication.ErrTargetChanged) {
			return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "저장된 리뷰 게시 payload 재개 직전에 base 또는 head가 변경되었습니다", false, nil)
		} else if err != nil {
			return r.resolveUnverified(ctx, run, leaseToken, err)
		}
	}
	confirmed, err := r.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return r.resolveUnverified(ctx, run, leaseToken, err)
	}
	if confirmed.BaseSHA != run.BaseSHA || confirmed.HeadSHA != run.HeadSHA {
		invalidation, err := r.invalidatePublication(ctx, target, run, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다")
		if err != nil {
			return err
		}
		return r.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다", false, invalidation)
	}
	finalization := reviewworkflow.ReviewPublicationFinalization{
		Status:           reviewworkflow.RunStatusPartial,
		Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
		AdvanceWatermark: false,
	}
	if storedPublicationFound && storedPublication.PayloadHash != "" {
		finalization = storedPublication.Finalization
	}
	return r.finishWithLease(ctx, run, leaseToken, finalization.Status, finalization.Detail, finalization.AdvanceWatermark, nil)
}
