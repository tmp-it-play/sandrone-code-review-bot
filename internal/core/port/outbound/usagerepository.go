package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/usage"
)

type UsageRepository interface {
	Record(ctx context.Context, event usage.Event) error
	Snapshot(ctx context.Context) ([]usage.Snapshot, error)
}
