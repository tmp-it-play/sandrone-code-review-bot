package reviewpullrequest

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

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
	prepared, err := u.deps.Publications.PrepareReviewPublication(ctx, runID, runLease, marker, payload, finalization, preparedAt, preparedAt.Add(u.deps.Retention))
	if err != nil {
		return err
	}
	return u.publishPreparedReview(ctx, target, runID, runLease, prepared)
}

func (u *UseCase) publishPreparedReview(ctx context.Context, target pullrequest.Target, runID uint64, runLease string, prepared reviewworkflow.ReviewPublication) error {
	return u.publications.Publish(ctx, target, runID, runLease, prepared)
}

func (u *UseCase) completeReviewPublication(ctx context.Context, runID uint64, runLease string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	return u.publications.Complete(ctx, runID, runLease, marker, channel, externalID, expiresAt)
}

func (u *UseCase) finalizePublishedReview(ctx context.Context, target pullrequest.Target, runID uint64, runLease string, marker string, desiredStatus reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	publishedState, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return reviewworkflow.RunStatusPublishing, err
	}
	if publishedState.HeadSHA != target.HeadSHA || publishedState.BaseSHA != target.BaseSHA {
		return u.supersedePublishedReview(ctx, target, marker, runID, runLease, changedHeadSHA(target.HeadSHA, publishedState.HeadSHA), "리뷰 게시 중 base 또는 head가 변경되었습니다")
	}
	publicationCheckedAt := u.deps.Clock.Now()
	if err := u.deps.Runs.RenewRun(ctx, runID, runLease, publicationCheckedAt, publicationCheckedAt.Add(reviewRunLease)); err != nil {
		if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
			return u.supersedePublishedReview(ctx, target, marker, runID, runLease, "", "리뷰 게시 중 더 최신인 실행이 확인되었습니다")
		}
		return reviewworkflow.RunStatusPublishing, err
	}
	actualStatus, err := u.finishWorkflow(ctx, runID, runLease, desiredStatus, detail, advanceWatermark)
	if errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		return u.supersedePublishedReview(ctx, target, marker, runID, runLease, "", "리뷰 실행 완료 중 더 최신인 실행이 확인되었습니다")
	}
	return actualStatus, err
}
