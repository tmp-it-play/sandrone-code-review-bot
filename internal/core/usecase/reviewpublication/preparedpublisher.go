package reviewpublication

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

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
	published, err := p.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if err != nil {
		return err
	}
	if published {
		return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	reviewID, submitErr := p.deps.Publisher.SubmitReview(ctx, target, prepared.Marker, prepared.Payload.Body, prepared.Payload.Comments)
	if submitErr == nil {
		return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReview, reviewID, prepared.ExpiresAt)
	}
	if errors.Is(submitErr, publication.ErrTargetChanged) {
		return submitErr
	}
	p.deps.Logger.Warn("리뷰를 제출하지 못해 요약 코멘트로 대신합니다", "target", target.Reference(), "error", submitErr)
	published, reconcileErr := p.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr != nil {
		return errors.Join(submitErr, reconcileErr)
	}
	if published {
		return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	commentID, commentErr := p.deps.Publisher.CreateComment(ctx, target, prepared.Payload.FallbackBody)
	if commentErr == nil {
		return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelComment, commentID, prepared.ExpiresAt)
	}
	published, reconcileErr = p.deps.Publisher.PublicationExists(ctx, target, prepared.Marker)
	if reconcileErr == nil && published {
		return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, prepared.ExpiresAt)
	}
	return errors.Join(submitErr, commentErr, reconcileErr)
}

func (p *PreparedPublisher) Complete(ctx context.Context, runID uint64, runLeaseToken string, marker string, channel string, externalID int64, expiresAt time.Time) error {
	return p.deps.Receipts.CompleteReviewPublication(ctx, runID, runLeaseToken, marker, channel, externalID, p.deps.Clock.Now(), expiresAt)
}
