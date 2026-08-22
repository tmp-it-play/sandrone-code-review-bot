package outbound

import (
	"context"
	"time"
)

type Deduplicator interface {
	FirstSeen(ctx context.Context, key string, retention time.Duration) (bool, error)
}
