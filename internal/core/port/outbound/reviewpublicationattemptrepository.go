package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationAttemptRepository interface {
	BeginPublicationAttempt(ctx context.Context, runID uint64, token string) (reviewworkflow.PublicationAttempt, error)
	RecordPublicationFailure(ctx context.Context, runID uint64, token string, attempt int, retryAfter time.Time, detail string) (time.Time, error)
	SelectPublicationFallback(ctx context.Context, runID uint64, token string) error
	PublicationAttemptsExhausted(ctx context.Context, runID uint64) (bool, error)
}
