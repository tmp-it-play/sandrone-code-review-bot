package reviewpublication

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) reconcileInvalidations(ctx context.Context) {
	for processed := 0; processed < r.config.InvalidationLimit; processed++ {
		now := r.deps.Clock.Now()
		invalidations, err := r.deps.Invalidations.ClaimPublicationInvalidations(ctx, now, now.Add(r.config.PublicationLease), 1)
		if err != nil {
			r.deps.Logger.Error("게시 무효화 재시도 대상을 claim하지 못했습니다", "error", err)
			return
		}
		for _, invalidation := range invalidations {
			r.reconcileInvalidation(ctx, invalidation)
		}
		if len(invalidations) == 0 {
			return
		}
	}
}

func (r *Reconciler) reconcileInvalidation(ctx context.Context, invalidation reviewworkflow.PublicationInvalidation) {
	now := r.deps.Clock.Now()
	if !invalidation.ExpiresAt.After(now) {
		return
	}
	target := pullrequest.Target{
		InstallationID: invalidation.InstallationID,
		Owner:          invalidation.Owner,
		Repository:     invalidation.Repository,
		Number:         invalidation.Number,
	}
	attemptDeadline := now.Add(r.config.ReconcileTimeout)
	if invalidation.ExpiresAt.Before(attemptDeadline) {
		attemptDeadline = invalidation.ExpiresAt
	}
	attemptContext, cancel := context.WithDeadline(ctx, attemptDeadline)
	var err error
	for _, marker := range invalidation.Markers() {
		var invalidateErr error
		if invalidation.UpsertsProgressComment(marker) {
			invalidateErr = r.replaceTerminalProgressComment(attemptContext, target, marker, invalidation.ReasonFor(marker))
		} else {
			invalidateErr = r.deps.Publisher.InvalidatePublication(attemptContext, target, marker, invalidation.ReasonFor(marker))
		}
		if invalidateErr != nil {
			err = errors.Join(err, invalidateErr)
		}
	}
	cancel()
	now = r.deps.Clock.Now()
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	if err == nil {
		if completeErr := r.deps.Invalidations.CompletePublicationInvalidation(storeContext, invalidation.ID, invalidation.LeaseToken, now); completeErr != nil {
			r.deps.Logger.Warn("게시 무효화 완료를 저장하지 못했습니다", "run", invalidation.RunID, "error", completeErr)
		}
		return
	}
	nextAttemptAt := now.Add(publicationInvalidationRetryDelay(invalidation.Attempts))
	uncertainUntil := now.Add(reviewworkflow.PublicationInvalidationFenceDelay)
	if uncertainUntil.After(nextAttemptAt) {
		nextAttemptAt = uncertainUntil
	}
	if nextAttemptAt.After(invalidation.ExpiresAt) {
		nextAttemptAt = invalidation.ExpiresAt
	}
	if retryErr := r.deps.Invalidations.RetryPublicationInvalidation(storeContext, invalidation.ID, invalidation.LeaseToken, now, nextAttemptAt, uncertainUntil, err.Error()); retryErr != nil {
		r.deps.Logger.Warn("게시 무효화 재시도를 저장하지 못했습니다", "run", invalidation.RunID, "error", errors.Join(err, retryErr))
		return
	}
	r.deps.Logger.Warn("게시 무효화에 실패해 재시도합니다", "run", invalidation.RunID, "next_attempt_at", nextAttemptAt, "error", err)
}
