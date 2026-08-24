package reviewpublication

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (r *Reconciler) invalidatePublication(ctx context.Context, target pullrequest.Target, run reviewworkflow.Run, detail string) (*reviewworkflow.PublicationInvalidation, error) {
	body := invalidationBody(detail)
	err := r.deps.Publisher.InvalidatePublication(ctx, target, reviewworkflow.PublicationMarker(run.Key), body)
	if err == nil {
		return nil, nil
	}
	if run.HeartbeatAt.After(r.deps.Clock.Now().Add(-r.config.OrphanAfter)) {
		return nil, err
	}
	return r.deferredInvalidation(run, body, err), nil
}

func (r *Reconciler) deferredInvalidation(run reviewworkflow.Run, body string, failure error) *reviewworkflow.PublicationInvalidation {
	now := r.deps.Clock.Now()
	return &reviewworkflow.PublicationInvalidation{
		RunID:          run.ID,
		InstallationID: run.InstallationID,
		Owner:          run.Owner,
		Repository:     run.Repository,
		Number:         run.Number,
		Marker:         reviewworkflow.PublicationMarker(run.Key),
		Reason:         body,
		LastError:      failure.Error(),
		NextAttemptAt:  now,
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
