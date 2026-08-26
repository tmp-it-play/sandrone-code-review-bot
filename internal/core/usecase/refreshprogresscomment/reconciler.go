package refreshprogresscomment

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const progressRefreshStoreTimeout = 10 * time.Second

type Reconciler struct {
	deps   Dependencies
	config Config
}

func New(deps Dependencies, config Config) *Reconciler {
	return &Reconciler{deps: deps, config: config}
}

func (r *Reconciler) Reconcile(ctx context.Context) {
	now := r.deps.Clock.Now()
	refreshes, err := r.deps.Refreshes.ClaimProgressCommentRefreshes(ctx, now, now.Add(r.config.Lease), r.config.Limit)
	if err != nil {
		r.deps.Logger.Error("진행 코멘트 갱신 대상을 claim하지 못했습니다", "error", err)
		return
	}
	concurrency := r.config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(refreshes) {
		concurrency = len(refreshes)
	}
	if concurrency == 0 {
		return
	}
	semaphore := make(chan struct{}, concurrency)
	var group sync.WaitGroup
	for _, current := range refreshes {
		refresh := current
		group.Add(1)
		go func() {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			r.refresh(ctx, refresh)
		}()
	}
	group.Wait()
}

func (r *Reconciler) refresh(ctx context.Context, refresh progresscomment.Refresh) {
	now := r.deps.Clock.Now()
	deadline := now.Add(r.config.Timeout)
	leaseDeadline := refresh.LeaseExpiresAt.Add(-reviewworkflow.PublicationInvalidationFenceDelay - progressRefreshStoreTimeout)
	if !refresh.LeaseExpiresAt.IsZero() && leaseDeadline.Before(deadline) {
		deadline = leaseDeadline
	}
	if refresh.ExpiresAt.Before(deadline) {
		deadline = refresh.ExpiresAt
	}
	attemptContext, cancel := context.WithDeadline(ctx, deadline)
	mutationAttempted, err := r.apply(attemptContext, refresh, now)
	cancel()
	storedAt := r.deps.Clock.Now()
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), progressRefreshStoreTimeout)
	defer storeCancel()
	if err != nil {
		nextAttemptAt := progressRefreshRetryAt(err, storedAt)
		uncertainUntil := time.Time{}
		if mutationAttempted {
			uncertainUntil = storedAt.Add(reviewworkflow.PublicationInvalidationFenceDelay)
			if uncertainUntil.After(nextAttemptAt) {
				nextAttemptAt = uncertainUntil
			}
		}
		if !refresh.ExpiresAt.After(nextAttemptAt) {
			nextAttemptAt = time.Time{}
		}
		if retryErr := r.deps.Refreshes.RetryProgressCommentRefresh(storeContext, refresh, storedAt, nextAttemptAt, uncertainUntil); retryErr != nil {
			r.deps.Logger.Warn("진행 코멘트 갱신 재시도를 저장하지 못했습니다", "marker", refresh.Marker, "error", errors.Join(err, retryErr))
			return
		}
		r.deps.Logger.Warn("진행 코멘트 갱신에 실패해 재시도합니다", "marker", refresh.Marker, "error", err)
		return
	}
	nextRefreshAt := storedAt.Add(progresscomment.RefreshInterval)
	if !refresh.ExpiresAt.After(nextRefreshAt) {
		nextRefreshAt = time.Time{}
	}
	if err := r.deps.Refreshes.CompleteProgressCommentRefresh(storeContext, refresh, storedAt, nextRefreshAt); err != nil {
		r.deps.Logger.Warn("진행 코멘트 갱신 완료를 저장하지 못했습니다", "marker", refresh.Marker, "error", err)
	}
}

func (r *Reconciler) apply(ctx context.Context, refresh progresscomment.Refresh, now time.Time) (bool, error) {
	key, valid := reviewworkflow.ProgressMarkerKey(refresh.Marker)
	if !valid {
		return false, errors.New("진행 코멘트 갱신 marker가 올바르지 않습니다")
	}
	body := r.deps.Renderer.ProgressBody(review.ProgressMessage(refresh.MessageTheme, key, refresh.Sequence), refresh.Marker)
	commentID, exists, err := r.deps.Publisher.FindComment(ctx, refresh.Target, refresh.Marker)
	if err != nil {
		return false, err
	}
	if exists {
		return true, r.deps.Publisher.UpdateComment(ctx, refresh.Target, commentID, body)
	}
	if now.Before(refresh.CreateNotBefore) {
		return false, nil
	}
	_, createErr := r.deps.Publisher.CreateComment(ctx, refresh.Target, body)
	if createErr == nil {
		return true, nil
	}
	_, reconciled, reconcileErr := r.deps.Publisher.FindComment(ctx, refresh.Target, refresh.Marker)
	if reconciled {
		return true, createErr
	}
	return true, errors.Join(createErr, reconcileErr)
}

func progressRefreshRetryAt(err error, now time.Time) time.Time {
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(err, &scheduled) && scheduled.RetryAt().After(now) {
		return scheduled.RetryAt()
	}
	return now.Add(progresscomment.RefreshInterval)
}
