package reviewpublication

import (
	"context"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) publicationInvalidation(run reviewworkflow.Run, detail string, failure error) *reviewworkflow.PublicationInvalidation {
	return r.deferredInvalidation(run, invalidationBody(detail), failure)
}

func (r *Reconciler) applyPublicationInvalidation(ctx context.Context, target pullrequest.Target, invalidation *reviewworkflow.PublicationInvalidation) error {
	if invalidation == nil {
		return nil
	}
	if _, valid := reviewworkflow.ProgressMarkerForPublicationMarker(invalidation.Marker); !valid {
		return nil
	}
	return r.deps.Publisher.InvalidateReview(ctx, target, invalidation.Marker, invalidation.Reason)
}

func (r *Reconciler) deferredInvalidation(run reviewworkflow.Run, body string, failure error) *reviewworkflow.PublicationInvalidation {
	now := r.deps.Clock.Now()
	lastError := "게시 무효화 최종 확인 대기 중"
	if failure != nil {
		lastError = failure.Error()
	}
	return &reviewworkflow.PublicationInvalidation{
		RunID:          run.ID,
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		Marker:         reviewworkflow.PublicationMarker(run.Key),
		ProgressMarker: strings.TrimSpace(run.ProgressMarker),
		Reason:         body,
		LastError:      lastError,
		NextAttemptAt:  now.Add(reviewworkflow.PublicationInvalidationFenceDelay),
		ExpiresAt:      now.Add(r.config.Retention),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func invalidationBody(detail string) string {
	return "⚠️ 이 리뷰는 게시 과정에서 더 최신인 실행 또는 PR 기준점이 확인되어 무효화되었습니다.\n\n" + detail
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
