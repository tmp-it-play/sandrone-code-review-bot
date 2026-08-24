package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationWorker struct {
	workflows         outbound.ReviewWorkflowRepository
	source            outbound.PullRequestSource
	publisher         outbound.ReviewPublisher
	reviews           outbound.ReviewRepository
	clock             outbound.Clock
	logger            *slog.Logger
	retention         time.Duration
	interval          time.Duration
	runLease          time.Duration
	publicationLease  time.Duration
	orphanAfter       time.Duration
	reconcileTimeout  time.Duration
	batchSize         int
	invalidationLimit int
	afterID           uint64
	throughID         uint64
}

func NewReviewPublicationWorker(workflows outbound.ReviewWorkflowRepository, source outbound.PullRequestSource, publisher outbound.ReviewPublisher, reviews outbound.ReviewRepository, clock outbound.Clock, logger *slog.Logger, retention time.Duration) *ReviewPublicationWorker {
	return &ReviewPublicationWorker{
		workflows:         workflows,
		source:            source,
		publisher:         publisher,
		reviews:           reviews,
		clock:             clock,
		logger:            logger,
		retention:         retention,
		interval:          10 * time.Minute,
		runLease:          20 * time.Minute,
		publicationLease:  5 * time.Minute,
		orphanAfter:       7 * 24 * time.Hour,
		reconcileTimeout:  90 * time.Second,
		batchSize:         10,
		invalidationLimit: 10,
	}
}

func (w *ReviewPublicationWorker) Run(ctx context.Context) {
	w.reconcile(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

func (w *ReviewPublicationWorker) reconcile(ctx context.Context) {
	w.reconcileInvalidations(ctx)
	w.reconcileRuns(ctx)
}

func (w *ReviewPublicationWorker) reconcileRuns(ctx context.Context) {
	now := w.clock.Now()
	if w.throughID == 0 {
		highWatermark, err := w.workflows.PublicationCandidateHighWatermark(ctx, now)
		if err != nil {
			w.logger.Error("리뷰 게시 결과 조정 범위를 읽지 못했습니다", "error", err)
			return
		}
		if highWatermark == 0 {
			w.afterID = 0
			return
		}
		w.throughID = highWatermark
	}
	runs, err := w.workflows.PublicationCandidates(ctx, now, w.afterID, w.throughID, w.batchSize)
	if err != nil {
		w.logger.Error("리뷰 게시 결과 조정 대상을 읽지 못했습니다", "error", err)
		return
	}
	for _, run := range runs {
		w.afterID = run.ID
		reconcileContext, cancel := context.WithTimeout(ctx, w.reconcileTimeout)
		err := w.reconcileRun(reconcileContext, run)
		cancel()
		if err != nil {
			w.logger.Warn("리뷰 게시 결과를 조정하지 못했습니다", "run", run.ID, "target", fmt.Sprintf("%s/%s#%d", run.Owner, run.Repository, run.Number), "error", err)
		}
	}
	if len(runs) < w.batchSize || w.afterID >= w.throughID {
		w.afterID = 0
		w.throughID = 0
	}
}

func (w *ReviewPublicationWorker) reconcileRun(ctx context.Context, run reviewworkflow.Run) error {
	target := pullrequest.Target{
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		BaseSHA:        run.BaseSHA,
		HeadSHA:        run.HeadSHA,
	}
	now := w.clock.Now()
	leaseToken, err := w.workflows.AcquireRun(ctx, run.ID, now, now.Add(w.runLease))
	if errors.Is(err, reviewworkflow.ErrRunLeased) || errors.Is(err, reviewworkflow.ErrPublicationLeased) {
		return nil
	}
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		invalidation, invalidateErr := w.invalidatePublication(ctx, target, run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다")
		if invalidateErr != nil {
			return invalidateErr
		}
		return w.finishSuperseded(ctx, run, "더 최신인 리뷰 실행이 게시 조정 대상을 대체했습니다", invalidation)
	}
	if err != nil {
		return err
	}
	if leaseToken == "" {
		return nil
	}
	defer w.releaseRun(ctx, run.ID, leaseToken)
	claimedAt := w.clock.Now()
	if err := w.workflows.ClaimPublication(ctx, run.ID, leaseToken, claimedAt, claimedAt.Add(w.publicationLease)); err != nil {
		return err
	}
	current, err := w.source.PullRequest(ctx, target)
	if err != nil {
		return w.resolveUnverified(ctx, run, leaseToken, err)
	}
	if current.BaseSHA != run.BaseSHA || current.HeadSHA != run.HeadSHA {
		invalidation, err := w.invalidatePublication(ctx, target, run, "게시 조정 전에 base 또는 head가 변경되었습니다")
		if err != nil {
			return err
		}
		return w.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "게시 조정 전에 base 또는 head가 변경되었습니다", false, invalidation)
	}
	marker := reviewworkflow.PublicationMarker(run.Key)
	storedPublication, storedPublicationFound, err := w.workflows.ReviewPublication(ctx, run.ID, leaseToken)
	if err != nil {
		return w.resolveUnverified(ctx, run, leaseToken, err)
	}
	if storedPublicationFound && storedPublication.Marker != marker {
		return w.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("저장된 리뷰 게시 marker가 실행과 일치하지 않습니다"))
	}
	published, err := w.publisher.PublicationExists(ctx, target, marker)
	if err != nil {
		return w.resolveUnverified(ctx, run, leaseToken, err)
	}
	if !published && !storedPublicationFound {
		if run.HeartbeatAt.After(w.clock.Now().Add(-w.orphanAfter)) {
			return nil
		}
		return w.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusFailed, "7일 동안 GitHub 게시 marker를 확인하지 못했습니다", false, nil)
	}
	_, found, err := w.reviews.ByRunID(ctx, run.ID)
	if err != nil {
		return w.resolveUnverified(ctx, run, leaseToken, err)
	}
	if !found {
		return w.resolveUnverified(ctx, run, leaseToken, fmt.Errorf("게시 marker는 있지만 내부 리뷰 이력이 없습니다"))
	}
	if published {
		expiresAt := run.StartedAt.Add(w.retention)
		if storedPublicationFound {
			expiresAt = storedPublication.ExpiresAt
		}
		if err := w.completeReviewPublication(ctx, run.ID, leaseToken, marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, expiresAt); err != nil {
			return err
		}
	} else if storedPublication.Status == reviewworkflow.ReviewPublicationStatusPrepared {
		if err := w.publishPreparedPublication(ctx, target, run.ID, leaseToken, storedPublication); errors.Is(err, publication.ErrTargetChanged) {
			return w.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "저장된 리뷰 게시 payload 재개 직전에 base 또는 head가 변경되었습니다", false, nil)
		} else if err != nil {
			return w.resolveUnverified(ctx, run, leaseToken, err)
		}
	}
	confirmed, err := w.source.PullRequest(ctx, target)
	if err != nil {
		return w.resolveUnverified(ctx, run, leaseToken, err)
	}
	if confirmed.BaseSHA != run.BaseSHA || confirmed.HeadSHA != run.HeadSHA {
		invalidation, err := w.invalidatePublication(ctx, target, run, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다")
		if err != nil {
			return err
		}
		return w.finishWithLease(ctx, run, leaseToken, reviewworkflow.RunStatusSuperseded, "리뷰 게시 재개 중 base 또는 head가 변경되었습니다", false, invalidation)
	}
	finalization := reviewworkflow.ReviewPublicationFinalization{
		Status:           reviewworkflow.RunStatusPartial,
		Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
		AdvanceWatermark: false,
	}
	if storedPublicationFound && storedPublication.PayloadHash != "" {
		finalization = storedPublication.Finalization
	}
	return w.finishWithLease(ctx, run, leaseToken, finalization.Status, finalization.Detail, finalization.AdvanceWatermark, nil)
}

func (w *ReviewPublicationWorker) publishPreparedPublication(ctx context.Context, target pullrequest.Target, runID uint64, leaseToken string, prepared reviewworkflow.ReviewPublication) error {
	if prepared.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		return nil
	}
	if prepared.Status != reviewworkflow.ReviewPublicationStatusPrepared {
		return errors.New("게시할 수 없는 리뷰 publication 상태입니다")
	}
	reviewID, submitErr := w.publisher.SubmitReview(ctx, target, prepared.Marker, prepared.Payload.Body, prepared.Payload.Comments)
	if submitErr == nil {
		return w.completeReviewPublication(ctx, runID, leaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReview, reviewID, prepared.ExpiresAt)
	}
	if errors.Is(submitErr, publication.ErrTargetChanged) {
		return submitErr
	}
	published, reconcileErr := w.publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr != nil {
		return errors.Join(submitErr, reconcileErr)
	}
	if published {
		return w.completeReviewPublication(ctx, runID, leaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	w.logger.Warn("저장된 리뷰를 제출하지 못해 canonical fallback 코멘트로 대신합니다", "target", target.Reference(), "error", submitErr)
	commentID, commentErr := w.publisher.CreateComment(ctx, target, prepared.Payload.FallbackBody)
	if commentErr == nil {
		return w.completeReviewPublication(ctx, runID, leaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelComment, commentID, prepared.ExpiresAt)
	}
	published, reconcileErr = w.publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr == nil && published {
		return w.completeReviewPublication(ctx, runID, leaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	return errors.Join(submitErr, commentErr, reconcileErr)
}

func (w *ReviewPublicationWorker) completeReviewPublication(ctx context.Context, runID uint64, leaseToken string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	completedAt := w.clock.Now()
	return w.workflows.CompleteReviewPublication(ctx, runID, leaseToken, marker, channel, externalID, completedAt, expiresAt)
}

func (w *ReviewPublicationWorker) resolveUnverified(ctx context.Context, run reviewworkflow.Run, leaseToken string, cause error) error {
	if run.HeartbeatAt.After(w.clock.Now().Add(-w.orphanAfter)) {
		return cause
	}
	detail := "7일 동안 GitHub 게시 결과를 확정하지 못했습니다"
	invalidation := w.deferredInvalidation(run, invalidationBody(detail), cause)
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := w.finishWithLease(finishContext, run, leaseToken, reviewworkflow.RunStatusFailed, detail, false, invalidation); err != nil {
		return errors.Join(cause, err)
	}
	w.logger.Warn("GitHub 게시 결과를 확정하지 못해 리뷰 실행을 종료했습니다", "run", run.ID, "error", cause)
	return nil
}

func (w *ReviewPublicationWorker) releaseRun(ctx context.Context, runID uint64, leaseToken string) {
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.workflows.ReleaseRun(releaseContext, runID, leaseToken, w.clock.Now()); err != nil {
		w.logger.Warn("게시 조정 run lease를 해제하지 못했습니다", "run", runID, "error", err)
	}
}

func (w *ReviewPublicationWorker) finishSuperseded(ctx context.Context, run reviewworkflow.Run, detail string, invalidation *reviewworkflow.PublicationInvalidation) error {
	now := w.clock.Now()
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := w.workflows.FinishRun(finishContext, run.ID, reviewworkflow.RunResult{
		Status:                  reviewworkflow.RunStatusSuperseded,
		Error:                   detail,
		TerminalAt:              now,
		ExpiresAt:               now.Add(w.retention),
		PublicationInvalidation: invalidation,
	})
	return err
}

func (w *ReviewPublicationWorker) finishWithLease(ctx context.Context, run reviewworkflow.Run, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool, invalidation *reviewworkflow.PublicationInvalidation) error {
	now := w.clock.Now()
	finishContext, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	_, err := w.workflows.FinishRun(finishContext, run.ID, reviewworkflow.RunResult{
		Status:                  status,
		Error:                   detail,
		TerminalAt:              now,
		ExpiresAt:               now.Add(w.retention),
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
		invalidation, invalidateErr := w.invalidatePublication(ctx, target, run, detail)
		if invalidateErr != nil {
			return invalidateErr
		}
		finalizeContext, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, err = w.workflows.FinishRun(finalizeContext, run.ID, reviewworkflow.RunResult{
			Status:                  reviewworkflow.RunStatusSuperseded,
			Error:                   detail,
			TerminalAt:              now,
			ExpiresAt:               now.Add(w.retention),
			LeaseToken:              leaseToken,
			PublicationInvalidation: invalidation,
		})
		finalizeCancel()
	}
	return err
}

func (w *ReviewPublicationWorker) invalidatePublication(ctx context.Context, target pullrequest.Target, run reviewworkflow.Run, detail string) (*reviewworkflow.PublicationInvalidation, error) {
	body := invalidationBody(detail)
	err := w.publisher.InvalidatePublication(ctx, target, reviewworkflow.PublicationMarker(run.Key), body)
	if err == nil {
		return nil, nil
	}
	if run.HeartbeatAt.After(w.clock.Now().Add(-w.orphanAfter)) {
		return nil, err
	}
	return w.deferredInvalidation(run, body, err), nil
}

func (w *ReviewPublicationWorker) deferredInvalidation(run reviewworkflow.Run, body string, failure error) *reviewworkflow.PublicationInvalidation {
	now := w.clock.Now()
	return &reviewworkflow.PublicationInvalidation{
		RunID:          run.ID,
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		Marker:         reviewworkflow.PublicationMarker(run.Key),
		Reason:         body,
		LastError:      failure.Error(),
		NextAttemptAt:  now,
		ExpiresAt:      now.Add(w.retention),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (w *ReviewPublicationWorker) reconcileInvalidations(ctx context.Context) {
	for processed := 0; processed < w.invalidationLimit; processed++ {
		now := w.clock.Now()
		invalidations, err := w.workflows.ClaimPublicationInvalidations(ctx, now, now.Add(w.publicationLease), 1)
		if err != nil {
			w.logger.Error("게시 무효화 재시도 대상을 claim하지 못했습니다", "error", err)
			return
		}
		for _, invalidation := range invalidations {
			w.reconcileInvalidation(ctx, invalidation)
		}
		if len(invalidations) == 0 {
			return
		}
	}
}

func (w *ReviewPublicationWorker) reconcileInvalidation(ctx context.Context, invalidation reviewworkflow.PublicationInvalidation) {
	now := w.clock.Now()
	if !invalidation.ExpiresAt.After(now) {
		return
	}
	target := pullrequest.Target{
		InstallationID: invalidation.InstallationID,
		Owner:          invalidation.Owner,
		Repository:     invalidation.Repository,
		Number:         invalidation.Number,
	}
	attemptDeadline := now.Add(w.reconcileTimeout)
	if invalidation.ExpiresAt.Before(attemptDeadline) {
		attemptDeadline = invalidation.ExpiresAt
	}
	attemptContext, cancel := context.WithDeadline(ctx, attemptDeadline)
	err := w.publisher.InvalidatePublication(attemptContext, target, invalidation.Marker, invalidation.Reason)
	cancel()
	now = w.clock.Now()
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	if err == nil {
		if completeErr := w.workflows.CompletePublicationInvalidation(storeContext, invalidation.ID, invalidation.LeaseToken, now); completeErr != nil {
			w.logger.Warn("게시 무효화 완료를 저장하지 못했습니다", "run", invalidation.RunID, "error", completeErr)
		}
		return
	}
	nextAttemptAt := now.Add(publicationInvalidationRetryDelay(invalidation.Attempts))
	if nextAttemptAt.After(invalidation.ExpiresAt) {
		nextAttemptAt = invalidation.ExpiresAt
	}
	if retryErr := w.workflows.RetryPublicationInvalidation(storeContext, invalidation.ID, invalidation.LeaseToken, now, nextAttemptAt, err.Error()); retryErr != nil {
		w.logger.Warn("게시 무효화 재시도를 저장하지 못했습니다", "run", invalidation.RunID, "error", errors.Join(err, retryErr))
		return
	}
	w.logger.Warn("게시 무효화에 실패해 재시도합니다", "run", invalidation.RunID, "next_attempt_at", nextAttemptAt, "error", err)
}

func publicationInvalidationRetryDelay(attempts int) time.Duration {
	delay := 10 * time.Minute
	for attempt := 0; attempt < attempts && delay < 24*time.Hour; attempt++ {
		delay *= 2
		if delay > 24*time.Hour {
			delay = 24 * time.Hour
		}
	}
	return delay
}

func invalidationBody(detail string) string {
	return "⚠️ 이 리뷰는 게시 과정에서 더 최신인 실행 또는 PR 기준점이 확인되어 무효화되었습니다.\n\n" + detail
}
