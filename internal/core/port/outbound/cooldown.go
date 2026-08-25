package outbound

import (
	"context"
	"time"
)

type Cooldown interface {
	Active(ctx context.Context, provider string) (bool, error)
	Mark(ctx context.Context, provider string, duration time.Duration) (time.Time, error)
	EndsAt(ctx context.Context, provider string) (time.Time, bool, error)
}
