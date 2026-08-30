package reviewenqueue

import (
	"context"
	"errors"
	"fmt"
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
	if q.skipNoopHeadChange(ctx, task) {
		return nil
	}
	prepare, theme, err := q.progressPreparation(ctx, &task)
	if err != nil {
		return err
	}
	if prepare {
		q.prepareAdmission(&task, theme)
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
	task.ProgressMessageTheme = progresscomment.NormalizeTheme(admission.ProgressMessageTheme)
	if !admission.Accepted {
		q.deps.Logger.Info("이미 등록된 리뷰 작업의 진행 코멘트를 확인합니다", "target", task.Target.Reference())
	}
	if progressErr := q.EnsureReviewProgress(ctx, task); progressErr != nil {
		q.deps.Logger.Warn("등록된 리뷰 작업의 진행 코멘트를 즉시 준비하지 못해 worker에서 다시 시도합니다", "target", task.Target.Reference(), "error", progressErr)
	}
	return nil
}

func (q *ProgressQueue) skipNoopHeadChange(ctx context.Context, task job.ReviewJob) bool {
	if task.Trigger != review.TriggerPullRequestPushed || task.PreviousHeadSHA == "" || task.Target.HeadSHA == "" {
		return false
	}
	for attempt := 0; attempt < 3; attempt++ {
		anchor, latestFound, err := q.deps.Runs.LatestRunAnchor(ctx, task.Target)
		if err != nil {
			q.deps.Logger.Warn("최신 리뷰의 기준점을 확인하지 못해 리뷰 작업을 등록합니다", "target", task.Target.Reference(), "error", err)
			return false
		}
		skip, comparisonBaseSHA, compareErr := q.evaluateNoopHeadChange(ctx, task, anchor, latestFound)
		confirmedAnchor, confirmedFound, confirmErr := q.deps.Runs.LatestRunAnchor(ctx, task.Target)
		if confirmErr != nil {
			q.deps.Logger.Warn("head 변경 판단 후 최신 리뷰 기준점을 재확인하지 못해 리뷰 작업을 등록합니다", "target", task.Target.Reference(), "error", confirmErr)
			return false
		}
		if latestFound != confirmedFound || anchor != confirmedAnchor {
			continue
		}
		if compareErr != nil {
			q.deps.Logger.Warn("push의 실제 파일 변경 여부를 확인하지 못해 리뷰 작업을 등록합니다", "target", task.Target.Reference(), "previous_head", comparisonBaseSHA, "head", task.Target.HeadSHA, "error", compareErr)
			return false
		}
		if skip {
			q.deps.Logger.Info("파일 변경이 없는 head 이동을 건너뜁니다", "target", task.Target.Reference(), "previous_head", comparisonBaseSHA, "head", task.Target.HeadSHA)
		}
		return skip
	}
	q.deps.Logger.Warn("head 변경 판단 중 최신 리뷰 기준점이 계속 바뀌어 리뷰 작업을 등록합니다", "target", task.Target.Reference())
	return false
}

func (q *ProgressQueue) evaluateNoopHeadChange(ctx context.Context, task job.ReviewJob, anchor reviewworkflow.RunAnchor, latestFound bool) (bool, string, error) {
	comparisonBaseSHA := task.PreviousHeadSHA
	if latestFound {
		if anchor.BaseSHA != task.Target.BaseSHA || anchor.ReplacementPending {
			return false, comparisonBaseSHA, nil
		}
		if anchor.HeadSHA != "" {
			comparisonBaseSHA = anchor.HeadSHA
		}
	}
	if comparisonBaseSHA == task.Target.HeadSHA {
		return true, comparisonBaseSHA, nil
	}
	files, err := q.deps.Source.ChangedFilesBetween(ctx, task.Target, comparisonBaseSHA, task.Target.HeadSHA)
	return err == nil && len(files) == 0, comparisonBaseSHA, err
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
		MessageTheme:    progresscomment.NormalizeTheme(task.ProgressMessageTheme),
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

func (q *ProgressQueue) progressPreparation(ctx context.Context, task *job.ReviewJob) (bool, progresscomment.Theme, error) {
	theme := progresscomment.ThemeProgramming
	if task.Target.HeadSHA == "" {
		request, err := q.deps.Source.PullRequest(ctx, task.Target)
		if err != nil {
			return false, theme, fmt.Errorf("진행 코멘트 설정을 위한 Pull Request를 읽지 못했습니다: %w", err)
		}
		task.Target.BaseSHA = request.BaseSHA
		task.Target.BaseRef = request.BaseRef
		task.Target.HeadSHA = request.HeadSHA
	}
	config, err := q.deps.Settings.RepoConfig(ctx, task.Target)
	if err != nil {
		return false, theme, fmt.Errorf("즉시 진행 코멘트를 위한 저장소 설정을 읽지 못했습니다: %w", err)
	}
	theme = progresscomment.NormalizeTheme(config.Sandrone.ProgressMessageTheme)
	if !task.Trigger.IsAutomatic() {
		return true, theme, nil
	}
	if !config.Sandrone.AutoReview {
		return false, theme, nil
	}
	if task.Trigger == review.TriggerPullRequestDraftOpened && !config.Sandrone.AutoReviewOnDraft {
		return false, theme, nil
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
			return true, theme, nil
		}
		return continues, theme, nil
	}
	return true, theme, nil
}

func (q *ProgressQueue) prepareAdmission(task *job.ReviewJob, theme progresscomment.Theme) {
	key := progressKey(*task)
	if key == "" {
		return
	}
	marker := reviewworkflow.ProgressMarker(key)
	task.ProgressMarker = marker
	task.ProgressMessageTheme = progresscomment.NormalizeTheme(theme)
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
	body := q.deps.Renderer.ProgressBody(review.ProgressMessage(task.ProgressMessageTheme, key, 0), marker)
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
