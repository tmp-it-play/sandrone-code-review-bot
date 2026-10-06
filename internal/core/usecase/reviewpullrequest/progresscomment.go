package reviewpullrequest

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const progressUpdateTimeout = 20 * time.Second
const progressMutationLease = 3*progressUpdateTimeout + reviewworkflow.PublicationInvalidationFenceDelay + 20*time.Second
const progressMutationRecoveryTimeout = 6 * time.Minute

func requestProgress(task job.ReviewJob) *progressSession {
	marker := strings.TrimSpace(task.ProgressMarker)
	if marker == "" {
		return nil
	}
	return &progressSession{
		commentID:       task.ProgressCommentID,
		marker:          marker,
		createNotBefore: task.ProgressNotBefore,
	}
}

func (u *UseCase) startProgress(run reviewworkflow.Run, existing *progressSession) *progressSession {
	commentID := int64(0)
	handoffMarker := ""
	createNotBefore := time.Time{}
	uncertainUntil := time.Time{}
	if existing != nil {
		existing.Stop()
		commentID = existing.CommentID()
		canonicalMarker := reviewworkflow.ProgressMarker(run.Key)
		for _, marker := range existing.Markers() {
			if marker != canonicalMarker {
				handoffMarker = marker
				break
			}
		}
		existing.stateMu.Lock()
		createNotBefore = existing.createNotBefore
		uncertainUntil = existing.uncertainUntil
		existing.stateMu.Unlock()
	}
	return &progressSession{
		commentID:       commentID,
		marker:          reviewworkflow.ProgressMarker(run.Key),
		handoffMarker:   handoffMarker,
		runID:           run.ID,
		createMissing:   true,
		createNotBefore: createNotBefore,
		uncertainUntil:  uncertainUntil,
	}
}

func (u *UseCase) markProgressMutationUncertain(session *progressSession, err error) {
	if err != nil {
		session.MarkMutationUncertain(u.deps.Clock.Now().Add(reviewworkflow.PublicationInvalidationFenceDelay))
	}
}

func (u *UseCase) replaceProgress(ctx context.Context, target pullrequest.Target, progress *progressSession, body string, checkMessage string, createMissing bool) (bool, error) {
	if progress == nil {
		return false, nil
	}
	progress.Stop()
	fenceContext, fenceCancel := context.WithTimeout(context.WithoutCancel(ctx), reviewworkflow.PublicationInvalidationFenceDelay+progressUpdateTimeout)
	if err := u.waitForProgressMutationFence(fenceContext, target, progress); err != nil {
		fenceCancel()
		return true, err
	}
	fenceCancel()
	marker := progressMutationMarker(progress)
	claimedAt := u.deps.Clock.Now()
	claimContext, claimCancel := context.WithTimeout(context.WithoutCancel(ctx), progressMutationRecoveryTimeout)
	leaseToken, owned, claimErr := u.claimProgressMutation(claimContext, progress.RunID(), marker, claimedAt)
	claimCancel()
	if claimErr != nil {
		return true, claimErr
	}
	if !owned {
		return true, nil
	}
	u.deps.Checks.Complete(context.WithoutCancel(ctx), target, progress.Markers(), progresscomment.CheckConclusionNeutral, checkMessage)
	body = progressResultBody(progress, body)
	cachedID := progress.CommentID()
	lookupContext, lookupCancel := context.WithTimeout(context.WithoutCancel(ctx), progressUpdateTimeout)
	commentID, exists, err := u.findProgressComment(lookupContext, target, progress)
	lookupCancel()
	if err != nil {
		return true, errors.Join(err, u.completeProgressMutation(ctx, marker, leaseToken))
	}
	if !exists {
		if cachedID > 0 || !progress.CanCreate() || !createMissing {
			return true, u.completeProgressMutation(ctx, marker, leaseToken)
		}
		createContext, createCancel := context.WithTimeout(context.WithoutCancel(ctx), progressUpdateTimeout)
		createdID, createErr := u.deps.Publisher.CreateComment(createContext, target, body)
		createCancel()
		if createErr == nil {
			progress.SetCommentID(createdID)
			return true, u.completeProgressMutation(ctx, marker, leaseToken)
		}
		u.markProgressMutationUncertain(progress, createErr)
		reconcileContext, reconcileCancel := context.WithTimeout(context.WithoutCancel(ctx), progressUpdateTimeout)
		reconciledID, reconciled, reconcileErr := u.findProgressComment(reconcileContext, target, progress)
		reconcileCancel()
		if reconciled {
			progress.SetCommentID(reconciledID)
			return true, u.fenceProgressMutation(ctx, progress, marker, leaseToken)
		}
		return true, errors.Join(createErr, reconcileErr, u.fenceProgressMutation(ctx, progress, marker, leaseToken))
	}
	progress.SetCommentID(commentID)
	updateContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), progressUpdateTimeout)
	updateErr := u.deps.Publisher.UpdateComment(updateContext, target, commentID, body)
	cancel()
	if updateErr != nil {
		u.markProgressMutationUncertain(progress, updateErr)
		u.deps.Logger.Warn("진행 코멘트를 결과 안내로 바꾸지 못했습니다", "target", target.Reference(), "comment", commentID, "error", updateErr)
		return true, errors.Join(updateErr, u.fenceProgressMutation(ctx, progress, marker, leaseToken))
	}
	return true, u.completeProgressMutation(ctx, marker, leaseToken)
}

func (u *UseCase) claimProgressMutation(ctx context.Context, runID uint64, marker string, claimedAt time.Time) (string, bool, error) {
	var conflict error
	for {
		leaseToken, owned, err := u.deps.Runs.ClaimProgressCommentMutation(ctx, runID, marker, claimedAt, claimedAt.Add(progressMutationLease))
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
		claimedAt = u.deps.Clock.Now()
	}
}

func progressMutationMarker(progress *progressSession) string {
	if progress.handoffMarker != "" {
		return progress.handoffMarker
	}
	return progress.Marker()
}

func (u *UseCase) completeProgressMutation(ctx context.Context, marker string, leaseToken string) error {
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return u.deps.Runs.CompleteProgressCommentMutation(storeContext, marker, leaseToken, u.deps.Clock.Now())
}

func (u *UseCase) fenceProgressMutation(ctx context.Context, progress *progressSession, marker string, leaseToken string) error {
	failedAt := u.deps.Clock.Now()
	uncertainUntil := failedAt.Add(reviewworkflow.PublicationInvalidationFenceDelay)
	progress.MarkMutationUncertain(uncertainUntil)
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return u.deps.Runs.FenceProgressCommentMutation(storeContext, marker, leaseToken, failedAt, uncertainUntil)
}

func (u *UseCase) waitForProgressMutationFence(ctx context.Context, target pullrequest.Target, progress *progressSession) error {
	if progress == nil {
		return nil
	}
	if progress.CommentID() == 0 {
		lookupContext, lookupCancel := context.WithTimeout(ctx, progressUpdateTimeout)
		commentID, exists, err := u.findProgressComment(lookupContext, target, progress)
		lookupCancel()
		if err == nil && exists {
			progress.SetCommentID(commentID)
			progress.ResolveDiscoveredComment()
		}
	}
	return progress.WaitForMutationFence(ctx)
}

func progressResultBody(progress *progressSession, body string) string {
	if progress == nil {
		return body
	}
	for _, marker := range progress.Markers() {
		marker = strings.TrimSpace(marker)
		if marker != "" && !strings.Contains(body, marker) {
			body = strings.TrimRight(body, "\n") + "\n\n" + marker
		}
	}
	return body
}

func (u *UseCase) findProgressComment(ctx context.Context, target pullrequest.Target, progress *progressSession) (int64, bool, error) {
	for _, marker := range progress.Markers() {
		commentID, exists, err := u.deps.Publisher.FindComment(ctx, target, marker)
		if err != nil || exists {
			return commentID, exists, err
		}
	}
	return 0, false, nil
}

func (u *UseCase) reconcileTerminalProgress(ctx context.Context, target pullrequest.Target, run reviewworkflow.Run, progress *progressSession) error {
	notice, reconcile := terminalProgressNotice(run.Status)
	if !reconcile {
		return nil
	}
	if progress == nil {
		progress = &progressSession{marker: reviewworkflow.ProgressMarker(run.Key), runID: run.ID}
	}
	progress.SetRunID(run.ID)
	_, err := u.replaceProgress(ctx, target, progress, u.deps.Renderer.NoticeBody(notice), notice.Message, false)
	return err
}

func terminalProgressNotice(status reviewworkflow.RunStatus) (review.Notice, bool) {
	switch status {
	case reviewworkflow.RunStatusSkipped:
		return review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경 사항이 없어 이번 리뷰를 종료했습니다."}, true
	case reviewworkflow.RunStatusSuperseded:
		return review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}, true
	case reviewworkflow.RunStatusFailed:
		return review.Notice{Kind: review.NoticeFailed, Message: "리뷰를 완료하지 못해 이번 실행을 종료했습니다."}, true
	case reviewworkflow.RunStatusCancelled:
		return review.Notice{Kind: review.NoticeFailed, Message: "리뷰 실행이 취소되었습니다."}, true
	default:
		return review.Notice{}, false
	}
}
