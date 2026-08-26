package reviewpublication

import (
	"context"
	"errors"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const reviewSummaryMutationLease = 35 * time.Minute
const reviewSummaryMutationTimeout = 30 * time.Minute

type PreparedPublisher struct {
	deps Dependencies
}

func New(deps Dependencies) *PreparedPublisher {
	return &PreparedPublisher{deps: deps}
}

func (p *PreparedPublisher) Publish(ctx context.Context, target pullrequest.Target, runID uint64, runLeaseToken string, prepared reviewworkflow.ReviewPublication) error {
	if prepared.Status == reviewworkflow.ReviewPublicationStatusCompleted {
		if prepared.PayloadHash == "" {
			return nil
		}
		if prepared.Channel == reviewworkflow.ReviewPublicationChannelComment && len(prepared.Payload.Comments) > 0 {
			if err := p.deps.Publisher.VerifyTarget(ctx, target); err != nil {
				return err
			}
			_, err := p.publishSummary(ctx, target, prepared, prepared.Payload.FallbackBody)
			return err
		}
		_, _, err := p.publishEffects(ctx, target, prepared)
		return err
	}
	if prepared.Status != reviewworkflow.ReviewPublicationStatusPrepared {
		return errors.New("게시할 수 없는 리뷰 publication 상태입니다")
	}
	channel, externalID, err := p.publishEffects(ctx, target, prepared)
	if err != nil {
		return err
	}
	return p.Complete(ctx, runID, runLeaseToken, prepared.Marker, channel, externalID, prepared.ExpiresAt)
}

func (p *PreparedPublisher) publishEffects(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication) (string, int64, error) {
	if err := p.deps.Publisher.VerifyTarget(ctx, target); err != nil {
		return "", 0, err
	}
	commentID, err := p.publishSummary(ctx, target, prepared, prepared.Payload.Body)
	if err != nil {
		return "", 0, err
	}
	if err := p.deps.Publisher.VerifyTarget(ctx, target); err != nil {
		return "", 0, err
	}
	if len(prepared.Payload.Comments) == 0 {
		return reviewworkflow.ReviewPublicationChannelComment, commentID, nil
	}
	reviewID, submitErr := p.deps.Publisher.SubmitReview(ctx, target, prepared.Marker, prepared.Marker, prepared.Payload.Comments)
	if errors.Is(submitErr, publication.ErrTargetChanged) {
		return "", 0, submitErr
	}
	if errors.Is(submitErr, publication.ErrInlineReviewRejected) {
		p.deps.Logger.Warn("인라인 리뷰를 제출하지 못해 요약 코멘트에 지적을 포함합니다", "target", target.Reference(), "error", submitErr)
		if err := p.deps.Publisher.VerifyTarget(ctx, target); err != nil {
			return "", 0, errors.Join(submitErr, err)
		}
		fallbackID, commentErr := p.publishSummary(ctx, target, prepared, prepared.Payload.FallbackBody)
		if commentErr != nil {
			return "", 0, errors.Join(submitErr, commentErr)
		}
		return reviewworkflow.ReviewPublicationChannelComment, fallbackID, nil
	}
	if submitErr != nil {
		return "", 0, submitErr
	}
	return reviewworkflow.ReviewPublicationChannelReview, reviewID, nil
}

func (p *PreparedPublisher) publishSummary(ctx context.Context, target pullrequest.Target, prepared reviewworkflow.ReviewPublication, body string) (int64, error) {
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
	commentIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		foundIDs, err := p.deps.Publisher.FindComments(ctx, target, marker)
		if err != nil {
			return 0, false, false, err
		}
		for _, commentID := range foundIDs {
			if _, exists := seen[commentID]; exists {
				continue
			}
			seen[commentID] = struct{}{}
			commentIDs = append(commentIDs, commentID)
		}
	}
	if len(commentIDs) == 0 {
		return 0, false, false, nil
	}
	var updateErr error
	mutationAttempted := false
	for _, commentID := range commentIDs {
		mutationAttempted = true
		if err := p.deps.Publisher.UpdateComment(ctx, target, commentID, body); err != nil {
			updateErr = errors.Join(updateErr, err)
		}
	}
	return commentIDs[len(commentIDs)-1], true, mutationAttempted, updateErr
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
