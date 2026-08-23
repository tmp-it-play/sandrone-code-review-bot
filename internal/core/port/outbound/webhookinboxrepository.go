package outbound

import (
	"context"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookinbox"
)

type WebhookInboxRepository interface {
	Store(ctx context.Context, delivery webhookinbox.Delivery) error
	ClaimNext(ctx context.Context, now time.Time, leaseExpiresAt time.Time) (webhookinbox.Delivery, bool, error)
	Complete(ctx context.Context, id uint64, leaseToken string, completedAt time.Time, expiresAt time.Time) error
	Retry(ctx context.Context, id uint64, leaseToken string, availableAt time.Time, lastError string) error
	Reject(ctx context.Context, id uint64, leaseToken string, rejectedAt time.Time, expiresAt time.Time, lastError string) error
	DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error)
}
