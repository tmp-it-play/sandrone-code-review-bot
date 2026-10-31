package reviewpublication

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const reviewSummaryMutationLease = 35 * time.Minute
const reviewSummaryMutationTimeout = 30 * time.Minute

var errFallbackAlreadyPublished = errors.New("요약 코멘트에 인라인 지적이 이미 게시되어 있습니다")

type PreparedPublisher struct {
	deps Dependencies
}

func New(deps Dependencies) *PreparedPublisher {
	return &PreparedPublisher{deps: deps}
}

func (p *PreparedPublisher) Publish(ctx context.Context, target pullrequest.Target, runID uint64, runLeaseToken string, prepared reviewworkflow.ReviewPublication) error {
	if prepared.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		return nil
	}
	if prepared.Status != reviewworkflow.ReviewPublicationStatusPrepared {
		return errors.New("게시할 수 없는 리뷰 publication 상태입니다")
	}
	attempt, err := p.deps.Receipts.BeginPublicationAttempt(ctx, runID, runLeaseToken)
	if errors.Is(err, reviewworkflow.ErrPublicationAttemptsExhausted) {
		return p.confirmFinalAttempt(ctx, target, runID, runLeaseToken, err)
	}
	if err != nil {
		return err
	}
	prepared.Channel = attempt.Channel
	channel, externalID, err := p.publishEffects(ctx, target, prepared, runLeaseToken)
	if err == nil {
		p.deps.Checks.Complete(context.WithoutCancel(ctx), target, summaryProgressMarkers(prepared, prepared.Payload.Body), progresscomment.CheckConclusionSuccess, reviewCompletedCheckMessage)
		err = p.Complete(ctx, runID, runLeaseToken, prepared.Marker, channel, externalID, prepared.ExpiresAt)
	}
	if err == nil || errors.Is(err, publication.ErrTargetChanged) || errors.Is(err, reviewworkflow.ErrRunSuperseded) {
		return err
	}
	var retryAfter time.Time
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(err, &scheduled) {
		retryAfter = scheduled.RetryAt()
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	next, storeErr := p.deps.Receipts.RecordPublicationFailure(storeCtx, runID, runLeaseToken, attempt.Number, retryAfter, err.Error())
	if storeErr != nil {
		return errors.Join(storeErr, err)
	}
	if attempt.Number == reviewworkflow.MaxPublicationAttempts {
		if !next.IsZero() {
			return &publication.RetryAtError{At: next, Cause: err}
		}
		return p.confirmFinalAttempt(storeCtx, target, runID, runLeaseToken, errors.Join(reviewworkflow.ErrPublicationAttemptsExhausted, err))
	}
	return &publication.RetryAtError{At: next, Cause: err}
}

func (p *PreparedPublisher) publishEffects(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, leaseToken string) (string, int64, error) {
	verifiedTarget, err := p.deps.Publisher.VerifyTarget(ctx, target)
	if err != nil {
		return "", 0, err
	}
	target = verifiedTarget
	if prepared.Channel == reviewworkflow.ReviewPublicationChannelComment {
		id, err := p.publishSummaryComment(ctx, target, prepared, prepared.Payload.FallbackBody)
		return prepared.Channel, id, err
	}
	commentID, err := p.publishSummaryComment(ctx, target, prepared, prepared.Payload.Body)
	if errors.Is(err, errFallbackAlreadyPublished) {
		return p.selectAndPublishFallback(ctx, target, prepared, leaseToken)
	}
	if err != nil {
		return "", 0, err
	}
	verifiedTarget, err = p.deps.Publisher.VerifyTarget(ctx, target)
	if err != nil {
		return "", 0, err
	}
	target = verifiedTarget
	if len(prepared.Payload.Comments) == 0 {
		return reviewworkflow.ReviewPublicationChannelComment, commentID, nil
	}
	reviewID, submitErr := p.deps.Publisher.SubmitReview(ctx, target, prepared.Marker, prepared.Marker, prepared.Payload.Comments)
	if errors.Is(submitErr, publication.ErrTargetChanged) {
		return "", 0, submitErr
	}
	if errors.Is(submitErr, publication.ErrInlineReviewRejected) {
		p.deps.Logger.Warn("인라인 리뷰를 제출하지 못해 요약 코멘트에 지적을 포함합니다", "target", target.Reference(), "error", submitErr)
		return p.selectAndPublishFallback(ctx, target, prepared, leaseToken)
	}
	if submitErr != nil {
		return "", 0, submitErr
	}
	return reviewworkflow.ReviewPublicationChannelReview, reviewID, nil
}

func (p *PreparedPublisher) selectAndPublishFallback(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, token string) (string, int64, error) {
	if err := p.deps.Receipts.SelectPublicationFallback(ctx, prepared.RunID, token); err != nil {
		return "", 0, err
	}
	verified, err := p.deps.Publisher.VerifyTarget(ctx, target)
	if err != nil {
		return "", 0, err
	}
	id, err := p.publishSummaryComment(ctx, verified, prepared, prepared.Payload.FallbackBody)
	return reviewworkflow.ReviewPublicationChannelComment, id, err
}

func (p *PreparedPublisher) publishSummaryComment(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, body string) (int64, error) {
	sharedMarker := sharedProgressMarker(prepared)
	if sharedMarker == "" {
		commentID, _, _, err := p.publishSummaryEffect(ctx, target, prepared, body, "")
		return commentID, err
	}
	claimedAt := p.deps.Clock.Now()
	leaseToken, owned, claimErr := p.deps.Ownership.ClaimProgressCommentMutation(ctx, prepared.RunID, sharedMarker, claimedAt, claimedAt.Add(reviewSummaryMutationLease))
	if claimErr != nil {
		return 0, claimErr
	}
	if !owned {
		return 0, reviewworkflow.ErrRunSuperseded
	}
	mutationContext, mutationCancel := context.WithDeadline(ctx, claimedAt.Add(reviewSummaryMutationTimeout))
	commentID, mutationAttempted, uncertain, effectErr := p.publishSummaryEffect(mutationContext, target, prepared, body, sharedMarker)
	mutationCancel()
	if uncertain || effectErr != nil && mutationAttempted {
		fenceErr := p.fenceSummaryMutation(ctx, sharedMarker, leaseToken)
		return commentID, errors.Join(effectErr, fenceErr)
	}
	completeErr := p.completeSummaryMutation(ctx, sharedMarker, leaseToken)
	return commentID, errors.Join(effectErr, completeErr)
}

func (p *PreparedPublisher) publishSummaryEffect(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, body string, claimedMarker string) (int64, bool, bool, error) {
	existingID, exists, mutationAttempted, err := p.updateSummaryComments(ctx, target, prepared, body, claimedMarker)
	if err != nil {
		return 0, mutationAttempted, false, err
	}
	if exists {
		return existingID, mutationAttempted, false, nil
	}
	createdID, createErr := p.deps.Publisher.CreateComment(ctx, target, body)
	if createErr == nil {
		return createdID, true, false, nil
	}
	reconciledID, reconciled, _, reconcileErr := p.updateSummaryComments(ctx, target, prepared, body, claimedMarker)
	if reconciled {
		return reconciledID, true, true, reconcileErr
	}
	return 0, true, true, errors.Join(createErr, reconcileErr)
}

func (p *PreparedPublisher) updateSummaryComments(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, body string, claimedMarker string) (int64, bool, bool, error) {
	markers := []string{prepared.Marker}
	canonicalProgressMarker := ""
	if progressMarker, ok := reviewworkflow.ProgressMarkerForPublicationMarker(prepared.Marker); ok {
		canonicalProgressMarker = progressMarker
		markers = append(markers, progressMarker)
	}
	for _, progressMarker := range reviewworkflow.ProgressMarkers(prepared.Payload.Body, prepared.Marker) {
		if progressMarker != canonicalProgressMarker && progressMarker != claimedMarker {
			owned, err := p.deps.Ownership.OwnsProgressMarker(ctx, prepared.RunID, progressMarker)
			if err != nil {
				return 0, false, false, err
			}
			if !owned {
				continue
			}
		}
		markers = append(markers, progressMarker)
	}
	comments := make([]pullrequest.Comment, 0)
	seen := make(map[int64]struct{})
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		foundComments, err := p.deps.Publisher.FindComments(ctx, target, marker)
		if err != nil {
			return 0, false, false, err
		}
		for _, comment := range foundComments {
			commentID := comment.ID
			if _, exists := seen[commentID]; exists {
				continue
			}
			seen[commentID] = struct{}{}
			comments = append(comments, comment)
		}
	}
	if len(comments) == 0 {
		return 0, false, false, nil
	}
	var updateErr error
	mutationAttempted := false
	publicBody := review.SanitizePublicBody(body)
	if len(prepared.Payload.Comments) > 0 && body == prepared.Payload.Body {
		fallback := review.SanitizePublicBody(prepared.Payload.FallbackBody)
		if fallback != publicBody {
			for _, comment := range comments {
				if comment.Body == fallback {
					return comment.ID, true, false, errFallbackAlreadyPublished
				}
			}
		}
	}
	for _, comment := range comments {
		if comment.Body == publicBody {
			continue
		}
		mutationAttempted = true
		if err := p.deps.Publisher.UpdateComment(ctx, target, comment.ID, publicBody); err != nil {
			updateErr = errors.Join(updateErr, err)
		}
	}
	return comments[len(comments)-1].ID, true, mutationAttempted, updateErr
}

func summaryProgressMarkers(prepared reviewworkflow.ReviewPublication, body string) []string {
	markers := reviewworkflow.ProgressMarkers(body, prepared.Marker)
	if canonicalMarker, ok := reviewworkflow.ProgressMarkerForPublicationMarker(prepared.Marker); ok {
		markers = append([]string{canonicalMarker}, markers...)
	}
	return markers
}

func sharedProgressMarker(prepared reviewworkflow.ReviewPublication) string {
	canonicalMarker, _ := reviewworkflow.ProgressMarkerForPublicationMarker(prepared.Marker)
	for _, marker := range reviewworkflow.ProgressMarkers(prepared.Payload.Body, prepared.Marker) {
		if marker != canonicalMarker {
			return marker
		}
	}
	return ""
}

func (p *PreparedPublisher) completeSummaryMutation(ctx context.Context, marker string, leaseToken string) error {
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return p.deps.Ownership.CompleteProgressCommentMutation(storeContext, marker, leaseToken, p.deps.Clock.Now())
}

func (p *PreparedPublisher) fenceSummaryMutation(ctx context.Context, marker string, leaseToken string) error {
	failedAt := p.deps.Clock.Now()
	storeContext, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()
	return p.deps.Ownership.FenceProgressCommentMutation(storeContext, marker, leaseToken, failedAt, failedAt.Add(reviewworkflow.PublicationInvalidationFenceDelay))
}

func (p *PreparedPublisher) Complete(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	return p.deps.Receipts.CompleteReviewPublication(ctx, runID, runLeaseToken, marker, channel, externalID, p.deps.Clock.Now(), expiresAt)
}
