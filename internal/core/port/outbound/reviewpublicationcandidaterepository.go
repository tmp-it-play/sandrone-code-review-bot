package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewPublicationCandidateRepository interface {
	PublicationCandidateHighWatermark(ctx context.Context, before time.Time) (uint64, error)
	PublicationCandidates(ctx context.Context, before time.Time, afterID uint64, throughID uint64, limit int) ([]reviewworkflow.Run, error)
}
