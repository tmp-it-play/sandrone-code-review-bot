package reviewenqueue

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const progressProducerTimeout = 20 * time.Second
const progressProducerMutationLease = progressProducerTimeout + reviewworkflow.PublicationInvalidationFenceDelay + 10*time.Second
const progressRecoveryTimeout = progressProducerMutationLease + progressProducerTimeout + 10*time.Second

type ProgressQueue struct {
	deps Dependencies
}

func New(deps Dependencies) *ProgressQueue {
	return &ProgressQueue{deps: deps}
}

func (q *ProgressQueue) EnqueueReview(ctx context.Context, task job.ReviewJob) error {
	prepare := q.shouldPrepare(ctx, task)
	if prepare {
		q.prepareAdmission(&task)
	}
	admission, err := q.deps.Queue.EnqueueReviewAdmission(ctx, task)
	if err != nil || !prepare {
		return err
	}
	if strings.TrimSpace(task.ProgressMarker) == "" {
		return nil
	}
	if !admission.Accepted && !admission.ProgressRecoverable {
		return nil
	}
	if !admission.Accepted {
		q.deps.Logger.Info("이미 등록된 리뷰 작업의 진행 코멘트를 확인합니다", "target", task.Target.Reference())
	}
	if progressErr := q.EnsureReviewProgress(ctx, task); progressErr != nil {
		q.deps.Logger.Warn("등록된 리뷰 작업의 진행 코멘트를 즉시 준비하지 못해 worker에서 다시 시도합니다", "target", task.Target.Reference(), "error", progressErr)
	}
	return nil
}

func (q *ProgressQueue) EnsureReviewProgress(ctx context.Context, task job.ReviewJob) error {
	return q.ensureReviewProgress(context.WithoutCancel(ctx), task, progressProducerTimeout)
}

func (q *ProgressQueue) RecoverReviewProgress(ctx context.Context, task job.ReviewJob) error {
	return q.ensureReviewProgress(ctx, task, progressRecoveryTimeout)
}

func (q *ProgressQueue) ensureReviewProgress(ctx context.Context, task job.ReviewJob, producerTimeout time.Duration) error {
	if strings.TrimSpace(task.ProgressMarker) == "" {
		return nil
	}
	now := q.deps.Clock.Now()
	refreshContext, refreshCancel := context.WithTimeout(ctx, 10*time.Second)
	refreshActive, refreshErr := q.deps.Runs.EnsureProgressCommentRefresh(refreshContext, progresscomment.Refresh{
		Marker:          task.ProgressMarker,
		Target:          task.Target,
		Sequence:        1,
		CreatedAt:       now,
		CreateNotBefore: task.ProgressNotBefore,
		NextRefreshAt:   now.Add(progresscomment.RefreshInterval),
		ExpiresAt:       now.Add(progresscomment.RefreshLifetime),
	})
	refreshCancel()
	if refreshErr != nil {
		return refreshErr
	}
	if refreshActive {
		return q.ensureProgressComment(ctx, task, producerTimeout)
	}
	return nil
}

func (q *ProgressQueue) EnqueueSummary(ctx context.Context, task job.SummaryJob) error {
	return q.deps.Queue.EnqueueSummary(ctx, task)
}

func (q *ProgressQueue) EnqueueReply(ctx context.Context, task job.ReplyJob) error {
	return q.deps.Queue.EnqueueReply(ctx, task)
}

func (q *ProgressQueue) shouldPrepare(ctx context.Context, task job.ReviewJob) bool {
	if !task.Trigger.IsAutomatic() {
		return true
	}
	config, err := q.deps.Settings.RepoConfig(ctx, task.Target)
	if err != nil {
		q.deps.Logger.Warn("즉시 진행 코멘트를 위한 자동 리뷰 설정을 읽지 못했습니다", "target", task.Target.Reference(), "error", err)
		return true
	}
	if !config.Sandrone.AutoReview {
		return false
	}
	if task.Trigger == review.TriggerPullRequestDraftOpened && !config.Sandrone.AutoReviewOnDraft {
		return false
	}
	if task.Trigger == review.TriggerPullRequestPushed && !config.Sandrone.AutoReviewOnPush {
		activityBoundary := task.RequestReceivedAt
		if activityBoundary.IsZero() {
			activityBoundary = task.SnapshotObservedAt
		}
		continues, continuationErr := q.deps.Runs.HasRegisteredReviewContinuation(ctx, reviewworkflow.Run{
			Owner:              task.Target.Owner,
			Repository:         task.Target.Repository,
			Number:             task.Target.Number,
			HeadSHA:            task.Target.HeadSHA,
			SnapshotObservedAt: task.SnapshotObservedAt,
		}, activityBoundary)
		if continuationErr != nil {
			q.deps.Logger.Warn("즉시 진행 코멘트를 위한 등록 리뷰 연속성을 확인하지 못했습니다", "target", task.Target.Reference(), "error", continuationErr)
			return true
		}
		return continues
	}
	return true
}

func (q *ProgressQueue) prepareAdmission(task *job.ReviewJob) {
	key := progressKey(*task)
	if key == "" {
		return
	}
	marker := reviewworkflow.ProgressMarker(key)
	task.ProgressMarker = marker
	task.ProgressNotBefore = q.deps.Clock.Now().Add(progressProducerTimeout + reviewworkflow.PublicationInvalidationFenceDelay)
}

func (q *ProgressQueue) ensureProgressComment(ctx context.Context, task job.ReviewJob, claimTimeout time.Duration) error {
	marker := strings.TrimSpace(task.ProgressMarker)
	if marker == "" {
		return nil
	}
	claimContext, claimCancel := context.WithTimeout(ctx, claimTimeout)
	claimedAt := q.deps.Clock.Now()
	leaseToken, owned, claimErr := q.claimProgressCommentMutation(claimContext, marker, claimedAt)
	claimCancel()
	if claimErr != nil {
		return claimErr
	}
	if !owned {
		return nil
	}
	producerContext, producerCancel := context.WithTimeout(ctx, progressProducerTimeout)
	defer producerCancel()
	_, exists, err := q.deps.Publisher.FindComment(producerContext, task.Target, marker)
	if err != nil {
		completeErr := q.completeProgressMutation(ctx, marker, leaseToken)
		return errors.Join(err, completeErr)
	}
	if exists {
		return q.completeProgressMutation(ctx, marker, leaseToken)
	}
	if producerContext.Err() != nil {
		return errors.Join(producerContext.Err(), q.completeProgressMutation(ctx, marker, leaseToken))
	}
	key := progressKey(task)
	body := q.deps.Renderer.ProgressBody(review.ProgressMessage(key, 0), marker)
	_, createErr := q.deps.Publisher.CreateComment(producerContext, task.Target, body)
	if createErr == nil {
		return q.completeProgressMutation(ctx, marker, leaseToken)
	}
	_, reconciled, reconcileErr := q.deps.Publisher.FindComment(producerContext, task.Target, marker)
	uncertainUntil, fenceErr := q.fenceProgressMutation(ctx, marker, leaseToken)
	retryErr := &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: uncertainUntil}
	if reconciled {
		return errors.Join(createErr, fenceErr, retryErr)
	}
	return errors.Join(createErr, reconcileErr, fenceErr, retryErr)
}

func (q *ProgressQueue) claimProgressCommentMutation(ctx context.Context, marker string, claimedAt time.Time) (string, bool, error) {
	var conflict error
	for {
		leaseToken, owned, err := q.deps.Runs.ClaimProgressCommentMutation(ctx, 0, marker, claimedAt, claimedAt.Add(progressProducerMutationLease))
		if !errors.Is(err, reviewworkflow.ErrProgressCommentLeased) {
			return leaseToken, owned, err
		}
		conflict = err
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", false, conflict
		case <-timer.C:
		}
		claimedAt = q.deps.Clock.Now()
	}
}

func (q *ProgressQueue) completeProgressMutation(ctx context.Context, marker string, leaseToken string) error {
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return q.deps.Runs.CompleteProgressCommentMutation(storeContext, marker, leaseToken, q.deps.Clock.Now())
}

func (q *ProgressQueue) fenceProgressMutation(ctx context.Context, marker string, leaseToken string) (time.Time, error) {
	failedAt := q.deps.Clock.Now()
	uncertainUntil := failedAt.Add(reviewworkflow.PublicationInvalidationFenceDelay)
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return uncertainUntil, q.deps.Runs.FenceProgressCommentMutation(storeContext, marker, leaseToken, failedAt, uncertainUntil)
}

func progressKey(task job.ReviewJob) string {
	identity := strings.TrimSpace(task.RequestIdentity)
	if identity == "" {
		identity = strings.Join([]string{string(task.Trigger), task.Target.BaseSHA, task.Target.HeadSHA, task.SnapshotOrderKey}, ":")
	}
	return job.OperationKey("review-progress:"+string(task.Trigger), task.Target, identity, task.CommentID, task.InThread)
}
