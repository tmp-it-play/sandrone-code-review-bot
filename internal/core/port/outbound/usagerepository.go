package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/usage"
)

type UsageRepository interface {
	Record(ctx context.Context, event usage.Event) error
	Snapshot(ctx context.Context) ([]usage.Snapshot, error)
	DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error)
}
