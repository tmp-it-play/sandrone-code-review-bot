package reviewpullrequest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

const reviewPromptVersion = "review-v4"

func newWorkflowRun(task job.ReviewJob, target pullrequest.Target, config setting.RepoConfig, modelPolicyHash string, startedAt time.Time, snapshotObservedAt time.Time) (reviewworkflow.Run, error) {
	configHash, err := hashJSON(config)
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 설정 hash를 만들지 못했습니다: %w", err)
	}
	requestIdentity := workflowRequestIdentity(task)
	key := workflowHash(
		"review-run-v1",
		strconv.FormatInt(target.InstallationID, 10),
		target.Owner,
		target.Repository,
		strconv.Itoa(target.Number),
		target.BaseSHA,
		target.HeadSHA,
		configHash,
		reviewPromptVersion,
		modelPolicyHash,
		requestIdentity,
	)
	return reviewworkflow.Run{
		Key:                key,
		InstallationID:     target.InstallationID,
		Owner:              target.Owner,
		Repository:         target.Repository,
		Number:             target.Number,
		BaseSHA:            target.BaseSHA,
		HeadSHA:            target.HeadSHA,
		ConfigHash:         configHash,
		PromptVersion:      reviewPromptVersion,
		ModelPolicyHash:    modelPolicyHash,
		RequestIdentity:    requestIdentity,
		Trigger:            task.Trigger,
		Status:             reviewworkflow.RunStatusPlanning,
		SnapshotObservedAt: snapshotObservedAt,
		SnapshotOrderKey:   task.SnapshotOrderKey,
		StartedAt:          startedAt,
		HeartbeatAt:        startedAt,
	}, nil
}

func (u *UseCase) finishWorkflow(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowWithOutcome(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, advanceWatermark)
}

func (u *UseCase) finishWorkflowWithOutcome(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	terminalAt := u.deps.Clock.Now()
	return u.deps.Workflows.FinishRun(ctx, runID, reviewworkflow.RunResult{
		Status:           status,
		ReviewOutcome:    outcome,
		Error:            detail,
		TerminalAt:       terminalAt,
		ExpiresAt:        terminalAt.Add(u.deps.Retention),
		AdvanceWatermark: advanceWatermark,
		LeaseToken:       leaseToken,
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

func transitionCoverage(items []reviewworkflow.CoverageItem, unitHash string, status reviewworkflow.CoverageStatus, reviewedAt *time.Time) {
	for index := range items {
		if items[index].UnitHash != unitHash || items[index].Status != reviewworkflow.CoverageStatusPlanned {
			continue
		}
		items[index].Status = status
		items[index].ReviewedAt = reviewedAt
	}
}

func coverageDetail(summary reviewworkflow.CoverageSummary) string {
	return fmt.Sprintf(
		"coverage total=%d reviewed=%d failed=%d deferred=%d skipped=%d pending=%d",
		summary.Total,
		summary.Reviewed,
		summary.Failed,
		summary.Deferred,
		summary.Skipped,
		summary.Pending,
	)
}

func workflowRequestIdentity(task job.ReviewJob) string {
	if task.RequestIdentity != "" {
		return task.RequestIdentity
	}
	if task.Trigger.IsAutomatic() {
		return ""
	}
	if task.CommentID != 0 {
		return "comment:" + strconv.FormatInt(task.CommentID, 10)
	}
	return workflowHash("manual-review-v1", string(task.Trigger), task.Invoker, task.Instruction)
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return workflowHash(string(encoded)), nil
}

func workflowHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (u *UseCase) prepareAndPublishReview(ctx context.Context, target pullrequest.Target, runID uint64, runLease string, marker string, view review.SummaryView, result review.Result, attribution review.Attribution, style review.Style, finalization reviewworkflow.ReviewPublicationFinalization) error {
	inline := result.Inline()
	comments := make([]review.InlineComment, 0, len(inline))
	for _, finding := range inline {
		comment := review.InlineComment{
			Path: finding.File,
			Line: finding.Line,
			Body: u.deps.Renderer.InlineBody(finding, attribution, style),
		}
		if finding.EndLine > finding.Line {
			comment.StartLine = finding.Line
			comment.Line = finding.EndLine
		}
		comments = append(comments, comment)
	}
	body := u.deps.Renderer.SummaryBody(view) + "\n" + marker
	degraded := view
	degraded.Fallback = append(append([]review.Finding{}, result.Fallback()...), inline...)
	degraded.InlineCount = 0
	payload := reviewworkflow.ReviewPublicationPayload{
		Body:         body,
		Comments:     comments,
		FallbackBody: u.deps.Renderer.SummaryBody(degraded) + "\n" + marker,
	}
	preparedAt := u.deps.Clock.Now()
	prepared, err := u.deps.Workflows.PrepareReviewPublication(ctx, runID, runLease, marker, payload, finalization, preparedAt, preparedAt.Add(u.deps.Retention))
	if err != nil {
		return err
	}
	return u.publishPreparedReview(ctx, target, runID, runLease, prepared)
}

func (u *UseCase) publishPreparedReview(ctx context.Context, target pullrequest.Target, runID uint64, runLease string, prepared reviewworkflow.ReviewPublication) error {
	if prepared.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		return nil
	}
	if prepared.Status != reviewworkflow.ReviewPublicationStatusPrepared {
		return errors.New("게시할 수 없는 리뷰 publication 상태입니다")
	}
	published, err := u.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if err != nil {
		return err
	}
	if published {
		return u.completeReviewPublication(ctx, runID, runLease, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	reviewID, submitErr := u.deps.Publisher.SubmitReview(ctx, target, prepared.Marker, prepared.Payload.Body, prepared.Payload.Comments)
	if submitErr == nil {
		return u.completeReviewPublication(ctx, runID, runLease, prepared.Marker, reviewworkflow.ReviewPublicationChannelReview, reviewID, prepared.ExpiresAt)
	}
	if errors.Is(submitErr, publication.ErrTargetChanged) {
		return submitErr
	}
	u.deps.Logger.Warn("리뷰를 제출하지 못해 요약 코멘트로 대신합니다", "target", target.Reference(), "error", submitErr)
	published, reconcileErr := u.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr != nil {
		return errors.Join(submitErr, reconcileErr)
	}
	if published {
		return u.completeReviewPublication(ctx, runID, runLease, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	commentID, commentErr := u.deps.Publisher.CreateComment(ctx, target, prepared.Payload.FallbackBody)
	if commentErr == nil {
		return u.completeReviewPublication(ctx, runID, runLease, prepared.Marker, reviewworkflow.ReviewPublicationChannelComment, commentID, prepared.ExpiresAt)
	}
	published, reconcileErr = u.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr == nil && published {
		return u.completeReviewPublication(ctx, runID, runLease, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	return errors.Join(submitErr, commentErr, reconcileErr)
}

func (u *UseCase) completeReviewPublication(ctx context.Context, runID uint64, runLease string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	completedAt := u.deps.Clock.Now()
	return u.deps.Workflows.CompleteReviewPublication(ctx, runID, runLease, marker, channel, externalID, completedAt, expiresAt)
}

func (u *UseCase) finalizePublishedReview(ctx context.Context, target pullrequest.Target, runID uint64, runLease string, marker string, desiredStatus reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	publishedState, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return reviewworkflow.RunStatusPublishing, err
	}
	if publishedState.HeadSHA != target.HeadSHA || publishedState.BaseSHA != target.BaseSHA {
		return u.supersedePublishedReview(ctx, target, marker, runID, runLease, "리뷰 게시 중 base 또는 head가 변경되었습니다")
	}
	publicationCheckedAt := u.deps.Clock.Now()
	if err := u.deps.Workflows.RenewRun(ctx, runID, runLease, publicationCheckedAt, publicationCheckedAt.Add(reviewRunLease)); err != nil {
		if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
			return u.supersedePublishedReview(ctx, target, marker, runID, runLease, "리뷰 게시 중 더 최신인 실행이 확인되었습니다")
		}
		return reviewworkflow.RunStatusPublishing, err
	}
	actualStatus, err := u.finishWorkflow(ctx, runID, runLease, desiredStatus, detail, advanceWatermark)
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		return u.supersedePublishedReview(ctx, target, marker, runID, runLease, "리뷰 실행 완료 중 더 최신인 실행이 확인되었습니다")
	}
	return actualStatus, err
}
