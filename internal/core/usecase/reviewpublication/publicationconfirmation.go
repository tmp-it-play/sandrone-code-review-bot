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

func (p *PreparedPublisher) confirmFinalAttempt(ctx context.Context, target pullrequest.Target, runID uint64, token string, exhausted error) error {
	prepared, found, err := p.deps.Receipts.ReviewPublication(ctx, runID, token)
	if err != nil {
		return err
	}
	if !found {
		return exhausted
	}
	if prepared.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		return nil
	}
	verified, err := p.deps.Publisher.VerifyTarget(ctx, target)
	if err != nil {
		if errors.Is(err, publication.ErrTargetChanged) {
			return err
		}
		return p.deferConfirmation(ctx, runID, token, err)
	}
	comments, err := p.deps.Publisher.FindComments(ctx, verified, prepared.Marker)
	if err != nil {
		return p.deferConfirmation(ctx, runID, token, err)
	}
	if len(comments) == 0 {
		return exhausted
	}
	body := comments[0].Body
	for _, comment := range comments[1:] {
		if comment.Body != body {
			return exhausted
		}
	}
	summary := review.SanitizePublicBody(prepared.Payload.Body)
	fallback := review.SanitizePublicBody(prepared.Payload.FallbackBody)
	channel := reviewworkflow.ReviewPublicationChannelComment
	externalID := comments[len(comments)-1].ID
	fallbackPublished := len(prepared.Payload.Comments) > 0 && body == fallback && (fallback != summary || prepared.Channel == channel)
	if !fallbackPublished {
		if body != summary || prepared.Channel == channel {
			return exhausted
		}
		if len(prepared.Payload.Comments) > 0 {
			id, exists, err := p.deps.Publisher.FindReview(ctx, verified, prepared.Marker)
			if err != nil {
				return p.deferConfirmation(ctx, runID, token, err)
			}
			if !exists {
				return exhausted
			}
			channel = reviewworkflow.ReviewPublicationChannelReview
			externalID = id
		}
	}
	err = p.Complete(ctx, prepared.RunID, token, prepared.Marker, channel, externalID, prepared.ExpiresAt)
	if err != nil {
		return err
	}
	p.deps.Checks.Complete(ctx, verified, summaryProgressMarkers(prepared, body), progresscomment.CheckConclusionSuccess, reviewCompletedCheckMessage)
	return nil
}

func (p *PreparedPublisher) deferConfirmation(ctx context.Context, runID uint64, token string, cause error) error {
	next := p.deps.Clock.Now().Add(10 * time.Minute)
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(cause, &scheduled) && scheduled.RetryAt().After(next) {
		next = scheduled.RetryAt()
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	next, err := p.deps.Receipts.RecordPublicationFailure(storeCtx, runID, token, reviewworkflow.MaxPublicationAttempts, next, cause.Error())
	if err != nil {
		return errors.Join(err, cause)
	}
	return &publication.RetryAtError{At: next, Cause: cause}
}
