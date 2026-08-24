package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type ReviewRetentionRepository interface {
	ReconcileOrphans(ctx context.Context, staleBefore time.Time, terminalAt time.Time, expiresAt time.Time, limit int) (reviewworkflow.OrphanReconciliation, error)
	DeleteExpired(ctx context.Context, now time.Time, legacyCutoff time.Time, limit int) (reviewworkflow.CleanupResult, error)
}
